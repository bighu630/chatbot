package platform

type Platform string

const (
	PlatformTelegram Platform = "telegram"
	PlatformQQ       Platform = "qq"
)

type ChatType string

const (
	PrivateChat ChatType = "private"
	GroupChat   ChatType = "group"
)

type ChatRef struct {
	Type ChatType
	ID   string
	Name string
}

type UserRef struct {
	ID       string
	Name     string
	Username string
}

type MessageRef struct {
	Platform  Platform
	ChatID    string
	MessageID string
	SenderID  string
}

type TextContent struct {
	Text string
}

type Mention struct {
	TargetID   string
	TargetName string
}

type ImageContent struct {
	FileID  string
	URL     string
	Type    string
	Data    []byte
}

type Message struct {
	Platform Platform
	Chat     ChatRef
	Sender   UserRef
	Content  TextContent
	Mentions []Mention
	Images   []ImageContent
	ReplyTo  *MessageRef
}

func (m Message) ThreadKey() string {
	return string(m.Platform) + ":" + m.Chat.ID
}

func (m Message) IsGroup() bool {
	return m.Chat.Type == GroupChat
}

func (m Message) ShouldTriggerChat(botName string) bool {
	if m.ReplyTo != nil && m.ReplyTo.SenderID == botName {
		return true
	}
	for _, mention := range m.Mentions {
		if mention.TargetName == botName || mention.TargetID == botName {
			return true
		}
	}
	return false
}
