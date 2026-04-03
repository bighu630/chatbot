package handler

import (
	"chatbot/internal/admin"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/rs/zerolog/log"
)

func NewFeedbackHandler(notifier *admin.FeedbackNotifier) handlers.Response {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		if notifier == nil || !notifier.Enabled() {
			_, err := b.SendMessage(ctx.EffectiveChat.Id, "反馈功能暂未配置，请稍后再试。", nil)
			return err
		}

		content := extractFeedbackContent(ctx.EffectiveMessage.GetText())
		if content == "" {
			_, err := b.SendMessage(ctx.EffectiveChat.Id, "用法：/feedback 这里填写你的反馈内容", nil)
			return err
		}

		meta := admin.FeedbackMeta{
			UserID:    ctx.EffectiveSender.Id(),
			Username:  ctx.EffectiveSender.Username(),
			ChatType:  ctx.EffectiveChat.Type,
			ChatID:    ctx.EffectiveChat.Id,
			ChatTitle: ctx.EffectiveChat.Title,
			Content:   content,
		}
		if err := notifier.NotifyFeedback(b, meta); err != nil {
			log.Error().Err(err).Msg("failed to deliver feedback to admin")
			_, sendErr := b.SendMessage(ctx.EffectiveChat.Id, "反馈发送失败，请稍后再试。", nil)
			if sendErr != nil {
				return sendErr
			}
			return err
		}

		_, err := b.SendMessage(ctx.EffectiveChat.Id, "反馈已收到。", nil)
		return err
	}
}

func extractFeedbackContent(text string) string {
	fields := strings.Fields(text)
	if len(fields) <= 1 {
		return ""
	}
	return strings.TrimSpace(strings.Join(fields[1:], " "))
}
