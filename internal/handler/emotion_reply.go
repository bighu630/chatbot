package handler

import (
	"bytes"
	"chatbot/internal/ai"
	"chatbot/pkg/config"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/rs/zerolog/log"
)

const emotionReplyTriggerRate = 0.8
const emotionReplyDownloadLimit = 20 * 1024 * 1024
const emotionReplyEnabled = false

type emotionReplyClient struct {
	cfg        config.EmotionConfig
	httpClient *http.Client
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

func (g *geminiHandler) maybeSendEmotionReply(b *gotgbot.Bot, ctx *ext.Context, chatContext string, userMessage string, botReply string) {
	if !emotionReplyFeatureEnabled() {
		log.Debug().Msg("skip emotion reply because feature is disabled")
		return
	}
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

	nsfwMode := defaultGroupEmotionNSFWMode
	if g.groupEmotionNSFW != nil {
		nsfwMode = g.groupEmotionNSFW.mode(ctx.EffectiveChat.Id)
	}

	var (
		params ai.EmotionSearchParams
		err    error
	)
	if nsfwMode == groupEmotionNSFWModeOnlyNSFW {
		params = buildEmotionSearchParamsForNSFWOnly()
		log.Debug().Int64("chat_id", ctx.EffectiveChat.Id).Msg("skip ai emotion param generation because nsfw mode is only-nsfw")
	} else {
		builder, ok := g.ai.(ai.EmotionSearchBuilder)
		if !ok {
			log.Warn().Msg("skip emotion reply because ai provider cannot build emotion search params")
			return
		}
		params, err = builder.BuildEmotionSearchParams(chatContext, userMessage, botReply)
		if err != nil {
			log.Warn().Err(err).Msg("failed to build emotion search params")
			return
		}
		params.TopK = 5
		params.MaxDistance = 0.75
		params.Source = "telegram-sticker"
	}
	if g.groupEmotionNSFW != nil {
		nsfwMode = g.groupEmotionNSFW.apply(&params, ctx.EffectiveChat.Id)
	} else {
		// Safe default when config is unavailable.
		value := false
		params.IsNSFW = &value
	}
	log.Info().
		Float64("joy", params.Scores.Joy).
		Float64("anger", params.Scores.Anger).
		Float64("sadness", params.Scores.Sadness).
		Float64("fear", params.Scores.Fear).
		Float64("disgust", params.Scores.Disgust).
		Float64("surprise", params.Scores.Surprise).
		Int("top_k", params.TopK).
		Bool("has_max_distance", params.MaxDistance != 0).
		Bool("has_source", params.Source != "").
		Int("nsfw_mode", nsfwMode).
		Any("is_nsfw", params.IsNSFW).
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

func buildEmotionSearchParamsForNSFWOnly() ai.EmotionSearchParams {
	params := ai.EmotionSearchParams{
		TopK: 5,
	}
	return params
}

func (c *emotionReplyClient) searchImage(params ai.EmotionSearchParams) (emotionImageMatch, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return emotionImageMatch{}, err
	}
	url := strings.TrimRight(c.cfg.APIBaseURL, "/") + "/v1/emotion-assets/search"
	log.Info().
		Str("url", url).
		Str("params_json", string(body)).
		Msg("send emotion search request")
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
	logEmotionSearchMatches(result.Data.Matches)
	if len(result.Data.Matches) == 0 {
		log.Info().Int("matched_count", result.Data.MatchedCount).Msg("emotion search returned no matches")
		return emotionImageMatch{}, nil
	}
	match, ok := selectRandomEmotionMatch(result.Data.Matches)
	if !ok {
		log.Info().Int("matched_count", result.Data.MatchedCount).Msg("emotion search returned only empty matches")
		return emotionImageMatch{}, nil
	}
	if match.IsNSFW {
		if match.ImageURL == "" {
			log.Info().
				Int("matched_count", result.Data.MatchedCount).
				Int64("selected_asset_id", match.AssetID).
				Float64("selected_nsfw_score", match.NSFWScore).
				Bool("selected_is_nsfw", match.IsNSFW).
				Msg("skip nsfw emotion match because image_url is empty")
			return emotionImageMatch{}, nil
		}
		log.Info().
			Int("matched_count", result.Data.MatchedCount).
			Str("selected_image_url", match.ImageURL).
			Float64("selected_nsfw_score", match.NSFWScore).
			Bool("selected_is_nsfw", match.IsNSFW).
			Float64("selected_distance", match.Distance).
			Msg("emotion search selected nsfw match and force image send")
		return emotionImageMatch{
			imageURL: match.ImageURL,
		}, nil
	}
	log.Info().
		Int("matched_count", result.Data.MatchedCount).
		Int64("selected_asset_id", match.AssetID).
		Str("selected_image_url", match.ImageURL).
		Str("selected_tg_sticker_id", match.TelegramStickerID).
		Float64("selected_nsfw_score", match.NSFWScore).
		Bool("selected_is_nsfw", match.IsNSFW).
		Float64("selected_distance", match.Distance).
		Msg("emotion search selected match")
	return emotionImageMatch{
		imageURL:    match.ImageURL,
		tgStickerID: match.TelegramStickerID,
	}, nil
}

func logEmotionSearchMatches(matches []emotionSearchMatch) {
	for index, match := range matches {
		log.Info().
			Int("match_index", index).
			Int64("asset_id", match.AssetID).
			Str("image_url", match.ImageURL).
			Str("tg_sticker_id", match.TelegramStickerID).
			Float64("distance", match.Distance).
			Float64("nsfw_score", match.NSFWScore).
			Bool("is_nsfw", match.IsNSFW).
			Msg("emotion search returned match")
	}
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

func emotionReplyFeatureEnabled() bool {
	return emotionReplyEnabled
}
