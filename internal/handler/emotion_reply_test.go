package handler

import (
	"chatbot/internal/ai"
	"chatbot/pkg/config"
	"io"
	"net/http"
	"strings"
	"testing"
)

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
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
			"data": {
				"matched_count": 1,
				"matches": [
					{"asset_id": 1, "image_url": "https://example.com/a.png", "telegram_sticker_id": "CAAC123", "distance": 0.1}
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
	if match.imageURL != "https://example.com/a.png" {
		t.Fatalf("imageURL = %q, want https://example.com/a.png", match.imageURL)
	}
	if match.tgStickerID != "CAAC123" {
		t.Fatalf("tgStickerID = %q, want CAAC123", match.tgStickerID)
	}
}

func TestNewEmotionReplyClientDisabled(t *testing.T) {
	if client := newEmotionReplyClient(config.EmotionConfig{}); client != nil {
		t.Fatal("expected disabled emotion config to return nil client")
	}
}
