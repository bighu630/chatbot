package handler

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"chatbot/pkg/config"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

func TestStickerUploadStoreMarkAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sticker_uploads.json")
	store, err := newStickerUploadStore(path)
	if err != nil {
		t.Fatal(err)
	}

	store.Mark("unique-1", "processed", "uploaded")
	if !store.Has("unique-1") {
		t.Fatal("expected marked sticker to be recorded")
	}

	reloaded, err := newStickerUploadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Has("unique-1") {
		t.Fatal("expected sticker record to survive reload")
	}
}

func TestStickerUploadQueueBatching(t *testing.T) {
	handler := &stickerUploadHandler{
		store:   &stickerUploadStore{records: map[string]stickerUploadRecord{}},
		queue:   make([]stickerUploadJob, 0, stickerUploadBatchSize),
		pending: make(map[string]struct{}),
	}

	for i := 0; i < stickerUploadBatchSize-1; i++ {
		if handler.enqueue(stickerUploadJob{fileUniqueID: string(rune('a' + i))}) {
			t.Fatal("expected queue below batch size not to flush")
		}
	}
	if !handler.enqueue(stickerUploadJob{fileUniqueID: "z"}) {
		t.Fatal("expected queue at batch size to flush")
	}

	batch := handler.takeBatch(false)
	if len(batch) != stickerUploadBatchSize {
		t.Fatalf("takeBatch len = %d, want %d", len(batch), stickerUploadBatchSize)
	}
}

func TestStickerShouldCollectFromChat(t *testing.T) {
	h := &stickerUploadHandler{
		adminIDs: buildAdminIDSet([]int64{10001}),
	}

	groupCtx := &ext.Context{
		EffectiveChat: &gotgbot.Chat{Type: "group"},
	}
	if !h.shouldCollectFromChat(groupCtx) {
		t.Fatal("expected group sticker to be collected")
	}

	adminPrivateCtx := &ext.Context{
		EffectiveChat:   &gotgbot.Chat{Type: "private"},
		EffectiveSender: &gotgbot.Sender{User: &gotgbot.User{Id: 10001}},
	}
	if !h.shouldCollectFromChat(adminPrivateCtx) {
		t.Fatal("expected admin private sticker to be collected")
	}

	nonAdminPrivateCtx := &ext.Context{
		EffectiveChat:   &gotgbot.Chat{Type: "private"},
		EffectiveSender: &gotgbot.Sender{User: &gotgbot.User{Id: 20002}},
	}
	if h.shouldCollectFromChat(nonAdminPrivateCtx) {
		t.Fatal("expected non-admin private sticker to be skipped")
	}
}

func TestStickerUploadIncludesTGStickerID(t *testing.T) {
	h := &stickerUploadHandler{
		cfg: config.EmotionConfig{APIBaseURL: "https://example.test"},
		httpClient: &http.Client{Transport: testRoundTripper(func(r *http.Request) (*http.Response, error) {
			if err := r.ParseMultipartForm(1024 * 1024); err != nil {
				t.Fatal(err)
			}
			if got := r.MultipartForm.Value[emotionTelegramStickerIDField]; len(got) != 1 || got[0] != "CAAC_STICKER" {
				t.Fatalf("%s = %#v, want CAAC_STICKER", emotionTelegramStickerIDField, got)
			}
			if files := r.MultipartForm.File["images"]; len(files) != 1 {
				t.Fatalf("images count = %d, want 1", len(files))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(`{"code":0,"message":"success","data":{"created_count":1,"total_count":1,"skipped_count":0,"assets":[],"skipped":[]}}`)),
			}, nil
		})},
	}
	err := h.uploadImages([]preparedStickerImage{
		{
			fileUniqueID: "unique",
			tgStickerID:  "CAAC_STICKER",
			name:         "unique.png",
			contentType:  "image/png",
			data:         []byte("png"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}
