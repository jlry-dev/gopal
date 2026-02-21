package bot

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type discordBot struct {
	logger *slog.Logger
}

type Bot interface {
	Run()
}

func NewBot(logger *slog.Logger) Bot {
	return &discordBot{
		logger: logger,
	}
}

func (b *discordBot) Run() {
	botToken, isPresent := os.LookupEnv("BOT_TOKEN")
	if !isPresent {
		b.logger.Error("missing BOT_TOKEN env variable")
		os.Exit(-1)
	}

	// create a session
	discord, err := discordgo.New("Bot " + botToken)
	if err != nil {
		b.logger.Error("failed to create a session", slog.String("ERROR", err.Error()))
		os.Exit(-1)
	}

	// add a event handler
	discord.AddHandler(newMessage)

	// open session
	discord.Open()
	defer discord.Close() // close session, after function termination

	fmt.Println("Bot running....")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}

func newMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	switch {
	case strings.Contains(m.Content, "?join"):
		vs, err := s.State.VoiceState(m.GuildID, m.Author.ID)
		if err != nil {
			if errors.Is(discordgo.ErrStateNotFound, err) {
				s.ChannelMessageSend(m.ChannelID, "Hoy bugok wala ka sa voice channel!")
				return
			}

			log.Fatal("There was an err")
		}

		s.ChannelVoiceJoin(vs.GuildID, vs.ChannelID, true, true)
	}
}
