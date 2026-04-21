package handler

import (
	"chatbot/internal/ai"
	"chatbot/pkg/config"
	"encoding/json"
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
		var payload ai.EmotionSearchParams
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.TopK != 5 {
			t.Fatalf("top_k = %d, want 5", payload.TopK)
		}
		if payload.IsNSFW != nil {
			t.Fatalf("is_nsfw = %#v, want nil by default", payload.IsNSFW)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
			"data": {
				"matched_count": 3,
				"matches": [
					{"asset_id": 0, "image_url": "", "telegram_sticker_id": "", "distance": 0.05},
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

func TestGroupEmotionNSFWApplyToSearchPayload(t *testing.T) {
	cfg := &GroupEmotionNSFWConfig{Groups: map[string]int{"-1001": 1}}
	params := ai.EmotionSearchParams{}
	mode := cfg.apply(&params, -1001)
	if mode != 1 {
		t.Fatalf("mode = %d, want 1", mode)
	}
	if params.IsNSFW == nil || !*params.IsNSFW {
		t.Fatalf("is_nsfw = %#v, want true", params.IsNSFW)
	}
}

func TestNewEmotionReplyClientDisabled(t *testing.T) {
	if client := newEmotionReplyClient(config.EmotionConfig{}); client != nil {
		t.Fatal("expected disabled emotion config to return nil client")
	}
}

func TestBuildEmotionSearchParamsForNSFWOnly(t *testing.T) {
	params := buildEmotionSearchParamsForNSFWOnly()
	values := []float64{
		params.Scores.Joy,
		params.Scores.Anger,
		params.Scores.Sadness,
		params.Scores.Fear,
		params.Scores.Disgust,
		params.Scores.Surprise,
	}
	for _, value := range values {
		if value != 0 {
			t.Fatalf("value = %v, want 0", value)
		}
	}
	if params.TopK != 5 {
		t.Fatalf("top_k = %d, want 5", params.TopK)
	}
	if params.MaxDistance != 0 {
		t.Fatalf("max_distance = %v, want 0", params.MaxDistance)
	}
	if params.Source != "" {
		t.Fatalf("source = %q, want empty", params.Source)
	}
}

func TestBuildEmotionSearchParamsForNSFWOnlyJSON(t *testing.T) {
	params := buildEmotionSearchParamsForNSFWOnly()
	value := true
	params.IsNSFW = &value

	body, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if strings.Contains(got, "max_distance") {
		t.Fatalf("json = %s, should not contain max_distance", got)
	}
	if strings.Contains(got, "source") {
		t.Fatalf("json = %s, should not contain source", got)
	}
	if strings.Contains(got, "tags") {
		t.Fatalf("json = %s, should not contain tags", got)
	}
	if !strings.Contains(got, `"is_nsfw":true`) {
		t.Fatalf("json = %s, should contain is_nsfw true", got)
	}
}
