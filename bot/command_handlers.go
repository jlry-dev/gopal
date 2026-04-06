package bot

import (
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
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
	if !ok {
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
		// TODO: join bot to channel
		// TODO: play track
	}
}
