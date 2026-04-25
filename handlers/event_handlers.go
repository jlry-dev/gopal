package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/jlry-dev/gopal/queue"
)

func OnTrackStart(r ReplyHandler) func(disgolink.Player, lavalink.TrackStartEvent) {
	return func(player disgolink.Player, e lavalink.TrackStartEvent) {
		embed := discord.NewEmbedBuilder().
			SetTitle("▶️ Now Playing").
			SetDescription(fmt.Sprintf("%v - %v", e.Track.Info.Title, e.Track.Info.Author)).
			SetColor(0x00ADD8).
			Build()

		var data TrackRequestData
		if err := e.Track.UserData.Unmarshal(&data); err == nil {
			r.SendWithEmbed(&embed, &data.GuildID, &data.ChannelID)
		}
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
