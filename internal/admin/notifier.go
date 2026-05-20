package admin

import (
	"chatbot/pkg/config"
	"fmt"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

const feedbackSeparator = "========== FEEDBACK =========="
const startupSeparator = "========== BOT STATUS =========="

type FeedbackNotifier struct {
	chatIDs []int64
}

func NewFeedbackNotifier(cfg config.AdminConfig) *FeedbackNotifier {
	ids := make([]int64, 0, len(cfg.ChatIDs))
	for _, id := range cfg.ChatIDs {
		if id == 0 {
			continue
		}
		ids = append(ids, id)
	}
	return &FeedbackNotifier{chatIDs: ids}
}

func (n *FeedbackNotifier) Enabled() bool {
	return len(n.chatIDs) > 0
}

func (n *FeedbackNotifier) NotifyFeedback(b *gotgbot.Bot, meta FeedbackMeta) error {
	if !n.Enabled() {
		return fmt.Errorf("admin feedback receiver is not configured")
	}

	infoMessage := buildFeedbackInfo(meta)
	var errs []string
	for _, chatID := range n.chatIDs {
		if _, err := b.SendMessage(chatID, infoMessage, nil); err != nil {
			errs = append(errs, fmt.Sprintf("chat %d info message: %v", chatID, err))
			continue
		}
		if _, err := b.SendMessage(chatID, meta.Content, nil); err != nil {
			errs = append(errs, fmt.Sprintf("chat %d content message: %v", chatID, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("notify feedback: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (n *FeedbackNotifier) NotifyStartup(b *gotgbot.Bot, startedAt time.Time) error {
	if !n.Enabled() {
		return fmt.Errorf("admin startup receiver is not configured")
	}

	message := buildStartupInfo(startedAt)
	var errs []string
	for _, chatID := range n.chatIDs {
		if _, err := b.SendMessage(chatID, message, nil); err != nil {
			errs = append(errs, fmt.Sprintf("chat %d startup message: %v", chatID, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("notify startup: %s", strings.Join(errs, "; "))
	}
	return nil
}

type FeedbackMeta struct {
	UserID    int64
	Username  string
	ChatType  string
	ChatID    int64
	ChatTitle string
	Content   string
}

func buildFeedbackInfo(meta FeedbackMeta) string {
	var lines []string
	lines = append(lines, feedbackSeparator)
	lines = append(lines, "[Feedback]")
	lines = append(lines, fmt.Sprintf("from_user_id: %d", meta.UserID))
	lines = append(lines, fmt.Sprintf("from_username: %s", orUnknown(meta.Username)))
	lines = append(lines, fmt.Sprintf("chat_type: %s", orUnknown(meta.ChatType)))
	lines = append(lines, fmt.Sprintf("chat_id: %d", meta.ChatID))
	if meta.ChatTitle != "" {
		lines = append(lines, fmt.Sprintf("chat_title: %s", meta.ChatTitle))
	}
	lines = append(lines, "")
	lines = append(lines, "反馈正文见下一条消息。")
	lines = append(lines, feedbackSeparator)
	return strings.Join(lines, "\n")
}

func buildStartupInfo(startedAt time.Time) string {
	var lines []string
	lines = append(lines, startupSeparator)
	lines = append(lines, "[Startup]")
	lines = append(lines, "机器人启动成功。")
	lines = append(lines, fmt.Sprintf("started_at: %s", startedAt.Format("2006-01-02 15:04:05 -07:00")))
	lines = append(lines, startupSeparator)
	return strings.Join(lines, "\n")
}

func orUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
