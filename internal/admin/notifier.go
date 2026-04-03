package admin

import (
	"chatbot/pkg/config"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
)

const feedbackSeparator = "========== FEEDBACK =========="
const startupSeparator = "========== SERVICE =========="

type Notifier struct {
	chatIDs []int64
}

func NewNotifier(cfg config.AdminConfig) *Notifier {
	ids := make([]int64, 0, len(cfg.ChatIDs))
	for _, id := range cfg.ChatIDs {
		if id == 0 {
			continue
		}
		ids = append(ids, id)
	}
	return &Notifier{chatIDs: ids}
}

func (n *Notifier) Enabled() bool {
	return len(n.chatIDs) > 0
}

func (n *Notifier) NotifyFeedback(b *gotgbot.Bot, meta FeedbackMeta) error {
	if !n.Enabled() {
		return fmt.Errorf("admin feedback receiver is not configured")
	}

	infoMessage := buildFeedbackInfo(meta)
	return n.sendMessages(b, func(chatID int64) error {
		if _, err := b.SendMessage(chatID, infoMessage, nil); err != nil {
			return fmt.Errorf("info message: %w", err)
		}
		if _, err := b.SendMessage(chatID, meta.Content, nil); err != nil {
			return fmt.Errorf("content message: %w", err)
		}
		return nil
	})
}

func (n *Notifier) NotifyServiceStarted(b *gotgbot.Bot) error {
	if !n.Enabled() {
		return nil
	}

	message := buildServiceStartedMessage()
	return n.sendMessages(b, func(chatID int64) error {
		if _, err := b.SendMessage(chatID, message, nil); err != nil {
			return err
		}
		return nil
	})
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

func buildServiceStartedMessage() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}

	lines := []string{
		startupSeparator,
		"[Service Started]",
		fmt.Sprintf("host: %s", hostname),
		fmt.Sprintf("time: %s", time.Now().Format(time.RFC3339)),
		startupSeparator,
	}
	return strings.Join(lines, "\n")
}

func (n *Notifier) sendMessages(b *gotgbot.Bot, send func(chatID int64) error) error {
	var errs []string
	for _, chatID := range n.chatIDs {
		if err := send(chatID); err != nil {
			errs = append(errs, fmt.Sprintf("chat %d: %v", chatID, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("notify admin: %s", strings.Join(errs, "; "))
	}
	return nil
}

func orUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
