package model

type GroupData struct {
	ID  int    `gorm:"column:id;primaryKey;AUTO_INCREMENT"`
	KFC string `gorm:"column:kfc;type:text;comment:KFC"`
}

func (GroupData) TableName() string {
	return "chat"
}
