package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

type discordBot struct {
	logger  *slog.Logger
	handler *eventHandler
}

type Bot interface {
	Run()
}

func NewBot(logger *slog.Logger) Bot {
	newHandlr := &eventHandler{
		logger: logger,
	}

	return &discordBot{
		logger:  logger,
		handler: newHandlr,
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
	discord.AddHandler(b.handler.botHandler)

	// open session
	discord.Open()
	defer discord.Close() // close session, after function termination

	fmt.Println("Bot running....")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}

type eventHandler struct {
	logger *slog.Logger
}

func (e *eventHandler) botHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	switch {
	case strings.HasPrefix(m.Content, "?join"):
		vs, err := s.State.VoiceState(m.GuildID, m.Author.ID)
		if err != nil {
			if errors.Is(discordgo.ErrStateNotFound, err) {
				s.ChannelMessageSend(m.ChannelID, "Hoy bugok wala ka sa voice channel!")
				return
			}

			e.logger.Error("error retrieving voice state", slog.String("ERROR", err.Error()))
			return
		}

		_, err = s.ChannelVoiceJoin(vs.GuildID, vs.ChannelID, true, true)
		if err != nil {
			e.logger.Error("error connecting to voice channel", slog.String("ERROR", err.Error()))
			return
		}
	case strings.HasPrefix(m.Content, "?play"):
		gs, err := s.State.Guild(m.GuildID)
		if err != nil {
			e.logger.Error("error getting guild state", slog.String("ERROR", err.Error()))
			return
		}

		var isJoined bool
		// Check if naa sa voice call ang bot
		for _, vs := range gs.VoiceStates {
			if vs.UserID == s.State.User.ID {
				isJoined = true
			}
		}

		if !isJoined {
			s.ChannelMessageSend(m.ChannelID, "GoPal is not in a voice channel")
			return
		}

		// Parse the song title
		title := strings.Join(strings.Fields(m.Content)[1:], " ")

		url := findMusicURL(title)
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("URL:%v", url))

		// download
		wd, isPresent := os.LookupEnv("WORKING_DIR")
		if !isPresent {
			e.logger.Error("missing BOT_TOKEN env variable")
			os.Exit(-1)
		}

		uid := uuid.New()

		cmd := exec.Command("yt-dlp",
			"-x",
			"--audio-format", "mp3",
			"-o", fmt.Sprintf("%s/downloads/%s.%%(ext)s", wd, uid),
			url,
		)
		if err := cmd.Run(); err != nil {
			e.logger.Error("error trying to download audio", slog.String("ERROR", err.Error()))
			return
		}

		// stream
	}
}

func findMusicURL(title string) (url string) {
	devKey, isPresent := os.LookupEnv("GOOGLE_DEV_KEY")
	if !isPresent {
		log.Fatal("missing GOOGLE_DEV_KEY env variable")
	}

	service, err := youtube.NewService(context.TODO(), option.WithAPIKey(devKey))
	if err != nil {
		// TODO : Make this not fatal
		log.Fatalf("Error creating new YouTube client: %v", err)
	}

	// Make the API call to YouTube.
	call := service.Search.List([]string{"id"}).
		Q("music" + title).
		Type("video").
		VideoCategoryId("10"). // 10 = Music para sa category
		MaxResults(1)

	response, err := call.Do()
	if err != nil {
		// TODO : Make this not fatal
		log.Fatalf("failed to search: %v", err)
	}

	item := response.Items[0]
	return fmt.Sprintf("https://www.youtube.com/watch?v=%v", item.Id.VideoId)
}
