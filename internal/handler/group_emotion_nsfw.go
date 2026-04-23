package handler

import (
	"chatbot/internal/ai"
	"chatbot/internal/storage/repo"
	"errors"
	"sync"

	"github.com/rs/zerolog/log"
)

const (
	defaultGroupEmotionNSFWMode  = 0
	groupEmotionNSFWModeSafe     = 0
	groupEmotionNSFWModeOnlyNSFW = 1
	groupEmotionNSFWModeMixed    = 2
)

type GroupEmotionNSFWConfig struct {
	mu     sync.RWMutex
	repo   repo.GroupConfig
	cached map[int64]int
	loaded map[int64]struct{}
}

func NewGroupEmotionNSFWConfig(stores ...repo.GroupConfig) *GroupEmotionNSFWConfig {
	var store repo.GroupConfig
	if len(stores) > 0 {
		store = stores[0]
	} else {
		var err error
		store, err = repo.InitGroupConfigRepo()
		if err != nil {
			log.Error().Err(err).Msg("failed to init group config repo for emotion nsfw")
		}
	}
	return &GroupEmotionNSFWConfig{
		repo:   store,
		cached: make(map[int64]int),
		loaded: make(map[int64]struct{}),
	}
}

func (c *GroupEmotionNSFWConfig) mode(chatID int64) int {
	mode := defaultGroupEmotionNSFWMode
	if c == nil {
		return mode
	}

	c.mu.RLock()
	value, ok := c.cached[chatID]
	_, loaded := c.loaded[chatID]
	c.mu.RUnlock()
	if ok {
		return clampGroupEmotionNSFWMode(value)
	}
	if loaded || c.repo == nil {
		return mode
	}

	record, err := c.repo.GetByChatID(chatID)
	if err != nil {
		log.Error().Err(err).Int64("chat_id", chatID).Msg("failed to load group emotion nsfw mode")
		return mode
	}
	if record != nil && record.EmotionNSFWMode != nil {
		mode = clampGroupEmotionNSFWMode(*record.EmotionNSFWMode)
	}

	c.mu.Lock()
	c.loaded[chatID] = struct{}{}
	if record != nil && record.EmotionNSFWMode != nil {
		c.cached[chatID] = mode
	}
	c.mu.Unlock()
	return mode
}

func (c *GroupEmotionNSFWConfig) setGroupMode(chatID int64, groupName string, mode int) error {
	if c == nil {
		return errors.New("group emotion nsfw config is nil")
	}
	mode = clampGroupEmotionNSFWMode(mode)
	if c.repo == nil {
		return errors.New("group config repo is nil")
	}
	if err := c.repo.SetEmotionNSFWMode(chatID, groupName, mode); err != nil {
		return err
	}

	c.mu.Lock()
	c.cached[chatID] = mode
	c.loaded[chatID] = struct{}{}
	c.mu.Unlock()
	return nil
}

func (c *GroupEmotionNSFWConfig) apply(params *ai.EmotionSearchParams, chatID int64) int {
	if params == nil {
		return defaultGroupEmotionNSFWMode
	}
	mode := c.mode(chatID)
	switch mode {
	case groupEmotionNSFWModeSafe:
		value := false
		params.IsNSFW = &value
	case groupEmotionNSFWModeOnlyNSFW:
		value := true
		params.IsNSFW = &value
	default:
		params.IsNSFW = nil
	}
	return mode
}

func clampGroupEmotionNSFWMode(mode int) int {
	if mode < groupEmotionNSFWModeSafe {
		return groupEmotionNSFWModeSafe
	}
	if mode > groupEmotionNSFWModeMixed {
		return groupEmotionNSFWModeMixed
	}
	return mode
}
