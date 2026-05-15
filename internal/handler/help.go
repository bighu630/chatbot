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

    活跃度：

        1. 在群聊中使用 "/activity 0-20" 可以设置摘星随机插话的活跃度

        2. 0 表示关闭随机插话，1 是默认值，20 表示放大 20 倍

    NSFW：

        1. 在群聊中使用 "/nsfw 0|1|2" 可以设置表情搜索的 NSFW 模式

        2. 0 只搜正常图，1 只搜 NSFW，2 不做 NSFW 过滤

    youtubeMusic下载：

        私聊或者群聊里发送youtubeMusic链接，摘星会下载音乐并唱给你听

    反馈：

        1. 使用 "/feedback 反馈内容" 提交反馈

        2. bot 会把你的反馈转发给管理员


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
