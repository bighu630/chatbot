package repo

import (
	"chatbot/internal/storage"
	"chatbot/internal/storage/model"
	"errors"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GroupConfig interface {
	GetByChatID(chatID int64) (*model.GroupConfig, error)
	SetReplyMultiplier(chatID int64, groupName string, multiplier float64) error
	SetEmotionNSFWMode(chatID int64, groupName string, mode int) error
	SetPersona(chatID int64, groupName string, persona string, updatedBy int64) error
	ClearPersona(chatID int64, groupName string) error
}

type groupConfigStorage struct {
	db *gorm.DB
}

func InitGroupConfigRepo() (GroupConfig, error) {
	db := storage.InitDB()
	if db == nil {
		log.Error().Msg("failed to init database")
		return nil, errors.New("failed to init database")
	}

	groupConfig := &model.GroupConfig{}
	if err := db.AutoMigrate(groupConfig); err != nil {
		log.Error().Err(err).Msg("failed to auto migrate group config table")
		return nil, err
	}

	return &groupConfigStorage{db: db}, nil
}

func (s *groupConfigStorage) GetByChatID(chatID int64) (*model.GroupConfig, error) {
	record := &model.GroupConfig{}
	if err := s.db.Where("chat_id = ?", chatID).First(record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return record, nil
}

func (s *groupConfigStorage) SetReplyMultiplier(chatID int64, groupName string, multiplier float64) error {
	record := &model.GroupConfig{
		ChatID:          chatID,
		GroupName:       groupName,
		ReplyMultiplier: &multiplier,
	}
	assignments := map[string]any{
		"reply_multiplier": multiplier,
		"updated_at":       time.Now(),
	}
	if groupName != "" {
		assignments["group_name"] = groupName
	}
	return s.upsert(record, assignments)
}

func (s *groupConfigStorage) SetEmotionNSFWMode(chatID int64, groupName string, mode int) error {
	record := &model.GroupConfig{
		ChatID:          chatID,
		GroupName:       groupName,
		EmotionNSFWMode: &mode,
	}
	assignments := map[string]any{
		"emotion_nsfw_mode": mode,
		"updated_at":        time.Now(),
	}
	if groupName != "" {
		assignments["group_name"] = groupName
	}
	return s.upsert(record, assignments)
}

func (s *groupConfigStorage) SetPersona(chatID int64, groupName string, persona string, updatedBy int64) error {
	record := &model.GroupConfig{
		ChatID:           chatID,
		GroupName:        groupName,
		Persona:          persona,
		PersonaUpdatedBy: updatedBy,
	}
	assignments := map[string]any{
		"persona":            persona,
		"persona_updated_by": updatedBy,
		"updated_at":         time.Now(),
	}
	if groupName != "" {
		assignments["group_name"] = groupName
	}
	return s.upsert(record, assignments)
}

func (s *groupConfigStorage) ClearPersona(chatID int64, groupName string) error {
	record := &model.GroupConfig{
		ChatID:    chatID,
		GroupName: groupName,
	}
	assignments := map[string]any{
		"persona":            "",
		"persona_updated_by": int64(0),
		"updated_at":         time.Now(),
	}
	if groupName != "" {
		assignments["group_name"] = groupName
	}
	return s.upsert(record, assignments)
}

func (s *groupConfigStorage) upsert(record *model.GroupConfig, assignments map[string]any) error {
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "chat_id"}},
		DoUpdates: clause.Assignments(assignments),
	}).Create(record).Error
}
