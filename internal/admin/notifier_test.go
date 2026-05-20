package admin

import (
	"chatbot/pkg/config"
	"strings"
	"testing"
	"time"
)

func TestNewFeedbackNotifierSkipsZeroChatID(t *testing.T) {
	notifier := NewFeedbackNotifier(config.AdminConfig{
		ChatIDs: []int64{0, 123, 0, 456},
	})

	if len(notifier.chatIDs) != 2 {
		t.Fatalf("chat id count = %d, want 2", len(notifier.chatIDs))
	}
	if notifier.chatIDs[0] != 123 || notifier.chatIDs[1] != 456 {
		t.Fatalf("chat ids = %#v, want [123 456]", notifier.chatIDs)
	}
}

func TestBuildStartupInfo(t *testing.T) {
	startedAt := time.Date(2026, time.May, 20, 9, 30, 45, 0, time.FixedZone("CST", 8*3600))
	message := buildStartupInfo(startedAt)

	for _, want := range []string{
		"========== BOT STATUS ==========",
		"[Startup]",
		"机器人启动成功。",
		"started_at: 2026-05-20 09:30:45 +08:00",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message = %q, want substring %q", message, want)
		}
	}
}
