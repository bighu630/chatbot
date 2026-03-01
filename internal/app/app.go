package app

import (
	"chatbot/internal/bot"
	"chatbot/internal/cloud/tencent"
	handler "chatbot/internal/handler"
	"chatbot/internal/handler/quotation"
	"chatbot/internal/scheduler"
	"chatbot/internal/storage"
	"chatbot/pkg/config"
	"chatbot/pkg/logger"

	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

func Start(cfg *config.Config) {
	logger.Init(cfg.Log)

	storage.Configure(&cfg.Storage)
	storage.InitDB()

	tgWebHook := bot.NewWebHookConnect(cfg.WebHookConfig)
	tencent.NewTencentClient(cfg.TencentConfig)

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
		print("无法初始化 --语录控制器-- ", err)
	}
	quotationCtrl.Register(tgWebHook.RegisterHandlerWithCmd)
	tgWebHook.RegisterHandler(quotationCtrl)
	tgWebHook.RegisterHandler(quotationCtrl.NewCallbackHander())

	// audioHandler := handler.NewAudioHandler()
	// tgWebHook.RegisterHandler(audioHandler)
	timer := scheduler.NewTimekeeper()

	timer.RegisterCmd(tgWebHook.RegisterHandlerWithCmd)
	timer.Start()

	// help帮助
	tgWebHook.RegisterHandlerWithCmd(handler.NewHelpHandler(), "help")

	tgVerify := handler.NewTgJoinVerificationHandler()
	tgWebHook.RegisterHandler(tgVerify)
	tgWebHook.RegisterHandler(tgVerify.NewCallbackHander())

	// tgAutoCall.Start()
	tgWebHook.Start()
}
