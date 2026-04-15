package bot

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

type TrackRequestData struct {
	RequestedBy string       `json:"requested_by"`
	ChannelID   snowflake.ID `json:"channel_id"`
}

func (b *gopal) play(e *events.MessageCreate) {
	client := e.Client()
	user := e.Message.Author

	message := e.Message

	contentSlice := strings.Fields(message.Content)
	identifier := strings.Join(contentSlice[1:], " ")

	if identifier == "" {
		return
	}

	userVoiceState, ok := client.Caches.VoiceState(*e.GuildID, user.ID)
	if !ok {
		client.Rest.CreateMessage(e.ChannelID, discord.MessageCreate{
			Content: "You must be in a voice channel to use this command.",
		})
	}

	botVoiceState, ok := client.Caches.VoiceState(*e.GuildID, client.ID())
	if ok {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if *userVoiceState.ChannelID != *botVoiceState.ChannelID {
			client.Rest.CreateMessage(e.ChannelID, discord.MessageCreate{
				Content: "Bot is singing on a different voice channel.",
			})

			return
		}

		query := fmt.Sprintf("ytmsearch:%v", identifier)
		b.loadAndPlay(ctx, query, &user, &e.ChannelID, e.GuildID)

	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// make the bot join
		err := client.UpdateVoiceState(ctx, *e.GuildID, userVoiceState.ChannelID, true, true)
		if err != nil {
			b.logger.Error("failed to join voice channel", slog.String("ERROR", err.Error()))
		}

		query := fmt.Sprintf("ytmsearch:%v", identifier)

		b.loadAndPlay(ctx, query, &user, &e.ChannelID, e.GuildID)
	}
}

func (b *gopal) loadAndPlay(ctx context.Context, query string, user *discord.User, channelID, guildID *snowflake.ID) {
	var toPlay *lavalink.Track
	b.disgoLink.client.BestNode().LoadTracksHandler(ctx, query, disgolink.NewResultHandler(
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
		ChannelID:   *channelID,
	})

	player := b.disgoLink.client.Player(*guildID)

	// check if currently playing
	if player.Track() != nil {
		queue := b.queueManager.Get(*guildID)
		queue.Push(&trackWithData)

		b.client.Rest.CreateMessage(*channelID, discord.MessageCreate{
			Content: fmt.Sprintf("Added %v to the queue", trackWithData.Info.Title),
		})

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
