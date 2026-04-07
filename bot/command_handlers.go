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
)

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
		if userVoiceState.ChannelID != botVoiceState.ChannelID {
			client.Rest.CreateMessage(e.ChannelID, discord.MessageCreate{
				Content: "Bot is singing on a different voice channel.",
			})

			return
		}

		// TODO: check if currently playing
		// TODO: add to queue if currently playing
		// TODO: play if not currently playing
	} else {
		query := fmt.Sprintf("ytmsearch:%v", identifier)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

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
				log.Println("Found", len(tracks), "tracks")
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

		err := client.UpdateVoiceState(ctx, *e.GuildID, userVoiceState.ChannelID, false, false)
		if err != nil {
			b.logger.Error("failed to join voice channel", slog.String("ERROR", err.Error()))
		}

		player := b.disgoLink.client.Player(*e.GuildID)

		err = player.Update(ctx, lavalink.WithTrack(*toPlay))
		if err != nil {
			log.Fatal("Failed to play track:", err)
		}
	}
}
