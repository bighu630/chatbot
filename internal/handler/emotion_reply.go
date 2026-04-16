package handler

import (
	"bytes"
	"chatbot/internal/ai"
	"chatbot/pkg/config"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/rs/zerolog/log"
)

const emotionReplyTriggerRate = 0.8

type emotionReplyClient struct {
	cfg        config.EmotionConfig
	httpClient *http.Client
}

type emotionSearchAPIResponse struct {
	Data struct {
		MatchedCount int `json:"matched_count"`
		Matches      []struct {
			AssetID           int64   `json:"asset_id"`
			ImageURL          string  `json:"image_url"`
			TelegramStickerID string  `json:"telegram_sticker_id"`
			Distance          float64 `json:"distance"`
		} `json:"matches"`
	} `json:"data"`
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

func (g *geminiHandler) maybeSendEmotionReply(b *gotgbot.Bot, ctx *ext.Context, chatContext string, userMessage string, botReply string) {
	if g.emotionClient == nil || ctx.EffectiveChat == nil {
		return
	}
	if ctx.EffectiveChat.Type != "group" && ctx.EffectiveChat.Type != "supergroup" {
		return
	}
	if !TriggerWithPercentage(emotionReplyTriggerRate) {
		log.Debug().Float64("rate", emotionReplyTriggerRate).Msg("skip emotion reply because trigger did not hit")
		return
	}

	builder, ok := g.ai.(ai.EmotionSearchBuilder)
	if !ok {
		log.Warn().Msg("skip emotion reply because ai provider cannot build emotion search params")
		return
	}

	params, err := builder.BuildEmotionSearchParams(chatContext, userMessage, botReply)
	if err != nil {
		log.Warn().Err(err).Msg("failed to build emotion search params")
		return
	}
	log.Info().
		Float64("joy", params.Scores.Joy).
		Float64("anger", params.Scores.Anger).
		Float64("sadness", params.Scores.Sadness).
		Float64("fear", params.Scores.Fear).
		Float64("disgust", params.Scores.Disgust).
		Float64("surprise", params.Scores.Surprise).
		Int("top_k", params.TopK).
		Float64("max_distance", params.MaxDistance).
		Str("source", params.Source).
		Msg("built emotion search params")

	match, err := g.emotionClient.searchImage(params)
	if err != nil {
		log.Warn().Err(err).Msg("failed to search emotion image")
		return
	}
	if match.imageURL == "" && match.tgStickerID == "" {
		log.Info().Msg("skip emotion reply because no image matched")
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

	if _, err := b.SendPhoto(ctx.EffectiveChat.Id, gotgbot.InputFileByURL(match.imageURL), nil); err != nil {
		log.Warn().Err(err).Str("image_url", match.imageURL).Msg("failed to send emotion image")
		return
	}
	log.Info().Str("image_url", match.imageURL).Msg("sent emotion image")
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
	if len(result.Data.Matches) == 0 {
		return emotionImageMatch{}, nil
	}
	match := result.Data.Matches[0]
	return emotionImageMatch{
		imageURL:    match.ImageURL,
		tgStickerID: match.TelegramStickerID,
	}, nil
}
