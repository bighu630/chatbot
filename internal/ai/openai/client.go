package openai

import (
	"chatbot/internal/ai"
	"chatbot/internal/ai/openaiutil"
	"chatbot/internal/storage/model"
	"chatbot/internal/storage/repo"
	"chatbot/pkg/config"
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

const (
	saveTime = 100 * time.Hour
)

var _ ai.AiInterface = (*openAi)(nil)

type openAi struct {
	db     repo.Chat
	client *openai.Client
	http   openai.HTTPDoer
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
		for _, u := range db.GetAllUser() {
			msgs, err := db.GetMsgByTime(time.Now().Add(-saveTime), time.Now(), u)
			if err != nil {
				log.Error().Err(err).Msg("failed to get chat record")
				continue
			}
			var chatMessages []openai.ChatCompletionMessage
			for _, m := range msgs {
				chatMessages = append(chatMessages, openai.ChatCompletionMessage{
					Role:    getRole(m.IsUser),
					Content: m.Msg,
				})
			}
			css[u] = chatMessages
		}

	}

	g := &openAi{db: db, client: client, http: openai_config.HTTPClient, cfg: cfg, ctx: ctx, chats: css}
	go g.autoDeleteDB()
	log.Info().Msg("openai init success")
	return g
}

func (o *openAi) createChatCompletion(req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	return openaiutil.CreateChatCompletion(o.ctx, o.http, o.cfg, req)
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

	chatMessages = append(chatMessages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: msg,
	})
	if o.db != nil {

		if err := o.db.Add(model.NewChat(chatId, true, msg)); err != nil {
			log.Error().Err(err).Msg("failed to add chat record")
		}
	}

	var lastErr error
	for range 3 {
		resp, err := o.createChatCompletion(openai.ChatCompletionRequest{
			Model:       o.cfg.OpenAiModel,
			Temperature: 1.3, // 对话适用1.3
			Messages:    chatMessages,
		})
		if err != nil {
			lastErr = err
			log.Error().
				Err(err).
				Str("model", o.cfg.OpenAiModel).
				Str("base_url", o.cfg.OpenAiBaseUrl).
				Msg("failed to send message to openai")
		} else {
			result := resp.Choices[0].Message.Content
			chatMessages = append(chatMessages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: result,
			})
			o.chats[chatId] = chatMessages
			if o.db != nil {
				if err := o.db.Add(model.NewChat(chatId, false, result)); err != nil {
					log.Error().Err(err).Msg("failed to add chat record")
					return "", err
				}
			}
			return result, nil
		}
	}
	chatMessages = append(chatMessages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: "I got something wrong. I'll try again.",
	})
	if o.db != nil {
		if err := o.db.Add(model.NewChat(chatId, false, "I got something wrong. I'll try again.")); err != nil {
			log.Error().Err(err).Msg("failed to add chat record")
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", errors.New("failed to send message to openai")
}

func (o *openAi) AddChatMsg(chatId string, userSay string, botSay string) error {
	var chatMessages []openai.ChatCompletionMessage
	var ok bool
	if chatMessages, ok = o.chats[chatId]; !ok {
		return nil
	}
	chatMessages = append(chatMessages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userSay,
	}, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: botSay,
	})
	o.chats[chatId] = chatMessages
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
