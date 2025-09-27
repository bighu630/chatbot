package handler

import (
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
)

const Help = `用法：

    cp语录:

        1. 引用其他人的消息

        2. 回复关键词 [mua,mua~,摸摸,抱抱] 等

        3. 有60%概率触发，摘星会引用你引用的话，并发🍬

    骂人：

        1. 引用其他人的消息

        2. 回复 [骂他，咬他]，其中 他 可以替换为 她 它 ta

        3. 100%触发,摘星会引用你引用而话，并骂他

    chatgpt：

        1. 在群聊中使用 "/chat msg" 可以与摘星聊天，MSG可以是任意内容

        2. 在群聊里引用摘星的话，摘星会以为你在和他聊天，@则不会

        3. 私聊摘星，摘星会与你对话

    youtubeMusic下载：

        私聊或者群聊里发送youtubeMusic链接，摘星会下载音乐并唱给你听


> 摘星是bot的名字：@ytbmusicPlaerBot
> 在这里可以看到摘星的源代码：https://github.com/bighu630/tg_bot

你们的star是作者最大的动力😀
`

func NewHelpHandler() handlers.Response {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		_, err := b.SendMessage(ctx.EffectiveChat.Id, Help, nil)
		return err
	}
}
