package handler

import (
	"chatbot/internal/storage/repo"
	"fmt"
	"strings"
	"sync"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/rs/zerolog/log"
)

const (
	defaultGroupPersona      = "平时像普通群友随意聊天；遇到提问时，切换成思路清晰但不装腔的学霸模式。"
	maxGroupPersonaRuneCount = 300
)

type GroupPersonaManager struct {
	repo repo.GroupPersona

	mu        sync.RWMutex
	cache     map[int64]string
	forceNext map[int64]bool
}

func NewGroupPersonaManager(personaRepo repo.GroupPersona) *GroupPersonaManager {
	return &GroupPersonaManager{
		repo:      personaRepo,
		cache:     make(map[int64]string),
		forceNext: make(map[int64]bool),
	}
}

func (m *GroupPersonaManager) Persona(chatID int64) string {
	if m == nil {
		return defaultGroupPersona
	}

	m.mu.RLock()
	persona, ok := m.cache[chatID]
	m.mu.RUnlock()
	if ok {
		if persona == "" {
			return defaultGroupPersona
		}
		return persona
	}

	if m.repo == nil {
		return defaultGroupPersona
	}

	record, err := m.repo.GetByChatID(chatID)
	if err != nil {
		log.Error().Err(err).Int64("chat_id", chatID).Msg("failed to load group persona")
		return defaultGroupPersona
	}

	persona = defaultGroupPersona
	if record != nil && strings.TrimSpace(record.Persona) != "" {
		persona = strings.TrimSpace(record.Persona)
	}

	m.mu.Lock()
	if persona == defaultGroupPersona {
		m.cache[chatID] = ""
	} else {
		m.cache[chatID] = persona
	}
	m.mu.Unlock()
	return persona
}

func (m *GroupPersonaManager) Set(chatID int64, persona string, updatedBy int64) error {
	if m == nil {
		return fmt.Errorf("group persona manager is nil")
	}

	persona = strings.TrimSpace(persona)
	if err := validateGroupPersona(persona); err != nil {
		return err
	}
	if m.repo == nil {
		return fmt.Errorf("group persona repo is nil")
	}
	if err := m.repo.Upsert(chatID, persona, updatedBy); err != nil {
		return err
	}

	m.mu.Lock()
	m.cache[chatID] = persona
	m.forceNext[chatID] = true
	m.mu.Unlock()
	return nil
}

func (m *GroupPersonaManager) Clear(chatID int64) error {
	if m == nil {
		return fmt.Errorf("group persona manager is nil")
	}
	if m.repo == nil {
		return fmt.Errorf("group persona repo is nil")
	}
	if err := m.repo.Delete(chatID); err != nil {
		return err
	}

	m.mu.Lock()
	m.cache[chatID] = ""
	m.forceNext[chatID] = true
	m.mu.Unlock()
	return nil
}

func (m *GroupPersonaManager) ConsumeForceNext(chatID int64) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.forceNext[chatID] {
		return false
	}
	delete(m.forceNext, chatID)
	return true
}

func validateGroupPersona(persona string) error {
	persona = strings.TrimSpace(persona)
	if persona == "" {
		return fmt.Errorf("人设不能为空")
	}
	if len([]rune(persona)) > maxGroupPersonaRuneCount {
		return fmt.Errorf("人设长度不能超过 %d 个中文字符", maxGroupPersonaRuneCount)
	}
	return nil
}

func NewGroupPersonaHandler(manager *GroupPersonaManager, adminUserIDs []int64) handlers.Response {
	adminIDs := buildAdminIDSet(adminUserIDs)
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		if ctx == nil || ctx.EffectiveChat == nil || ctx.EffectiveMessage == nil {
			return nil
		}
		if ctx.EffectiveChat.Type != "group" && ctx.EffectiveChat.Type != "supergroup" {
			_, err := b.SendMessage(ctx.EffectiveChat.Id, "这个命令只能在群聊里使用。", nil)
			return err
		}

		allowed, err := canManageGroupReplyActivity(b, ctx, adminIDs)
		if err != nil {
			log.Warn().Err(err).Int64("chat_id", ctx.EffectiveChat.Id).Msg("failed to check group persona permission")
		}
		if !allowed {
			_, sendErr := b.SendMessage(ctx.EffectiveChat.Id, "只有本群管理员或机器人管理员可以设置群人设。", nil)
			if sendErr != nil {
				return sendErr
			}
			return err
		}

		persona, ok := parseGroupPersona(ctx.EffectiveMessage.GetText())
		if !ok {
			_, err := b.SendMessage(ctx.EffectiveChat.Id, "用法：/persona 这里填写群人设。长度不超过 300 个中文字符，机器人名字固定是摘星，不可修改。", nil)
			return err
		}

		operatorID := int64(0)
		if ctx.EffectiveSender != nil {
			operatorID = ctx.EffectiveSender.Id()
		}
		if err := manager.Set(ctx.EffectiveChat.Id, persona, operatorID); err != nil {
			_, sendErr := b.SendMessage(ctx.EffectiveChat.Id, fmt.Sprintf("群人设保存失败：%v。", err), nil)
			if sendErr != nil {
				return sendErr
			}
			return err
		}

		log.Info().
			Int64("chat_id", ctx.EffectiveChat.Id).
			Int64("operator_id", operatorID).
			Int("persona_length", len([]rune(persona))).
			Msg("group persona updated")
		_, err = b.SendMessage(ctx.EffectiveChat.Id, "已设置本群人设。机器人名字固定是摘星，不会被修改；下一次群聊回复会强制按新人设走一次完整分支。", nil)
		return err
	}
}

func NewGroupPersonaClearHandler(manager *GroupPersonaManager, adminUserIDs []int64) handlers.Response {
	adminIDs := buildAdminIDSet(adminUserIDs)
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		if ctx == nil || ctx.EffectiveChat == nil || ctx.EffectiveMessage == nil {
			return nil
		}
		if ctx.EffectiveChat.Type != "group" && ctx.EffectiveChat.Type != "supergroup" {
			_, err := b.SendMessage(ctx.EffectiveChat.Id, "这个命令只能在群聊里使用。", nil)
			return err
		}

		allowed, err := canManageGroupReplyActivity(b, ctx, adminIDs)
		if err != nil {
			log.Warn().Err(err).Int64("chat_id", ctx.EffectiveChat.Id).Msg("failed to check group persona clear permission")
		}
		if !allowed {
			_, sendErr := b.SendMessage(ctx.EffectiveChat.Id, "只有本群管理员或机器人管理员可以清理群人设。", nil)
			if sendErr != nil {
				return sendErr
			}
			return err
		}

		if err := manager.Clear(ctx.EffectiveChat.Id); err != nil {
			_, sendErr := b.SendMessage(ctx.EffectiveChat.Id, "清理群人设失败，请稍后再试。", nil)
			if sendErr != nil {
				return sendErr
			}
			return err
		}

		operatorID := int64(0)
		if ctx.EffectiveSender != nil {
			operatorID = ctx.EffectiveSender.Id()
		}
		log.Info().
			Int64("chat_id", ctx.EffectiveChat.Id).
			Int64("operator_id", operatorID).
			Msg("group persona cleared")
		_, err = b.SendMessage(ctx.EffectiveChat.Id, "已清理本群自定义人设，恢复为默认摘星人设；下一次群聊回复会强制按默认人设走一次完整分支。", nil)
		return err
	}
}

func parseGroupPersona(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}

	parts := strings.SplitN(text, " ", 2)
	if len(parts) < 2 {
		parts = strings.SplitN(text, "\n", 2)
		if len(parts) < 2 {
			return "", false
		}
	}

	persona := strings.TrimSpace(parts[1])
	if persona == "" {
		return "", false
	}
	return persona, true
}
