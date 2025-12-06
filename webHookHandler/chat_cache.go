package handler

import (
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

func (c *chatCache) AddMsg(group string, user string, msg string) {
	c.chatLock.Lock()
	defer c.chatLock.Unlock()
	log.Info().Str("group", group).Str("user", user).Str("msgs", msg).Msg("收到一个群消息")
	c.chatCache[group] = append(c.chatCache[group], tgMsg{user, msg})
	if len(c.chatCache[group]) > 50 {
		c.chatCache[group] = c.chatCache[group][:50]
	}
}

func (c *chatCache) GetChatMsgAndClean(group string) string {
	c.chatLock.Lock()
	defer c.chatLock.Unlock()
	msgs, ok := c.chatCache[group]
	if !ok {
		return ""
	}
	resp := ""
	for _, m := range msgs {
		resp += fmt.Sprintf("%s: %s||", m.User, m.Msg)
	}
	if len(resp) > 2 {
		resp = resp[:len(resp)-2]
		log.Info().Str("chatCache", resp).Msg("读取群消息缓存")
	}
	c.chatCache[group] = []tgMsg{}
	return resp
}
