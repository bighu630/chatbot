package scheduler

import "github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"

type Timer interface {
	Start()
	Register(func(handler handlers.Response, cmd string))
}
