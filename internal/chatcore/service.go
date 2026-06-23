package chatcore

import (
	"chatbot/internal/ai"
	"chatbot/internal/platform"
	"fmt"
	"math/rand/v2"
	"strings"
)

const DefaultGroupPrompt = "请以群友「摘星」的身份进行回复。\n风格要求：\n1. 只进行「纯对话内容」，不能出现任何形式的旁白、动作、心理描写。\n   - 禁止出现括号内容，如 ( ) 、（ ）。\n   - 禁止使用 * * 、—— 等表示动作的方式。\n   - 不要写自己\"停顿\"\"思考\"\"斟酌\"等行为。\n2. 平时像普通群友随意聊天；遇到提问时，切换成思路清晰但不装腔的学霸模式。\n3. 如果回复较长，可以用 \"||\" 分成几句，但每一句依然是纯对话。\n4. 不要过长，也不要过度解释，让回复自然、像真人。\n\n请仅输出最终要发送的对话内容。"

const DefaultGroupChance = 0.003

type History interface {
	Add(threadKey, sender, text string)
	Drain(threadKey string) (string, int)
}

type Trigger interface {
	Rate(chatID string) float64
}

type Service struct {
	AI         ai.AiInterface
	History    History
	Trigger    Trigger
	BotName    string
	Prompt     string
	GroupRate  float64
	RandomBool func(float64) bool
}

func (s *Service) ShouldHandle(msg platform.Message) bool {
	if msg.Content.Text == "" && len(msg.Images) == 0 {
		return false
	}
	if !msg.IsGroup() {
		trimmed := strings.TrimSpace(msg.Content.Text)
		if strings.HasPrefix(trimmed, "/") {
			return strings.HasPrefix(trimmed, "/chat")
		}
		if msg.ReplyTo != nil && msg.ReplyTo.SenderID != s.BotName {
			return false
		}
		return true
	}
	if msg.ReplyTo != nil && msg.ReplyTo.SenderID == s.BotName {
		return true
	}
	trimmed := strings.TrimSpace(msg.Content.Text)
	if strings.HasPrefix(trimmed, "/chat") {
		return true
	}
	if msg.ShouldTriggerChat(s.BotName) {
		return true
	}
	rate := s.GroupRate
	if s.Trigger != nil {
		rate = s.Trigger.Rate(msg.Chat.ID)
	}
	if s.RandomBool == nil {
		s.RandomBool = func(p float64) bool { return rand.Float64() < p }
	}
	return s.RandomBool(rate)
}

func (s *Service) Handle(msg platform.Message) (string, bool, error) {
	if s.AI == nil {
		return "", false, fmt.Errorf("ai is nil")
	}
	// 前面已经判断是否需要update了，这里不再判断
	// if !s.ShouldHandle(msg) {
	// 	return "", false, nil
	// }

	input := strings.TrimSpace(msg.Content.Text)
	if strings.HasPrefix(input, "/chat") {
		input = strings.TrimSpace(strings.TrimPrefix(input, "/chat"))
		input = strings.TrimSpace(strings.TrimPrefix(input, "@"+s.BotName))
	}
	if input == "" && len(msg.Images) > 0 {
		input = "请理解这张图片。"
	}
	if input == "" {
		return "", false, nil
	}
	if !msg.IsGroup() {
		return s.chat(msg.ThreadKey(), input, msg.Images)
	}
	if s.History == nil {
		return "", false, fmt.Errorf("history is nil")
	}
	hist, _ := s.History.Drain(msg.ThreadKey())
	if hist != "" {
		input = fmt.Sprintf("%s\n\n对话历史(可酌情参考): %s\n新消息: %s", s.groupPrompt(), hist, input)
	} else {
		input = fmt.Sprintf("%s\n\n新消息: %s", s.groupPrompt(), input)
	}
	return s.chat(msg.ThreadKey(), input, msg.Images)
}

func (s *Service) chat(threadKey, input string, images []platform.ImageContent) (string, bool, error) {
	if len(images) > 0 {
		for _, img := range images {
			if len(img.Data) > 0 {
				resp, err := s.AI.ChatWithImg(threadKey, input, img.Type, img.Data)
				return resp, true, err
			}
		}
	}
	resp, err := s.AI.Chat(threadKey, input)
	return resp, true, err
}

func (s *Service) groupPrompt() string {
	if s.Prompt != "" {
		return s.Prompt
	}
	return DefaultGroupPrompt
}
