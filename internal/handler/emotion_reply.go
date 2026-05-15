package handler

import (
	"bytes"
	"chatbot/internal/ai"
	"chatbot/pkg/config"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

const (
	defaultEmotionAPIBaseURL   = "https://emo.whosworld.fun"
	emotionReplyTriggerRate    = 0.8
	emotionReplyDownloadLimit  = 20 * 1024 * 1024
	emotionReplyTopK           = 5
	emotionReplyMaxDistance    = 0.75
	emotionReplySourceTelegram = "telegram-sticker"
)

type emotionReplyClient struct {
	cfg        config.EmotionConfig
	httpClient *http.Client
}

type emotionParamBuilder struct {
	ctx    context.Context
	client *openai.Client
	model  string
}

type emotionSearchAPIResponse struct {
	Data struct {
		MatchedCount int                  `json:"matched_count"`
		Matches      []emotionSearchMatch `json:"matches"`
	} `json:"data"`
}

type emotionSearchMatch struct {
	AssetID           int64   `json:"asset_id"`
	ImageURL          string  `json:"image_url"`
	TelegramStickerID string  `json:"telegram_sticker_id"`
	Distance          float64 `json:"distance"`
	NSFWScore         float64 `json:"nsfw_score"`
	IsNSFW            bool    `json:"is_nsfw"`
}

type emotionImageMatch struct {
	imageURL    string
	tgStickerID string
}

func newEmotionReplyClient(cfg config.EmotionConfig) *emotionReplyClient {
	if !cfg.Enable {
		return nil
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = defaultEmotionAPIBaseURL
	}
	return &emotionReplyClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func newEmotionParamBuilder(cfg config.Ai) *emotionParamBuilder {
	if strings.TrimSpace(cfg.OpenAiKey) == "" || strings.TrimSpace(cfg.OpenAiModel) == "" {
		return nil
	}
	openaiConfig := openai.DefaultConfig(cfg.OpenAiKey)
	openaiConfig.BaseURL = cfg.OpenAiBaseUrl
	return &emotionParamBuilder{
		ctx:    context.Background(),
		client: openai.NewClientWithConfig(openaiConfig),
		model:  cfg.OpenAiModel,
	}
}

func (g *geminiHandler) maybeSendEmotionReply(b *gotgbot.Bot, ctx *ext.Context, userMessage string, botReply string) {
	if g == nil || g.ai == nil || g.emotionClient == nil || ctx == nil || ctx.EffectiveChat == nil {
		return
	}
	if ctx.EffectiveChat.Type != "group" && ctx.EffectiveChat.Type != "supergroup" {
		return
	}
	if !TriggerWithPercentage(emotionReplyTriggerRate) {
		return
	}

	params, err := g.buildEmotionSearchParams(userMessage, botReply)
	if err != nil {
		log.Warn().Err(err).Msg("failed to build emotion search params")
		return
	}
	if g.groupEmotionNSFW != nil {
		g.groupEmotionNSFW.apply(&params, ctx.EffectiveChat.Id)
	} else {
		value := false
		params.IsNSFW = &value
	}

	match, err := g.emotionClient.searchImage(params)
	if err != nil {
		log.Warn().Err(err).Msg("failed to search emotion image")
		return
	}
	if match.tgStickerID != "" {
		if _, err := b.SendSticker(ctx.EffectiveChat.Id, gotgbot.InputFileByID(match.tgStickerID), nil); err != nil {
			log.Warn().Err(err).Str("tg_sticker_id", match.tgStickerID).Msg("failed to send emotion sticker")
			return
		}
		log.Info().Str("tg_sticker_id", match.tgStickerID).Msg("sent emotion sticker")
		return
	}
	if match.imageURL == "" {
		return
	}
	if _, err := b.SendPhoto(ctx.EffectiveChat.Id, gotgbot.InputFileByURL(match.imageURL), nil); err != nil {
		log.Warn().Err(err).Str("image_url", match.imageURL).Msg("failed to send emotion image by url; fallback to upload")
		name, data, downloadErr := g.emotionClient.downloadEmotionImage(match.imageURL)
		if downloadErr != nil {
			log.Warn().Err(downloadErr).Str("image_url", match.imageURL).Msg("failed to download emotion image for upload fallback")
			return
		}
		if _, uploadErr := b.SendPhoto(ctx.EffectiveChat.Id, gotgbot.InputFileByReader(name, bytes.NewReader(data)), nil); uploadErr != nil {
			log.Warn().Err(uploadErr).Str("image_url", match.imageURL).Msg("failed to send emotion image by upload fallback")
			return
		}
		log.Info().Str("image_url", match.imageURL).Str("file_name", name).Msg("sent emotion image by upload fallback")
		return
	}
	log.Info().Str("image_url", match.imageURL).Msg("sent emotion image")
}

func (g *geminiHandler) buildEmotionSearchParams(userMessage string, botReply string) (ai.EmotionSearchParams, error) {
	if g == nil || g.emotionPromptBuilder == nil {
		return ai.EmotionSearchParams{}, errors.New("emotion prompt builder is unavailable")
	}
	content, err := g.emotionPromptBuilder.Build(userMessage, botReply)
	if err != nil {
		return ai.EmotionSearchParams{}, err
	}
	return content, nil
}

func buildEmotionPrompt(userMessage string, botReply string) string {
	return strings.TrimSpace(fmt.Sprintf(`
你是一个表情检索参数生成器。
你需要根据一次群聊触发消息和机器人回复，给出六维情绪分数。

输出要求：
1. 只能输出 JSON 对象。
2. 字段只能包含 joy、anger、sadness、fear、disgust、surprise。
3. 每个字段值必须是 0 到 1 之间的小数。
4. 不要输出 markdown，不要解释。

群友说：%s
你说：%s
`, userMessage, botReply))
}

func (b *emotionParamBuilder) Build(userMessage string, botReply string) (ai.EmotionSearchParams, error) {
	if b == nil || b.client == nil || strings.TrimSpace(b.model) == "" {
		return ai.EmotionSearchParams{}, errors.New("emotion prompt builder is unavailable")
	}
	resp, err := b.client.CreateChatCompletion(b.ctx, openai.ChatCompletionRequest{
		Model:       b.model,
		Temperature: 0.2,
		MaxTokens:   200,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: buildEmotionPrompt(userMessage, botReply),
			},
		},
	})
	if err != nil {
		return ai.EmotionSearchParams{}, err
	}
	if len(resp.Choices) == 0 {
		return ai.EmotionSearchParams{}, errors.New("emotion prompt returned no choices")
	}
	scores, err := parseEmotionScores(resp.Choices[0].Message.Content)
	if err != nil {
		return ai.EmotionSearchParams{}, err
	}
	return ai.EmotionSearchParams{
		Scores:      scores,
		TopK:        emotionReplyTopK,
		MaxDistance: emotionReplyMaxDistance,
		Source:      emotionReplySourceTelegram,
	}, nil
}

func parseEmotionScores(content string) (ai.EmotionScores, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var scores ai.EmotionScores
	if err := json.Unmarshal([]byte(content), &scores); err != nil {
		return ai.EmotionScores{}, err
	}
	return normalizeEmotionScores(scores), nil
}

func normalizeEmotionScores(scores ai.EmotionScores) ai.EmotionScores {
	scores.Joy = clampUnitFloat(scores.Joy)
	scores.Anger = clampUnitFloat(scores.Anger)
	scores.Sadness = clampUnitFloat(scores.Sadness)
	scores.Fear = clampUnitFloat(scores.Fear)
	scores.Disgust = clampUnitFloat(scores.Disgust)
	scores.Surprise = clampUnitFloat(scores.Surprise)
	return scores
}

func clampUnitFloat(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func (c *emotionReplyClient) searchImage(params ai.EmotionSearchParams) (emotionImageMatch, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return emotionImageMatch{}, err
	}
	url := strings.TrimRight(c.cfg.APIBaseURL, "/") + "/v1/emotion-assets/search"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return emotionImageMatch{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("x-api-key", c.cfg.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return emotionImageMatch{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return emotionImageMatch{}, fmt.Errorf("emotion search failed: %s: %s", resp.Status, string(respBody))
	}

	var result emotionSearchAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return emotionImageMatch{}, err
	}
	match, ok := selectRandomEmotionMatch(result.Data.Matches)
	if !ok {
		return emotionImageMatch{}, nil
	}
	return emotionImageMatch{
		imageURL:    match.ImageURL,
		tgStickerID: match.TelegramStickerID,
	}, nil
}

func selectRandomEmotionMatch(matches []emotionSearchMatch) (emotionSearchMatch, bool) {
	valid := make([]emotionSearchMatch, 0, len(matches))
	for _, match := range matches {
		if match.ImageURL != "" || match.TelegramStickerID != "" {
			valid = append(valid, match)
		}
	}
	if len(valid) == 0 {
		return emotionSearchMatch{}, false
	}
	return valid[rand.N(len(valid))], true
}

func (c *emotionReplyClient) downloadEmotionImage(imageURL string) (string, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, imageURL, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", nil, fmt.Errorf("download emotion image failed: %s: %s", resp.Status, string(body))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, emotionReplyDownloadLimit+1))
	if err != nil {
		return "", nil, err
	}
	if len(data) > emotionReplyDownloadLimit {
		return "", nil, fmt.Errorf("emotion image is larger than %d bytes", emotionReplyDownloadLimit)
	}
	return deriveEmotionImageName(imageURL), data, nil
}

func deriveEmotionImageName(imageURL string) string {
	parsed, err := url.Parse(imageURL)
	if err != nil {
		return "emotion-image"
	}
	name := path.Base(parsed.Path)
	if name == "." || name == "/" || name == "" {
		return "emotion-image"
	}
	return name
}
