package handler

import (
	"chatbot/internal/ai"
	"chatbot/pkg/config"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBuildEmotionPrompt(t *testing.T) {
	prompt := buildEmotionPrompt("你们怎么又开始吵了", "别急，一个个说。")
	if !strings.Contains(prompt, "群友说：你们怎么又开始吵了") {
		t.Fatalf("prompt = %q, want tagged user message", prompt)
	}
	if !strings.Contains(prompt, "你说：别急，一个个说。") {
		t.Fatalf("prompt = %q, want tagged bot reply", prompt)
	}
}

func TestParseEmotionScores(t *testing.T) {
	scores, err := parseEmotionScores("```json\n{\"joy\":1.2,\"anger\":-0.3,\"sadness\":0.4,\"fear\":0.5,\"disgust\":0.6,\"surprise\":0.7}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if scores.Joy != 1 {
		t.Fatalf("joy = %v, want 1", scores.Joy)
	}
	if scores.Anger != 0 {
		t.Fatalf("anger = %v, want 0", scores.Anger)
	}
	if scores.Sadness != 0.4 || scores.Fear != 0.5 || scores.Disgust != 0.6 || scores.Surprise != 0.7 {
		t.Fatalf("scores = %#v, want normalized values", scores)
	}
}

func TestBuildEmotionSearchParamsUsesHandleText(t *testing.T) {
	fake := &fakeEmotionAI{
		handleTextResp: `{"joy":0.1,"anger":0.2,"sadness":0.3,"fear":0.4,"disgust":0.5,"surprise":0.6}`,
	}
	handler := &geminiHandler{ai: fake}

	params, err := handler.buildEmotionSearchParams("别吵了", "都冷静点。")
	if err != nil {
		t.Fatal(err)
	}
	if fake.handleTextCalled != 1 {
		t.Fatalf("HandleText called %d times, want 1", fake.handleTextCalled)
	}
	if fake.chatCalled != 0 {
		t.Fatalf("Chat called %d times, want 0", fake.chatCalled)
	}
	if !strings.Contains(fake.lastHandleTextPrompt, "群友说：别吵了") {
		t.Fatalf("prompt = %q, want tagged user message", fake.lastHandleTextPrompt)
	}
	if !strings.Contains(fake.lastHandleTextPrompt, "你说：都冷静点。") {
		t.Fatalf("prompt = %q, want tagged bot reply", fake.lastHandleTextPrompt)
	}
	if params.TopK != 5 || params.MaxDistance != 0.75 || params.Source != "telegram-sticker" {
		t.Fatalf("params = %#v, want fixed search defaults", params)
	}
	if params.Scores.Surprise != 0.6 {
		t.Fatalf("scores = %#v, want parsed values", params.Scores)
	}
}

func TestEmotionReplyClientSearchImage(t *testing.T) {
	client := newEmotionReplyClient(config.EmotionConfig{
		Enable:     true,
		APIBaseURL: "https://example.test",
		APIKey:     "secret",
	})
	client.httpClient = &http.Client{Transport: testRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/emotion-assets/search" {
			t.Fatalf("path = %q, want /v1/emotion-assets/search", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "secret" {
			t.Fatalf("x-api-key = %q, want secret", got)
		}
		var payload ai.EmotionSearchParams
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.TopK != 5 {
			t.Fatalf("top_k = %d, want 5", payload.TopK)
		}
		if payload.Source != "telegram-sticker" {
			t.Fatalf("source = %q, want telegram-sticker", payload.Source)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
			"data": {
				"matched_count": 2,
				"matches": [
					{"asset_id": 1, "image_url": "https://example.com/a.png", "telegram_sticker_id": "CAAC123", "distance": 0.1},
					{"asset_id": 2, "image_url": "https://example.com/b.png", "telegram_sticker_id": "CAAC456", "distance": 0.2}
				]
			}
		}`)),
		}, nil
	})}

	match, err := client.searchImage(ai.EmotionSearchParams{
		Scores:      ai.EmotionScores{Joy: 0.8},
		TopK:        5,
		MaxDistance: 0.75,
		Source:      "telegram-sticker",
	})
	if err != nil {
		t.Fatal(err)
	}
	validMatches := map[string]string{
		"https://example.com/a.png": "CAAC123",
		"https://example.com/b.png": "CAAC456",
	}
	if gotStickerID, ok := validMatches[match.imageURL]; !ok || gotStickerID != match.tgStickerID {
		t.Fatalf("match = %#v, want one valid non-empty match", match)
	}
}

func TestNewEmotionReplyClientDisabled(t *testing.T) {
	if client := newEmotionReplyClient(config.EmotionConfig{}); client != nil {
		t.Fatal("expected disabled emotion config to return nil client")
	}
}

func TestDownloadEmotionImage(t *testing.T) {
	client := newEmotionReplyClient(config.EmotionConfig{
		Enable:     true,
		APIBaseURL: "https://example.test",
	})
	client.httpClient = &http.Client{Transport: testRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://example.com/assets/emotion.png" {
			t.Fatalf("url = %q, want https://example.com/assets/emotion.png", r.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("pngdata")),
		}, nil
	})}

	name, data, err := client.downloadEmotionImage("https://example.com/assets/emotion.png")
	if err != nil {
		t.Fatal(err)
	}
	if name != "emotion.png" {
		t.Fatalf("name = %q, want emotion.png", name)
	}
	if string(data) != "pngdata" {
		t.Fatalf("data = %q, want pngdata", string(data))
	}
}

func TestDeriveEmotionImageName(t *testing.T) {
	if got := deriveEmotionImageName("https://example.com/assets/emotion.png"); got != "emotion.png" {
		t.Fatalf("name = %q, want emotion.png", got)
	}
	if got := deriveEmotionImageName("https://example.com"); got != "emotion-image" {
		t.Fatalf("name = %q, want emotion-image", got)
	}
}

type fakeEmotionAI struct {
	handleTextResp       string
	handleTextCalled     int
	chatCalled           int
	lastHandleTextPrompt string
}

func (f *fakeEmotionAI) Name() string { return "fake" }

func (f *fakeEmotionAI) HandleText(msg string) (string, error) {
	f.handleTextCalled++
	f.lastHandleTextPrompt = msg
	return f.handleTextResp, nil
}

func (f *fakeEmotionAI) HandleTextWithImg(msg string, imgType string, imgData []byte) (string, error) {
	return "", errors.New("unexpected HandleTextWithImg call")
}

func (f *fakeEmotionAI) Chat(chatId string, msg string) (string, error) {
	f.chatCalled++
	return "", errors.New("unexpected Chat call")
}

func (f *fakeEmotionAI) ChatWithImg(chatId string, msg string, imgType string, imgData []byte) (string, error) {
	return "", errors.New("unexpected ChatWithImg call")
}

func (f *fakeEmotionAI) AddChatMsg(chatId string, userSay string, botSay string) error {
	return nil
}

func (f *fakeEmotionAI) Translate(text string) (string, error) {
	return "", errors.New("unexpected Translate call")
}
