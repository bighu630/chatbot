package openai

import (
	"chatbot/internal/ai"
	"chatbot/internal/storage/model"
	"chatbot/internal/storage/repo"
	"chatbot/pkg/config"
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

const (
	saveTime               = 100 * time.Hour
	historyLoadMaxParallel = 8
)

var _ ai.AiInterface = (*openAi)(nil)

type openAi struct {
	db     repo.Chat
	client *openai.Client
	cfg    config.Ai
	ctx    context.Context
	chats  map[string][]openai.ChatCompletionMessage
}

func NewOpenAi(cfg config.Ai) *openAi {
	ctx := context.Background()
	db, err := repo.InitChatDB()
	if err != nil {
		log.Panic().Err(err)
	}
	openai_config := openai.DefaultConfig(cfg.OpenAiKey)
	openai_config.BaseURL = cfg.OpenAiBaseUrl
	client := openai.NewClientWithConfig(openai_config)

	getRole := func(b bool) string {
		if b {
			return openai.ChatMessageRoleUser
		}
		return openai.ChatMessageRoleAssistant
	}

	css := make(map[string][]openai.ChatCompletionMessage)
	if db != nil {
		css = loadChatHistory(db, getRole)
	}

	g := &openAi{db, client, cfg, ctx, css}
	go g.autoDeleteDB()
	log.Info().Msg("openai init success")
	return g
}

func loadChatHistory(db repo.Chat, getRole func(bool) string) map[string][]openai.ChatCompletionMessage {
	users := db.GetAllUser()
	if len(users) == 0 {
		return map[string][]openai.ChatCompletionMessage{}
	}

	now := time.Now()
	from := now.Add(-saveTime)

	workerCount := min(historyLoadMaxParallel, len(users))
	if cpu := runtime.GOMAXPROCS(0); cpu > 0 && cpu < workerCount {
		workerCount = cpu
	}
	if workerCount < 1 {
		workerCount = 1
	}

	type historyResult struct {
		user     string
		messages []openai.ChatCompletionMessage
	}

	jobs := make(chan string)
	results := make(chan historyResult, len(users))

	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for user := range jobs {
				msgs, err := db.GetMsgByTime(from, now, user)
				if err != nil {
					log.Error().Err(err).Str("user", user).Msg("failed to get chat record")
					continue
				}

				chatMessages := make([]openai.ChatCompletionMessage, 0, len(msgs))
				skippedEmpty := 0
				for _, m := range msgs {
					if chatMessage, ok := buildTextMessage(getRole(m.IsUser), m.Msg); ok {
						chatMessages = append(chatMessages, chatMessage)
					} else {
						skippedEmpty++
					}
				}
				if skippedEmpty > 0 {
					log.Warn().Str("user", user).Int("skipped_empty_history", skippedEmpty).Msg("skip empty chat history while loading")
				}

				results <- historyResult{
					user:     user,
					messages: chatMessages,
				}
			}
		}()
	}

	go func() {
		for _, user := range users {
			jobs <- user
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	css := make(map[string][]openai.ChatCompletionMessage, len(users))
	for result := range results {
		css[result.user] = result.messages
	}

	return css
}

func (o openAi) Name() string {
	return "openai"
}
func (o *openAi) HandleTextWithImg(msg string, imgType string, imgData []byte) (string, error) {
	return o.HandleText(msg)
}

func (o *openAi) HandleText(msg string) (string, error) {
	resp, err := o.client.CreateCompletion(o.ctx, openai.CompletionRequest{
		Model:     o.cfg.OpenAiModel,
		Prompt:    msg,
		MaxTokens: 200,
	})
	if err != nil {
		log.Error().Err(err).Msg("could not get response from openai")
		return "", err
	}
	result := resp.Choices[0].Text
	return result, nil
}

// openAi不支持
func (o *openAi) ChatWithImg(chatId string, msg string, imgType string, imgData []byte) (string, error) {
	return o.Chat(chatId, msg)
}

func (o *openAi) Chat(chatId string, msg string) (string, error) {
	log.Debug().Str("getMsg", msg).Msg("get an chat message")
	var chatMessages []openai.ChatCompletionMessage
	var ok bool
	if chatMessages, ok = o.chats[chatId]; !ok {
		chatMessages = []openai.ChatCompletionMessage{}
	}

	if len(chatMessages) > 29 {
		chatMessages = chatMessages[len(chatMessages)-30:]
	}
	chatMessages = sanitizeChatMessages(chatMessages)

	userMessage, ok := buildTextMessage(openai.ChatMessageRoleUser, msg)
	if !ok {
		return "", errors.New("empty chat message")
	}
	chatMessages = append(chatMessages, userMessage)
	if o.db != nil {

		if err := o.db.Add(model.NewChat(chatId, true, userMessage.Content)); err != nil {
			log.Error().Err(err).Msg("failed to add chat record")
		}
	}

	for range 3 {
		resp, err := o.client.CreateChatCompletion(o.ctx, openai.ChatCompletionRequest{
			Model:       o.cfg.OpenAiModel,
			Temperature: 1.3, // 对话适用1.3
			Messages:    chatMessages,
		})
		if err != nil {
			log.Error().Err(err).Msg("failed to send message to openai")
		} else {
			result := strings.TrimSpace(resp.Choices[0].Message.Content)
			if result == "" {
				log.Error().Msg("openai returned empty chat content")
				continue
			}
			assistantMessage, _ := buildTextMessage(openai.ChatMessageRoleAssistant, result)
			chatMessages = append(chatMessages, assistantMessage)
			o.chats[chatId] = chatMessages
			if err := o.db.Add(model.NewChat(chatId, false, assistantMessage.Content)); err != nil {
				log.Error().Err(err).Msg("failed to add chat record")
				return "", err
			}
			return assistantMessage.Content, nil
		}
	}
	chatMessages = append(chatMessages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: "I got something wrong. I'll try again.",
	})
	if err := o.db.Add(model.NewChat(chatId, false, "I got something wrong. I'll try again.")); err != nil {
		log.Error().Err(err).Msg("failed to add chat record")
	}
	return "", errors.New("failed to send message to openai")
}

func (o *openAi) AddChatMsg(chatId string, userSay string, botSay string) error {
	var chatMessages []openai.ChatCompletionMessage
	var ok bool
	if chatMessages, ok = o.chats[chatId]; !ok {
		return nil
	}
	if userMessage, ok := buildTextMessage(openai.ChatMessageRoleUser, userSay); ok {
		chatMessages = append(chatMessages, userMessage)
	}
	if assistantMessage, ok := buildTextMessage(openai.ChatMessageRoleAssistant, botSay); ok {
		chatMessages = append(chatMessages, assistantMessage)
	}
	o.chats[chatId] = sanitizeChatMessages(chatMessages)
	return nil
}

func (o *openAi) Translate(text string) (string, error) {
	return "", errors.New("implement me")
}

func (o *openAi) autoDeleteDB() {
	ticker := time.NewTicker(saveTime)
	t := time.Now()
	for {
		select {
		case <-ticker.C:
			o.db.DeleteMsgBeforeTime(t)
			t = time.Now()
		}
	}
}

func buildTextMessage(role string, content string) (openai.ChatCompletionMessage, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return openai.ChatCompletionMessage{}, false
	}
	return openai.ChatCompletionMessage{
		Role:    role,
		Content: content,
	}, true
}

func sanitizeChatMessages(messages []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
	if len(messages) == 0 {
		return messages
	}

	sanitized := messages[:0]
	for _, message := range messages {
		if cleaned, ok := buildTextMessage(message.Role, message.Content); ok {
			sanitized = append(sanitized, cleaned)
		}
	}
	return sanitized
}
