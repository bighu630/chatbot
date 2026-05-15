package handler

import (
	"chatbot/internal/ai"
	"chatbot/internal/storage/model"
	"testing"
)

func TestGroupEmotionNSFWConfigDefault(t *testing.T) {
	cfg := NewGroupEmotionNSFWConfig(&fakeGroupConfigRepo{})
	if got := cfg.mode(-1001); got != groupEmotionNSFWModeSafe {
		t.Fatalf("mode = %d, want %d", got, groupEmotionNSFWModeSafe)
	}
}

func TestGroupEmotionNSFWConfigModes(t *testing.T) {
	cfg := NewGroupEmotionNSFWConfig(&fakeGroupConfigRepo{
		records: map[int64]*model.GroupConfig{
			-1001: {ChatID: -1001, EmotionNSFWMode: intPtr(0)},
			-1002: {ChatID: -1002, EmotionNSFWMode: intPtr(1)},
			-1003: {ChatID: -1003, EmotionNSFWMode: intPtr(2)},
			-1004: {ChatID: -1004, EmotionNSFWMode: intPtr(99)},
		},
	})
	tests := []struct {
		chatID int64
		want   int
	}{
		{-1001, 0},
		{-1002, 1},
		{-1003, 2},
		{-1004, 2},
		{-1005, 0},
	}
	for _, tt := range tests {
		if got := cfg.mode(tt.chatID); got != tt.want {
			t.Fatalf("mode(%d) = %d, want %d", tt.chatID, got, tt.want)
		}
	}
}

func TestGroupEmotionNSFWApply(t *testing.T) {
	cfg := NewGroupEmotionNSFWConfig(&fakeGroupConfigRepo{
		records: map[int64]*model.GroupConfig{
			-1001: {ChatID: -1001, EmotionNSFWMode: intPtr(0)},
			-1002: {ChatID: -1002, EmotionNSFWMode: intPtr(1)},
			-1003: {ChatID: -1003, EmotionNSFWMode: intPtr(2)},
		},
	})
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
	store := &fakeGroupConfigRepo{}
	cfg := NewGroupEmotionNSFWConfig(store)
	if err := cfg.setGroupMode(-1001, "测试群", 1); err != nil {
		t.Fatal(err)
	}
	if got := cfg.mode(-1001); got != 1 {
		t.Fatalf("mode = %d, want 1", got)
	}
	record := store.records[-1001]
	if record == nil || record.GroupName != "测试群" || record.EmotionNSFWMode == nil || *record.EmotionNSFWMode != 1 {
		t.Fatalf("record = %#v, want saved mode and group name", record)
	}
}

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }
