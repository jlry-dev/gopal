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
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/godave"
)

type gopal struct {
	logger          *slog.Logger
	disgoLinkClient *disgolink.Client
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

	dl := NewDisgoLink()
	b.disgoLinkClient = &dl.lavalinkClient

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
		bot.WithEventListenerFunc(dl.onVoiceStateUpdate),
		bot.WithEventListenerFunc(dl.onVoiceServerUpdate),
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
