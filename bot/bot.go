package bot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
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
			gateway.IntentGuildMessages,
			gateway.IntentGuildVoiceStates,
		)),
		bot.WithVoiceManagerConfigOpts(
			voice.WithDaveSessionCreateFunc(godave.NewNoopSession),
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
}

func handler(m *events.MessageCreate) {
	bot := m.Client()
	user := m.Message.Member.User

	// Return when bot is the same
	if bot.ID() == user.ID {
		return
	}

	fmt.Println(m.Message.Content)

	switch {
	case strings.HasPrefix(m.Message.Content, "?play"):
		bot.Rest.CreateMessage(m.ChannelID, discord.MessageCreate{
			Content: "Place holder response.",
		})
	}
}
