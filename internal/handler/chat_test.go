package handler

import (
	"chatbot/internal/storage/model"
	"testing"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

func TestIsChatCommand(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{text: "/chat hello", want: true},
		{text: "/chat@my_bot hello", want: true},
		{text: "/feedback hello", want: false},
		{text: "/help", want: false},
		{text: "hello", want: false},
	}

	for _, tt := range tests {
		if got := isChatCommand(tt.text); got != tt.want {
			t.Fatalf("isChatCommand(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestIsSlashCommand(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{text: "/feedback hello", want: true},
		{text: "/chat hi", want: true},
		{text: "hello", want: false},
		{text: "", want: false},
	}

	for _, tt := range tests {
		if got := isSlashCommand(tt.text); got != tt.want {
			t.Fatalf("isSlashCommand(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestHasBotMention(t *testing.T) {
	bot := &gotgbot.Bot{User: gotgbot.User{Id: 100, Username: "my_bot"}}

	msg := &gotgbot.Message{
		Text: "hello @my_bot world",
		Entities: []gotgbot.MessageEntity{
			{Type: "mention", Offset: 6, Length: 7},
		},
	}
	if !hasBotMention(msg, bot) {
		t.Fatal("expected mention in middle of message to trigger")
	}

	msg = &gotgbot.Message{
		Text: "hello there",
		Entities: []gotgbot.MessageEntity{
			{Type: "text_mention", User: &gotgbot.User{Id: 100}},
		},
	}
	if !hasBotMention(msg, bot) {
		t.Fatal("expected text_mention to trigger")
	}

	msg = &gotgbot.Message{
		Text: "hello @other_bot world",
		Entities: []gotgbot.MessageEntity{
			{Type: "mention", Offset: 6, Length: 10},
		},
	}
	if hasBotMention(msg, bot) {
		t.Fatal("expected other bot mention not to trigger")
	}
}

type fakeChatRepo struct {
	count int64
	err   error
}

func (f fakeChatRepo) Add(chat *model.Chat) error {
	return nil
}

func (f fakeChatRepo) GetMsgByTime(from, to time.Time, user string) ([]*model.Chat, error) {
	return nil, nil
}

func (f fakeChatRepo) CountMsgByTime(from, to time.Time, user string, isUser bool) (int64, error) {
	return f.count, f.err
}

func (f fakeChatRepo) GetAllUser() []string {
	return nil
}

func (f fakeChatRepo) DeleteMsgBeforeTime(from time.Time) error {
	return nil
}

func TestAllowPrivateChat(t *testing.T) {
	handler := &geminiHandler{chatRepo: fakeChatRepo{count: privateChatDailyLimit - 1}}
	if !handler.allowPrivateChat("alice") {
		t.Fatal("expected private chat below limit to be allowed")
	}

	handler = &geminiHandler{chatRepo: fakeChatRepo{count: privateChatDailyLimit}}
	if handler.allowPrivateChat("alice") {
		t.Fatal("expected private chat at limit to be rejected")
	}
}

func TestLimitReplyLength(t *testing.T) {
	got := limitReplyLength("你好世界", 2)
	if got != "你好" {
		t.Fatalf("limitReplyLength truncated to %q, want %q", got, "你好")
	}

	unchanged := limitReplyLength("hello", 10)
	if unchanged != "hello" {
		t.Fatalf("limitReplyLength changed short input to %q", unchanged)
	}
}

func TestPrivateChatKey(t *testing.T) {
	ctx := &ext.Context{
		EffectiveChat: &gotgbot.Chat{
			Id:   12345,
			Type: "private",
		},
	}

	if got := privateChatKey(ctx); got != "private:12345" {
		t.Fatalf("privateChatKey() = %q, want %q", got, "private:12345")
	}
}

func TestImageForChatAnalysis(t *testing.T) {
	bot := &gotgbot.Bot{User: gotgbot.User{Id: 100, Username: "bot"}}

	currentPhoto := gotgbot.PhotoSize{FileId: "current"}
	ctx := &ext.Context{
		EffectiveMessage: &gotgbot.Message{
			Photo: []gotgbot.PhotoSize{currentPhoto},
			ReplyToMessage: &gotgbot.Message{
				From:  &gotgbot.User{Id: 100, Username: "bot"},
				Photo: []gotgbot.PhotoSize{{FileId: "reply"}},
			},
		},
	}
	got, ok := imageForChatAnalysis(ctx, bot)
	if !ok || got.FileId != "current" {
		t.Fatalf("current photo should be selected, got %q ok=%v", got.FileId, ok)
	}

	ctx = &ext.Context{
		EffectiveMessage: &gotgbot.Message{
			ReplyToMessage: &gotgbot.Message{
				From:  &gotgbot.User{Id: 200, Username: "alice"},
				Photo: []gotgbot.PhotoSize{{FileId: "reply-user"}},
			},
		},
	}
	got, ok = imageForChatAnalysis(ctx, bot)
	if !ok || got.FileId != "reply-user" {
		t.Fatalf("user reply photo should be selected, got %q ok=%v", got.FileId, ok)
	}

	ctx = &ext.Context{
		EffectiveMessage: &gotgbot.Message{
			ReplyToMessage: &gotgbot.Message{
				From:  &gotgbot.User{Id: 100, Username: "bot"},
				Photo: []gotgbot.PhotoSize{{FileId: "reply-bot"}},
			},
		},
	}
	if got, ok = imageForChatAnalysis(ctx, bot); ok {
		t.Fatalf("bot reply photo should be skipped, got %q", got.FileId)
	}
}
