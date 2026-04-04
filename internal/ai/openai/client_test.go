package openai

import (
	"chatbot/pkg/config"
	"os"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

var openaiInstance *openAi

func init() {
	key := os.Getenv("OPENAI_API_KEY")
	model := os.Getenv("OPENAI_MODEL")
	baseURL := os.Getenv("OPENAI_URL")
	if key == "" {
		// skip initialisation when credentials are not provided
		return
	}
	openaiInstance = NewOpenAi(config.Ai{
		OpenAiKey:     key,
		OpenAiModel:   model,
		OpenAiBaseUrl: baseURL,
	})
}

func TestOpenAi_HandleText(t *testing.T) {
	if openaiInstance == nil {
		t.Skip("OPENAI_API_KEY not set")
	}
	resp, err := openaiInstance.HandleText("你是谁")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(resp)
}

func TestOpenAi_Chat(t *testing.T) {
	if openaiInstance == nil {
		t.Skip("OPENAI_API_KEY not set")
	}
	resp, err := openaiInstance.Chat("123", `看得见这个图片吗
对话包含图片内容这张图片的主体是一只浅灰色的猫，它拥有非常显著的异色瞳：从观察者角度看，它的左眼是蓝色，右眼是绿色。它的鼻子是粉红色的。`)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(resp)
}

func TestOpenAi_Chat_With_Context(t *testing.T) {
	if openaiInstance == nil {
		t.Skip("OPENAI_API_KEY not set")
	}
	resp, err := openaiInstance.Chat("123", "记住你最喜欢的水果是桃子")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(resp)
	resp, err = openaiInstance.Chat("123", "你最喜欢的水果是什么")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(resp)
}

func TestOpenAi_AddChatMsg(t *testing.T) {
	if openaiInstance == nil {
		t.Skip("OPENAI_API_KEY not set")
	}
	err := openaiInstance.AddChatMsg("123", "hello", "hi")
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpenAi_Translate(t *testing.T) {
	if openaiInstance == nil {
		t.Skip("OPENAI_API_KEY not set")
	}
	_, err := openaiInstance.Translate("hello")
	if err == nil {
		t.Fatal("err is nil")
	}
}

func TestBuildTextMessage(t *testing.T) {
	msg, ok := buildTextMessage("user", " hello ")
	if !ok {
		t.Fatal("expected non-empty message to be accepted")
	}
	if msg.Content != "hello" {
		t.Fatalf("buildTextMessage content = %q, want %q", msg.Content, "hello")
	}

	if _, ok := buildTextMessage("user", "   "); ok {
		t.Fatal("expected blank message to be rejected")
	}
}

func TestSanitizeChatMessages(t *testing.T) {
	messages := []openai.ChatCompletionMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "   "},
		{Role: "user", Content: "\nworld\n"},
	}

	sanitized := sanitizeChatMessages(messages)
	if len(sanitized) != 2 {
		t.Fatalf("sanitizeChatMessages len = %d, want 2", len(sanitized))
	}
	if sanitized[1].Content != "world" {
		t.Fatalf("sanitizeChatMessages content = %q, want %q", sanitized[1].Content, "world")
	}
}
