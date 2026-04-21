package repo

import (
	"chatbot/internal/storage"
	"chatbot/internal/storage/model"
	"errors"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GroupPersona interface {
	GetByChatID(chatID int64) (*model.GroupPersona, error)
	Upsert(chatID int64, persona string, updatedBy int64) error
	Delete(chatID int64) error
}

type groupPersonaStorage struct {
	db *gorm.DB
}

func InitGroupPersonaRepo() (GroupPersona, error) {
	db := storage.InitDB()
	if db == nil {
		log.Error().Msg("failed to init database")
		return nil, errors.New("failed to init database")
	}

	persona := &model.GroupPersona{}
	if err := db.AutoMigrate(persona); err != nil {
		log.Error().Err(err).Msg("failed to auto migrate group persona table")
		return nil, err
	}

	return &groupPersonaStorage{db: db}, nil
}

func (s *groupPersonaStorage) GetByChatID(chatID int64) (*model.GroupPersona, error) {
	record := &model.GroupPersona{}
	if err := s.db.Where("chat_id = ?", chatID).First(record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return record, nil
}

func (s *groupPersonaStorage) Upsert(chatID int64, persona string, updatedBy int64) error {
	record := &model.GroupPersona{
		ChatID:    chatID,
		Persona:   persona,
		UpdatedBy: updatedBy,
	}

	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "chat_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"persona":    persona,
			"updated_by": updatedBy,
		}),
	}).Create(record).Error
}

func (s *groupPersonaStorage) Delete(chatID int64) error {
	return s.db.Where("chat_id = ?", chatID).Delete(&model.GroupPersona{}).Error
}
