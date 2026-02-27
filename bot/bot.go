package bot

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"

	"github.com/bwmarrin/dgvoice"
	"github.com/bwmarrin/discordgo"
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
		connections: &voiceConnections{
			connections: make(map[string]*discordgo.VoiceConnection),
		},
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
	logger      *slog.Logger
	connections *voiceConnections
}

type voiceConnections struct {
	connections map[string]*discordgo.VoiceConnection
	vcLock      sync.RWMutex
}

func (vc *voiceConnections) AddConnection(guildID string, connection *discordgo.VoiceConnection) {
	vc.vcLock.Lock()
	vc.connections[guildID] = connection
	vc.vcLock.Unlock()
}

func (vc *voiceConnections) Disconnect(guildID string) {
	vc.vcLock.Lock()
	delete(vc.connections, guildID)
	vc.vcLock.Unlock()
}

func (vc *voiceConnections) GetConnection(guildID string) *discordgo.VoiceConnection {
	vc.vcLock.RLock()
	c, ok := vc.connections[guildID]
	vc.vcLock.RUnlock()

	if !ok {
		return nil
	}

	return c
}

func (e *eventHandler) botHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	switch {
	case strings.HasPrefix(m.Content, "?play"):

		vc := e.connections.GetConnection(m.GuildID)
		if vc == nil {
			vs, err := s.State.VoiceState(m.GuildID, m.Author.ID)
			if err != nil {
				if errors.Is(discordgo.ErrStateNotFound, err) {
					s.ChannelMessageSend(m.ChannelID, "Hoy bugok wala ka sa voice channel!")
					return
				}

				e.logger.Error("error retrieving voice state", slog.String("ERROR", err.Error()))
				return
			}

			vc, err = s.ChannelVoiceJoin(vs.GuildID, vs.ChannelID, true, true)
			if err != nil {
				e.logger.Error("error connecting to voice channel", slog.String("ERROR", err.Error()))
				return
			}

			e.connections.AddConnection(m.GuildID, vc)
		}

		// Parse the song title
		title := strings.Join(strings.Fields(m.Content)[1:], " ")

		url := findMusicURL(title)
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("URL:%v", url))

		// download
		yt_dlp := exec.Command("yt-dlp",
			"-x",
			"--audio-format", "mp3",
			url,
			"-o-",
		)
		ytdlpOut, _ := yt_dlp.StdoutPipe()

		ffmpeg := exec.Command("ffmpeg", "-i", "pipe:0", "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")

		ffmpeg.Stdin = ytdlpOut
		ffmpegOut, _ := ffmpeg.StdoutPipe()

		pcmChan := make(chan []int16, 2)
		go dgvoice.SendPCM(vc, pcmChan)
		defer close(pcmChan)

		// stream
		go func() {
			defer ytdlpOut.Close()
			yt_dlp.Run()
		}()

		vc.Speaking(true)
		defer vc.Speaking(false)

		done := false

		go func() {
			defer ffmpegOut.Close()
			err := ffmpeg.Run()
			if err != nil {
				e.logger.Error("error trying to stream using ffmpeg", slog.String("ERROR", err.Error()))
				return
			}

			done = true
		}()

		for {
			audioBuffer := make([]int16, 960*2)
			binary.Read(ffmpegOut, binary.LittleEndian, &audioBuffer)
			if done {
				break
			}

			pcmChan <- audioBuffer
		}
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
