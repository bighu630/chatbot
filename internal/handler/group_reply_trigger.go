package handler

import (
	"chatbot/internal/storage/repo"
	"errors"
	"sync"

	"github.com/rs/zerolog/log"
)

const (
	defaultGroupReplyMultiplier = 1.0
	minGroupReplyMultiplier     = 0.0
	maxGroupReplyMultiplier     = 20.0
	randomGroupReplyBaseRate    = 0.003
)

type GroupReplyTriggerConfig struct {
	mu     sync.RWMutex
	repo   repo.GroupConfig
	cached map[int64]float64
	loaded map[int64]struct{}
}

func NewGroupReplyTriggerConfig(stores ...repo.GroupConfig) *GroupReplyTriggerConfig {
	var store repo.GroupConfig
	if len(stores) > 0 {
		store = stores[0]
	} else {
		var err error
		store, err = repo.InitGroupConfigRepo()
		if err != nil {
			log.Error().Err(err).Msg("failed to init group config repo for reply trigger")
		}
	}
	return &GroupReplyTriggerConfig{
		repo:   store,
		cached: make(map[int64]float64),
		loaded: make(map[int64]struct{}),
	}
}

func (c *GroupReplyTriggerConfig) rate(chatID int64) float64 {
	return clampPercentage(randomGroupReplyBaseRate * c.multiplier(chatID))
}

func (c *GroupReplyTriggerConfig) multiplier(chatID int64) float64 {
	multiplier := defaultGroupReplyMultiplier
	if c == nil {
		return multiplier
	}

	c.mu.RLock()
	value, ok := c.cached[chatID]
	_, loaded := c.loaded[chatID]
	c.mu.RUnlock()
	if ok {
		return clampGroupReplyMultiplier(value)
	}
	if loaded || c.repo == nil {
		return multiplier
	}

	record, err := c.repo.GetByChatID(chatID)
	if err != nil {
		log.Error().Err(err).Int64("chat_id", chatID).Msg("failed to load group reply multiplier")
		return multiplier
	}
	if record != nil && record.ReplyMultiplier != nil {
		multiplier = clampGroupReplyMultiplier(*record.ReplyMultiplier)
	}

	c.mu.Lock()
	c.loaded[chatID] = struct{}{}
	if record != nil && record.ReplyMultiplier != nil {
		c.cached[chatID] = multiplier
	}
	c.mu.Unlock()
	return multiplier
}

func (c *GroupReplyTriggerConfig) setGroupMultiplier(chatID int64, groupName string, multiplier float64) error {
	if c == nil {
		return errors.New("group reply trigger config is nil")
	}
	multiplier = clampGroupReplyMultiplier(multiplier)
	if c.repo == nil {
		return errors.New("group config repo is nil")
	}
	if err := c.repo.SetReplyMultiplier(chatID, groupName, multiplier); err != nil {
		return err
	}

	c.mu.Lock()
	c.cached[chatID] = multiplier
	c.loaded[chatID] = struct{}{}
	c.mu.Unlock()
	return nil
}

func clampGroupReplyMultiplier(multiplier float64) float64 {
	if multiplier < minGroupReplyMultiplier {
		return minGroupReplyMultiplier
	}
	if multiplier > maxGroupReplyMultiplier {
		return maxGroupReplyMultiplier
	}
	return multiplier
}

func clampPercentage(percentage float64) float64 {
	if percentage < 0.0 {
		return 0.0
	}
	if percentage > 1.0 {
		return 1.0
	}
	return percentage
}
