package openaiutil

import (
	"bytes"
	"chatbot/pkg/config"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

const invalidChatResponsePreviewLimit = 320

func CreateChatCompletion(
	ctx context.Context,
	httpClient openai.HTTPDoer,
	cfg config.Ai,
	reqBody openai.ChatCompletionRequest,
) (openai.ChatCompletionResponse, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf("marshal chat completion request: %w", err)
	}

	endpoint := strings.TrimRight(cfg.OpenAiBaseUrl, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf("build chat completion request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if cfg.OpenAiKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.OpenAiKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return openai.ChatCompletionResponse{}, err
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf("read chat completion response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
		return openai.ChatCompletionResponse{}, buildChatCompletionError(resp.Status, resp.StatusCode, resp.Header.Get("Content-Type"), rawBody)
	}

	var result openai.ChatCompletionResponse
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf(
			"invalid chat completion response: status=%s content_type=%q body_preview=%q: %w",
			resp.Status,
			resp.Header.Get("Content-Type"),
			bodyPreview(rawBody),
			err,
		)
	}
	return result, nil
}

func buildChatCompletionError(status string, statusCode int, contentType string, rawBody []byte) error {
	var errResp openai.ErrorResponse
	if err := json.Unmarshal(rawBody, &errResp); err == nil && errResp.Error != nil {
		errResp.Error.HTTPStatus = status
		errResp.Error.HTTPStatusCode = statusCode
		return errResp.Error
	}

	return fmt.Errorf(
		"chat completion request failed: status=%s content_type=%q body_preview=%q",
		status,
		contentType,
		bodyPreview(rawBody),
	)
}

func bodyPreview(rawBody []byte) string {
	preview := strings.TrimSpace(string(rawBody))
	if preview == "" {
		return "<empty>"
	}
	preview = strings.Join(strings.Fields(preview), " ")
	runes := []rune(preview)
	if len(runes) <= invalidChatResponsePreviewLimit {
		return preview
	}
	return string(runes[:invalidChatResponsePreviewLimit]) + "..."
}
