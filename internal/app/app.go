package app

import (
	"chatbot/internal/admin"
	"chatbot/internal/bot"
	"chatbot/internal/cloud/tencent"
	handler "chatbot/internal/handler"
	"chatbot/internal/handler/quotation"
	"chatbot/internal/scheduler"
	"chatbot/internal/storage"
	"chatbot/pkg/config"
	"chatbot/pkg/logger"
	"fmt"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/rs/zerolog/log"
)

// Start wires all components together and runs the bot.
// It returns an error if any required component fails to initialise.
func Start(cfg *config.Config) error {
	logger.Init(cfg.Log)

	storage.Configure(&cfg.Storage)
	if db := storage.InitDB(); db == nil {
		return fmt.Errorf("storage: failed to initialise database")
	}

	tgWebHook := bot.NewWebHookConnect(cfg.WebHookConfig)
	if tgWebHook == nil {
		return fmt.Errorf("bot: failed to create webhook connection")
	}
	adminNotifier := admin.NewNotifier(cfg.Admin)
	tgWebHook.SetOnStarted(func(b *gotgbot.Bot) error {
		if err := bot.RegisterBotCommands(b); err != nil {
			return fmt.Errorf("register bot commands: %w", err)
		}
		if err := adminNotifier.NotifyServiceStarted(b); err != nil {
			return fmt.Errorf("notify service started: %w", err)
		}
		return nil
	})

	if _, err := tencent.NewTencentClient(cfg.TencentConfig); err != nil {
		log.Warn().Err(err).Msg("tencent client unavailable; voice transcription disabled")
	}

	var ymbHandler ext.Handler
	var gaiHandler ext.Handler
	var quotationsHandler ext.Handler
	if cfg.Emotion.Enable {
		stickerHandler, err := handler.NewStickerUploadHandler(cfg.Emotion, cfg.Admin.ChatIDs)
		if err != nil {
			log.Warn().Err(err).Msg("sticker upload handler unavailable")
		} else {
			tgWebHook.RegisterHandler(stickerHandler)
		}
	}
	if cfg.Ytdlp.Enable {
		ymbHandler = handler.NewYoutubeHandler(cfg.Ytdlp.Path)
		tgWebHook.RegisterHandler(ymbHandler)
	}
	if cfg.Ai.Enable {
		groupReplyTrigger := handler.NewGroupReplyTriggerConfig()
		groupEmotionNSFW := handler.NewGroupEmotionNSFWConfig()
		gaiHandler = handler.NewGeminiHandler(cfg.Ai, cfg.Emotion, groupReplyTrigger, groupEmotionNSFW)
		tgWebHook.RegisterHandler(gaiHandler)
		activityHandler := handler.NewGroupReplyActivityHandler(groupReplyTrigger, cfg.Admin.ChatIDs)
		tgWebHook.RegisterHandlerWithCmd(activityHandler, "activity")
		tgWebHook.RegisterHandlerWithCmd(activityHandler, "setactivity")
		nsfwHandler := handler.NewGroupEmotionNSFWHandler(groupEmotionNSFW, cfg.Admin.ChatIDs)
		tgWebHook.RegisterHandlerWithCmd(nsfwHandler, "nsfw")
		tgWebHook.RegisterHandlerWithCmd(nsfwHandler, "setnsfw")
	}
	if cfg.Storage.Enable {
		quotationsHandler = quotation.NewQuotationsHandler()
		tgWebHook.RegisterHandler(quotationsHandler)
	}
	quotationCtrl, err := quotation.NewQuotationHandler()
	if err != nil {
		return fmt.Errorf("quotation handler: %w", err)
	}
	quotationCtrl.Register(tgWebHook.RegisterHandlerWithCmd)
	tgWebHook.RegisterHandler(quotationCtrl)
	tgWebHook.RegisterHandler(quotationCtrl.NewCallbackHander())

	// audioHandler := handler.NewAudioHandler()
	// tgWebHook.RegisterHandler(audioHandler)
	timer := scheduler.NewTimekeeper()
	timer.RegisterCmd(tgWebHook.RegisterHandlerWithCmd)
	timer.Start()

	tgWebHook.RegisterHandlerWithCmd(handler.NewHelpHandler(), "help")

	tgVerify := handler.NewTgJoinVerificationHandler()
	tgWebHook.RegisterHandler(tgVerify)
	tgWebHook.RegisterHandler(tgVerify.NewCallbackHander())
	tgWebHook.RegisterHandlerWithCmd(handler.NewFeedbackHandler(adminNotifier), "feedback")

	// blocks until webhook stops
	tgWebHook.Start()
	return nil
}
