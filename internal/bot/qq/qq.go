package qq

import (
	"chatbot/internal/bot/qqonebot"
	"chatbot/internal/chatcore"
	"chatbot/internal/platform"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Adapter struct {
	core   *chatcore.Service
	client *qqonebot.Client
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	replyIndex map[string]platform.MessageRef
	botID string
}

func New(core *chatcore.Service, client *qqonebot.Client) *Adapter {
	ctx, cancel := context.WithCancel(context.Background())
	botID := ""
	if core != nil {
		botID = core.BotName
	}
	return &Adapter{core: core, client: client, ctx: ctx, cancel: cancel, replyIndex: make(map[string]platform.MessageRef), botID: botID}
}

func (a *Adapter) Start() {
	if a == nil || a.core == nil || a.client == nil {
		return
	}
	if err := a.client.Connect(a.ctx); err != nil {
		return
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		for {
			select {
			case <-a.ctx.Done():
				return
			case evt, ok := <-a.client.Events():
				if !ok {
					return
				}
				if !evt.Message.IsGroup() {
					continue
				}
				msg := a.toPlatformMessage(evt.Message)
				resp, handled, err := a.core.Handle(msg)
				if err != nil || !handled {
					continue
				}
				if replyID := replyMessageID(msg); replyID != 0 {
					if id, err := a.client.SendGroupMessageWithReply(evt.Message.GroupID, resp, replyID); err == nil && id != 0 {
						a.recordBotMessage(evt.Message.GroupID, id)
					}
					continue
				}
				if id, err := a.client.SendGroupMessageWithReply(evt.Message.GroupID, resp, 0); err == nil && id != 0 {
					a.recordBotMessage(evt.Message.GroupID, id)
				}
			}
		}
	}()
}

func (a *Adapter) Stop() {
	if a == nil {
		return
	}
	if a.cancel != nil {
		a.cancel()
	}
	if a.client != nil {
		_ = a.client.Close()
	}
	a.wg.Wait()
}

func (a *Adapter) toPlatformMessage(m qqonebot.IncomingMessage) platform.Message {
	msg := platform.Message{
		Platform: platform.PlatformQQ,
		Chat: platform.ChatRef{Type: platform.GroupChat, ID: strconv.FormatInt(m.GroupID, 10)},
		Sender: platform.UserRef{ID: strconv.FormatInt(m.UserID, 10), Name: firstNonEmpty(m.Sender.Card, m.Sender.Nickname), Username: firstNonEmpty(m.Sender.Card, m.Sender.Nickname)},
		Content: platform.TextContent{Text: m.Text()},
	}
	if m.MessageID != 0 {
		a.recordMessage(m.GroupID, m.MessageID, strconv.FormatInt(m.UserID, 10))
	}
	if m.Reply != nil && m.Reply.MessageID != 0 {
		if ref, ok := a.lookupMessage(m.GroupID, m.Reply.MessageID); ok {
			msg.ReplyTo = &ref
		} else {
			msg.ReplyTo = &platform.MessageRef{SenderID: strconv.FormatInt(m.UserID, 10), MessageID: strconv.FormatInt(m.Reply.MessageID, 10)}
		}
	}
	for _, seg := range m.Message {
		switch seg.Type {
		case "at":
			if qq := firstNonEmpty(seg.Data["qq"], seg.Data["user_id"]); qq != "" {
				msg.Mentions = append(msg.Mentions, platform.Mention{TargetID: qq, TargetName: qq})
			}
		case "image":
			img := platform.ImageContent{URL: firstNonEmpty(seg.Data["url"], seg.Data["file"], seg.Data["file_id"]), FileID: firstNonEmpty(seg.Data["file"], seg.Data["file_id"]), Type: firstNonEmpty(seg.Data["type"], "image")}
			if data, err := downloadImage(img.URL); err == nil {
				img.Data = data
			}
			msg.Images = append(msg.Images, img)
		}
	}
	return msg
}

func (a *Adapter) recordMessage(groupID, messageID int64, senderID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.replyIndex[indexKey(groupID, messageID)] = platform.MessageRef{Platform: platform.PlatformQQ, ChatID: strconv.FormatInt(groupID, 10), MessageID: strconv.FormatInt(messageID, 10), SenderID: senderID}
}

func (a *Adapter) recordBotMessage(groupID, messageID int64) {
	a.recordMessage(groupID, messageID, a.botID)
}

func (a *Adapter) lookupMessage(groupID, messageID int64) (platform.MessageRef, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ref, ok := a.replyIndex[indexKey(groupID, messageID)]
	return ref, ok
}

func replyMessageID(msg platform.Message) int64 {
	if msg.ReplyTo == nil {
		return 0
	}
	if v, err := strconv.ParseInt(msg.ReplyTo.MessageID, 10, 64); err == nil {
		return v
	}
	return 0
}

func indexKey(groupID, messageID int64) string {
	return fmt.Sprintf("%d:%d", groupID, messageID)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func downloadImage(url string) ([]byte, error) {
	if strings.TrimSpace(url) == "" {
		return nil, fmt.Errorf("empty url")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download image: http %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
