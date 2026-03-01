package kfc

import (
	"chatbot/internal/storage/repo"
	"sync"
	"time"

	"math/rand"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/rs/zerolog/log"
)

const (
	KFCKEY = "kfc"
)

type kfcTimer struct {
	mu        sync.Mutex
	quotation repo.Quotations
	tgBot     map[int64]*gotgbot.Bot
	sender    map[int64]*kfcSender
}

func NewKFC() *kfcTimer {
	q, err := repo.InitQuotations()
	if err != nil {
		log.Error().Err(err)
		return nil
	}
	return &kfcTimer{
		quotation: q,
		tgBot:     make(map[int64]*gotgbot.Bot),
		sender:    make(map[int64]*kfcSender),
	}
}

func (k *kfcTimer) newStartCmd() handlers.Response {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		k.mu.Lock()
		defer k.mu.Unlock()
		k.tgBot[ctx.EffectiveChat.Id] = b
		sender := newKfcSender(k.quotation, b, ctx.EffectiveChat.Id)
		if _, ok := k.sender[ctx.EffectiveChat.Id]; ok {
			log.Debug().Msg("this cat had add kfs timer")
			_, err := b.SendMessage(ctx.EffectiveChat.Id, "kfc bot had add", nil)
			return err
		}
		k.sender[ctx.EffectiveChat.Id] = sender
		sender.start()
		log.Debug().Int64("chatId", ctx.EffectiveChat.Id).Msg("start kfc")
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "start kfc", nil)
		return err
	}
}

func (k *kfcTimer) newStopCmd() handlers.Response {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		k.mu.Lock()
		defer k.mu.Unlock()
		chatList := []int64{}
		for chat := range k.tgBot {
			if chat != ctx.EffectiveChat.Id {
				chatList = append(chatList, chat)
			}
		}
		if sender, ok := k.sender[ctx.EffectiveChat.Id]; ok {
			sender.Stop()
			delete(k.sender, ctx.EffectiveChat.Id)
			_, err := b.SendMessage(ctx.EffectiveChat.Id, "stop kfc", nil)
			return err
		}
		log.Debug().Int64("chatId", ctx.EffectiveChat.Id).Msg("stop kfc")
		_, err := b.SendMessage(ctx.EffectiveChat.Id, "kfc not running", nil)
		return err
	}
}

func (k *kfcTimer) Start() {

}

func (k *kfcTimer) Register(reg func(handler handlers.Response, cmd string)) {
	reg(k.newStartCmd(), "startkfc")
	reg(k.newStopCmd(), "stopkfc")
}

type kfcSender struct {
	bot    *gotgbot.Bot
	chatID int64
	qutedb repo.Quotations
	stop   chan (struct{})
}

func newKfcSender(qute repo.Quotations, b *gotgbot.Bot, chatID int64) *kfcSender {
	return &kfcSender{
		bot:    b,
		qutedb: qute,
		chatID: chatID,
		stop:   make(chan struct{}, 1),
	}
}

func (k *kfcSender) start() {
	go func() {
		// 设置中国时区
		loc, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			log.Fatal().AnErr("set asia timezone failed", err)
		}
		// 获取当前时间
		now := time.Now().In(loc)

		// 找到下一个周四的日期
		offset := (4 - int(now.Weekday()) + 7) % 7
		nextThursday := now.AddDate(0, 0, offset)

		// 设置为当天 8:00 的起始时间
		start := time.Date(nextThursday.Year(), nextThursday.Month(), nextThursday.Day(), 8, 0, 0, 0, loc)

		// 随机选择从 8:00 到 16:00 之间的一个时刻（8 小时 * 3600 秒）
		randomSeconds := rand.Int63n(8 * 3600)
		randomTime := start.Add(time.Duration(randomSeconds) * time.Second)
		// 计算到随机时间的延迟
		delay := time.Until(randomTime)

		select {
		case <-time.After(delay):
			kfc, err := k.qutedb.GetRandomOne(KFCKEY)
			if err != nil {
				log.Error().Err(err).Msg("failed to get kfc quotation")
				kfc = "VME5o"
			}
			k.bot.SendMessage(k.chatID, kfc, nil)
			select {
			case <-time.After(24 * time.Hour):
				k.start()
			case <-k.stop:
				return
			}
		case <-k.stop:
			return
		}
	}()

}

func (k *kfcSender) Stop() {
	k.stop <- struct{}{}
}
