package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/jlry-dev/gopal/queue"
	"github.com/jlry-dev/gopal/recommender"
)

func OnTrackStart(logger *slog.Logger, r ReplyHandler) func(disgolink.Player, lavalink.TrackStartEvent) {
	return func(player disgolink.Player, e lavalink.TrackStartEvent) {
		var data TrackRequestData
		if err := e.Track.UserData.Unmarshal(&data); err != nil {
			logger.Warn("failed to unmarshal track user data", "error", err)
			return
		}

		track := e.Track.Info

		embed := buildNowPlayingEmbed(
			track.Title,
			uriString(track.URI),
			track.Author,
		)

		r.SendWithEmbed(&embed, &data.GuildID, &data.ChannelID)
	}
}

func OnTrackEnd(logger *slog.Logger, queueManager queue.QueueManager, rcdr recommender.Recommender, cmdHandler CommandHandler) func(disgolink.Player, lavalink.TrackEndEvent) {
	return func(player disgolink.Player, e lavalink.TrackEndEvent) {
		if !e.Reason.MayStartNext() {
			return
		}

		queue := queueManager.Get(e.GuildID())

		if queue.Len() <= 0 {
			var data TrackRequestData
			if err := e.Track.UserData.Unmarshal(&data); err != nil {
				logger.Warn("failed to unmarshal track user data, skipping recommendation", "error", err)
				return
			}
			if data.User == nil {
				logger.Warn("track has no requester, skipping recommendation")
				return
			}

			track := e.Track.Info
			next := rcdr.GetSimilarTrack(track.Title, track.Author)
			if next == "" {
				logger.Warn("recommender returned no track, stopping playback")
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			query := fmt.Sprintf("ytmsearch:%v", next)
			cmdHandler.LoadAndPlay(ctx, query, data.User, &data.ChannelID, &data.GuildID)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		queue.PlayNext(ctx, player)
	}
}

func buildNowPlayingEmbed(
	trackTitle string,
	trackURL string,
	artist string,
) discord.Embed {
	return discord.NewEmbed().
		WithTitle("").
		WithColor(0x00ADD8).
		WithDescriptionf("▶️ **Now Playing [%s - %s](%s)**", trackTitle, artist, trackURL)
}
