package handler

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/rs/zerolog/log"
)

const CallbackPrefix = "verify"

var blacklist = struct {
	mu   sync.Mutex
	data map[int64]time.Time
}{data: make(map[int64]time.Time)}

// tgJoinVerificationHandler 处理入群验证
type tgJoinVerificationHandler struct{}

func NewTgJoinVerificationHandler() *tgJoinVerificationHandler {
	return &tgJoinVerificationHandler{}
}

func (h *tgJoinVerificationHandler) Name() string {
	return "tgJoinVerification"
}

func (h *tgJoinVerificationHandler) CheckUpdate(b *gotgbot.Bot, ctx *ext.Context) bool {
	return ctx.EffectiveChat.Type == "group" && ctx.EffectiveMessage != nil && ctx.EffectiveMessage.NewChatMembers != nil
}

func (h *tgJoinVerificationHandler) HandleUpdate(b *gotgbot.Bot, ctx *ext.Context) error {
	for _, member := range ctx.EffectiveMessage.NewChatMembers {
		if member.IsBot {
			continue
		}
		go handleNewMember(b, ctx.EffectiveChat.Id, member.Id, member.Username)
	}
	return nil
}

func (h *tgJoinVerificationHandler) NewCallbackHander() handlers.CallbackQuery {

	filter := func(cq *gotgbot.CallbackQuery) bool {
		return strings.HasPrefix(cq.Data, CallbackPrefix)
	}
	callbackHandler := func(b *gotgbot.Bot, ctx *ext.Context) error {
		key := strings.TrimPrefix(ctx.Update.CallbackQuery.Data, CallbackPrefix)
		switch key {
		case "0":
			//TODO: 验证成功
			msg, _ := b.SendMessage(ctx.EffectiveChat.Id, "欢迎加入聊群", nil)
			checkUserCache[ctx.CallbackQuery.From.Id] = struct{}{}
			time.Sleep(3 * time.Second)
			b.DeleteMessage(ctx.EffectiveChat.Id, msg.MessageId, nil)
			return nil
		default:
			msg, _ := b.SendMessage(ctx.EffectiveChat.Id, "验证失败", nil)
			time.Sleep(3 * time.Second)
			b.DeleteMessage(ctx.EffectiveChat.Id, msg.MessageId, nil)
			return nil
		}
	}
	return handlers.NewCallback(filter, callbackHandler)
}

func handleNewMember(b *gotgbot.Bot, chatID int64, userID int64, username string) {
	member, err := b.GetChatMember(chatID, b.Id, nil)
	if err != nil {
		log.Err(err).Msg("failed to get bot info")
		return
	}
	if member.GetStatus() != "administrator" {
		log.Info().Msg("not admin")
		return
	}
	log.Info().Str("user", username).Msg("user join group")
	md5Src := strconv.Itoa(int(chatID + userID))
	hash := md5.Sum([]byte(md5Src))
	md5Hex := hex.EncodeToString(hash[:])
	options := make(map[string]int, 4)
	for i := range 4 {
		if i == 0 {
			options[md5Hex[0:8]] = i
		} else if i == 3 {
			options[md5Hex[8*i:]] = i
		} else {
			options[md5Hex[8*i:8*i+8]] = i
		}
	}
	innerMsg := make([]int64, 4)
	nwwnMsg := fmt.Sprintf(" @%s 那我问你", username)
	msg, err := b.SendMessage(chatID, nwwnMsg, &gotgbot.SendMessageOpts{})
	if err != nil {
		b.SendMessage(chatID, fmt.Sprintf("处理出错 err:%v", err), &gotgbot.SendMessageOpts{})
		return
	}
	innerMsg = append(innerMsg, msg.MessageId)

	md5QuescationMsg := fmt.Sprintf("`%s` 的md5 的前8位是", md5Src)
	inlinKeyboardMarkup := gotgbot.InlineKeyboardMarkup{}
	inlinKeyboard1 := []gotgbot.InlineKeyboardButton{}
	inlinKeyboard2 := []gotgbot.InlineKeyboardButton{}
	i := 0
	for k, v := range options {
		if i < 2 {
			inlinKeyboard1 = append(inlinKeyboard1, gotgbot.InlineKeyboardButton{
				Text:         k,
				CallbackData: CallbackPrefix + strconv.Itoa(v),
			})
		} else {
			inlinKeyboard2 = append(inlinKeyboard2, gotgbot.InlineKeyboardButton{
				Text:         k,
				CallbackData: CallbackPrefix + strconv.Itoa(v),
			})
		}
		i++
	}
	inlinKeyboardMarkup.InlineKeyboard = append(inlinKeyboardMarkup.InlineKeyboard, inlinKeyboard1, inlinKeyboard2)
	msg, err = b.SendMessage(chatID, md5QuescationMsg, &gotgbot.SendMessageOpts{
		ReplyMarkup: &inlinKeyboardMarkup,
	})
	if err != nil {
		b.SendMessage(chatID, fmt.Sprintf("处理出错 err:%v", err), &gotgbot.SendMessageOpts{})
		return
	}
	innerMsg = append(innerMsg, msg.MessageId)

	msg, err = b.SendMessage(chatID, "回答我", nil)
	if err != nil {
		innerMsg = append(innerMsg, msg.MessageId)
	}

	// 启动定时任务
	go startVerificationTimer(context.Background(), b, chatID, userID, innerMsg)
}

var checkUserCache map[int64]struct{}

func startVerificationTimer(ctx context.Context, b *gotgbot.Bot, chatID int64, userID int64, msgids []int64) {
	defer func() {
		b.DeleteMessages(chatID, msgids, nil)
	}()
	for {
		select {
		case <-time.After(90 * time.Second):
			// 删除用户
			b.BanChatMember(chatID, userID, &gotgbot.BanChatMemberOpts{RevokeMessages: true})
			log.Warn().Int64("user", userID).Msg("verification failed")
			return
		default:
			if _, ok := checkUserCache[userID]; ok {
				log.Info().Int64("user", userID).Msg("verification success")

				return
			}
		}
		time.Sleep(time.Second)
	}
}
