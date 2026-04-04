package gemini

import (
	"chatbot/internal/ai"
	"chatbot/internal/storage/model"
	"chatbot/internal/storage/repo"
	"chatbot/pkg/config"
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/genai"
)

const (
	saveTime               = 100 * time.Hour
	historyLoadMaxParallel = 8
)

var _ ai.AiInterface = (*gemini)(nil)

type gemini struct {
	client    *genai.Client
	chats     map[string]*genai.Chat
	modelName string
	ctx       context.Context
	db        repo.Chat
}

func NewGemini(cfg config.Ai) *gemini {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: cfg.GeminiKey})
	if err != nil {
		log.Panic().Err(err)
	}
	db, err := repo.InitChatDB()
	if err != nil {
		log.Panic().Err(err)
	}
	modelName := cfg.GeminiModel
	if modelName == "" {
		modelName = "gemini-2.5-flash"
	}

	css := loadChatHistory(ctx, client, db, modelName)
	g := &gemini{client, css, modelName, ctx, db}
	go g.autoDeleteDB()
	return g
}

func loadChatHistory(ctx context.Context, client *genai.Client, db repo.Chat, modelName string) map[string]*genai.Chat {
	users := db.GetAllUser()
	if len(users) == 0 {
		return map[string]*genai.Chat{}
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
		user string
		chat *genai.Chat
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

				history := make([]*genai.Content, 0, len(msgs))
				skippedEmpty := 0
				for _, m := range msgs {
					content := strings.TrimSpace(m.Msg)
					if content == "" {
						skippedEmpty++
						continue
					}
					if m.IsUser {
						history = append(history, genai.NewContentFromText(content, genai.RoleUser))
					} else {
						history = append(history, genai.NewContentFromText(content, genai.RoleModel))
					}
				}
				if skippedEmpty > 0 {
					log.Warn().Str("user", user).Int("skipped_empty_history", skippedEmpty).Msg("skip empty chat history while loading")
				}

				chat, err := client.Chats.Create(ctx, modelName, nil, history)
				if err != nil {
					log.Error().Err(err).Str("user", user).Msg("failed to create chat from history")
					continue
				}

				results <- historyResult{
					user: user,
					chat: chat,
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

	css := make(map[string]*genai.Chat, len(users))
	for result := range results {
		css[result.user] = result.chat
	}

	return css
}

func (g gemini) Name() string {
	return "gemini"
}

func (g *gemini) HandleTextWithImg(msg string, imgType string, imgData []byte) (string, error) {
	resp, err := g.client.Models.GenerateContent(g.ctx, g.modelName,
		[]*genai.Content{genai.NewContentFromBytes(imgData, imgType, genai.RoleUser), genai.Text(msg)[0]}, nil)
	if err != nil {
		log.Error().Err(err).Msg("could not get response from gemini")
		return "", err
	}
	result := fmt.Sprint(resp.Candidates[0].Content.Parts[0].Text)
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
	result := fmt.Sprint(resp.Candidates[0].Content.Parts[0].Text)
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
	if err = g.db.Add(model.NewChat(chatId, true, msg)); err != nil {
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
			if err := g.db.Add(model.NewChat(chatId, false, result)); err != nil {
				log.Error().Err(err).Msg("failed to add chat record")
				return "", err
			}
			return result, nil
		}
	}
	hs := cs.History(true)
	hs = append(hs, genai.Text("处理错误，我忽略这个回答")...)
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
