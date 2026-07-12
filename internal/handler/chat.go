package handler

import (
	"chatbot/internal/ai"
	"chatbot/internal/ai/gemini"
	"chatbot/internal/ai/openai"
	"chatbot/internal/chatcore"
	"chatbot/internal/handler/update"
	"chatbot/internal/platform"
	"chatbot/pkg/config"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/rs/zerolog/log"
)

const chatMsgSaveTime = 60 * time.Minute
const HandlerNameChat = "CHAT"

var _ ext.Handler = (*geminiHandler)(nil)

var gai *geminiHandler

type geminiHandler struct {
	takeList             map[string]*takeInfo
	chatCache            *chatCache
	core                 *chatcore.Service
	ai                   ai.AiInterface
	imgHandlerAi         ai.AiInterface
	emotionClient        *emotionReplyClient
	emotionPromptBuilder *emotionParamBuilder
	groupReplyTrigger    *GroupReplyTriggerConfig
	groupEmotionNSFW     *GroupEmotionNSFWConfig
}

type takeInfo struct {
	mu          sync.Mutex
	tokeListMe  []string
	tokeListYou []string
	lastTime    time.Time
}

func NewGeminiHandler(cfg config.Ai, emotionCfg config.EmotionConfig, groupReplyTrigger *GroupReplyTriggerConfig, groupEmotionNSFW *GroupEmotionNSFWConfig) ext.Handler {
	var aiProvider ai.AiInterface = openai.NewOpenAi(cfg)
	cache := NewChatCache()
	if groupReplyTrigger == nil {
		groupReplyTrigger = NewGroupReplyTriggerConfig()
	}
	if groupEmotionNSFW == nil {
		groupEmotionNSFW = NewGroupEmotionNSFWConfig()
	}
	gai = &geminiHandler{
		takeList:             make(map[string]*takeInfo),
		chatCache:            cache,
		ai:                   aiProvider,
		emotionClient:        newEmotionReplyClient(emotionCfg),
		emotionPromptBuilder: newEmotionParamBuilder(cfg),
		groupReplyTrigger:    groupReplyTrigger,
		groupEmotionNSFW:     groupEmotionNSFW,
	}
	gai.core = &chatcore.Service{
		AI:        aiProvider,
		History:   cache,
		Trigger:   groupReplyTrigger,
		BotName:   "",
		GroupRate: randomGroupReplyBaseRate,
	}
	if cfg.GeminiKey != "" {
		gai.imgHandlerAi = gemini.NewGemini(config.Ai{GeminiKey: cfg.GeminiKey, GeminiModel: "gemini-2.5-flash"})
	}
	update.GetUpdater().Register(false, HandlerNameChat, func(b *gotgbot.Bot, ctx *ext.Context) bool {
		if ctx.EffectiveMessage == nil || ctx.EffectiveChat == nil {
			return false
		}
		if ctx.EffectiveChat.Type == "private" {
			if strings.HasPrefix(ctx.EffectiveMessage.Text, "/") || ctx.CallbackQuery != nil {
				return ctx.EffectiveMessage.ReplyToMessage == nil
			}
			return (ctx.EffectiveMessage.ReplyToMessage == nil || ctx.EffectiveMessage.ReplyToMessage.From.Username != b.Username)
		}
		if ctx.EffectiveMessage.ReplyToMessage != nil && ctx.EffectiveMessage.ReplyToMessage.From.Username == b.Username {
			return true
		}
		for _, ent := range ctx.EffectiveMessage.Entities {
			if ent.Type == "mention" && strings.HasPrefix(ctx.EffectiveMessage.Text, "@"+b.Username+" ") {
				return true
			}
		}
		if strings.HasPrefix(ctx.EffectiveMessage.Text, "/chat ") {
			return true
		}
		if ctx.EffectiveMessage.Text == "" && len(ctx.EffectiveMessage.Photo) == 0 {
			return false
		}
		if ctx.EffectiveChat.Type == "group" || ctx.EffectiveChat.Type == "supergroup" {
			msg := ctx.EffectiveMessage.Text
			if len(msg) > 0 {
				cache.AddMsg(ctx.EffectiveChat.Title, ctx.EffectiveSender.User.Username, msg)
			}
		}
		return false
	})
	return gai
}

func (g *geminiHandler) Name() string { return HandlerNameChat }

func (g *geminiHandler) CheckUpdate(b *gotgbot.Bot, ctx *ext.Context) bool {
	return update.Updater.CheckUpdate(g.Name(), b, ctx)
}

func (g *geminiHandler) HandleUpdate(b *gotgbot.Bot, ctx *ext.Context) error {
	log.Debug().Msg("get an chat message")
	msg, ok := telegramMessageFromContext(b, ctx)
	if !ok {
		return nil
	}
	if after, ok0 := strings.CutPrefix(msg.Content.Text, "@"+b.Username+" "); ok0 {
		msg.Content.Text = after
	}
	resp, handled, err := g.core.Handle(msg)
	if err != nil {
		return err
	}
	if !handled {
		return nil
	}
	err = sendChatResponse(resp, b, ctx)
	if err != nil {
		return err
	}
	g.maybeSendEmotionReply(b, ctx, msg.Content.Text, resp)
	return nil
}

func telegramMessageFromContext(b *gotgbot.Bot, ctx *ext.Context) (platform.Message, bool) {
	if ctx == nil || ctx.EffectiveChat == nil || ctx.EffectiveMessage == nil {
		return platform.Message{}, false
	}
	msg := platform.Message{
		Platform: platform.PlatformTelegram,
		Chat: platform.ChatRef{
			Type: telegramChatType(ctx.EffectiveChat.Type),
			ID:   strconv.FormatInt(ctx.EffectiveChat.Id, 10),
			Name: ctx.EffectiveChat.Title,
		},
		Sender: platform.UserRef{
			ID:       strconv.FormatInt(ctx.EffectiveSender.Id(), 10),
			Name:     ctx.EffectiveSender.Name(),
			Username: ctx.EffectiveSender.Username(),
		},
		Content: platform.TextContent{Text: ctx.EffectiveMessage.Text},
	}
	if ctx.EffectiveMessage.ReplyToMessage != nil {
		msg.ReplyTo = &platform.MessageRef{SenderID: ctx.EffectiveMessage.ReplyToMessage.From.Username}
	}
	for _, ent := range ctx.EffectiveMessage.Entities {
		if ent.Type == "mention" {
			msg.Mentions = append(msg.Mentions, platform.Mention{TargetName: b.Username})
		}
	}
	return msg, true
}

func telegramChatType(chatType string) platform.ChatType {
	switch chatType {
	case "group", "supergroup":
		return platform.GroupChat
	default:
		return platform.PrivateChat
	}
}

func sendChatResponse(resp string, b *gotgbot.Bot, ctx *ext.Context) error {
	parts := strings.Split(resp, "||")
	for _, m := range parts {
		m = formatAiResp(strings.TrimSpace(m))
		if m == "" {
			continue
		}
		_, err := b.SendMessage(ctx.EffectiveChat.Id, m, &gotgbot.SendMessageOpts{ParseMode: "Markdown"})
		if err != nil {
			_, err = b.SendMessage(ctx.EffectiveChat.Id, m, &gotgbot.SendMessageOpts{})
			if err != nil {
				return err
			}
		}
		time.Sleep(1 * time.Second)
	}
	return nil
}

func (g *geminiHandler) handleChat(b *gotgbot.Bot, ctx *ext.Context, ai ai.AiInterface) error {
	return nil
}
