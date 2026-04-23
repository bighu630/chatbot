package model

import "time"

type GroupConfig struct {
	ID              int64     `gorm:"column:id;primaryKey;AUTO_INCREMENT"`
	ChatID          int64     `gorm:"column:chat_id;uniqueIndex;not null"`
	GroupName       string    `gorm:"column:group_name;type:varchar(255)"`
	ReplyMultiplier *float64  `gorm:"column:reply_multiplier;type:double"`
	EmotionNSFWMode *int      `gorm:"column:emotion_nsfw_mode;type:int"`
	CreatedAt       time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (GroupConfig) TableName() string {
	return "group_config"
}
