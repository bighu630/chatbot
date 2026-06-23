package handler

import (
	"context"
	"math/rand/v2"
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

func TriggerWithPercentage(percentage float64) bool {
	if percentage < 0.0 {
		percentage = 0.0
	}
	if percentage > 1.0 {
		percentage = 1.0
	}
	return rand.Float64() < percentage
}
