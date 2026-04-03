package bot

import "github.com/PaulSonOfLars/gotgbot/v2"

var defaultCommands = []gotgbot.BotCommand{
	{Command: "help", Description: "查看帮助"},
	{Command: "chat", Description: "和机器人聊天"},
	{Command: "feedback", Description: "提交反馈给管理员"},
	{Command: "add", Description: "添加语录"},
	{Command: "admin", Description: "设置语录管理员"},
	{Command: "startkfc", Description: "开启 KFC 定时提醒"},
	{Command: "stopkfc", Description: "关闭 KFC 定时提醒"},
}

func RegisterBotCommands(bot *gotgbot.Bot) error {
	_, err := bot.SetMyCommands(defaultCommands, &gotgbot.SetMyCommandsOpts{
		Scope: gotgbot.BotCommandScopeDefault{},
	})
	return err
}
