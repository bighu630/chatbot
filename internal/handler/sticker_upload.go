package handler

import (
	"bytes"
	"chatbot/pkg/config"
	"chatbot/pkg/util"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/rs/zerolog/log"
)

const (
	defaultEmotionAPIBaseURL      = "https://emo.whosworld.fun"
	defaultStickerUploadedFile    = "./data/sticker_uploads.json"
	stickerUploadBatchSize        = 5
	stickerUploadFlushInterval    = 10 * time.Hour
	videoStickerMaxFileSize       = 700 * 1024
	telegramFileDownloadURLFormat = "https://api.telegram.org/file/bot%s/%s"
	emotionTelegramStickerIDField = "telegram_sticker_id"
)

var _ ext.Handler = (*stickerUploadHandler)(nil)

type stickerUploadHandler struct {
	cfg        config.EmotionConfig
	store      *stickerUploadStore
	httpClient *http.Client
	adminIDs   map[int64]struct{}

	mu      sync.Mutex
	queue   []stickerUploadJob
	pending map[string]struct{}
	bot     *gotgbot.Bot
}

type stickerUploadJob struct {
	fileID       string
	fileUniqueID string
	isAnimated   bool
	isVideo      bool
	fileSize     int64
}

type stickerUploadRecord struct {
	Status    string    `json:"status"`
	Reason    string    `json:"reason,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type stickerUploadStore struct {
	path    string
	mu      sync.Mutex
	records map[string]stickerUploadRecord
}

type preparedStickerImage struct {
	fileUniqueID string
	tgStickerID  string
	name         string
	contentType  string
	data         []byte
}

func NewStickerUploadHandler(cfg config.EmotionConfig, adminUserIDs []int64) (ext.Handler, error) {
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = defaultEmotionAPIBaseURL
	}
	if cfg.UploadedFile == "" {
		cfg.UploadedFile = defaultStickerUploadedFile
	}

	store, err := newStickerUploadStore(cfg.UploadedFile)
	if err != nil {
		return nil, err
	}

	h := &stickerUploadHandler{
		cfg:        cfg,
		store:      store,
		httpClient: &http.Client{Timeout: 2 * time.Minute},
		queue:      make([]stickerUploadJob, 0, stickerUploadBatchSize),
		pending:    make(map[string]struct{}),
		adminIDs:   buildAdminIDSet(adminUserIDs),
	}
	go h.flushPeriodically()
	return h, nil
}

func (h *stickerUploadHandler) Name() string {
	return "sticker-upload"
}

func (h *stickerUploadHandler) CheckUpdate(b *gotgbot.Bot, ctx *ext.Context) bool {
	if h == nil || !h.cfg.Enable || ctx == nil || ctx.EffectiveChat == nil || ctx.EffectiveMessage == nil || ctx.EffectiveMessage.Sticker == nil {
		return false
	}
	if !h.shouldCollectFromChat(ctx) {
		sticker := ctx.EffectiveMessage.Sticker
		log.Info().
			Int64("chat_id", ctx.EffectiveChat.Id).
			Str("chat_type", ctx.EffectiveChat.Type).
			Str("file_unique_id", sticker.FileUniqueId).
			Msg("skip sticker upload because chat is not eligible")
		return false
	}
	return true
}

func (h *stickerUploadHandler) shouldCollectFromChat(ctx *ext.Context) bool {
	if ctx == nil || ctx.EffectiveChat == nil {
		return false
	}

	switch ctx.EffectiveChat.Type {
	case "group", "supergroup":
		return true
	case "private":
		senderID := int64(0)
		if ctx.EffectiveSender != nil {
			senderID = ctx.EffectiveSender.Id()
		}
		_, ok := h.adminIDs[senderID]
		return ok
	default:
		return false
	}
}

func buildAdminIDSet(ids []int64) map[int64]struct{} {
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

func (h *stickerUploadHandler) HandleUpdate(b *gotgbot.Bot, ctx *ext.Context) error {
	sticker := ctx.EffectiveMessage.Sticker
	log.Info().
		Int64("chat_id", ctx.EffectiveChat.Id).
		Str("chat_type", ctx.EffectiveChat.Type).
		Str("file_unique_id", sticker.FileUniqueId).
		Str("file_id", sticker.FileId).
		Bool("is_animated", sticker.IsAnimated).
		Bool("is_video", sticker.IsVideo).
		Int64("file_size", sticker.FileSize).
		Msg("received sticker for emotion upload")
	if sticker.FileUniqueId == "" || sticker.FileId == "" {
		log.Warn().Msg("skip sticker upload because file id is empty")
		return nil
	}

	h.mu.Lock()
	h.bot = b
	h.mu.Unlock()

	if sticker.IsVideo && sticker.FileSize > videoStickerMaxFileSize {
		log.Info().
			Str("file_unique_id", sticker.FileUniqueId).
			Int64("file_size", sticker.FileSize).
			Msg("skip sticker upload because video sticker is larger than limit")
		h.store.Mark(sticker.FileUniqueId, "skipped", "video sticker larger than 700KB")
		return nil
	}

	if h.store.Has(sticker.FileUniqueId) {
		log.Debug().Str("file_unique_id", sticker.FileUniqueId).Msg("skip sticker upload because sticker was already processed")
		return nil
	}

	job := stickerUploadJob{
		fileID:       sticker.FileId,
		fileUniqueID: sticker.FileUniqueId,
		isAnimated:   sticker.IsAnimated,
		isVideo:      sticker.IsVideo,
		fileSize:     sticker.FileSize,
	}

	if h.enqueue(job) {
		log.Info().
			Str("file_unique_id", sticker.FileUniqueId).
			Int("batch_size", stickerUploadBatchSize).
			Msg("sticker upload batch is ready")
		go h.flushBatch(b, false)
	}
	return nil
}

func (h *stickerUploadHandler) enqueue(job stickerUploadJob) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.pending[job.fileUniqueID]; ok {
		log.Debug().Str("file_unique_id", job.fileUniqueID).Msg("skip sticker upload because sticker is already pending")
		return false
	}
	h.queue = append(h.queue, job)
	h.pending[job.fileUniqueID] = struct{}{}
	log.Info().
		Str("file_unique_id", job.fileUniqueID).
		Int("queue_size", len(h.queue)).
		Msg("sticker queued for emotion upload")
	return len(h.queue) >= stickerUploadBatchSize
}

func (h *stickerUploadHandler) flushPeriodically() {
	ticker := time.NewTicker(stickerUploadFlushInterval)
	defer ticker.Stop()
	for range ticker.C {
		h.mu.Lock()
		b := h.bot
		h.mu.Unlock()
		h.flushBatch(b, true)
	}
}

func (h *stickerUploadHandler) flushBatch(b *gotgbot.Bot, flushAll bool) {
	jobs := h.takeBatch(flushAll)
	if len(jobs) == 0 {
		return
	}
	log.Info().Int("count", len(jobs)).Bool("flush_all", flushAll).Msg("start sticker upload flush")
	if b == nil {
		log.Warn().Int("count", len(jobs)).Msg("skip timed sticker upload flush because bot is unavailable")
		h.requeue(jobs)
		return
	}

	images := make([]preparedStickerImage, 0, len(jobs))
	for _, job := range jobs {
		log.Info().
			Str("file_unique_id", job.fileUniqueID).
			Bool("is_animated", job.isAnimated).
			Bool("is_video", job.isVideo).
			Int64("file_size", job.fileSize).
			Msg("prepare sticker image for upload")
		image, err := h.prepareStickerImage(b, job)
		if err != nil {
			log.Warn().Err(err).Str("file_unique_id", job.fileUniqueID).Msg("failed to prepare sticker image")
			h.store.Mark(job.fileUniqueID, "skipped", err.Error())
			continue
		}
		images = append(images, image)
	}

	if len(images) == 0 {
		log.Info().Int("count", len(jobs)).Msg("skip sticker upload flush because no images were prepared")
		return
	}

	log.Info().Int("count", len(images)).Msg("upload sticker image batch to emotion service")
	if err := h.uploadImages(images); err != nil {
		log.Error().Err(err).Int("count", len(images)).Msg("failed to upload sticker batch")
		h.requeue(jobs)
		return
	}

	for _, image := range images {
		h.store.Mark(image.fileUniqueID, "processed", "uploaded or skipped by emotion service")
	}
	log.Info().Int("count", len(images)).Msg("sticker batch uploaded")
}

func (h *stickerUploadHandler) takeBatch(flushAll bool) []stickerUploadJob {
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.queue) == 0 || (!flushAll && len(h.queue) < stickerUploadBatchSize) {
		return nil
	}

	n := stickerUploadBatchSize
	if flushAll || len(h.queue) < n {
		n = len(h.queue)
	}
	jobs := append([]stickerUploadJob(nil), h.queue[:n]...)
	h.queue = h.queue[n:]
	for _, job := range jobs {
		delete(h.pending, job.fileUniqueID)
	}
	return jobs
}

func (h *stickerUploadHandler) requeue(jobs []stickerUploadJob) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, job := range jobs {
		if h.store.Has(job.fileUniqueID) {
			continue
		}
		if _, ok := h.pending[job.fileUniqueID]; ok {
			continue
		}
		h.queue = append(h.queue, job)
		h.pending[job.fileUniqueID] = struct{}{}
	}
}

func (h *stickerUploadHandler) prepareStickerImage(b *gotgbot.Bot, job stickerUploadJob) (preparedStickerImage, error) {
	file, err := b.GetFile(job.fileID, nil)
	if err != nil {
		return preparedStickerImage{}, err
	}
	if file.FilePath == "" {
		return preparedStickerImage{}, errors.New("telegram file path is empty")
	}
	fileSize := job.fileSize
	if fileSize == 0 {
		fileSize = file.FileSize
	}
	if job.isVideo && fileSize > videoStickerMaxFileSize {
		return preparedStickerImage{}, errors.New("video sticker larger than 700KB")
	}

	log.Info().
		Str("file_unique_id", job.fileUniqueID).
		Str("file_path", file.FilePath).
		Int64("file_size", fileSize).
		Msg("download telegram sticker file")
	raw, ext, err := h.downloadTelegramFile(b, file.FilePath)
	if err != nil {
		return preparedStickerImage{}, err
	}
	log.Info().
		Str("file_unique_id", job.fileUniqueID).
		Str("extension", ext).
		Int("bytes", len(raw)).
		Msg("convert telegram sticker file")
	data, contentType, outputExt, err := convertStickerData(raw, ext, job.isAnimated || job.isVideo)
	if err != nil {
		return preparedStickerImage{}, err
	}
	log.Info().
		Str("file_unique_id", job.fileUniqueID).
		Str("content_type", contentType).
		Str("output_ext", outputExt).
		Int("bytes", len(data)).
		Msg("sticker image prepared")

	return preparedStickerImage{
		fileUniqueID: job.fileUniqueID,
		tgStickerID:  job.fileID,
		name:         job.fileUniqueID + outputExt,
		contentType:  contentType,
		data:         data,
	}, nil
}

func (h *stickerUploadHandler) downloadTelegramFile(b *gotgbot.Bot, filePath string) ([]byte, string, error) {
	url := fmt.Sprintf(telegramFileDownloadURLFormat, b.Token, filePath)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("telegram file download failed: %s", resp.Status)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), strings.ToLower(filepath.Ext(filePath)), nil
}

func (h *stickerUploadHandler) uploadImages(images []preparedStickerImage) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, image := range images {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="images"; filename="%s"`, image.name))
		header.Set("Content-Type", image.contentType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return err
		}
		if _, err := part.Write(image.data); err != nil {
			return err
		}
		if image.tgStickerID != "" {
			if err := writer.WriteField(emotionTelegramStickerIDField, image.tgStickerID); err != nil {
				return err
			}
		}
	}
	if err := writer.WriteField("source", "telegram-sticker"); err != nil {
		return err
	}
	if err := writer.WriteField("tags", "telegram"); err != nil {
		return err
	}
	if err := writer.WriteField("tags", "sticker"); err != nil {
		return err
	}
	if err := writer.WriteField("analyze", "true"); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	url := strings.TrimRight(h.cfg.APIBaseURL, "/") + "/v1/emotion-assets"
	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if h.cfg.APIKey != "" {
		req.Header.Set("x-api-key", h.cfg.APIKey)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("emotion upload failed: %s: %s", resp.Status, string(respBody))
	}
	return nil
}

func newStickerUploadStore(path string) (*stickerUploadStore, error) {
	store := &stickerUploadStore{
		path:    path,
		records: make(map[string]stickerUploadRecord),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *stickerUploadStore) Has(fileUniqueID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.records[fileUniqueID]
	return ok
}

func (s *stickerUploadStore) Mark(fileUniqueID string, status string, reason string) {
	if fileUniqueID == "" {
		return
	}
	s.mu.Lock()
	s.records[fileUniqueID] = stickerUploadRecord{
		Status:    status,
		Reason:    reason,
		UpdatedAt: time.Now(),
	}
	if err := s.saveLocked(); err != nil {
		log.Error().Err(err).Str("file_unique_id", fileUniqueID).Msg("failed to save sticker upload store")
	}
	s.mu.Unlock()
}

func (s *stickerUploadStore) load() error {
	if s.path == "" {
		return errors.New("sticker upload store path is empty")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	return json.Unmarshal(data, &s.records)
}

func (s *stickerUploadStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func convertStickerData(data []byte, ext string, animated bool) ([]byte, string, string, error) {
	ext = strings.ToLower(ext)
	switch ext {
	case ".tgs":
		return convertTgsToGif(data)
	case ".webm":
		return convertVideoToGif(data, ext)
	case ".gif":
		return data, "image/gif", ".gif", nil
	default:
		if animated {
			return nil, "", "", fmt.Errorf("unsupported animated sticker format: %s", ext)
		}
		pngData, err := util.ToPNG(strings.TrimPrefix(ext, "."), data)
		if err != nil {
			return nil, "", "", err
		}
		return pngData, "image/png", ".png", nil
	}
}

func convertVideoToGif(data []byte, ext string) ([]byte, string, string, error) {
	return convertWithCommand(data, ext, ".gif", "ffmpeg", []string{"-y", "-i", "{input}", "-vf", "fps=15,scale=512:-1:flags=lanczos", "{output}"}...)
}

func convertTgsToGif(data []byte) ([]byte, string, string, error) {
	if _, err := exec.LookPath("lottie_convert.py"); err == nil {
		return convertWithCommand(data, ".tgs", ".gif", "lottie_convert.py", "{input}", "{output}")
	}
	if _, err := exec.LookPath("rlottie-converter"); err == nil {
		return convertWithCommand(data, ".tgs", ".gif", "rlottie-converter", "{input}", "{output}")
	}
	return nil, "", "", errors.New("tgs sticker requires lottie_convert.py or rlottie-converter")
}

func convertWithCommand(data []byte, inputExt string, outputExt string, command string, args ...string) ([]byte, string, string, error) {
	dir, err := os.MkdirTemp("", "sticker-convert-*")
	if err != nil {
		return nil, "", "", err
	}
	defer os.RemoveAll(dir)

	inputPath := filepath.Join(dir, "input"+inputExt)
	outputPath := filepath.Join(dir, "output"+outputExt)
	if inputExt == ".tgs" {
		data, err = decompressTGS(data)
		if err != nil {
			return nil, "", "", err
		}
		inputPath = filepath.Join(dir, "input.json")
	}
	if err := os.WriteFile(inputPath, data, 0o644); err != nil {
		return nil, "", "", err
	}

	for i, arg := range args {
		args[i] = strings.ReplaceAll(strings.ReplaceAll(arg, "{input}", inputPath), "{output}", outputPath)
	}
	cmd := exec.Command(command, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, "", "", fmt.Errorf("%s failed: %w: %s", command, err, string(output))
	}

	converted, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, "", "", err
	}
	return converted, "image/gif", outputExt, nil
}

func decompressTGS(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return data, nil
	}
	defer reader.Close()
	return io.ReadAll(reader)
}
