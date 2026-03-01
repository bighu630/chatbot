package handler

import (
	"chatbot/internal/cloud/tencent"
	"chatbot/internal/handler/update"
	"chatbot/pkg/util"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/rs/zerolog/log"
)

var _ ext.Handler = (*AudioHandler)(nil)

type AudioHandler struct {
	tencentClient *tencent.TencentClient
}

func NewAudioHandler() *AudioHandler {
	update.GetUpdater().Register(true, "audio", func(b *gotgbot.Bot, ctx *ext.Context) bool {
		return ctx.EffectiveMessage.Voice != nil
	})
	return &AudioHandler{tencentClient: tencent.GetTencentClient()}
}

func (a *AudioHandler) Name() string {
	return "audio"
}

func (a *AudioHandler) CheckUpdate(b *gotgbot.Bot, ctx *ext.Context) bool {
	return update.GetUpdater().CheckUpdate(a.Name(), b, ctx)
}

func (a *AudioHandler) HandleUpdate(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveMessage.Voice != nil {
		audioFile, err := util.DownloadAduioByFileID(ctx.EffectiveMessage.Voice.FileId, b)
		if err != nil {
			log.Error().Err(err).Msg("failed to download file")
			return err
		}
		resp, err := a.tencentClient.AudioToText(audioFile)
		if err != nil {
			log.Error().Err(err).Msg("failed to get audio text")
			return err
		}
		log.Debug().Str("resp", resp)
		log.Debug().Msg("AudioToText")
		ctx.EffectiveMessage.Reply(b, resp, nil)
	}
	return nil
}
