package handler

import (
	"context"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

func setBotStatusWithContext(ctx context.Context, b *gotgbot.Bot, tgctx *ext.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				b.SendChatAction(tgctx.EffectiveChat.Id, "typing", nil)
				time.Sleep(7 * time.Second)
			}

		}
	}()
}

func formatAiResp(str string) string {
	str = strings.ReplaceAll(str, " **", "- **")
	str = strings.ReplaceAll(str, "\n* ", "\n- ")
	str = strings.ReplaceAll(str, "#-", "#")
	return str
}
