package admin

import (
	"strings"
	"testing"
)

func TestBuildFeedbackInfoDoesNotCloseSeparator(t *testing.T) {
	meta := FeedbackMeta{
		UserID:    1,
		Username:  "tester",
		ChatType:  "private",
		ChatID:    2,
		ChatTitle: "demo",
		Content:   "hello",
	}

	info := buildFeedbackInfo(meta)
	if !strings.HasPrefix(info, feedbackSeparator) {
		t.Fatalf("feedback info should start with separator, got %q", info)
	}
	if strings.HasSuffix(info, feedbackSeparator) {
		t.Fatalf("feedback info should not end with separator, got %q", info)
	}
}

func TestBuildFeedbackContentAppendsSeparator(t *testing.T) {
	content := buildFeedbackContent("hello")
	if !strings.HasSuffix(content, feedbackSeparator) {
		t.Fatalf("feedback content should end with separator, got %q", content)
	}
}
