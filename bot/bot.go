package bot

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/godave"
)

type gopal struct {
	logger *slog.Logger
}

type Bot interface {
	Run()
}

func NewBot(logger *slog.Logger) Bot {
	return &gopal{
		logger: logger,
	}
}

func (b *gopal) Run() {
	botToken, isPresent := os.LookupEnv("BOT_TOKEN")
	if !isPresent {
		b.logger.Error("missing BOT_TOKEN env variable")
		os.Exit(-1)
	}

	client, err := disgo.New(botToken,
		bot.WithGatewayConfigOpts(gateway.WithIntents(
			gateway.IntentMessageContent,
			gateway.IntentGuilds,
			gateway.IntentGuildMessages,
			gateway.IntentGuildVoiceStates,
		)),
		bot.WithVoiceManagerConfigOpts(
			voice.WithDaveSessionCreateFunc(godave.NewNoopSession),
		),

		// Configure what to save on the cache
		bot.WithCacheConfigOpts(
			cache.WithCaches(
				cache.FlagVoiceStates,
			),
		),
		// add event listeners
		bot.WithEventListenerFunc(handler),
	)
	if err != nil {
		panic(err)
	}

	if err = client.OpenGateway(context.TODO()); err != nil {
		panic(err)
	}

	b.logger.Info("Bot started")
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)
	<-s

	// Clean up
	client.Close(context.Background())
}

func handler(m *events.MessageCreate) {
	bot := m.Client()
	user := m.Message.Author

	message := m.Message
	// Return when bot is the same
	if bot.ID() == user.ID {
		return
	}

	if !strings.HasPrefix(message.Content, "?") {
		return
	}

	switch {
	case strings.HasPrefix(message.Content, "?play"):
		userVoiceState, userIsJoined := bot.Caches.VoiceState(*message.GuildID, user.ID)

		if !userIsJoined {
			bot.Rest.CreateMessage(m.ChannelID, discord.MessageCreate{
				Content: "User is not in a voice channel.",
			})
		}

		botVoiceState, botIsJoined := bot.Caches.VoiceState(*message.GuildID, bot.ID())

		if !botIsJoined {
			// Bot is not in a voice channel

			err := bot.UpdateVoiceState(
				context.TODO(),
				*message.GuildID,
				userVoiceState.ChannelID,
				true,
				true,
			)
			if err != nil {
				log.Printf("Failed to join voice channel: %v", err)
			}
		} else if *botVoiceState.ChannelID != *userVoiceState.ChannelID {
			// Bot and user are both in a voice channel
			// pero dili parehas ug voice channel
			bot.Rest.CreateMessage(m.ChannelID, discord.MessageCreate{
				Content: "Bot is busy in a different voice channel.",
			})
		} else {
			// Both are on the same channel
			// TODO: play and queue
			log.Println("TODO: play something or queue something")
		}
	}
}
