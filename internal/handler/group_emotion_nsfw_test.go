package handler

import (
	"os"
	"path/filepath"
	"testing"

	"chatbot/internal/ai"
)

func TestGroupEmotionNSFWConfigDefault(t *testing.T) {
	cfg := NewGroupEmotionNSFWConfig(filepath.Join(t.TempDir(), "missing.json"))
	if got := cfg.mode(-1001); got != groupEmotionNSFWModeSafe {
		t.Fatalf("mode = %d, want %d", got, groupEmotionNSFWModeSafe)
	}
}

func TestGroupEmotionNSFWConfigModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "group_emotion_nsfw.json")
	if err := os.WriteFile(path, []byte(`{
		"default": 2,
		"groups": {
			"-1001": 0,
			"-1002": 1,
			"-1003": 2,
			"-1004": 99
		}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := NewGroupEmotionNSFWConfig(path)
	tests := []struct {
		chatID int64
		want   int
	}{
		{-1001, 0},
		{-1002, 1},
		{-1003, 2},
		{-1004, 2},
		{-1005, 2},
	}
	for _, tt := range tests {
		if got := cfg.mode(tt.chatID); got != tt.want {
			t.Fatalf("mode(%d) = %d, want %d", tt.chatID, got, tt.want)
		}
	}
}

func TestGroupEmotionNSFWApply(t *testing.T) {
	cfg := &GroupEmotionNSFWConfig{
		Default: 0,
		Groups: map[string]int{
			"-1001": 0,
			"-1002": 1,
			"-1003": 2,
		},
	}
	tests := []struct {
		chatID int64
		want   *bool
	}{
		{-1001, boolPtr(false)},
		{-1002, boolPtr(true)},
		{-1003, nil},
	}
	for _, tt := range tests {
		params := ai.EmotionSearchParams{}
		cfg.apply(&params, tt.chatID)
		switch {
		case tt.want == nil && params.IsNSFW != nil:
			t.Fatalf("chat %d expected nil is_nsfw", tt.chatID)
		case tt.want != nil && params.IsNSFW == nil:
			t.Fatalf("chat %d expected non-nil is_nsfw", tt.chatID)
		case tt.want != nil && *params.IsNSFW != *tt.want:
			t.Fatalf("chat %d is_nsfw = %v, want %v", tt.chatID, *params.IsNSFW, *tt.want)
		}
	}
}

func TestGroupEmotionNSFWSetAndSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "group_emotion_nsfw.json")
	cfg := NewGroupEmotionNSFWConfig(path)
	if err := cfg.setGroupMode(-1001, 1); err != nil {
		t.Fatal(err)
	}
	reloaded := NewGroupEmotionNSFWConfig(path)
	if got := reloaded.mode(-1001); got != 1 {
		t.Fatalf("mode = %d, want 1", got)
	}
}

func boolPtr(v bool) *bool {
	return &v
}
