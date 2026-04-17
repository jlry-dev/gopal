package gopal

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/voice"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/godave"
	"github.com/jlry-dev/gopal/config"
	"github.com/jlry-dev/gopal/handlers"
	"github.com/jlry-dev/gopal/queue"
)

type gopal struct {
	logger       *slog.Logger
	disgoLink    *config.DisgoLink
	queueManager queue.QueueManager
	client       *bot.Client
	cmdHandler   handlers.CommandHandler
	replyer      handlers.ReplyHandler
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

	dl := config.Connect(client.ApplicationID)

	replyer := handlers.NewReplyer(b.logger, client)
	queueManager := queue.NewQueueManager()
	cmdHandler := handlers.NewCommandHandler(b.logger, dl, queueManager, replyer)

	b.disgoLink = dl
	b.replyer = replyer
	b.cmdHandler = cmdHandler
	b.queueManager = queueManager

	// Need para ma forward ang event padulnog sa disgolink
	client.AddEventListeners(
		bot.NewListenerFunc(dl.OnVoiceServerUpdateHandler),
		bot.NewListenerFunc(dl.OnVoiceStateUpdateHandler),
	)

	dl.AddListeners(
		disgolink.NewListenerFunc(handlers.OnTrackStart(replyer)),
		disgolink.NewListenerFunc(handlers.OnTrackEnd(queueManager)),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	err = client.OpenGateway(ctx)
	cancel()
	if err != nil {
		panic(err)
	}

	b.logger.Info("Bot started")
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)
	<-s

	// Clean up
	client.Close(context.Background())
	dl.Close()
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

	data := &handlers.EventDTO{
		GuildID:   e.GuildID,
		ChannelID: &e.ChannelID,
		Client:    bot,
		User:      &user,
		Message:   message.Content,
	}

	switch {
	case strings.HasPrefix(message.Content, "?play"):
		b.cmdHandler.Play(data)
	case strings.HasPrefix(message.Content, "?stop"):
		b.cmdHandler.Stop(data)
	case strings.HasPrefix(message.Content, "?skip"):
		b.cmdHandler.Skip(data)

	}
}
