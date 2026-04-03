package handler

import "testing"

func TestIsChatCommand(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{text: "/chat hello", want: true},
		{text: "/chat@my_bot hello", want: true},
		{text: "/feedback hello", want: false},
		{text: "/help", want: false},
		{text: "hello", want: false},
	}

	for _, tt := range tests {
		if got := isChatCommand(tt.text); got != tt.want {
			t.Fatalf("isChatCommand(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestIsSlashCommand(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{text: "/feedback hello", want: true},
		{text: "/chat hi", want: true},
		{text: "hello", want: false},
		{text: "", want: false},
	}

	for _, tt := range tests {
		if got := isSlashCommand(tt.text); got != tt.want {
			t.Fatalf("isSlashCommand(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}
