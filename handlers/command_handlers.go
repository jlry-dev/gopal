package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
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
	RequestedBy string `json:"requested_by"`
	User        *discord.User
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
	LoadAndPlay(ctx context.Context, query string, user *discord.User, channelID, guildID *snowflake.ID)
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

	if e.GuildID == nil || e.ChannelID == nil {
		return
	}

	contentSlice := strings.Fields(message)
	identifier := strings.Join(contentSlice[1:], " ")

	if identifier == "" {
		return
	}

	userVoiceState, ok := client.Caches.VoiceState(*e.GuildID, user.ID)
	if !ok {
		h.replyer.Send(
			"**You must be in a voice channel to use this command.**",
			e.GuildID,
			e.ChannelID,
		)

		return
	}

	botVoiceState, ok := client.Caches.VoiceState(*e.GuildID, client.ID())
	if ok {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if *userVoiceState.ChannelID != *botVoiceState.ChannelID {
			h.replyer.Send(
				"**Bot is singing on a different voice channel.**",
				e.GuildID,
				e.ChannelID,
			)

			return
		}

		query := fmt.Sprintf("scsearch:%v", identifier)
		h.LoadAndPlay(ctx, query, user, e.ChannelID, e.GuildID)

	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// make the bot join
		err := client.UpdateVoiceState(ctx, *e.GuildID, userVoiceState.ChannelID, true, true)
		if err != nil {
			h.logger.Error("failed to join voice channel", slog.String("ERROR", err.Error()))
		}

		query := fmt.Sprintf("scsearch:%v", identifier)

		h.LoadAndPlay(ctx, query, user, e.ChannelID, e.GuildID)
	}
}

func (h *cmdHandlr) Stop(e *EventDTO) {
	client := e.Client

	if e.GuildID == nil {
		return
	}

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
	if e.GuildID == nil || e.ChannelID == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// update lavalink
	player := h.disgoLink.Player(*e.GuildID)

	queue := h.queueManager.Get(*e.GuildID)

	if queue.Len() == 0 {
		h.replyer.Send("Nothing is queued to skip.", e.GuildID, e.ChannelID)
		return
	}

	queue.PlayNext(ctx, player)
}

func (h *cmdHandlr) LoadAndPlay(ctx context.Context, query string, user *discord.User, channelID, guildID *snowflake.ID) {
	if guildID == nil || channelID == nil || user == nil {
		return
	}

	node := h.disgoLink.BestNode()
	if node == nil {
		h.logger.Error("no lavalink node available", "query", query)
		return
	}

	var toPlay *lavalink.Track
	node.LoadTracksHandler(ctx, query, disgolink.NewResultHandler(
		func(track lavalink.Track) {
			// Loaded a single track (from URL)
			toPlay = &track
			h.logger.Info("Loaded track", "title", track.Info.Title)
		},
		func(playlist lavalink.Playlist) {
			// Loaded a playlist
			h.logger.Info("Loaded playlist", "name", playlist.Info.Name)
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
			h.logger.Warn("No matches found for query", "query", query)
		},
		func(err error) {
			// Error loading tracks
			h.logger.Error("Error loading tracks", "query", query, "error", err)
		},
	))

	// no track found
	if toPlay == nil {
		return
	}

	trackWithData, err := toPlay.WithUserData(TrackRequestData{
		RequestedBy: user.Username,
		User:        user,
		GuildID:     *guildID,
		ChannelID:   *channelID,
	})
	if err != nil {
		h.logger.Error("failed to attach user data to track", "title", toPlay.Info.Title, "error", err)
		return
	}

	player := h.disgoLink.Player(*guildID)

	// check if currently playing
	if player.Track() != nil {
		queue := h.queueManager.Get(*guildID)
		queuePos := queue.Push(&trackWithData)

		// Recheck if the player ended while the track is being added to queue
		if player.Track() == nil {
			if err := h.waitForPlayerReady(ctx, player); err != nil {
				h.logger.Error("player did not become ready to play 1", "guild", *guildID, "error", err)
				return
			}
			queue.PlayNext(ctx, player)
			return
		}

		var thumbnailURL string
		if trackWithData.Info.ArtworkURL == nil && trackWithData.Info.SourceName == "youtube" {
			videoID := extractYouTubeID(uriString(trackWithData.Info.URI))
			thumbnailURL = fmt.Sprintf("https://img.youtube.com/vi/%s/mqdefault.jpg", videoID)
		}

		embed := buildQueueAddedEmbed(
			trackWithData.Info.Title,
			uriString(trackWithData.Info.URI),
			trackWithData.Info.Author,
			trackWithData.Info.Length.String(),
			queuePos,
			thumbnailURL,
		)

		h.replyer.SendWithEmbed(&embed, guildID, channelID)

		return
	}

	// if err := h.waitForPlayerReady(ctx, player); err != nil {
	// 	h.logger.Error("player did not become ready to play 2", "guild", *guildID, "error", err)
	// 	return
	// }

	err = player.Update(ctx, lavalink.WithTrack(trackWithData))
	if err != nil {
		h.logger.Error("failed to play track", "title", trackWithData.Info.Title, "error", err)
	}
}

// waitForPlayerReady blocks until the player's voice session is connected
// to Lavalink. The voice connection is established asynchronously after the
// bot joins a channel, so playing before it is ready silently drops the track.
func (h *cmdHandlr) waitForPlayerReady(ctx context.Context, player disgolink.Player) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		if player.State().Connected {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func uriString(uri *string) string {
	if uri == nil {
		return ""
	}
	return *uri
}

func extractYouTubeID(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return u.Query().Get("v")
}

func buildQueueAddedEmbed(
	trackTitle string,
	trackURL string,
	artist string,
	trackLength string,
	positionQueue int,
	thumbnailURL string,
) discord.Embed {
	boolptr := true

	return discord.NewEmbed().
		WithTitle("⏳ Added Track").
		WithColor(0x00ADD8).
		WithDescriptionf("**Track**\n **[%s - %s](%s)**", trackTitle, artist, trackURL).
		AddFields(
			discord.EmbedField{
				Name:   "Track Length",
				Value:  trackLength,
				Inline: &boolptr,
			},
			discord.EmbedField{
				Name:   "Position in queue",
				Value:  fmt.Sprintf("%d", positionQueue),
				Inline: &boolptr,
			},
		).
		WithImage(thumbnailURL)
}
