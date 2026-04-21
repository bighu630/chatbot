package handler

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestGroupReplyTriggerRateDefaults(t *testing.T) {
	cfg := NewGroupReplyTriggerConfig(filepath.Join(t.TempDir(), "missing.json"))

	if got := cfg.rate(-1001); !floatEquals(got, randomGroupReplyBaseRate) {
		t.Fatalf("default rate = %v, want %v", got, randomGroupReplyBaseRate)
	}
}

func TestGroupReplyTriggerRateOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "group_reply_trigger.json")
	if err := os.WriteFile(path, []byte(`{
		"groups": {
			"-1001": 0,
			"-1002": 10,
			"-1003": 99,
			"-1004": -1
		}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := NewGroupReplyTriggerConfig(path)
	tests := []struct {
		chatID int64
		want   float64
	}{
		{chatID: -1001, want: 0},
		{chatID: -1002, want: randomGroupReplyBaseRate * 10},
		{chatID: -1003, want: randomGroupReplyBaseRate * 10},
		{chatID: -1004, want: 0},
		{chatID: -1005, want: randomGroupReplyBaseRate},
	}

	for _, tt := range tests {
		if got := cfg.rate(tt.chatID); !floatEquals(got, tt.want) {
			t.Fatalf("rate(%d) = %v, want %v", tt.chatID, got, tt.want)
		}
	}
}

func TestGroupReplyTriggerDefaultOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "group_reply_trigger.json")
	if err := os.WriteFile(path, []byte(`{"default": 2}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := NewGroupReplyTriggerConfig(path)
	if got, want := cfg.rate(-1001), randomGroupReplyBaseRate*2; !floatEquals(got, want) {
		t.Fatalf("rate = %v, want %v", got, want)
	}
}

func TestGroupReplyTriggerSimpleMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "group_reply_trigger.json")
	if err := os.WriteFile(path, []byte(`{"-1001": 3}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := NewGroupReplyTriggerConfig(path)
	if got, want := cfg.rate(-1001), randomGroupReplyBaseRate*3; !floatEquals(got, want) {
		t.Fatalf("rate = %v, want %v", got, want)
	}
}

func TestGroupReplyTriggerSetAndSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "group_reply_trigger.json")
	cfg := NewGroupReplyTriggerConfig(path)
	if err := cfg.setGroupMultiplier(-1001, 8); err != nil {
		t.Fatal(err)
	}

	reloaded := NewGroupReplyTriggerConfig(path)
	if got, want := reloaded.rate(-1001), randomGroupReplyBaseRate*8; !floatEquals(got, want) {
		t.Fatalf("rate = %v, want %v", got, want)
	}
}

func floatEquals(a, b float64) bool {
	return math.Abs(a-b) < 0.0000001
}
