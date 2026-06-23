package app

import (
	"chatbot/internal/admin"
	"chatbot/internal/ai/openai"
	"chatbot/internal/bot"
	botadapter "chatbot/internal/bot"
	qqadapter "chatbot/internal/bot/qq"
	"chatbot/internal/bot/qqonebot"
	"chatbot/internal/chatcore"
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
	adminNotifier := admin.NewFeedbackNotifier(cfg.Admin)
	tgWebHook.SetAdminNotifier(adminNotifier)

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

	var aiProvider = openai.NewOpenAi(cfg.Ai)
	if !cfg.Ai.Enable && cfg.QQConfig.Enable {
		return fmt.Errorf("qq requires ai to be enabled")
	}
	groupReplyTrigger := handler.NewGroupReplyTriggerConfig()
	groupEmotionNSFW := handler.NewGroupEmotionNSFWConfig()
	if cfg.Ai.Enable {
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

	timer := scheduler.NewTimekeeper()
	timer.RegisterCmd(tgWebHook.RegisterHandlerWithCmd)
	timer.Start()

	tgWebHook.RegisterHandlerWithCmd(handler.NewHelpHandler(), "help")

	tgVerify := handler.NewTgJoinVerificationHandler()
	tgWebHook.RegisterHandler(tgVerify)
	tgWebHook.RegisterHandler(tgVerify.NewCallbackHander())
	tgWebHook.RegisterHandlerWithCmd(handler.NewFeedbackHandler(adminNotifier), "feedback")

	var qqRunner botadapter.Runner
	if cfg.QQConfig.Enable {
		qqCore := &chatcore.Service{
			AI:        aiProvider,
			History:   handler.NewChatCache(),
			BotName:   cfg.QQConfig.BotQQ,
			GroupRate: chatcore.DefaultGroupChance,
		}
		qqClient := qqonebot.New(cfg.QQConfig.WSAddr)
		qqRunner = qqadapter.New(qqCore, qqClient)
	}

	multi := botadapter.NewMultiRunner(tgWebHook, qqRunner)
	multi.Start()
	return nil
}
