package handler

import (
	"testing"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

func TestParseGroupReplyActivity(t *testing.T) {
	tests := []struct {
		text string
		want float64
		ok   bool
	}{
		{text: "/activity 0", want: 0, ok: true},
		{text: "/activity 10", want: 10, ok: true},
		{text: "/activity 20", want: 20, ok: true},
		{text: "/setactivity 2.5", want: 2.5, ok: true},
		{text: "/activity -1", ok: false},
		{text: "/activity 21", ok: false},
		{text: "/activity abc", ok: false},
		{text: "/activity", ok: false},
	}

	for _, tt := range tests {
		got, ok := parseGroupReplyActivity(tt.text)
		if ok != tt.ok {
			t.Fatalf("parseGroupReplyActivity(%q) ok = %v, want %v", tt.text, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Fatalf("parseGroupReplyActivity(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestFormatGroupReplyActivity(t *testing.T) {
	tests := []struct {
		value float64
		want  string
	}{
		{value: 0, want: "0"},
		{value: 10, want: "10"},
		{value: 20, want: "20"},
		{value: 2.5, want: "2.5"},
	}

	for _, tt := range tests {
		if got := formatGroupReplyActivity(tt.value); got != tt.want {
			t.Fatalf("formatGroupReplyActivity(%v) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestCanManageGroupReplyActivityAnonymousAdmin(t *testing.T) {
	ctx := &ext.Context{
		EffectiveChat: &gotgbot.Chat{Id: -1001, Type: "supergroup"},
		EffectiveMessage: &gotgbot.Message{
			SenderChat: &gotgbot.Chat{Id: -1001, Type: "supergroup"},
		},
	}
	allowed, err := canManageGroupReplyActivity(&gotgbot.Bot{}, ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("expected anonymous admin message to be allowed")
	}
}
