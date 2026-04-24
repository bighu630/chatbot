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

func NewGroupReplyActivityHandler(trigger *GroupReplyTriggerConfig, adminUserIDs []int64) handlers.Response {
	adminIDs := buildAdminIDSet(adminUserIDs)
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		if ctx == nil || ctx.EffectiveChat == nil || ctx.EffectiveMessage == nil {
			return nil
		}
		if ctx.EffectiveChat.Type != "group" && ctx.EffectiveChat.Type != "supergroup" {
			_, err := b.SendMessage(ctx.EffectiveChat.Id, "这个命令只能在群聊里使用。", nil)
			return err
		}

		multiplier, ok := parseGroupReplyActivity(ctx.EffectiveMessage.GetText())
		if !ok {
			_, err := b.SendMessage(ctx.EffectiveChat.Id, "用法：/activity 0-20，例如 /activity 3。0 表示关闭随机插话，20 表示放大 20 倍。", nil)
			return err
		}

		allowed, err := canManageGroupReplyActivity(b, ctx, adminIDs)
		if err != nil {
			log.Warn().Err(err).Int64("chat_id", ctx.EffectiveChat.Id).Msg("failed to check group reply activity permission")
		}
		if !allowed {
			_, sendErr := b.SendMessage(ctx.EffectiveChat.Id, "只有本群管理员或机器人管理员可以设置活跃度。", nil)
			if sendErr != nil {
				return sendErr
			}
			return err
		}

		if err := trigger.setGroupMultiplier(ctx.EffectiveChat.Id, ctx.EffectiveChat.Title, multiplier); err != nil {
			log.Error().Err(err).Int64("chat_id", ctx.EffectiveChat.Id).Float64("multiplier", multiplier).Msg("failed to save group reply activity")
			_, sendErr := b.SendMessage(ctx.EffectiveChat.Id, "活跃度保存失败，请稍后再试。", nil)
			if sendErr != nil {
				return sendErr
			}
			return err
		}

		log.Info().
			Int64("chat_id", ctx.EffectiveChat.Id).
			Float64("multiplier", multiplier).
			Int64("operator_id", ctx.EffectiveSender.Id()).
			Msg("group reply activity updated")
		_, err = b.SendMessage(ctx.EffectiveChat.Id, fmt.Sprintf("已设置本群活跃度为 %s。", formatGroupReplyActivity(multiplier)), nil)
		return err
	}
}

func parseGroupReplyActivity(text string) (float64, bool) {
	fields := strings.Fields(text)
	if len(fields) != 2 {
		return 0, false
	}
	value, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, false
	}
	if value < minGroupReplyMultiplier || value > maxGroupReplyMultiplier {
		return 0, false
	}
	return value, true
}

func canManageGroupReplyActivity(b *gotgbot.Bot, ctx *ext.Context, adminIDs map[int64]struct{}) (bool, error) {
	if ctx == nil || ctx.EffectiveChat == nil {
		return false, nil
	}
	if ctx.EffectiveMessage != nil && ctx.EffectiveMessage.SenderChat != nil && ctx.EffectiveMessage.SenderChat.Id == ctx.EffectiveChat.Id {
		// Anonymous group admins send messages on behalf of the chat itself.
		return true, nil
	}
	if ctx.EffectiveSender == nil {
		return false, nil
	}

	senderID := ctx.EffectiveSender.Id()
	if _, ok := adminIDs[senderID]; ok {
		return true, nil
	}

	member, err := b.GetChatMember(ctx.EffectiveChat.Id, senderID, nil)
	if err != nil {
		return false, err
	}
	status := member.GetStatus()
	return status == "administrator" || status == "creator", nil
}

func formatGroupReplyActivity(multiplier float64) string {
	if multiplier == float64(int64(multiplier)) {
		return strconv.FormatInt(int64(multiplier), 10)
	}
	return strconv.FormatFloat(multiplier, 'f', -1, 64)
}
