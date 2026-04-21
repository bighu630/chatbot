package model

import "time"

type GroupPersona struct {
	ID        int64     `gorm:"column:id;primaryKey;AUTO_INCREMENT"`
	ChatID    int64     `gorm:"column:chat_id;uniqueIndex;not null"`
	Persona   string    `gorm:"column:persona;type:text;not null"`
	UpdatedBy int64     `gorm:"column:updated_by;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (GroupPersona) TableName() string {
	return "group_persona"
}
