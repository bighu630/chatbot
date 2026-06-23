package qqonebot

import "testing"

func TestMessageTextFallback(t *testing.T) {
	msg := IncomingMessage{Message: []Segment{{Type: "text", Data: map[string]string{"text": "hi"}}}}
	if got := msg.Text(); got != "hi" {
		t.Fatalf("Text() = %q, want hi", got)
	}
}

func TestMessageTextPrefersRaw(t *testing.T) {
	msg := IncomingMessage{RawMessage: "raw hello"}
	if got := msg.Text(); got != "raw hello" {
		t.Fatalf("Text() = %q, want raw hello", got)
	}
}

func TestIsGroupMessage(t *testing.T) {
	msg := IncomingMessage{MessageType: "group"}
	if !msg.IsGroup() {
		t.Fatal("expected group message")
	}
}
