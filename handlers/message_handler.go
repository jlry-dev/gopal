package handlers

import (
	"fmt"
	"log/slog"

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
	logger *slog.Logger
	client *bot.Client
}

func NewReplyer(logger *slog.Logger, client *bot.Client) ReplyHandler {
	replyer := replyHandlr{
		logger: logger,
		client: client,
	}

	return &replyer
}

func (r *replyHandlr) Send(content string, guildID, channelID *snowflake.ID) {
	_, err := r.client.Rest.CreateMessage(
		*channelID,
		discord.MessageCreate{
			Content: content,
		},
	)
	if err != nil {
		r.logger.Error(fmt.Sprintf("failed to reply the message %v", err))
	}
}

func (r *replyHandlr) SendWithEmbed(embed *discord.Embed, guildID, channelID *snowflake.ID) {
	_, err := r.client.Rest.CreateMessage(
		*channelID,
		discord.NewMessageCreate().WithEmbeds(*embed),
	)
	if err != nil {
		r.logger.Error(fmt.Sprintf("failed to reply the message %v", err))
	}
}
