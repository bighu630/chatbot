package chatcore

import (
	"fmt"
	"sync"
)

type Message struct {
	Sender string
	Text   string
}

type Cache struct {
	mu   sync.Mutex
	data map[string][]Message
	cap  int
}

func NewCache(capacity int) *Cache {
	if capacity <= 0 {
		capacity = 20
	}
	return &Cache{data: make(map[string][]Message), cap: capacity}
}

func (c *Cache) Add(threadKey, sender, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[threadKey] = append(c.data[threadKey], Message{Sender: sender, Text: text})
	if len(c.data[threadKey]) > c.cap {
		c.data[threadKey] = c.data[threadKey][len(c.data[threadKey])-c.cap:]
	}
}

func (c *Cache) Drain(threadKey string) (string, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	msgs := c.data[threadKey]
	if len(msgs) == 0 {
		return "", 0
	}
	resp := ""
	for _, m := range msgs {
		resp += fmt.Sprintf("%s: %s||", m.Sender, m.Text)
	}
	if len(resp) > 2 {
		resp = resp[:len(resp)-2]
	}
	c.data[threadKey] = nil
	return resp, len(msgs)
}
