package update

import (
	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/rs/zerolog/log"
)

type checkUpdate func(b *gotgbot.Bot, ctx *ext.Context) bool

var Updater *updater

type updater struct {
	repeatable   map[string]checkUpdate
	unrepeatable map[string]checkUpdate
}

func GetUpdater() *updater {
	if Updater == nil {
		Updater = &updater{make(map[string]checkUpdate), make(map[string]checkUpdate)}
	}
	return Updater
}

// repeatable : 是否允许重复，如果允许，当存在多个chaeckUpdate成功时然返回true,否则放行
//
// typ ：类型标志
//
// checkFun : 校验更新用的方法，注意根据type区分，每个typ只保留最后一次注册的checkFun
// WARN: 谈及update时需要注意是否与其他update冲突
func (up *updater) Register(repeatable bool, typ string, checkFun checkUpdate) {
	if repeatable {
		// 无论是否存在，都使用新的
		up.repeatable[typ] = checkFun
	} else {
		up.unrepeatable[typ] = checkFun
	}
}

// 校验是否需要更新
func (up updater) CheckUpdate(typ string, b *gotgbot.Bot, ctx *ext.Context) bool {
	// 允许重复，则必然会响应
	if checkFun, ok := up.repeatable[typ]; ok {
		return checkFun(b, ctx)
	}
	// 不允许重复则需要校验是否存在其他允许重复的
	// 注意：如果两个不允许重复的则两个都不会响应
	if checkFun, ok := up.unrepeatable[typ]; ok {
		if checkFun(b, ctx) {
			for temptyp, v := range up.repeatable {
				if temptyp == typ {
					continue
				}

				if v(b, ctx) {
					log.Debug().Msg("当前更新存在冲突，故不更新")
					return false
				}
			}
			for temptyp, v := range up.unrepeatable {
				if temptyp == typ {
					continue
				}
				if v(b, ctx) {
					log.Warn().Msgf("当前更新存在冲突，故不更新, 但对方也不会更新,当前type: %s 对方type:%s", typ, temptyp)
					return false
				}
			}
			return true
		}
	}
	return false
}
