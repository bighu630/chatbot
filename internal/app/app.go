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

	if _, err := tencent.NewTencentClient(cfg.TencentConfig); err != nil {
		log.Warn().Err(err).Msg("tencent client unavailable; voice transcription disabled")
	}

	var ymbHandler ext.Handler
	var gaiHandler ext.Handler
	var quotationsHandler ext.Handler
	if cfg.Ytdlp.Enable {
		ymbHandler = handler.NewYoutubeHandler(cfg.Ytdlp.Path)
		tgWebHook.RegisterHandler(ymbHandler)
	}
	if cfg.Ai.Enable {
		gaiHandler = handler.NewGeminiHandler(cfg.Ai)
		tgWebHook.RegisterHandler(gaiHandler)
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
	tgWebHook.RegisterHandlerWithCmd(handler.NewFeedbackHandler(admin.NewFeedbackNotifier(cfg.Admin)), "feedback")

	// blocks until webhook stops
	tgWebHook.Start()
	return nil
}
