package openai

import (
	"chatbot/pkg/config"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
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

func TestOpenAi_Chat_InvalidJSONSuccessResponse(t *testing.T) {
	httpClient := &http.Client{Transport: testRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q, want /chat/completions", r.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
			Body:       io.NopCloser(strings.NewReader("data: upstream returned non-json")),
		}, nil
	})}

	cfg := config.Ai{
		OpenAiKey:     "test-key",
		OpenAiModel:   "test-model",
		OpenAiBaseUrl: "https://example.test",
	}

	instance := &openAi{
		http:   httpClient,
		cfg:    cfg,
		ctx:    context.Background(),
		chats:  map[string][]openai.ChatCompletionMessage{},
	}

	_, err := instance.Chat("chat-1", "hello")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid chat completion response") {
		t.Fatalf("error = %q, want invalid chat completion response", err.Error())
	}
	if !strings.Contains(err.Error(), "data: upstream returned non-json") {
		t.Fatalf("error = %q, want upstream body preview", err.Error())
	}
}

type testRoundTripper func(*http.Request) (*http.Response, error)

func (f testRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
