package bot

import (
	"chatbot/pkg/config"
	"crypto/sha256"
	"encoding/hex"
	"os"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type autoCallerConnect struct {
	bot        *gotgbot.Bot
	dispatcher *ext.Dispatcher
	updater    *ext.Updater
	token      string
	cmdMap     map[string]func(*gotgbot.Bot, int64)
	startAble  bool
}

func NewAutoCaller(whConfig config.WebHookConfig) *autoCallerConnect {

	var (
		token   = whConfig.Token
		address = whConfig.Address
		secret  = whConfig.Secret
	)
	if whConfig.Token == "" {
		token = os.Getenv("TOKEN")
		if token == "" {
			panic("TOKEN environment variable is empty")
		}
	}
	if whConfig.Address == "" {
		address = os.Getenv("WEBHOOK_ADDRESS")
		if address == "" {
			panic("WEBHOOK_ADDRESS environment variable is empty")
		}
	}
	if whConfig.Secret == "" {
		secret = os.Getenv("WEBHOOK_SECRET")
		if secret == "" {
			secretBytes := sha256.Sum256([]byte(uuid.NewString()))
			secret = hex.EncodeToString(secretBytes[:])
		}
	}

	_ = address
	_ = secret

	b, err := gotgbot.NewBot(token, nil)
	if err != nil {
		log.Error().Stack().Err(err).Msg("failed to create new bot")
	}
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			log.Error().Stack().Err(err).Msg("an error occurred while handling update")
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})

	updater := ext.NewUpdater(dispatcher, nil)

	// 添加命令处理器，启动报时功能

	return &autoCallerConnect{
		bot:        b,
		dispatcher: dispatcher,
		updater:    updater,
		token:      token,
		cmdMap:     make(map[string]func(*gotgbot.Bot, int64)),
	}

}

// TODO: 考虑多种类型的注册
func (a *autoCallerConnect) RegisterHandler(handler func(*gotgbot.Bot, int64), cmd string, successBack string) {
	a.startAble = true
	if _, ok := a.cmdMap[cmd]; cmd != "" && ok {
		log.Error().Msg("this cmd has register,we will not add it again")
		return
	}
	if cmd != "" {
		a.cmdMap[cmd] = handler
		a.dispatcher.AddHandler(handlers.NewCommand(cmd, func(b *gotgbot.Bot, ctx *ext.Context) error {
			ctx.EffectiveChat.SendMessage(b, successBack, nil)
			go a.cmdMap[cmd](b, ctx.EffectiveChat.Id)
			return nil
		}))
	} else {
		a.dispatcher.AddHandler(handlers.NewMessage(nil, func(b *gotgbot.Bot, ctx *ext.Context) error {
			// 将群组的 chatId 保存下来
			chatId := ctx.EffectiveChat.Id
			go handler(b, chatId)
			return nil
		}))
	}
}

func (a *autoCallerConnect) Start() {
	if !a.startAble {
		return
	}
	// 开始轮询更新
	err := a.updater.StartPolling(a.bot, &ext.PollingOpts{})
	if err != nil {
		log.Error().Err(err)
	}

	log.Info().Msg("auto call start")
	a.updater.Idle()
}

func (a *autoCallerConnect) Stop() {
	a.dispatcher.Stop()
	err := a.updater.Stop()
	if err != nil {
		log.Error().Stack().Err(err).Msg("failed to stop dispatcher")
	}
}
