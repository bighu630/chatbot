package handler

import (
	"chatbot/internal/ai"
	"chatbot/internal/storage/model"
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
	cfg := NewGroupEmotionNSFWConfig(&fakeGroupConfigRepo{
		records: map[int64]*model.GroupConfig{
			-1001: {ChatID: -1001, EmotionNSFWMode: intPtr(1)},
		},
	})
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

func TestEmotionReplyClientSearchImageForcePhotoForNSFW(t *testing.T) {
	client := newEmotionReplyClient(config.EmotionConfig{
		Enable:     true,
		APIBaseURL: "https://example.test",
		APIKey:     "secret",
	})
	client.httpClient = &http.Client{Transport: testRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
			"data": {
				"matched_count": 1,
				"matches": [
					{"asset_id": 1, "image_url": "https://example.com/nsfw.png", "telegram_sticker_id": "CAAC_NSFW", "distance": 0.1, "nsfw_score": 0.9, "is_nsfw": true}
				]
			}
		}`)),
		}, nil
	})}

	match, err := client.searchImage(ai.EmotionSearchParams{
		Scores: ai.EmotionScores{},
		TopK:   5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if match.imageURL != "https://example.com/nsfw.png" {
		t.Fatalf("imageURL = %q, want https://example.com/nsfw.png", match.imageURL)
	}
	if match.tgStickerID != "" {
		t.Fatalf("tgStickerID = %q, want empty when nsfw", match.tgStickerID)
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
