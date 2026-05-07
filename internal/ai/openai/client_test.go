package openai

import (
	"chatbot/pkg/config"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
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
	messages := []chatMessage{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "   "},
		{Role: "user", Content: "\nworld\n"},
		{Role: "user", Content: "对话历史(可酌情参考): a: b||c: d\n新消息: 今天吃什么\n对话包含图片内容一张图"},
		{Role: "assistant", Content: "请以群友「摘星」的身份进行回复。"},
	}

	sanitized := sanitizeChatMessages(messages)
	if len(sanitized) != 3 {
		t.Fatalf("sanitizeChatMessages len = %d, want 3", len(sanitized))
	}
	if sanitized[1].Content != "world" {
		t.Fatalf("sanitizeChatMessages content = %q, want %q", sanitized[1].Content, "world")
	}
	if sanitized[2].Content != "今天吃什么" {
		t.Fatalf("sanitizeChatMessages prompt content = %q, want real user message", sanitized[2].Content)
	}
}

func TestCleanChatHistoryContent(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		content string
		want    string
		wantOK  bool
	}{
		{
			name: "extract plain user message from lightweight group prompt",
			role: chatMessageRoleUser,
			content: `对话历史(可酌情参考): alice: 早||bob: 好
新消息: 今天吃什么`,
			want:   "今天吃什么",
			wantOK: true,
		},
		{
			name: "extract plain user message from full persona prompt",
			role: chatMessageRoleUser,
			content: `对话历史(可酌情参考): alice: 早
收到新消息: 你怎么看？

请以群友「摘星」的身份进行回复。
摘星人设： 平时像普通群友随意聊天。`,
			want:   "你怎么看？",
			wantOK: true,
		},
		{
			name: "strip image analysis from stored user message",
			role: chatMessageRoleUser,
			content: `看看这个
对话包含图片内容这张图是一只猫`,
			want:   "看看这个",
			wantOK: true,
		},
		{
			name:    "skip assistant prompt leak",
			role:    chatMessageRoleAssistant,
			content: "请以群友「摘星」的身份进行回复。",
			wantOK:  false,
		},
		{
			name:    "skip assistant persona leak without explicit prompt markers",
			role:    chatMessageRoleAssistant,
			content: `平时当普通群友聊天扯淡，碰到问题就切学霸模式讲清楚原理，但也只到这一步为止不会炫技。比较核心的自守点是不带插件操作也不代挂其他心智后台动作识别触发接口的外起支撑。`,
			wantOK:  false,
		},
		{
			name:    "keep normal assistant reply",
			role:    chatMessageRoleAssistant,
			content: "可以，先这么做。",
			want:    "可以，先这么做。",
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		got, ok := cleanChatHistoryContent(tt.role, tt.content)
		if ok != tt.wantOK || got != tt.want {
			t.Fatalf("%s: cleanChatHistoryContent() = %q, %v; want %q, %v", tt.name, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestCleanAssistantOutput(t *testing.T) {
	leak := `平时当普通群友聊天扯淡，碰到问题就切学霸模式讲清楚原理。比较核心的自守点是不带插件操作也不代挂其他心智后台动作识别触发接口。`
	if got := cleanAssistantOutput(leak); got != fallbackPersonaLeakReply {
		t.Fatalf("cleanAssistantOutput() = %q, want fallback reply", got)
	}

	normal := "我是摘星，群里普通聊天的。"
	if got := cleanAssistantOutput(normal); got != normal {
		t.Fatalf("cleanAssistantOutput() changed normal reply to %q", got)
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

	if !client.shouldFallback(&sdk.Error{StatusCode: 429, Message: "rate limit"}) {
		t.Fatal("expected 429 API error to trigger fallback")
	}

	if client.shouldFallback(&sdk.Error{StatusCode: 400, Message: "bad request"}) {
		t.Fatal("expected 400 bad request to stay on primary provider")
	}
}

func TestCreateProviderChatCompletionSendsThinkingDisabled(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q, want /chat/completions", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"test","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	client := sdk.NewClient(
		option.WithBaseURL(server.URL),
		option.WithAPIKey("test-key"),
		option.WithHTTPClient(server.Client()),
		option.WithMaxRetries(0),
	)
	o := &openAi{ctx: t.Context()}
	got, err := o.createProviderChatCompletion(openAiProvider{
		name:   "primary",
		client: &client,
		model:  "test-model",
	}, []chatMessage{{Role: chatMessageRoleUser, Content: "hello"}}, 0.2, 32)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Fatalf("response = %q, want ok", got)
	}

	thinking, ok := payload["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking = %#v, want object", payload["thinking"])
	}
	if thinking["type"] != thinkingTypeDisabled {
		t.Fatalf("thinking.type = %q, want %q", thinking["type"], thinkingTypeDisabled)
	}
	if payload["max_tokens"] != float64(32) {
		t.Fatalf("max_tokens = %#v, want 32", payload["max_tokens"])
	}
	if _, ok := payload["max_completion_tokens"]; ok {
		t.Fatalf("max_completion_tokens = %#v, want omitted for DeepSeek-compatible chat completions", payload["max_completion_tokens"])
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
