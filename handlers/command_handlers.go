package handlers

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"

	"github.com/jlry-dev/gopal/config"
	"github.com/jlry-dev/gopal/queue"
)

type TrackRequestData struct {
	RequestedBy string       `json:"requested_by"`
	GuildID     snowflake.ID `json:"guild_id"`
	ChannelID   snowflake.ID `json:"channel_id"`
}

type EventDTO struct {
	GuildID   *snowflake.ID
	ChannelID *snowflake.ID
	Client    *bot.Client
	User      *discord.User
	Message   string
}

type CommandHandler interface {
	Play(data *EventDTO)
	Stop(data *EventDTO)
	Skip(data *EventDTO)
}

type cmdHandlr struct {
	logger       *slog.Logger
	disgoLink    *config.DisgoLink
	queueManager queue.QueueManager
	replyer      ReplyHandler
}

func NewCommandHandler(logger *slog.Logger, disgoLink *config.DisgoLink, queueManager queue.QueueManager, replyer ReplyHandler) CommandHandler {
	return &cmdHandlr{
		logger:       logger,
		disgoLink:    disgoLink,
		queueManager: queueManager,
		replyer:      replyer,
	}
}

func (h *cmdHandlr) Play(e *EventDTO) {
	client := e.Client
	user := e.User
	message := e.Message

	contentSlice := strings.Fields(message)
	identifier := strings.Join(contentSlice[1:], " ")

	if identifier == "" {
		return
	}

	userVoiceState, ok := client.Caches.VoiceState(*e.GuildID, user.ID)
	if !ok {
		client.Rest.CreateMessage(*e.ChannelID, discord.MessageCreate{
			Content: "You must be in a voice channel to use this command.",
		})
	}

	botVoiceState, ok := client.Caches.VoiceState(*e.GuildID, client.ID())
	if ok {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if *userVoiceState.ChannelID != *botVoiceState.ChannelID {
			client.Rest.CreateMessage(*e.ChannelID, discord.MessageCreate{
				Content: "Bot is singing on a different voice channel.",
			})

			return
		}

		query := fmt.Sprintf("ytmsearch:%v", identifier)
		h.loadAndPlay(ctx, query, user, e.ChannelID, e.GuildID)

	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// make the bot join
		err := client.UpdateVoiceState(ctx, *e.GuildID, userVoiceState.ChannelID, true, true)
		if err != nil {
			h.logger.Error("failed to join voice channel", slog.String("ERROR", err.Error()))
		}

		query := fmt.Sprintf("ytmsearch:%v", identifier)

		h.loadAndPlay(ctx, query, user, e.ChannelID, e.GuildID)
	}
}

func (h *cmdHandlr) Stop(e *EventDTO) {
	client := e.Client

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.UpdateVoiceState(ctx, *e.GuildID, nil, false, false); err != nil {
		h.logger.Error("failed to update voice state (leaving)", slog.String("ERROR", err.Error()))
		return
	}

	// update lavalink
	player := h.disgoLink.Player(*e.GuildID)
	if err := player.Update(ctx, lavalink.WithNullTrack()); err != nil {
		h.logger.Error("failed to update player", slog.String("ERROR", err.Error()))
		return
	}

	// remove the queue
	h.queueManager.Remove(*e.GuildID)
}

func (h *cmdHandlr) Skip(e *EventDTO) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// update lavalink
	player := h.disgoLink.Player(*e.GuildID)

	// this stops the track which in turn trigger the EventTrackEnd and play the next track in queue
	if err := player.Update(ctx, lavalink.WithNullTrack()); err != nil {
		h.logger.Error("failed to update player", slog.String("ERROR", err.Error()))
		return
	}
}

func (h *cmdHandlr) loadAndPlay(ctx context.Context, query string, user *discord.User, channelID, guildID *snowflake.ID) {
	var toPlay *lavalink.Track
	h.disgoLink.BestNode().LoadTracksHandler(ctx, query, disgolink.NewResultHandler(
		func(track lavalink.Track) {
			// Loaded a single track (from URL)
			toPlay = &track
			log.Println("Loaded track:", track.Info.Title)
		},
		func(playlist lavalink.Playlist) {
			// Loaded a playlist
			log.Println("Loaded playlist:", playlist.Info.Name)
			if len(playlist.Tracks) > 0 {
				toPlay = &playlist.Tracks[0]
			}
		},
		func(tracks []lavalink.Track) {
			// Loaded search results
			if len(tracks) > 0 {
				toPlay = &tracks[0]
			}
		},
		func() {
			// No matches found
			log.Println("No matches found for query:", query)
		},
		func(err error) {
			// Error loading tracks
			log.Println("Error loading tracks:", err)
		},
	))

	// no track found
	if toPlay == nil {
		return
	}

	trackWithData, err := toPlay.WithUserData(TrackRequestData{
		RequestedBy: user.Username,
		GuildID:     *guildID,
		ChannelID:   *channelID,
	})

	player := h.disgoLink.Player(*guildID)

	// check if currently playing
	if player.Track() != nil {
		queue := h.queueManager.Get(*guildID)
		queue.Push(&trackWithData)

		h.replyer.Send(fmt.Sprintf("Added %v to the queue", trackWithData.Info.Title), guildID, channelID)

		// Recheck if the player ended while the track is being added to queue
		if player.Track() == nil {
			queue.PlayNext(ctx, player)
		}

		return
	}

	err = player.Update(ctx, lavalink.WithTrack(trackWithData))
	if err != nil {
		log.Fatal("Failed to play track:", err)
	}
}
