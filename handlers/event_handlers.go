package handlers

import (
	"context"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/jlry-dev/gopal/queue"
	"github.com/jlry-dev/gopal/recommender"
)

func OnTrackStart(r ReplyHandler, rcdr recommender.Recommender) func(disgolink.Player, lavalink.TrackStartEvent) {
	return func(player disgolink.Player, e lavalink.TrackStartEvent) {
		var data TrackRequestData
		if err := e.Track.UserData.Unmarshal(&data); err == nil {
			// TODO: log error here
		}

		track := e.Track.Info

		embed := buildNowPlayingEmbed(
			track.Title,
			*track.URI,
			track.Author,
		)

		rcdr.GetSimilarTrack(track.Title, track.Author)

		r.SendWithEmbed(&embed, &data.GuildID, &data.ChannelID)
	}
}

func OnTrackEnd(queueManager queue.QueueManager) func(disgolink.Player, lavalink.TrackEndEvent) {
	return func(player disgolink.Player, e lavalink.TrackEndEvent) {
		if !e.Reason.MayStartNext() {
			return
		}

		queue := queueManager.Get(e.GuildID())

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
	return discord.NewEmbedBuilder().
		SetTitle("").
		SetColor(0x00ADD8).
		SetDescriptionf("▶️ **Now Playing [%s - %s](%s)**", trackTitle, artist, trackURL).
		Build()
}
