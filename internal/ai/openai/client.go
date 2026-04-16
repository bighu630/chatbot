package openai

import (
	"chatbot/internal/ai"
	"chatbot/internal/storage/model"
	"chatbot/internal/storage/repo"
	"chatbot/pkg/config"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	chatCompletionMaxToken = 1000
	emotionSearchMaxToken  = 300
)

var _ ai.AiInterface = (*openAi)(nil)
var _ ai.EmotionSearchBuilder = (*openAi)(nil)

type openAi struct {
	db              repo.Chat
	client          *openai.Client
	fallbackClient  *openai.Client
	fallbackModel   string
	fallbackEnabled bool
	providerLock    sync.RWMutex
	cfg             config.Ai
	ctx             context.Context
	chats           map[string][]openai.ChatCompletionMessage
}

func NewOpenAi(cfg config.Ai) *openAi {
	ctx := context.Background()
	db, err := repo.InitChatDB()
	if err != nil {
		log.Panic().Err(err)
	}
	client := newClient(cfg.OpenAiKey, cfg.OpenAiBaseUrl)
	var fallbackClient *openai.Client
	fallbackModel := cfg.FallbackOpenAiModel
	if fallbackModel == "" {
		fallbackModel = cfg.OpenAiModel
	}
	if hasFallbackOpenAi(cfg) {
		fallbackKey := cfg.FallbackOpenAiKey
		if fallbackKey == "" {
			fallbackKey = cfg.OpenAiKey
		}
		fallbackBaseURL := cfg.FallbackOpenAiBaseUrl
		if fallbackBaseURL == "" {
			fallbackBaseURL = cfg.OpenAiBaseUrl
		}
		fallbackClient = newClient(fallbackKey, fallbackBaseURL)
		log.Info().Str("primary_model", cfg.OpenAiModel).Str("fallback_model", fallbackModel).Msg("openai fallback provider configured")
	}

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

	g := &openAi{
		db:             db,
		client:         client,
		fallbackClient: fallbackClient,
		fallbackModel:  fallbackModel,
		cfg:            cfg,
		ctx:            ctx,
		chats:          css,
	}
	go g.autoDeleteDB()
	log.Info().Msg("openai init success")
	return g
}

func newClient(apiKey string, baseURL string) *openai.Client {
	openaiConfig := openai.DefaultConfig(apiKey)
	openaiConfig.BaseURL = baseURL
	return openai.NewClientWithConfig(openaiConfig)
}

func hasFallbackOpenAi(cfg config.Ai) bool {
	return cfg.FallbackOpenAiKey != "" ||
		cfg.FallbackOpenAiModel != "" ||
		cfg.FallbackOpenAiBaseUrl != ""
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
	provider := o.currentProvider()
	resp, err := provider.client.CreateCompletion(o.ctx, openai.CompletionRequest{
		Model:     provider.model,
		Prompt:    msg,
		MaxTokens: 200,
	})
	if err != nil && provider.name == "primary" && o.shouldFallback(err) {
		o.activateFallback(err)
		provider = o.currentProvider()
		resp, err = provider.client.CreateCompletion(o.ctx, openai.CompletionRequest{
			Model:     provider.model,
			Prompt:    msg,
			MaxTokens: 200,
		})
	}
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
		resp, err := o.createChatCompletion(chatMessages)
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

func (o *openAi) BuildEmotionSearchParams(chatContext string, userMessage string, botReply string) (ai.EmotionSearchParams, error) {
	prompt := buildEmotionSearchPrompt(chatContext, userMessage, botReply)
	provider := o.currentProvider()
	resp, err := provider.client.CreateChatCompletion(o.ctx, openai.ChatCompletionRequest{
		Model:       provider.model,
		Temperature: 0.2,
		MaxTokens:   emotionSearchMaxToken,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: emotionSearchSystemPrompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
	})
	if err != nil && provider.name == "primary" && o.shouldFallback(err) {
		o.activateFallback(err)
		provider = o.currentProvider()
		resp, err = provider.client.CreateChatCompletion(o.ctx, openai.ChatCompletionRequest{
			Model:       provider.model,
			Temperature: 0.2,
			MaxTokens:   emotionSearchMaxToken,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: emotionSearchSystemPrompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
		})
	}
	if err != nil {
		return ai.EmotionSearchParams{}, err
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	params, err := parseEmotionSearchParams(content)
	if err != nil {
		return ai.EmotionSearchParams{}, err
	}
	return normalizeEmotionSearchParams(params), nil
}

type openAiProvider struct {
	name   string
	client *openai.Client
	model  string
}

func (o *openAi) currentProvider() openAiProvider {
	o.providerLock.RLock()
	useFallback := o.fallbackEnabled && o.fallbackClient != nil
	o.providerLock.RUnlock()

	if useFallback {
		return openAiProvider{
			name:   "fallback",
			client: o.fallbackClient,
			model:  o.fallbackModel,
		}
	}
	return openAiProvider{
		name:   "primary",
		client: o.client,
		model:  o.cfg.OpenAiModel,
	}
}

func (o *openAi) createChatCompletion(messages []openai.ChatCompletionMessage) (openai.ChatCompletionResponse, error) {
	provider := o.currentProvider()
	resp, err := provider.client.CreateChatCompletion(o.ctx, openai.ChatCompletionRequest{
		Model:       provider.model,
		Temperature: 1.3, // 对话适用1.3
		Messages:    messages,
		MaxTokens:   chatCompletionMaxToken,
	})
	if err == nil || provider.name == "fallback" || !o.shouldFallback(err) {
		return resp, err
	}

	o.activateFallback(err)
	provider = o.currentProvider()
	return provider.client.CreateChatCompletion(o.ctx, openai.ChatCompletionRequest{
		Model:       provider.model,
		Temperature: 1.3,
		Messages:    messages,
		MaxTokens:   chatCompletionMaxToken,
	})
}

func (o *openAi) shouldFallback(err error) bool {
	if o.fallbackClient == nil || err == nil {
		return false
	}

	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		if apiErr.HTTPStatusCode == 402 || apiErr.HTTPStatusCode == 403 || apiErr.HTTPStatusCode == 429 {
			return true
		}
		return isQuotaLikeError(apiErr.Message) || isQuotaLikeError(apiErr.Type) || isQuotaLikeError(fmt.Sprint(apiErr.Code))
	}

	var requestErr *openai.RequestError
	if errors.As(err, &requestErr) {
		if requestErr.HTTPStatusCode == 402 || requestErr.HTTPStatusCode == 403 || requestErr.HTTPStatusCode == 429 {
			return true
		}
		return isQuotaLikeError(requestErr.Error())
	}

	return isQuotaLikeError(err.Error())
}

func (o *openAi) activateFallback(err error) {
	if o.fallbackClient == nil {
		return
	}

	o.providerLock.Lock()
	defer o.providerLock.Unlock()
	if o.fallbackEnabled {
		return
	}

	o.fallbackEnabled = true
	log.Warn().
		Err(err).
		Str("primary_model", o.cfg.OpenAiModel).
		Str("fallback_model", o.fallbackModel).
		Msg("free openai provider exhausted, switching to fallback provider")
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

func isQuotaLikeError(message string) bool {
	message = strings.ToLower(message)
	keywords := []string{
		"quota",
		"insufficient",
		"rate limit",
		"rate_limit",
		"too many requests",
		"exceeded",
		"exhausted",
		"余额",
		"额度",
		"限流",
		"用完",
	}
	for _, keyword := range keywords {
		if strings.Contains(message, keyword) {
			return true
		}
	}
	return false
}

const emotionSearchSystemPrompt = `你是一个表情检索参数生成器。

你必须只输出 JSON，不要输出解释、Markdown 或多余文字。

输出 JSON schema:
{
  "scores": {
    "joy": number,
    "anger": number,
    "sadness": number,
    "fear": number,
    "disgust": number,
    "surprise": number
  },
  "top_k": number,
  "max_distance": number,
  "source": string,
  "tags": array|null
}

规则：
1. 每个情绪分数范围是 0 到 1。
2. 主要根据 bot 即将发送的回复判断情绪，用户消息和上下文只作为辅助。
3. 调侃、开心、哈哈、玩梗时 joy 较高。
4. 吐槽、嫌弃、无语时 disgust 或 anger 可以中等偏高。
5. 震惊、意外、反转时 surprise 较高。
6. 害怕、担心、被吓到时 fear 较高。
7. 安慰、低落、遗憾时 sadness 可以中等。
8. 中性回复不要给任何情绪过高，最高维度控制在 0.35 左右。
9. top_k 固定为 1。
10. max_distance 固定为 0.75。
11. source 固定为 "telegram-sticker"。
12. tags 默认 null。`

func buildEmotionSearchPrompt(chatContext string, userMessage string, botReply string) string {
	return fmt.Sprintf(`群聊上下文：
%s

用户最新消息：
%s

bot 即将发送的回复：
%s`, chatContext, userMessage, botReply)
}

func parseEmotionSearchParams(content string) (ai.EmotionSearchParams, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var params ai.EmotionSearchParams
	if err := json.Unmarshal([]byte(content), &params); err != nil {
		return ai.EmotionSearchParams{}, err
	}
	return params, nil
}

func normalizeEmotionSearchParams(params ai.EmotionSearchParams) ai.EmotionSearchParams {
	params.Scores.Joy = clamp01(params.Scores.Joy)
	params.Scores.Anger = clamp01(params.Scores.Anger)
	params.Scores.Sadness = clamp01(params.Scores.Sadness)
	params.Scores.Fear = clamp01(params.Scores.Fear)
	params.Scores.Disgust = clamp01(params.Scores.Disgust)
	params.Scores.Surprise = clamp01(params.Scores.Surprise)
	params.TopK = 1
	params.MaxDistance = 0.75
	params.Source = "telegram-sticker"
	if len(params.Tags) == 0 {
		params.Tags = nil
	}
	return params
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
