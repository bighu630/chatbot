package gemini

import (
	"chatbot/ai"
	"chatbot/config"
	"chatbot/storage/models"
	"chatbot/storage/storageImpl"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/genai"
)

const (
	saveTime = 100 * time.Hour
)

var _ ai.AiInterface = (*gemini)(nil)

type gemini struct {
	client    *genai.Client
	chats     map[string]*genai.Chat
	modelName string
	ctx       context.Context
	db        storageImpl.Chat
}

func NewGemini(cfg config.Ai) *gemini {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: cfg.GeminiKey})
	if err != nil {
		log.Panic().Err(err)
	}
	db, err := storageImpl.InitChatDB()
	if err != nil {
		log.Panic().Err(err)
	}
	modelName := cfg.GeminiModel
	if modelName == "" {
		modelName = "gemini-2.5-flash"
	}

	css := make(map[string]*genai.Chat)
	for _, u := range db.GetAllUser() {
		msgs, err := db.GetMsgByTime(time.Now().Add(-saveTime), time.Now(), u)
		if err != nil {
			log.Error().Err(err).Msg("failed to get chat record")
			continue
		}
		history := []*genai.Content{}
		for _, m := range msgs {
			if m.IsUser {
				history = append(history, genai.NewContentFromText(m.Msg, genai.RoleUser))
			} else {
				history = append(history, genai.NewContentFromText(m.Msg, genai.RoleModel))
			}
		}
		chat, _ := client.Chats.Create(ctx, modelName, nil, history)
		css[u] = chat
	}
	g := &gemini{client, css, modelName, ctx, db}
	go g.autoDeleteDB()
	return g
}

func (g gemini) Name() string {
	return "gemini"
}

func (g *gemini) HandleTextWithImg(msg string, imgType string, imgData []byte) (string, error) {
	resp, err := g.client.Models.GenerateContent(g.ctx, g.modelName,
		[]*genai.Content{genai.NewContentFromBytes(imgData, imgType, genai.RoleUser)}, nil)
	if err != nil {
		log.Error().Err(err).Msg("could not get response from gemini")
		return "", err
	}
	result := fmt.Sprint(resp.Candidates[0].Content.Parts[0])
	return result, nil
}

func (g *gemini) HandleText(msg string) (string, error) {
	input := msg
	resp, err := g.client.Models.GenerateContent(g.ctx,
		g.modelName,
		genai.Text(input), nil)
	if err != nil {
		log.Error().Err(err).Msg("could not get response from gemini")
		return "", err
	}
	result := fmt.Sprint(resp.Candidates[0].Content.Parts[0])
	return result, nil
}

func (g *gemini) ChatWithImg(chatId string, msg string, imgType string, imgData []byte) (string, error) {
	var resp *genai.GenerateContentResponse
	var err error
	cs := g.chats[chatId]
	if cs == nil {
		cs, err = g.client.Chats.Create(g.ctx, g.modelName, nil, nil)
		if err != nil {
			log.Error().Err(err).Msg("failed to create chat")
			return "", err
		}
		g.chats[chatId] = cs
	}
	if err = g.db.Add(models.NewChat(chatId, true, msg)); err != nil {
		log.Error().Err(err).Msg("failed to add chat record")
	}
	for range 3 {
		if len(imgData) > 0 {
			part := genai.NewPartFromBytes(imgData, imgType)
			part.Text = msg
			resp, err = cs.SendMessage(g.ctx, *part)
		} else {
			part := genai.NewPartFromText(msg)
			resp, err = cs.SendMessage(g.ctx, *part)
		}

		if err != nil {
			log.Error().Err(err).Msg("failed to send message to gemini")
		} else {
			result := resp.Candidates[0].Content.Parts[0].Text
			if err := g.db.Add(models.NewChat(chatId, false, result)); err != nil {
				log.Error().Err(err).Msg("failed to add chat record")
				return "", err
			}
			return result, nil
		}
	}
	return "", errors.New("failed to send message to gemini")
}

func (g *gemini) Chat(chatId string, msg string) (string, error) {
	return g.ChatWithImg(chatId, msg, "", nil)
}

func (g *gemini) AddChatMsg(chatId string, userSay string, botSay string) error {
	return nil
}

func (g *gemini) Translate(text string) (string, error) {
	return "", nil
}

func (g *gemini) autoDeleteDB() {
	ticker := time.NewTicker(saveTime)
	t := time.Now()
	for {
		<-ticker.C
		g.db.DeleteMsgBeforeTime(t)
		t = time.Now()
	}
}
