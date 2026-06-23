package platform

import "testing"

func TestGroupThreadKeyUsesPlatformAndGroup(t *testing.T) {
	msg := Message{
		Platform: PlatformQQ,
		Chat: ChatRef{
			Type: GroupChat,
			ID:   "-1001",
			Name: "测试群",
		},
		Sender: UserRef{ID: "u1", Name: "alice"},
		Content: TextContent{Text: "hello"},
	}

	if got, want := msg.ThreadKey(), "qq:-1001"; got != want {
		t.Fatalf("ThreadKey() = %q, want %q", got, want)
	}
}

func TestShouldTriggerGroupChat(t *testing.T) {
	cases := []struct {
		name string
		msg  Message
		want bool
	}{
		{
			name: "mention triggers",
			msg: Message{
				Platform: PlatformQQ,
				Chat:     ChatRef{Type: GroupChat, ID: "-1001"},
				Content:  TextContent{Text: "@bot 你好"},
				Mentions: []Mention{{TargetName: "bot"}},
			},
			want: true,
		},
		{
			name: "reply triggers",
			msg: Message{
				Platform: PlatformQQ,
				Chat:     ChatRef{Type: GroupChat, ID: "-1001"},
				Content:  TextContent{Text: "好的"},
				ReplyTo:  &MessageRef{SenderID: "bot"},
			},
			want: true,
		},
		{
			name: "plain group message does not trigger",
			msg: Message{
				Platform: PlatformQQ,
				Chat:     ChatRef{Type: GroupChat, ID: "-1001"},
				Content:  TextContent{Text: "just chat"},
			},
			want: false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.msg.ShouldTriggerChat("bot"); got != tt.want {
				t.Fatalf("ShouldTriggerChat() = %v, want %v", got, tt.want)
			}
		})
	}
}
