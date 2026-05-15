package handler

import (
	"chatbot/internal/ai"
	"chatbot/internal/ai/gemini"
	"chatbot/internal/ai/openai"
	"chatbot/internal/handler/update"
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
const chatMsgSaveTime = 60 * time.Minute

var _ ext.Handler = (*geminiHandler)(nil)

var gai *geminiHandler

type geminiHandler struct {
	takeList             map[string]*takeInfo
	chatCache            *chatCache
	ai                   ai.AiInterface
	imgHandlerAi         ai.AiInterface
	emotionClient        *emotionReplyClient
	emotionPromptBuilder *emotionParamBuilder
	groupReplyTrigger    *GroupReplyTriggerConfig
}

type takeInfo struct {
	mu          sync.Mutex
	tokeListMe  []string
	tokeListYou []string
	lastTime    time.Time
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

func NewGeminiHandler(cfg config.Ai, emotionCfg config.EmotionConfig, groupReplyTrigger *GroupReplyTriggerConfig) ext.Handler {
	var aiProvider ai.AiInterface
	aiProvider = openai.NewOpenAi(cfg)
	cache := NewChatCache()
	if groupReplyTrigger == nil {
		groupReplyTrigger = NewGroupReplyTriggerConfig()
	}
	gai = &geminiHandler{
		takeList:             make(map[string]*takeInfo),
		chatCache:            cache,
		ai:                   aiProvider,
		emotionClient:        newEmotionReplyClient(emotionCfg),
		emotionPromptBuilder: newEmotionParamBuilder(cfg),
		groupReplyTrigger:    groupReplyTrigger,
	}
	if cfg.GeminiKey != "" {
		gai.imgHandlerAi = gemini.NewGemini(config.Ai{GeminiKey: cfg.GeminiKey, GeminiModel: "gemini-2.5-flash"})
	}
	// 如果有其他的handler与这个冲突，当前handler会返回false
	update.GetUpdater().Register(false, gai.ai.Name(), func(b *gotgbot.Bot, ctx *ext.Context) bool {
		// youtube music handler
		if ctx.EffectiveChat.Type == "private" {
			// 如果引用为空，或者引用的对象不是bot
			if strings.HasPrefix(ctx.EffectiveMessage.Text, "/") || ctx.CallbackQuery != nil {
				return ctx.EffectiveMessage.ReplyToMessage == nil
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
		bc := strings.HasPrefix(ctx.EffectiveMessage.Text, "/chat ")
		if ctx.EffectiveMessage.Text == "" && len(ctx.EffectiveMessage.Photo) == 0 {
			return false
		}
		if bc {
			return bc
		} else {
			rate := randomGroupReplyBaseRate
			if gai.groupReplyTrigger != nil {
				rate = gai.groupReplyTrigger.rate(ctx.EffectiveChat.Id)
			}
			if TriggerWithPercentage(rate) && ctx.EffectiveMessage.ReplyToMessage == nil {
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
	sender := ctx.EffectiveSender.Username()
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

	// 如果是在群组里聊天，把聊天历史加上

	if ctx.EffectiveChat.Type == "group" || ctx.EffectiveChat.Type == "supergroup" {
		hmsg, _ := g.chatCache.GetChatMsgAndClean(sender)
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
	if len(ctx.EffectiveMessage.Photo) > 0 || (ctx.EffectiveMessage.ReplyToMessage != nil && len(ctx.EffectiveMessage.ReplyToMessage.Photo) > 0) {
		var p gotgbot.PhotoSize
		if len(ctx.EffectiveMessage.Photo) > 0 {
			p = ctx.EffectiveMessage.Photo[len(ctx.EffectiveMessage.Photo)-1]
		} else {
			p = ctx.EffectiveMessage.ReplyToMessage.Photo[len(ctx.EffectiveMessage.ReplyToMessage.Photo)-1]
		}
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
	log.Debug().Msgf("%s say: %s", sender, input)
	r := strings.Split(resp, "||")
	for _, m := range r {
		sendRespond(m, b, ctx)
		time.Sleep(1 * time.Second)
	}
	g.maybeSendEmotionReply(b, ctx, userMessage, resp)
	return nil

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
