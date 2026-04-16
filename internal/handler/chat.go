package handler

import (
	"chatbot/internal/ai"
	"chatbot/internal/ai/gemini"
	"chatbot/internal/ai/openai"
	"chatbot/internal/handler/update"
	"chatbot/internal/storage/repo"
	"chatbot/pkg/config"
	"chatbot/pkg/util"
	"context"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/rs/zerolog/log"
)

// 群聊对话保存一小时
const (
	chatMsgSaveTime          = 60 * time.Minute
	privateChatDailyLimit    = 30
	privateReplyMaxRuneCount = 800
)

var _ ext.Handler = (*geminiHandler)(nil)

var gai *geminiHandler

type geminiHandler struct {
	takeList      map[string]*takeInfo
	chatCache     *chatCache
	ai            ai.AiInterface
	imgHandlerAi  ai.AiInterface
	chatRepo      repo.Chat
	emotionClient *emotionReplyClient
}

type takeInfo struct {
	mu          sync.Mutex
	tokeListMe  []string
	tokeListYou []string
	lastTime    time.Time
}

func isChatCommand(text string) bool {
	command := firstToken(text)
	return command == "/chat" || strings.HasPrefix(command, "/chat@")
}

func isSlashCommand(text string) bool {
	return strings.HasPrefix(firstToken(text), "/")
}

func firstToken(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func TriggerWithPercentage(percentage float64) bool {
	// 确保概率在有效范围内
	if percentage < 0.0 {
		percentage = 0.0
	}
	if percentage > 1.0 {
		percentage = 1.0
	}

	// 生成一个0.0到1.0之间的随机浮点数
	// rand.Float64() 返回 [0.0, 1.0) 的随机浮点数
	randomValue := rand.Float64()

	// 如果生成的随机数小于指定的概率，则触发事件
	return randomValue < percentage
}

func NewGeminiHandler(cfg config.Ai, emotionCfg config.EmotionConfig) ext.Handler {
	var aiProvider ai.AiInterface
	aiProvider = openai.NewOpenAi(cfg)
	cache := NewChatCache()
	chatRepo, err := repo.InitChatDB()
	if err != nil {
		log.Error().Err(err).Msg("failed to init chat repo for handler")
	}
	gai = &geminiHandler{
		takeList:      make(map[string]*takeInfo),
		chatCache:     cache,
		ai:            aiProvider,
		chatRepo:      chatRepo,
		emotionClient: newEmotionReplyClient(emotionCfg),
	}
	if cfg.GeminiKey != "" {
		gai.imgHandlerAi = gemini.NewGemini(config.Ai{GeminiKey: cfg.GeminiKey, GeminiModel: "gemini-2.5-flash"})
	}
	// 如果有其他的handler与这个冲突，当前handler会返回false
	update.GetUpdater().Register(false, gai.ai.Name(), func(b *gotgbot.Bot, ctx *ext.Context) bool {
		// youtube music handler
		if ctx.EffectiveChat.Type == "private" {
			if ctx.CallbackQuery != nil {
				return false
			}
			if isChatCommand(ctx.EffectiveMessage.Text) {
				return ctx.EffectiveMessage.ReplyToMessage == nil
			}
			if isSlashCommand(ctx.EffectiveMessage.Text) {
				return false
			}
			return (ctx.EffectiveMessage.ReplyToMessage == nil || ctx.EffectiveMessage.ReplyToMessage.From.Username != b.Username)
		}
		if ctx.EffectiveMessage.ReplyToMessage != nil &&
			ctx.EffectiveMessage.ReplyToMessage.From.Username == b.Username {
			return true
		}
		for _, ent := range ctx.EffectiveMessage.Entities {
			if ent.Type == "mention" && strings.HasPrefix(ctx.EffectiveMessage.Text, "@"+b.Username+" ") {
				return true
			}
		}
		bc := isChatCommand(ctx.EffectiveMessage.Text)
		if ctx.EffectiveMessage.Text == "" && len(ctx.EffectiveMessage.Photo) == 0 {
			return false
		}
		if bc {
			return bc
		} else {
			if TriggerWithPercentage(0.003) && ctx.EffectiveMessage.ReplyToMessage == nil {
				return true
			}
			if ctx.EffectiveChat.Type == "group" || ctx.EffectiveChat.Type == "supergroup" {
				msg := ctx.EffectiveMessage.Text
				if len(msg) > 0 {
					userName := ctx.EffectiveSender.Name()
					if userName == "" {
						userName = ctx.EffectiveSender.Username()
					}
					cache.AddMsg(ctx.EffectiveChat.Title, ctx.EffectiveSender.User.Username, msg)
				}
			}
			return false
		}
	})
	return gai
}

func (g *geminiHandler) Name() string {
	return g.ai.Name()
}

func (g *geminiHandler) CheckUpdate(b *gotgbot.Bot, ctx *ext.Context) bool {
	return update.Updater.CheckUpdate(g.Name(), b, ctx)
}

func (g *geminiHandler) HandleUpdate(b *gotgbot.Bot, ctx *ext.Context) error {
	log.Debug().Msg("get an chat message")
	return g.handleChat(b, ctx, g.ai)
}

// 处理私聊对话
func (g *geminiHandler) handleChat(b *gotgbot.Bot, ctx *ext.Context, ai ai.AiInterface) error {
	sender := privateChatKey(ctx)
	isPrivateChat := ctx.EffectiveChat.Type == "private"
	if ctx.EffectiveChat.Type == "group" || ctx.EffectiveChat.Type == "supergroup" {
		sender = ctx.EffectiveChat.Title
		if sender == "" {
			sender = strconv.Itoa(int(ctx.EffectiveChat.Id))
		}
	}
	input := strings.TrimPrefix(ctx.EffectiveMessage.Text, "/chat ")
	userMessage := input
	if input == "/help" {
		_, err := b.SendMessage(ctx.EffectiveChat.Id, Help, nil)
		return err
	}
	if isPrivateChat && !g.allowPrivateChat(sender) {
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "私聊额度今天已经用完了，每天最多 30 次，明天再来。", nil)
		return err
	}

	// 如果是在群组里聊天，把聊天历史加上

	chatContext := ""
	if ctx.EffectiveChat.Type == "group" || ctx.EffectiveChat.Type == "supergroup" {
		hmsg, _ := g.chatCache.GetChatMsgAndClean(sender)
		chatContext = hmsg
		if time.Now().UnixMicro()%30 == 0 { // 有1/30的概率只提示历史对话
			input = fmt.Sprintf(`对话历史(可酌情参考): %s
新消息: %s`, hmsg, input)
		} else if len(hmsg) > 0 {
			input = fmt.Sprintf(`对话历史(可酌情参考): %s
收到新消息: %s

请以群友「摘星」的身份进行回复。
风格要求：
1. 只进行「纯对话内容」，不能出现任何形式的旁白、动作、心理描写。
   - 禁止出现括号内容，如 ( ) 、（ ）。
   - 禁止使用 * * 、—— 等表示动作的方式。
   - 不要写自己"停顿""思考""斟酌"等行为。
2. 平时像普通群友随意聊天；遇到提问时，切换成思路清晰但不装腔的学霸模式。
3. 如果回复较长，可以用 "||" 分成几句，但每一句依然是纯对话。
4. 不要过长，也不要过度解释，让回复自然、像真人。

请仅输出最终要发送的对话内容。`,
				hmsg, input)
		}
	}
	if p, ok := imageForChatAnalysis(ctx, b); ok {
		itype, data, err := util.DownloadImgByFileID(p.FileId, b)
		if err != nil {
			log.Error().Err(err).Msg("download img error")
		}
		if len(data) > 0 {
			imgInfo, err := g.imgHandlerAi.HandleTextWithImg("用中文描述这张图片的信息，给其他ai使用", itype, data)
			if err != nil {
				log.Error().Err(err).Msg("img info error")
			} else {
				log.Debug().Str("imgInfo", imgInfo).Msg("get img info success")
				imgInfo = strings.ReplaceAll(imgInfo, "*", "")
				imgInfo = strings.ReplaceAll(imgInfo, "-", "")
				imgInfo = strings.ReplaceAll(imgInfo, "\n\n", "\n")
				input += "\n对话包含图片内容" + imgInfo
			}
		}
	}

	c, cancel := context.WithCancel(context.Background())
	setBotStatusWithContext(c, b, ctx)
	defer cancel()

	resp, err := ai.Chat(sender, input)
	if err != nil {
		log.Error().Err(err).Msg("gemini chat error")
		ctx.EffectiveMessage.Reply(b, "gemini chat error", nil)
		return err
	}
	resp = limitReplyLength(resp, privateReplyMaxRuneCount)
	log.Debug().Msgf("%s say: %s", sender, input)
	r := strings.Split(resp, "||")
	for _, m := range r {
		sendRespond(m, b, ctx)
		time.Sleep(1 * time.Second)
	}
	g.maybeSendEmotionReply(b, ctx, chatContext, userMessage, resp)
	return nil

}

func imageForChatAnalysis(ctx *ext.Context, b *gotgbot.Bot) (gotgbot.PhotoSize, bool) {
	if ctx == nil || ctx.EffectiveMessage == nil {
		return gotgbot.PhotoSize{}, false
	}
	if photos := ctx.EffectiveMessage.Photo; len(photos) > 0 {
		return photos[len(photos)-1], true
	}

	reply := ctx.EffectiveMessage.ReplyToMessage
	if reply == nil || len(reply.Photo) == 0 {
		return gotgbot.PhotoSize{}, false
	}
	if isMessageFromBot(reply, b) {
		log.Info().Int64("message_id", reply.MessageId).Msg("skip image analysis because replied image was sent by bot")
		return gotgbot.PhotoSize{}, false
	}
	return reply.Photo[len(reply.Photo)-1], true
}

func isMessageFromBot(msg *gotgbot.Message, b *gotgbot.Bot) bool {
	if msg == nil || msg.From == nil || b == nil {
		return false
	}
	if b.Id != 0 && msg.From.Id == b.Id {
		return true
	}
	return b.Username != "" && msg.From.Username == b.Username
}

func (g *geminiHandler) allowPrivateChat(sender string) bool {
	if g.chatRepo == nil || sender == "" {
		return true
	}

	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	count, err := g.chatRepo.CountMsgByTime(startOfDay, now, sender, true)
	if err != nil {
		log.Error().Err(err).Str("sender", sender).Msg("failed to count private chat usage")
		return true
	}

	return count < privateChatDailyLimit
}

func limitReplyLength(resp string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}

	runes := []rune(resp)
	if len(runes) <= maxRunes {
		return resp
	}

	return strings.TrimSpace(string(runes[:maxRunes]))
}

func privateChatKey(ctx *ext.Context) string {
	if ctx == nil || ctx.EffectiveChat == nil {
		return ""
	}
	if ctx.EffectiveChat.Type == "private" {
		return fmt.Sprintf("private:%d", ctx.EffectiveChat.Id)
	}
	return ctx.EffectiveSender.Username()
}

func sendRespond(resp string, b *gotgbot.Bot, ctx *ext.Context) error {
	resp = formatAiResp(resp)
	log.Debug().Msgf("gemini say in chat: %s", resp)
	for range 3 {
		_, err := b.SendMessage(ctx.EffectiveChat.Id, resp, &gotgbot.SendMessageOpts{
			ParseMode: "Markdown",
		})
		if err != nil {
			log.Error().Err(err)
			log.Debug().Msg("try to use nil opt send reply(before is Markdown)")
			_, err := b.SendMessage(ctx.EffectiveChat.Id, resp, &gotgbot.SendMessageOpts{})
			return err
		} else {
			return nil
		}
	}
	return nil
}
