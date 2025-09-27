package log

import (
	"chatbot/config"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/natefinch/lumberjack"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func Init(cfg config.Log) {
	if cfg.TimeFormat != "" {
		zerolog.TimeFieldFormat = cfg.TimeFormat
	}
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	output.FormatLevel = func(i any) string {
		return strings.ToUpper(fmt.Sprintf("| %-6s|", i))
	}
	output.FormatMessage = func(i any) string {
		return fmt.Sprintf("\t%s\t", i)
	}
	output.FormatFieldName = func(i any) string {
		return fmt.Sprintf("%s:", i)
	}
	output.FormatFieldValue = func(i any) string {
		return strings.ToUpper(fmt.Sprintf("%s", i))
	}

	zerolog.SetGlobalLevel(zerolog.Level(cfg.Level))
	if cfg.Path != "" {
		logFile := &lumberjack.Logger{
			Filename:   cfg.Path,
			MaxSize:    10,   // 日志文件最大大小（以MB为单位）
			MaxBackups: 100,  // 保留的旧日志文件最大数量
			MaxAge:     300,  // 保留的旧日志文件最长天数
			Compress:   true, // 是否压缩旧日志文件
		}

		// 设置多输出：终端和文件
		multi := zerolog.MultiLevelWriter(zerolog.ConsoleWriter{Out: os.Stdout}, logFile)
		// 设置日志输出到文件
		log.Logger = log.Output(multi)
		log.Debug().Msg("start log with file")
	} else {
		log.Logger = log.Output(output)
	}
}
