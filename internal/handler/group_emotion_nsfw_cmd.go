package handler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/rs/zerolog/log"
)

func NewGroupEmotionNSFWHandler(cfg *GroupEmotionNSFWConfig, adminUserIDs []int64) handlers.Response {
	adminIDs := buildAdminIDSet(adminUserIDs)
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		if ctx == nil || ctx.EffectiveChat == nil || ctx.EffectiveMessage == nil {
			return nil
		}
		if ctx.EffectiveChat.Type != "group" && ctx.EffectiveChat.Type != "supergroup" {
			_, err := b.SendMessage(ctx.EffectiveChat.Id, "这个命令只能在群聊里使用。", nil)
			return err
		}

		mode, ok := parseGroupEmotionNSFWMode(ctx.EffectiveMessage.GetText())
		if !ok {
			_, err := b.SendMessage(ctx.EffectiveChat.Id, "用法：/nsfw 0|1|2。0=只搜正常图，1=只搜 NSFW，2=不做 NSFW 过滤。", nil)
			return err
		}

		allowed, err := canManageGroupReplyActivity(b, ctx, adminIDs)
		if err != nil {
			log.Warn().Err(err).Int64("chat_id", ctx.EffectiveChat.Id).Msg("failed to check group emotion nsfw permission")
		}
		if !allowed {
			_, sendErr := b.SendMessage(ctx.EffectiveChat.Id, "只有本群管理员或机器人管理员可以设置 NSFW 模式。", nil)
			if sendErr != nil {
				return sendErr
			}
			return err
		}

		if err := cfg.setGroupMode(ctx.EffectiveChat.Id, mode); err != nil {
			log.Error().Err(err).Int64("chat_id", ctx.EffectiveChat.Id).Int("mode", mode).Msg("failed to save group emotion nsfw mode")
			_, sendErr := b.SendMessage(ctx.EffectiveChat.Id, "NSFW 模式保存失败，请稍后再试。", nil)
			if sendErr != nil {
				return sendErr
			}
			return err
		}

		log.Info().
			Int64("chat_id", ctx.EffectiveChat.Id).
			Int("mode", mode).
			Int64("operator_id", ctx.EffectiveSender.Id()).
			Msg("group emotion nsfw mode updated")
		_, err = b.SendMessage(ctx.EffectiveChat.Id, fmt.Sprintf("已设置本群表情 NSFW 模式为 %s。", formatGroupEmotionNSFWMode(mode)), nil)
		return err
	}
}

func parseGroupEmotionNSFWMode(text string) (int, bool) {
	fields := strings.Fields(text)
	if len(fields) != 2 {
		return 0, false
	}
	value, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, false
	}
	if value < groupEmotionNSFWModeSafe || value > groupEmotionNSFWModeMixed {
		return 0, false
	}
	return value, true
}

func formatGroupEmotionNSFWMode(mode int) string {
	switch mode {
	case groupEmotionNSFWModeSafe:
		return "0（只搜正常图）"
	case groupEmotionNSFWModeOnlyNSFW:
		return "1（只搜 NSFW）"
	default:
		return "2（不过滤 NSFW）"
	}
}
