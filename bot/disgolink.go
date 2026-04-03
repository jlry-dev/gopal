package bot

import (
	"context"
	"log"

	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/snowflake/v2"
)

type disgoLink struct {
	lavalinkClient disgolink.Client
	node           disgolink.Node
}

func NewDisgoLink() disgoLink {
	lavaUID := snowflake.ID(1234567890)

	lavalinkClient := disgolink.New(lavaUID)

	node, err := lavalinkClient.AddNode(context.TODO(), disgolink.NodeConfig{
		Name:      "test", // a unique node name
		Address:   "localhost:2333",
		Password:  "youshallnotpass",
		Secure:    false, // ws or wss
		SessionID: "",    // only needed if you want to resume a previous lavalink session
	})
	if err != nil {
		log.Println("failed to add lavalink node")
	}

	dl := disgoLink{}
	dl.lavalinkClient = lavalinkClient
	dl.node = node

	return dl
}

func (d *disgoLink) onVoiceStateUpdate(event *events.GuildVoiceStateUpdate) {
	client := event.Client()

	// filter all non bot voice state updates out
	if event.VoiceState.UserID != client.ApplicationID {
		return
	}

	d.lavalinkClient.OnVoiceStateUpdate(context.TODO(), event.VoiceState.GuildID, event.VoiceState.ChannelID, event.VoiceState.SessionID)
}

func (d *disgoLink) onVoiceServerUpdate(event *events.VoiceServerUpdate) {
	d.lavalinkClient.OnVoiceServerUpdate(context.TODO(), event.GuildID, event.Token, *event.Endpoint)
}
