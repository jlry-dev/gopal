package bot

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
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

var ERR_MUSIC_NOT_FOUND = errors.New("music not found")

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
		connections: &guildConnections{
			guilds: make(map[string]*guild),
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

type guild struct {
	guildID         string
	voiceConnection *discordgo.VoiceConnection
}

func NewGuild(guildID string, vc *discordgo.VoiceConnection) *guild {
	return &guild{
		guildID:         guildID,
		voiceConnection: vc,
	}
}

func (g *guild) Stream(url string) error {
	vc := g.voiceConnection

	// download
	yt_dlp := exec.Command("yt-dlp", "-x", url, "-o-")
	ytdlpOut, _ := yt_dlp.StdoutPipe()

	ffmpeg := exec.Command("ffmpeg", "-re", "-i", "pipe:0", "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")
	ffmpeg.Stdin = ytdlpOut
	ffmpegOut, _ := ffmpeg.StdoutPipe()

	// stream
	if err := yt_dlp.Start(); err != nil {
		return fmt.Errorf("error streaming: failed to start yt-dlp process: %w", err)
	}
	defer yt_dlp.Process.Kill()

	if err := ffmpeg.Start(); err != nil {
		return fmt.Errorf("error streaming: failed to start ffmpeg process : %w", err)
	}
	defer ffmpeg.Process.Kill()

	vc.Speaking(true)
	defer vc.Speaking(false)

	pcmChan := make(chan []int16, 100)
	go dgvoice.SendPCM(vc, pcmChan)
	defer close(pcmChan)

	for {
		audioBuffer := make([]int16, 960*2)
		err := binary.Read(ffmpegOut, binary.LittleEndian, &audioBuffer)
		if err != nil {
			if errors.Is(io.EOF, err) || errors.Is(io.ErrUnexpectedEOF, err) {
				break
			}

			return fmt.Errorf("error streaming: something went wrong during streaming: %w", err)
		}

		pcmChan <- audioBuffer
	}

	return nil
}

// Tracks guilds the bot is connected
type guildConnections struct {
	guilds map[string]*guild
	gLock  sync.RWMutex
}

func (gc *guildConnections) AddGuild(guildID string, g *guild) {
	gc.gLock.Lock()
	gc.guilds[guildID] = g
	gc.gLock.Unlock()
}

func (gc *guildConnections) Disconnect(guildID string) {
	gc.gLock.Lock()
	delete(gc.guilds, guildID)
	gc.gLock.Unlock()
}

func (gc *guildConnections) GetGuild(guildID string) *guild {
	gc.gLock.RLock()
	g, ok := gc.guilds[guildID]
	gc.gLock.RUnlock()

	if !ok {
		return nil
	}

	return g
}

type eventHandler struct {
	logger      *slog.Logger
	connections *guildConnections
}

func (e *eventHandler) botHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	switch {
	case strings.HasPrefix(m.Content, "?play"):
		g := e.connections.GetGuild(m.GuildID)

		// If naka join na nag bot pero ang user wala
		// Dapat dili maka request ang user
		vs, err := s.State.VoiceState(m.GuildID, m.Author.ID)
		if err != nil {
			if errors.Is(discordgo.ErrStateNotFound, err) {
				s.ChannelMessageSend(m.ChannelID, "Hoy bugok wala ka sa voice channel!")
				return
			}

			e.logger.Error("error retrieving voice state", slog.String("ERROR", err.Error()))
			return
		}

		// Check if naka join na ang bot
		if g == nil {
			vc, err := s.ChannelVoiceJoin(vs.GuildID, vs.ChannelID, true, true)
			if err != nil {
				e.logger.Error("error connecting to voice channel", slog.String("ERROR", err.Error()))
				return
			}

			newGuild := NewGuild(vs.GuildID, vc)
			e.connections.AddGuild(m.GuildID, newGuild)

			// Since g(*guild) is nil we assign the newly created guild struct sa iya
			g = newGuild
		} else {
			userVoiceChannel := vs.ChannelID
			botVoiceChannel := g.voiceConnection.ChannelID

			// Compare if they are on the same voice channel
			if userVoiceChannel != botVoiceChannel {
				s.ChannelMessageSend(m.ChannelID, "You are not on the same voice channel with GoPal.")
				return
			}
		}

		// Parse the song title
		title := strings.Join(strings.Fields(m.Content)[1:], " ")

		url, err := findMusicURL(title)
		if err != nil {
			if errors.Is(ERR_MUSIC_NOT_FOUND, err) {
				s.ChannelMessageSend(m.ChannelID, "We can't find that music sorry :<")
				return
			}

			e.logger.Error("error trying to find music URL", slog.String("ERROR", err.Error()))
			return
		}

		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("URL:%v", url))

		if err := g.Stream(url); err != nil {
			e.logger.Error(err.Error())
		}

		s.ChannelMessageSend(m.GuildID, "Finished playing song")
	}
}

func findMusicURL(title string) (url string, err error) {
	devKey, isPresent := os.LookupEnv("GOOGLE_DEV_KEY")
	if !isPresent {
		log.Fatal("missing GOOGLE_DEV_KEY env variable")
	}

	service, err := youtube.NewService(context.TODO(), option.WithAPIKey(devKey))
	if err != nil {
		return "", fmt.Errorf("findMusicURL: failed to create new youtube service: %w", err)
	}

	// Make the API call to YouTube.
	call := service.Search.List([]string{"id"}).
		Q(title).
		Type("video").
		VideoCategoryId("10"). // 10 = Music para sa category
		MaxResults(1)

	response, err := call.Do()
	if err != nil {
		return "", fmt.Errorf("findMusicURL: failed to find youtube title: %w", err)
	}

	if len(response.Items) < 1 {
		return "", ERR_MUSIC_NOT_FOUND
	}

	item := response.Items[0]
	return fmt.Sprintf("https://www.youtube.com/watch?v=%v", item.Id.VideoId), nil
}
