package handler

import (
	"chatbot/internal/storage/model"
	"math"
	"testing"
)

func TestGroupReplyTriggerRateDefaults(t *testing.T) {
	cfg := NewGroupReplyTriggerConfig(&fakeGroupConfigRepo{})

	if got := cfg.rate(-1001); !floatEquals(got, randomGroupReplyBaseRate) {
		t.Fatalf("default rate = %v, want %v", got, randomGroupReplyBaseRate)
	}
}

func TestGroupReplyTriggerRateOverrides(t *testing.T) {
	cfg := NewGroupReplyTriggerConfig(&fakeGroupConfigRepo{
		records: map[int64]*model.GroupConfig{
			-1001: {ChatID: -1001, ReplyMultiplier: floatPtr(0)},
			-1002: {ChatID: -1002, ReplyMultiplier: floatPtr(20)},
			-1003: {ChatID: -1003, ReplyMultiplier: floatPtr(99)},
			-1004: {ChatID: -1004, ReplyMultiplier: floatPtr(-1)},
		},
	})

	tests := []struct {
		chatID int64
		want   float64
	}{
		{chatID: -1001, want: 0},
		{chatID: -1002, want: randomGroupReplyBaseRate * 20},
		{chatID: -1003, want: randomGroupReplyBaseRate * 20},
		{chatID: -1004, want: 0},
		{chatID: -1005, want: randomGroupReplyBaseRate},
	}

	for _, tt := range tests {
		if got := cfg.rate(tt.chatID); !floatEquals(got, tt.want) {
			t.Fatalf("rate(%d) = %v, want %v", tt.chatID, got, tt.want)
		}
	}
}

func TestGroupReplyTriggerSetAndSave(t *testing.T) {
	store := &fakeGroupConfigRepo{}
	cfg := NewGroupReplyTriggerConfig(store)
	if err := cfg.setGroupMultiplier(-1001, "测试群", 8); err != nil {
		t.Fatal(err)
	}

	if got, want := cfg.rate(-1001), randomGroupReplyBaseRate*8; !floatEquals(got, want) {
		t.Fatalf("rate = %v, want %v", got, want)
	}
	record := store.records[-1001]
	if record == nil || record.GroupName != "测试群" || record.ReplyMultiplier == nil || *record.ReplyMultiplier != 8 {
		t.Fatalf("record = %#v, want saved multiplier and group name", record)
	}
}

func floatEquals(a, b float64) bool {
	return math.Abs(a-b) < 0.0000001
}
