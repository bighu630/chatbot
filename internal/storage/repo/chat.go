package repo

import (
	"chatbot/internal/storage"
	"chatbot/internal/storage/model"
	"errors"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type Chat interface {
	Add(chat *model.Chat) error
	GetMsgByTime(from, to time.Time, user string) ([]*model.Chat, error)
	GetAllUser() []string
	DeleteMsgBeforeTime(from time.Time) error
}

var _ Chat = (*chatStorage)(nil)

type chatStorage struct {
	db    *gorm.DB
	table string
}

func InitChatDB() (Chat, error) {
	db := storage.InitDB()
	if db == nil {
		log.Error().Msg("failed to init database")
		return nil, errors.New("failed to init database")
	}
	c := &model.Chat{}
	if err := db.AutoMigrate(c); err != nil {
		log.Error().Msg("failed to auto migrate chat table")
		return nil, errors.New("failed to auto migrate chat table")
	}
	table := c.TableName()
	return &chatStorage{db, table}, nil
}

func (c *chatStorage) Add(chat *model.Chat) error {
	if err := c.db.Create(chat).Error; err != nil {
		log.Error().Err(err).Msg("failed to add chat record")
		return err
	}
	return nil
}

func (c *chatStorage) GetMsgByTime(from, to time.Time, user string) ([]*model.Chat, error) {
	var chat []*model.Chat
	if err := c.db.Where("time >= ? AND time <= ? AND user_name = ?", from, to, user).Order("ID ASC").Find(&chat).Error; err != nil {
		return nil, err
	}
	return chat, nil
}

func (c *chatStorage) GetAllUser() []string {
	var users []string
	if err := c.db.Model(&model.Chat{}).Distinct("user_name").Find(&users).Error; err != nil {
		log.Error().Err(err).Msg("failed to get all user")
		return nil
	}
	return users
}

func (c *chatStorage) DeleteMsgBeforeTime(t time.Time) error {
	return c.db.Where("time < ?", t).Delete(&model.Chat{}).Error
}
