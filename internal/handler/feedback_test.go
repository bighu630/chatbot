package handler

import "testing"

func TestExtractFeedbackContent(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "plain command with content",
			text: "/feedback hello world",
			want: "hello world",
		},
		{
			name: "command with bot mention",
			text: "/feedback@my_bot line one",
			want: "line one",
		},
		{
			name: "command with extra whitespace",
			text: "/feedback   multi   space   content  ",
			want: "multi space content",
		},
		{
			name: "command without content",
			text: "/feedback",
			want: "",
		},
		{
			name: "empty input",
			text: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractFeedbackContent(tt.text); got != tt.want {
				t.Fatalf("extractFeedbackContent(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}
