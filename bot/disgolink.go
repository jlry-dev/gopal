package bot

import (
	"context"
	"log"
	"os"

	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/snowflake/v2"
)

type disgoLink struct {
	disgolink.Client
}

func NewDisgoLink(botID snowflake.ID, ctx context.Context) *disgoLink {
	client := disgolink.New(botID)
	lavalinkAddr, ok := os.LookupEnv("LAVALINK_ADDR")
	if !ok {
		log.Fatal("missing LAVALINK_ADDR env variable")
	}

	lavalinkPasswd, ok := os.LookupEnv("LAVALINK_PASSWORD")
	if !ok {
		log.Fatal("missing LAVALINK_PASSWORD env variable")
	}

	_, err := client.AddNode(ctx, disgolink.NodeConfig{
		Name:      "GoPal Lavalink Node",
		Address:   lavalinkAddr,
		Password:  lavalinkPasswd,
		Secure:    false,
		SessionID: "",
	})
	if err != nil {
		log.Println("failed to add lavalink node")
	}

	dl := disgoLink{
		Client: client,
	}

	return &dl
}

func (d *disgoLink) onVoiceStateUpdate(event *events.GuildVoiceStateUpdate) {
	client := event.Client()

	// filter all non bot voice state updates out
	if event.VoiceState.UserID != client.ApplicationID {
		return
	}

	d.OnVoiceStateUpdate(context.Background(), event.VoiceState.GuildID, event.VoiceState.ChannelID, event.VoiceState.SessionID)
}

func (d *disgoLink) onVoiceServerUpdate(event *events.VoiceServerUpdate) {
	d.OnVoiceServerUpdate(context.Background(), event.GuildID, event.Token, *event.Endpoint)
}
