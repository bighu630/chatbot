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

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/rs/zerolog/log"
)

const (
	saveTime                  = 10 * time.Hour
	historyLoadMaxParallel    = 8
	chatCompletionMaxToken    = 1000
	chatCompletionTemperature = 0.2
	emotionSearchMaxToken     = 300
	thinkingTypeDisabled      = "disabled"
	fallbackPersonaLeakReply  = "我是摘星，群里普通聊天的。"
	chatMessageRoleUser       = "user"
	chatMessageRoleAssistant  = "assistant"
	chatMessageRoleSystem     = "system"
)

var _ ai.AiInterface = (*openAi)(nil)
var _ ai.EmotionSearchBuilder = (*openAi)(nil)

type openAi struct {
	db              repo.Chat
	client          *sdk.Client
	fallbackClient  *sdk.Client
	fallbackModel   string
	fallbackEnabled bool
	providerLock    sync.RWMutex
	cfg             config.Ai
	ctx             context.Context
	chats           map[string][]chatMessage
}

type chatMessage struct {
	Role    string
	Content string
}

func NewOpenAi(cfg config.Ai) *openAi {
	ctx := context.Background()
	db, err := repo.InitChatDB()
	if err != nil {
		log.Panic().Err(err)
	}
	client := newClient(cfg.OpenAiKey, cfg.OpenAiBaseUrl)
	var fallbackClient *sdk.Client
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
			return chatMessageRoleUser
		}
		return chatMessageRoleAssistant
	}

	css := make(map[string][]chatMessage)
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

func newClient(apiKey string, baseURL string) *sdk.Client {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if strings.TrimSpace(baseURL) != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := sdk.NewClient(opts...)
	return &client
}

func hasFallbackOpenAi(cfg config.Ai) bool {
	return cfg.FallbackOpenAiKey != "" ||
		cfg.FallbackOpenAiModel != "" ||
		cfg.FallbackOpenAiBaseUrl != ""
}

func loadChatHistory(db repo.Chat, getRole func(bool) string) map[string][]chatMessage {
	users := db.GetAllUser()
	if len(users) == 0 {
		return map[string][]chatMessage{}
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
		messages []chatMessage
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

				chatMessages := make([]chatMessage, 0, len(msgs))
				skippedEmpty := 0
				for _, m := range msgs {
					role := getRole(m.IsUser)
					content, ok := cleanChatHistoryContent(role, m.Msg)
					if !ok {
						skippedEmpty++
						continue
					}
					if chatMessage, ok := buildTextMessage(role, content); ok {
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

	css := make(map[string][]chatMessage, len(users))
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
	resp, err := provider.client.Completions.New(o.ctx, sdk.CompletionNewParams{
		Model: sdk.CompletionNewParamsModel(provider.model),
		Prompt: sdk.CompletionNewParamsPromptUnion{
			OfString: sdk.String(msg),
		},
		MaxTokens: sdk.Int(200),
	})
	if err != nil && provider.name == "primary" && o.shouldFallback(err) {
		o.activateFallback(err)
		provider = o.currentProvider()
		resp, err = provider.client.Completions.New(o.ctx, sdk.CompletionNewParams{
			Model: sdk.CompletionNewParamsModel(provider.model),
			Prompt: sdk.CompletionNewParamsPromptUnion{
				OfString: sdk.String(msg),
			},
			MaxTokens: sdk.Int(200),
		})
	}
	if err != nil {
		log.Error().Err(err).Msg("could not get response from openai")
		return "", err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", errors.New("openai returned no completion choices")
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
	var chatMessages []chatMessage
	var ok bool
	if chatMessages, ok = o.chats[chatId]; !ok {
		chatMessages = []chatMessage{}
	}

	if len(chatMessages) > 29 {
		chatMessages = chatMessages[len(chatMessages)-30:]
	}
	chatMessages = sanitizeChatMessages(chatMessages)

	userMessage, ok := buildTextMessage(chatMessageRoleUser, msg)
	if !ok {
		return "", errors.New("empty chat message")
	}
	historyContent, ok := cleanChatHistoryContent(chatMessageRoleUser, msg)
	if !ok {
		historyContent = userMessage.Content
	}
	historyUserMessage, ok := buildTextMessage(chatMessageRoleUser, historyContent)
	if !ok {
		historyUserMessage = userMessage
	}
	apiMessages := append(append([]chatMessage{}, chatMessages...), userMessage)
	if o.db != nil {

		if err := o.db.Add(model.NewChat(chatId, true, historyUserMessage.Content)); err != nil {
			log.Error().Err(err).Msg("failed to add chat record")
		}
	}

	for range 3 {
		resp, err := o.createChatCompletion(apiMessages)
		if err != nil {
			log.Error().Err(err).Msg("failed to send message to openai")
		} else {
			result := cleanAssistantOutput(resp)
			if result == "" {
				log.Error().Msg("openai returned empty chat content")
				continue
			}
			assistantMessage, _ := buildTextMessage(chatMessageRoleAssistant, result)
			chatMessages = append(chatMessages, historyUserMessage, assistantMessage)
			o.chats[chatId] = sanitizeChatMessages(chatMessages)
			if err := o.db.Add(model.NewChat(chatId, false, assistantMessage.Content)); err != nil {
				log.Error().Err(err).Msg("failed to add chat record")
				return "", err
			}
			return assistantMessage.Content, nil
		}
	}
	chatMessages = append(chatMessages, chatMessage{
		Role:    chatMessageRoleAssistant,
		Content: "I got something wrong. I'll try again.",
	})
	if err := o.db.Add(model.NewChat(chatId, false, "I got something wrong. I'll try again.")); err != nil {
		log.Error().Err(err).Msg("failed to add chat record")
	}
	return "", errors.New("failed to send message to openai")
}

func (o *openAi) AddChatMsg(chatId string, userSay string, botSay string) error {
	var chatMessages []chatMessage
	var ok bool
	if chatMessages, ok = o.chats[chatId]; !ok {
		return nil
	}
	if userMessage, ok := buildTextMessage(chatMessageRoleUser, userSay); ok {
		chatMessages = append(chatMessages, userMessage)
	}
	if assistantMessage, ok := buildTextMessage(chatMessageRoleAssistant, botSay); ok {
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
	resp, err := o.createProviderChatCompletion(provider, []chatMessage{
		{Role: chatMessageRoleSystem, Content: emotionSearchSystemPrompt},
		{Role: chatMessageRoleUser, Content: prompt},
	}, 0.2, emotionSearchMaxToken)
	if err != nil && provider.name == "primary" && o.shouldFallback(err) {
		o.activateFallback(err)
		provider = o.currentProvider()
		resp, err = o.createProviderChatCompletion(provider, []chatMessage{
			{Role: chatMessageRoleSystem, Content: emotionSearchSystemPrompt},
			{Role: chatMessageRoleUser, Content: prompt},
		}, 0.2, emotionSearchMaxToken)
	}
	if err != nil {
		return ai.EmotionSearchParams{}, err
	}

	content := strings.TrimSpace(resp)
	params, err := parseEmotionSearchParams(content)
	if err != nil {
		return ai.EmotionSearchParams{}, err
	}
	return normalizeEmotionSearchParams(params), nil
}

type openAiProvider struct {
	name   string
	client *sdk.Client
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

func (o *openAi) createChatCompletion(messages []chatMessage) (string, error) {
	provider := o.currentProvider()
	resp, err := o.createProviderChatCompletion(provider, messages, chatCompletionTemperature, chatCompletionMaxToken)
	if err == nil || provider.name == "fallback" || !o.shouldFallback(err) {
		return resp, err
	}

	o.activateFallback(err)
	provider = o.currentProvider()
	return o.createProviderChatCompletion(provider, messages, chatCompletionTemperature, chatCompletionMaxToken)
}

func (o *openAi) createProviderChatCompletion(provider openAiProvider, messages []chatMessage, temperature float64, maxTokens int64) (string, error) {
	resp, err := provider.client.Chat.Completions.New(o.ctx, sdk.ChatCompletionNewParams{
		Model:       sdk.ChatModel(provider.model),
		Messages:    toSDKMessages(messages),
		Temperature: sdk.Float(temperature),
		MaxTokens:   sdk.Int(maxTokens),
	}, thinkingDisabledRequestOption())
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", errors.New("openai returned no chat choices")
	}
	return resp.Choices[0].Message.Content, nil
}

func thinkingDisabledRequestOption() option.RequestOption {
	return option.WithJSONSet("thinking", map[string]any{"type": thinkingTypeDisabled})
}

func toSDKMessages(messages []chatMessage) []sdk.ChatCompletionMessageParamUnion {
	result := make([]sdk.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case chatMessageRoleSystem:
			result = append(result, sdk.SystemMessage(message.Content))
		case chatMessageRoleAssistant:
			result = append(result, sdk.AssistantMessage(message.Content))
		default:
			result = append(result, sdk.UserMessage(message.Content))
		}
	}
	return result
}

func (o *openAi) shouldFallback(err error) bool {
	if o.fallbackClient == nil || err == nil {
		return false
	}

	var apiErr *sdk.Error
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == 402 || apiErr.StatusCode == 403 || apiErr.StatusCode == 429 {
			return true
		}
		return isQuotaLikeError(apiErr.Message) || isQuotaLikeError(apiErr.Type) || isQuotaLikeError(fmt.Sprint(apiErr.Code))
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

func buildTextMessage(role string, content string) (chatMessage, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return chatMessage{}, false
	}
	return chatMessage{
		Role:    role,
		Content: content,
	}, true
}

func sanitizeChatMessages(messages []chatMessage) []chatMessage {
	if len(messages) == 0 {
		return messages
	}

	sanitized := messages[:0]
	for _, message := range messages {
		content, ok := cleanChatHistoryContent(message.Role, message.Content)
		if !ok {
			continue
		}
		if cleaned, ok := buildTextMessage(message.Role, content); ok {
			sanitized = append(sanitized, cleaned)
		}
	}
	return sanitized
}

func cleanChatHistoryContent(role string, content string) (string, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", false
	}

	if role == chatMessageRoleUser {
		content = extractVisibleUserMessage(content)
		content = stripImageAnalysis(content)
		return strings.TrimSpace(content), strings.TrimSpace(content) != ""
	}

	if containsInternalPromptMarker(content) || looksLikePersonaLeak(content) {
		return "", false
	}
	return content, true
}

func extractVisibleUserMessage(content string) string {
	markers := []string{
		"\n收到新消息:",
		"\n新消息:",
		"收到新消息:",
		"新消息:",
	}
	for _, marker := range markers {
		if idx := strings.Index(content, marker); idx >= 0 {
			content = content[idx+len(marker):]
			break
		}
	}

	endMarkers := []string{
		"\n\n请以群友",
		"\n请以群友",
		"\n机器人名字固定",
		"\n摘星人设：",
		"\n风格要求：",
		"\n请仅输出最终要发送的对话内容。",
	}
	for _, marker := range endMarkers {
		if idx := strings.Index(content, marker); idx >= 0 {
			content = content[:idx]
		}
	}
	return content
}

func stripImageAnalysis(content string) string {
	if idx := strings.Index(content, "\n对话包含图片内容"); idx >= 0 {
		return content[:idx]
	}
	if idx := strings.Index(content, "对话包含图片内容"); idx >= 0 {
		return content[:idx]
	}
	return content
}

func containsInternalPromptMarker(content string) bool {
	markers := []string{
		"对话历史(可酌情参考):",
		"请以群友「摘星」的身份进行回复。",
		"机器人名字固定是「摘星」",
		"摘星人设：",
		"风格要求：",
		"请仅输出最终要发送的对话内容。",
		"对话包含图片内容",
	}
	for _, marker := range markers {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func looksLikePersonaLeak(content string) bool {
	if strings.Contains(content, "普通群友") && strings.Contains(content, "学霸模式") {
		return true
	}
	markers := []string{
		"自守点",
		"有限源数据寄存器",
		"反应堆",
		"边界稳在只进行",
		"心智后台动作识别触发接口",
		"结构维护简化",
	}
	for _, marker := range markers {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func cleanAssistantOutput(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if containsInternalPromptMarker(content) || looksLikePersonaLeak(content) {
		log.Warn().Msg("replace assistant output because it looks like persona or prompt leak")
		return fallbackPersonaLeakReply
	}
	return content
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
1. 每个情绪分数范围是 0 到 1，只能输出 schema 中的六个情绪字段。
2. 主要根据 bot 即将发送的回复判断情绪，用户消息和上下文只作为辅助。
3. 先判断回复的主导语气，再把它映射到六维分数；不要平均撒分。
4. 中性、普通说明、事实回答不要给任何情绪过高，最高维度控制在 0.35 左右。
5. 轻微情绪最高维度通常 0.35-0.55；明显情绪最高维度通常 0.55-0.80；极强情绪才超过 0.80。
6. top_k 固定为 5。
7. max_distance 固定为 0.75。
8. source 固定为 "telegram-sticker"。
9. tags 默认 null。

六维基础倾向：
- 调侃、开心、哈哈、玩梗、卖萌：joy 较高。
- 吐槽、嫌弃、无语、阴阳怪气：disgust 或 anger 中等偏高。
- 震惊、意外、反转、突然发现：surprise 较高。
- 害怕、担心、不确定、慌张：fear 较高。
- 安慰、低落、遗憾、失落：sadness 中等。
- 被冒犯、反驳、强烈不满：anger 较高。

复合情绪参考配方：
- 害羞：fear 0.40 + disgust 0.30 + joy 0.20 + surprise 0.10。
- 内疚：sadness 0.35 + fear 0.25 + anger 0.20，另有羞耻感时提高 disgust 或 sadness。
- 嫉妒：anger 0.35 + fear 0.30 + sadness 0.25 + disgust 0.10。
- 羡慕：sadness 0.35 + anger 0.30 + fear 0.15 + disgust 0.10，带正向羡慕时可少量提高 joy。
- 羞耻：disgust 0.35 + sadness 0.30 + fear 0.20 + anger 0.15。
- 自豪：joy 0.55 + anger 0.15 + surprise 0.10；如果是温和自豪，降低 anger。
- 轻蔑：disgust 0.50 + anger 0.40 + joy 0.10。
- 焦虑：fear 0.45 + sadness 0.25 + surprise 0.15 + disgust 0.15。
- 孤独：sadness 0.40 + fear 0.25 + anger 0.20 + disgust 0.15。
- 兴奋：joy 0.55 + surprise 0.25 + fear 0.10 + anger 0.10。
- 敬畏：surprise 0.40 + fear 0.30 + joy 0.20 + disgust 0.10。
- 希望：joy 0.35 + fear 0.35 + surprise 0.30。
- 爱或亲昵：joy 0.30 + fear 0.20 + anger 0.15 + surprise 0.10；如果只是友好亲切，主要提高 joy，其他维度保持低。
- 怀旧：sadness 0.35 + joy 0.35 + fear 0.15 + anger 0.15。
- 讽刺：joy 0.30 + disgust 0.30 + anger 0.25 + surprise 0.15。
- 道德愤怒：anger 0.50 + disgust 0.30 + fear 0.20。
- 思乡：sadness 0.40 + fear 0.30 + joy 0.20 + disgust 0.10。
- 困惑：surprise 0.40 + fear 0.30 + sadness 0.20 + disgust 0.10。
- 投入或上头：joy 0.40 + surprise 0.30 + fear 0.20 + anger 0.10。

使用配方时按实际语气缩放强度：
- 如果只是轻微玩笑或轻微吐槽，把配方整体乘以 0.6-0.8。
- 如果回复短促但情绪明确，可以保留主导维度，压低次要维度。
- 不要输出 trust、desire、shame 等额外字段；这些只能折算进上面的六维。`

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
	params.TopK = 5
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
