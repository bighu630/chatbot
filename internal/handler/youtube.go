package handler

import (
	"chatbot/internal/handler/update"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const Name = "ytm"

var _ ext.Handler = (*youtubeHandler)(nil)

type youtubeHandler struct {
	ytdlpPath string
}

func NewYoutubeHandler(ytdlpPath string) ext.Handler {
	if ytdlpPath == "" {
		ytdlpPath = "yt-dlp"
	}
	update.GetUpdater().Register(true, Name, func(b *gotgbot.Bot, ctx *ext.Context) bool {
		msg := ctx.EffectiveMessage.Text
		if strings.Contains(msg, "music.youtube") {
			return true
		}
		if len(msg) == 11 {
			// 使用正则表达式 ^[a-zA-Z0-9]+$ 来匹配只包含字母和数字的字符串
			regex := regexp.MustCompile(`^[a-zA-Z0-9]+$`)
			return regex.MatchString(msg)
		}
		return false
	})
	return &youtubeHandler{ytdlpPath: ytdlpPath}
}

func (y *youtubeHandler) Name() string {
	return Name
}

func (y *youtubeHandler) CheckUpdate(b *gotgbot.Bot, ctx *ext.Context) bool {
	return update.GetUpdater().CheckUpdate(y.Name(), b, ctx)
}

// url demo = https://music.youtube.com/watch?v=s87joFadgXg&list=OLAK5uy_kVpwmyOiQxW6pTRUSauQ_Ms1Jbm9jMBLU v eversion
// we need cat of after list
func (y *youtubeHandler) HandleUpdate(b *gotgbot.Bot, ctx *ext.Context) error {
	log.Debug().Msg("get youtube music url")
	url := checkUrl(ctx.EffectiveMessage.Text)
	uuid := uuid.NewString()
	defer func() {
		os.RemoveAll(uuid)
	}()
	a := make(chan struct{})
	go func() {
		for {
			select {
			case <-a:
				return
			default:
				b.SendChatAction(ctx.EffectiveChat.Id, "record_voice", nil)
				time.Sleep(7 * time.Second)
			}
		}
	}()
	ytdlp := exec.Command(y.ytdlpPath, "-f", "ba", "-x", "--audio-format", "mp3", "-P", uuid, url)
	err := ytdlp.Run()
	a <- struct{}{}
	if err != nil {
		log.Error().Stack().Err(err)
		return err
	}
	mp3List, err := os.ReadDir(uuid)
	if err != nil {
		log.Error().Stack().Err(err).Msg("failed to read dir")
		return err
	}
	if len(mp3List) != 1 {
		log.Error().Stack().Msg("mp3 list len != 1")
		return errors.New("mp3 list len != 1")
	}
	musicFile := mp3List[0]
	if musicFile.IsDir() {
		log.Error().Stack().Msg("is dir")
		return errors.New("is dir")
	}
	name := musicFile.Name()
	path := uuid + "/" + name
	musicReader, err := os.Open(path)
	if err != nil {
		log.Error().Stack().Err(err).Msg("failed to open music file")
		return err
	}
	for i := 0; i < 3; i++ {
		_, err = b.SendAudio(ctx.EffectiveChat.Id, gotgbot.InputFileByReader(name, musicReader), nil)
		if err != nil {
			log.Error().Stack().Err(err).Msg("failed to send audio")
		} else {
			return nil
		}
	}
	return err
}

func checkUrl(url string) string {
	if strings.Contains(url, "&list=") {
		urlSplit := strings.Split(url, "&list=")
		url = urlSplit[0]
	}
	return url
}
