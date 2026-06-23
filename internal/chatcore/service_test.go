package chatcore

import (
	"chatbot/internal/ai"
	"chatbot/internal/platform"
	"testing"
)

type fakeAI struct {
	calls    []string
	imgCalls []string
	resp     string
}

func (f *fakeAI) Name() string { return "fake" }
func (f *fakeAI) HandleText(string) (string, error) { return "", nil }
func (f *fakeAI) HandleTextWithImg(string, string, []byte) (string, error) { return "", nil }
func (f *fakeAI) Chat(chatId string, msg string) (string, error) {
	f.calls = append(f.calls, chatId+"|"+msg)
	if f.resp == "" {
		return "ok", nil
	}
	return f.resp, nil
}
func (f *fakeAI) ChatWithImg(chatId string, msg string, imgType string, imgData []byte) (string, error) {
	f.imgCalls = append(f.imgCalls, chatId+"|"+msg+"|"+imgType+"|"+string(imgData))
	return "img-ok", nil
}
func (f *fakeAI) AddChatMsg(string, string, string) error { return nil }
func (f *fakeAI) Translate(string) (string, error) { return "", nil }

type fakeHistory struct {
	added []string
	drain string
}

func (f *fakeHistory) Add(threadKey, sender, text string) {
	f.added = append(f.added, threadKey+"|"+sender+"|"+text)
}
func (f *fakeHistory) Drain(threadKey string) (string, int) {
	return f.drain, 1
}

type fakeTrigger struct{ rate float64 }
func (f fakeTrigger) Rate(chatID string) float64 { return f.rate }

func TestServiceShouldHandlePrivateChats(t *testing.T) {
	svc := &Service{BotName: "bot"}
	if !svc.ShouldHandle(platform.Message{Platform: platform.PlatformTelegram, Chat: platform.ChatRef{Type: platform.PrivateChat, ID: "u1"}, Content: platform.TextContent{Text: "hello"}}) {
		t.Fatal("private text should trigger")
	}
	if svc.ShouldHandle(platform.Message{Platform: platform.PlatformTelegram, Chat: platform.ChatRef{Type: platform.PrivateChat, ID: "u1"}, Content: platform.TextContent{Text: "/help"}}) {
		t.Fatal("private command should not trigger chat")
	}
}

func TestServiceShouldHandleGroupMention(t *testing.T) {
	svc := &Service{BotName: "bot", RandomBool: func(float64) bool { return false }}
	msg := platform.Message{Platform: platform.PlatformQQ, Chat: platform.ChatRef{Type: platform.GroupChat, ID: "-1"}, Content: platform.TextContent{Text: "@bot 你好"}, Mentions: []platform.Mention{{TargetName: "bot"}}}
	if !svc.ShouldHandle(msg) {
		t.Fatal("mention should trigger")
	}
}

func TestServiceHandlePrivateChatsGoToAI(t *testing.T) {
	aiStub := &fakeAI{}
	resp, handled, err := (&Service{AI: aiStub, GroupRate: DefaultGroupChance}).Handle(platform.Message{
		Platform: platform.PlatformTelegram,
		Chat:     platform.ChatRef{Type: platform.PrivateChat, ID: "u1"},
		Sender:   platform.UserRef{Name: "alice"},
		Content:  platform.TextContent{Text: "hello"},
	})
	if err != nil || !handled || resp != "ok" {
		t.Fatalf("resp=%q handled=%v err=%v", resp, handled, err)
	}
	if len(aiStub.calls) != 1 {
		t.Fatalf("ai calls = %d, want 1", len(aiStub.calls))
	}
}

func TestServiceHandleGroupMentionTriggersAI(t *testing.T) {
	aiStub := &fakeAI{}
	history := &fakeHistory{drain: "alice: hi"}
	resp, handled, err := (&Service{AI: aiStub, History: history, BotName: "bot", GroupRate: DefaultGroupChance}).Handle(platform.Message{
		Platform: platform.PlatformQQ,
		Chat:     platform.ChatRef{Type: platform.GroupChat, ID: "-1"},
		Sender:   platform.UserRef{Name: "alice"},
		Content:  platform.TextContent{Text: "@bot 你好"},
		Mentions: []platform.Mention{{TargetName: "bot"}},
	})
	if err != nil || !handled || resp != "ok" {
		t.Fatalf("resp=%q handled=%v err=%v", resp, handled, err)
	}
	if len(aiStub.calls) != 1 {
		t.Fatalf("ai calls = %d, want 1", len(aiStub.calls))
	}
}

func TestServiceHandleGroupImageTriggersAI(t *testing.T) {
	aiStub := &fakeAI{}
	history := &fakeHistory{}
	resp, handled, err := (&Service{AI: aiStub, History: history, BotName: "bot", GroupRate: 0}).Handle(platform.Message{
		Platform: platform.PlatformQQ,
		Chat:     platform.ChatRef{Type: platform.GroupChat, ID: "-1"},
		Sender:   platform.UserRef{Name: "alice"},
		Content:  platform.TextContent{Text: "@bot 看图"},
		Mentions: []platform.Mention{{TargetName: "bot"}},
		Images:   []platform.ImageContent{{Type: "image/png", Data: []byte("img-bytes")}},
	})
	if err != nil || !handled || resp != "img-ok" {
		t.Fatalf("resp=%q handled=%v err=%v", resp, handled, err)
	}
	if len(aiStub.imgCalls) != 1 {
		t.Fatalf("img calls = %d, want 1", len(aiStub.imgCalls))
	}
}

var _ ai.AiInterface = (*fakeAI)(nil)
