package handler

import (
	"chatbot/internal/chatcore"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
)

type tgMsg struct {
	User string
	Msg  string
}

type chatCache struct {
	chatCache map[string][]tgMsg
	chatLock  sync.Mutex // 大粒度的锁目前没问题
}

func NewChatCache() *chatCache {
	return &chatCache{
		chatCache: make(map[string][]tgMsg),
	}
}

func (c *chatCache) Add(threadKey, sender, text string) {
	c.chatLock.Lock()
	defer c.chatLock.Unlock()
	log.Info().Str("group", threadKey).Str("user", sender).Str("msgs", text).Msg("收到一个群消息")
	c.chatCache[threadKey] = append(c.chatCache[threadKey], tgMsg{sender, text})
	if len(c.chatCache[threadKey]) > 20 {
		c.chatCache[threadKey] = c.chatCache[threadKey][len(c.chatCache[threadKey])-20:]
	}
}

func (c *chatCache) Drain(threadKey string) (string, int) {
	c.chatLock.Lock()
	defer c.chatLock.Unlock()
	msgs, ok := c.chatCache[threadKey]
	if !ok {
		return "", 0
	}
	l := len(msgs)
	resp := ""
	for _, m := range msgs {
		resp += fmt.Sprintf("%s: %s||", m.User, m.Msg)
	}
	if len(resp) > 2 {
		resp = resp[:len(resp)-2]
		log.Info().Str("chatCache", resp).Msg("读取群消息缓存")
	}
	c.chatCache[threadKey] = []tgMsg{}
	return resp, l
}

func (c *chatCache) AddMsg(group string, user string, msg string) {
	c.Add(group, user, msg)
}

func (c *chatCache) GetChatMsgAndClean(group string) (string, int) {
	return c.Drain(group)
}

var _ chatcore.History = (*chatCache)(nil)
