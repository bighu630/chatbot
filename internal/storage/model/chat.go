package model

import "time"

type Chat struct {
	ID       int       `gorm:"column:id;primaryKey;AUTO_INCREMENT"`
	Time     time.Time `gorm:"column:time;type:datetime;not null"`
	IsUser   bool      `gorm:"column:is_user;type:bool;not null"`
	UserName string    `gorm:"column:user_name;type:varchar(255)"`
	Msg      string    `gorm:"column:msg;type:text;not null"`
}

func NewChat(userName string, isUser bool, msg string) *Chat {
	return &Chat{
		Time:     time.Now(),
		IsUser:   isUser,
		UserName: userName,
		Msg:      msg,
	}
}

func (Chat) TableName() string {
	return "chat"
}
