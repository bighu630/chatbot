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

func Start() {
	logger.Init(config.GlobalConfig.Log)
	storage.InitDB()
	tgWebHook := bot.NewWebHookConnect(config.GlobalConfig.WebHookConfig)
	tencent.NewTencentClient(config.GlobalConfig.TencentConfig)

	var ymbHandler ext.Handler
	var gaiHandler ext.Handler
	var quotationsHandler ext.Handler
	if config.GlobalConfig.Ytdlp.Enable {
		ymbHandler = handler.NewYoutubeHandler(config.GlobalConfig.Ytdlp.Path)
		tgWebHook.RegisterHandler(ymbHandler)
	}
	if config.GlobalConfig.Ai.Enable {
		gaiHandler = handler.NewGeminiHandler(config.GlobalConfig.Ai)
		tgWebHook.RegisterHandler(gaiHandler)
	}
	if config.GlobalConfig.Storage.Enable {
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
