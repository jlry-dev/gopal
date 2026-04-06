package bot

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/godave"
)

type gopal struct {
	logger    *slog.Logger
	disgoLink *disgoLink
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
		bot.WithEventListenerFunc(b.onMessageCreate),
	)
	if err != nil {
		panic(err)
	}

	dl := NewDisgoLink(client.ApplicationID)
	b.disgoLink = dl

	// Need para ma forward ang event padulnog sa disgolink
	client.AddEventListeners(
		bot.NewListenerFunc(dl.onVoiceServerUpdate),
		bot.NewListenerFunc(dl.onVoiceStateUpdate),
	)

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

func (b *gopal) onMessageCreate(e *events.MessageCreate) {
	bot := e.Client()
	user := e.Message.Author

	message := e.Message
	// Return when bot is the same
	if bot.ID() == user.ID {
		return
	}

	if !strings.HasPrefix(message.Content, "?") {
		return
	}

	switch {
	case strings.HasPrefix(message.Content, "?play"):
		b.play(e)
	}
}
