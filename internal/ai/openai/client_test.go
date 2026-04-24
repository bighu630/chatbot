package openai

import (
	"chatbot/pkg/config"
	"encoding/json"
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

func TestHasFallbackOpenAi(t *testing.T) {
	if hasFallbackOpenAi(config.Ai{}) {
		t.Fatal("expected empty fallback config to be disabled")
	}

	if !hasFallbackOpenAi(config.Ai{FallbackOpenAiModel: "paid-model"}) {
		t.Fatal("expected fallback config with model to be enabled")
	}
}

func TestIsQuotaLikeError(t *testing.T) {
	tests := []struct {
		message string
		want    bool
	}{
		{message: "quota exceeded", want: true},
		{message: "rate limit reached", want: true},
		{message: "余额不足", want: true},
		{message: "Failed to deserialize JSON body", want: false},
	}

	for _, tt := range tests {
		if got := isQuotaLikeError(tt.message); got != tt.want {
			t.Fatalf("isQuotaLikeError(%q) = %v, want %v", tt.message, got, tt.want)
		}
	}
}

func TestShouldFallback(t *testing.T) {
	client := &openAi{
		fallbackClient: newClient("fallback-key", ""),
	}

	if !client.shouldFallback(&openai.APIError{HTTPStatusCode: 429, Message: "rate limit"}) {
		t.Fatal("expected 429 API error to trigger fallback")
	}

	if client.shouldFallback(&openai.APIError{HTTPStatusCode: 400, Message: "bad request"}) {
		t.Fatal("expected 400 bad request to stay on primary provider")
	}
}

func TestAddThinkingDisabled(t *testing.T) {
	body := []byte(`{"model":"test","messages":[]}`)
	updated, changed, err := addThinkingDisabled(body)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected request body to be updated")
	}

	var payload map[string]any
	if err := json.Unmarshal(updated, &payload); err != nil {
		t.Fatal(err)
	}
	thinking, ok := payload["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking = %#v, want object", payload["thinking"])
	}
	if thinking["type"] != thinkingTypeDisabled {
		t.Fatalf("thinking.type = %q, want %q", thinking["type"], thinkingTypeDisabled)
	}
}

func TestAddThinkingDisabledOverridesExistingValue(t *testing.T) {
	body := []byte(`{"model":"test","thinking":{"type":"enabled"}}`)
	updated, _, err := addThinkingDisabled(body)
	if err != nil {
		t.Fatal(err)
	}

	var payload map[string]any
	if err := json.Unmarshal(updated, &payload); err != nil {
		t.Fatal(err)
	}
	thinking := payload["thinking"].(map[string]any)
	if thinking["type"] != thinkingTypeDisabled {
		t.Fatalf("thinking.type = %q, want %q", thinking["type"], thinkingTypeDisabled)
	}
}

func TestParseEmotionSearchParams(t *testing.T) {
	content := "```json\n" +
		"{\n" +
		`  "scores": {"joy": 1.2, "anger": -0.1, "sadness": 0.1, "fear": 0, "disgust": 0.2, "surprise": 0.4},` + "\n" +
		`  "top_k": 2,` + "\n" +
		`  "max_distance": 0.2,` + "\n" +
		`  "source": "other",` + "\n" +
		`  "tags": null` + "\n" +
		"}\n" +
		"```"
	params, err := parseEmotionSearchParams(content)
	if err != nil {
		t.Fatal(err)
	}

	params = normalizeEmotionSearchParams(params)
	if params.Scores.Joy != 1 {
		t.Fatalf("joy = %v, want 1", params.Scores.Joy)
	}
	if params.Scores.Anger != 0 {
		t.Fatalf("anger = %v, want 0", params.Scores.Anger)
	}
	if params.TopK != 5 {
		t.Fatalf("top_k = %d, want 5", params.TopK)
	}
	if params.Source != "telegram-sticker" {
		t.Fatalf("source = %q, want telegram-sticker", params.Source)
	}
}
