package handlers

import (
	"fmt"
	"log/slog"
	"runtime"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

type ReplyDTO struct {
	GuildID   *snowflake.ID
	ChannelID *snowflake.ID
	Embed     *discord.Embed
	Content   string
}

type ReplyHandler interface {
	Send(content string, guildID, channelID *snowflake.ID)
	SendWithEmbed(embed *discord.Embed, guildID, channelID *snowflake.ID)
}

type replyHandlr struct {
	replyCh chan *ReplyDTO
}

func NewReplyer(logger *slog.Logger, client *bot.Client) ReplyHandler {
	workerCnt := runtime.NumCPU() * 5
	ch := make(chan *ReplyDTO, workerCnt*2)

	for range workerCnt {
		go func() {
			for data := range ch {
				if data.Embed != nil {
					_, err := client.Rest.CreateMessage(
						*data.ChannelID,
						discord.NewMessageCreate().WithEmbeds(*data.Embed),
					)
					if err != nil {
						logger.Error(fmt.Sprintf("failed to reply the message %v", err))
					}

				} else {
					_, err := client.Rest.CreateMessage(*data.ChannelID, discord.MessageCreate{
						Content: data.Content,
					})
					if err != nil {
						logger.Error(fmt.Sprintf("failed to reply the message %v", err))
					}

				}
			}
		}()
	}

	replyer := replyHandlr{
		replyCh: ch,
	}

	return &replyer
}

func (r *replyHandlr) Send(content string, guildID, channelID *snowflake.ID) {
	data := &ReplyDTO{
		GuildID:   guildID,
		ChannelID: channelID,
		Content:   content,
	}

	r.replyCh <- data
}

func (r *replyHandlr) SendWithEmbed(embed *discord.Embed, guildID, channelID *snowflake.ID) {
	data := &ReplyDTO{
		GuildID:   guildID,
		ChannelID: channelID,
		Embed:     embed,
	}

	r.replyCh <- data
}
