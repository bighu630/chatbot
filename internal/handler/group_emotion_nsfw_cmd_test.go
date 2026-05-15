package handler

import "testing"

func TestParseGroupEmotionNSFWMode(t *testing.T) {
	tests := []struct {
		text string
		want int
		ok   bool
	}{
		{text: "/nsfw 0", want: 0, ok: true},
		{text: "/nsfw 1", want: 1, ok: true},
		{text: "/setnsfw 2", want: 2, ok: true},
		{text: "/nsfw -1", ok: false},
		{text: "/nsfw 3", ok: false},
		{text: "/nsfw abc", ok: false},
		{text: "/nsfw", ok: false},
	}
	for _, tt := range tests {
		got, ok := parseGroupEmotionNSFWMode(tt.text)
		if ok != tt.ok {
			t.Fatalf("parseGroupEmotionNSFWMode(%q) ok = %v, want %v", tt.text, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Fatalf("parseGroupEmotionNSFWMode(%q) = %d, want %d", tt.text, got, tt.want)
		}
	}
}

func TestFormatGroupEmotionNSFWMode(t *testing.T) {
	tests := []struct {
		mode int
		want string
	}{
		{0, "0（只搜正常图）"},
		{1, "1（只搜 NSFW）"},
		{2, "2（不过滤 NSFW）"},
	}
	for _, tt := range tests {
		if got := formatGroupEmotionNSFWMode(tt.mode); got != tt.want {
			t.Fatalf("formatGroupEmotionNSFWMode(%d) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}
