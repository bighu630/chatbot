package qqonebot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Segment struct {
	Type string            `json:"type"`
	Data map[string]string `json:"data"`
}

type IncomingMessage struct {
	MessageType string    `json:"message_type"`
	GroupID     int64     `json:"group_id"`
	UserID      int64     `json:"user_id"`
	MessageID   int64     `json:"message_id"`
	RawMessage  string    `json:"raw_message"`
	Message     []Segment `json:"message"`
	Sender      struct {
		Nickname string `json:"nickname"`
		Card     string `json:"card"`
	} `json:"sender"`
	Reply *ReplyInfo `json:"reply"`
}

type ReplyInfo struct {
	MessageID int64 `json:"message_id"`
}

func (m IncomingMessage) Text() string {
	if strings.TrimSpace(m.RawMessage) != "" {
		return m.RawMessage
	}
	var b strings.Builder
	for _, seg := range m.Message {
		if seg.Type == "text" {
			b.WriteString(seg.Data["text"])
		}
	}
	return b.String()
}

func (m IncomingMessage) IsGroup() bool { return m.MessageType == "group" }

type Event struct {
	PostType string          `json:"post_type"`
	Message  IncomingMessage `json:"message"`
}

type responsePacket struct {
	Status  string          `json:"status"`
	Retcode int             `json:"retcode"`
	Data    json.RawMessage `json:"data"`
	Echo    string          `json:"echo"`
}

type Client struct {
	url    string
	header http.Header
	mu     sync.Mutex
	conn   *websocket.Conn
	seq    int64
	events chan Event
	pending map[string]chan responsePacket
	done   chan struct{}
}

func New(url string) *Client {
	return &Client{url: url, events: make(chan Event, 64), pending: make(map[string]chan responsePacket), done: make(chan struct{})}
}

func (c *Client) Connect(ctx context.Context) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.url, c.header)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	go c.readLoop()
	return nil
}

func (c *Client) Close() error {
	select {
	case <-c.done:
	default:
		close(c.done)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *Client) Events() <-chan Event {
	return c.events
}

func (c *Client) ReadEvent() (*Event, error) {
	evt, ok := <-c.events
	if !ok {
		return nil, fmt.Errorf("connection closed")
	}
	return &evt, nil
}

func (c *Client) SendGroupMessage(groupID int64, text string) error {
	_, err := c.SendGroupMessageWithReply(groupID, text, 0)
	return err
}

func (c *Client) SendGroupMessageWithReply(groupID int64, text string, replyToMessageID int64) (int64, error) {
	resp, err := c.call(map[string]any{
		"action": "send_group_msg",
		"params": map[string]any{"group_id": groupID, "message": text, "reply_to_message_id": replyToMessageID},
	})
	if err != nil {
		return 0, err
	}
	var out struct {
		MessageID int64 `json:"message_id"`
	}
	if len(resp.Data) > 0 {
		_ = json.Unmarshal(resp.Data, &out)
	}
	return out.MessageID, nil
}

func (c *Client) call(payload map[string]any) (*responsePacket, error) {
	c.mu.Lock()
	conn := c.conn
	c.seq++
	echo := fmt.Sprintf("%d", c.seq)
	respCh := make(chan responsePacket, 1)
	c.pending[echo] = respCh
	c.mu.Unlock()
	if conn == nil {
		return nil, fmt.Errorf("not connected")
	}
	payload["echo"] = echo
	if err := conn.WriteJSON(payload); err != nil {
		c.mu.Lock()
		delete(c.pending, echo)
		c.mu.Unlock()
		return nil, err
	}
	select {
	case resp := <-respCh:
		return &resp, nil
	case <-time.After(5 * time.Second):
		c.mu.Lock()
		delete(c.pending, echo)
		c.mu.Unlock()
		return nil, fmt.Errorf("onebot call timeout")
	case <-c.done:
		return nil, fmt.Errorf("connection closed")
	}
}

func (c *Client) readLoop() {
	defer close(c.events)
	for {
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn == nil {
			return
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var resp responsePacket
		if err := json.Unmarshal(data, &resp); err == nil && resp.Echo != "" {
			c.mu.Lock()
			ch := c.pending[resp.Echo]
			delete(c.pending, resp.Echo)
			c.mu.Unlock()
			if ch != nil {
				ch <- resp
			}
			continue
		}
		var evt Event
		if err := json.Unmarshal(data, &evt); err == nil && evt.PostType != "" {
			select {
			case c.events <- evt:
			case <-c.done:
				return
			}
		}
	}
}
