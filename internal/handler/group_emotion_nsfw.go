package handler

import (
	"bytes"
	"chatbot/internal/ai"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/rs/zerolog/log"
)

const (
	defaultGroupEmotionNSFWFile  = "./data/group_emotion_nsfw.json"
	defaultGroupEmotionNSFWMode  = 0
	groupEmotionNSFWModeSafe     = 0
	groupEmotionNSFWModeOnlyNSFW = 1
	groupEmotionNSFWModeMixed    = 2
)

type GroupEmotionNSFWConfig struct {
	mu      sync.RWMutex   `json:"-"`
	path    string         `json:"-"`
	Default int            `json:"default"`
	Groups  map[string]int `json:"groups"`
}

func NewGroupEmotionNSFWConfig(paths ...string) *GroupEmotionNSFWConfig {
	path := defaultGroupEmotionNSFWFile
	if len(paths) > 0 && paths[0] != "" {
		path = paths[0]
	}
	cfg := &GroupEmotionNSFWConfig{
		path:    path,
		Default: defaultGroupEmotionNSFWMode,
		Groups:  map[string]int{},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Warn().Err(err).Str("path", path).Msg("failed to read group emotion nsfw config; using defaults")
		}
		return cfg
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return cfg
	}
	if err := decodeGroupEmotionNSFWConfig(data, cfg); err != nil {
		log.Warn().Err(err).Str("path", path).Msg("failed to parse group emotion nsfw config; using defaults")
		return &GroupEmotionNSFWConfig{
			path:    path,
			Default: defaultGroupEmotionNSFWMode,
			Groups:  map[string]int{},
		}
	}
	cfg.Default = clampGroupEmotionNSFWMode(cfg.Default)
	if cfg.Groups == nil {
		cfg.Groups = map[string]int{}
	}
	for chatID, mode := range cfg.Groups {
		cfg.Groups[chatID] = clampGroupEmotionNSFWMode(mode)
	}
	log.Info().Str("path", path).Int("groups", len(cfg.Groups)).Int("default", cfg.Default).Msg("loaded group emotion nsfw config")
	return cfg
}

func decodeGroupEmotionNSFWConfig(data []byte, cfg *GroupEmotionNSFWConfig) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if _, ok := raw["default"]; ok {
		return json.Unmarshal(data, cfg)
	}
	if _, ok := raw["groups"]; ok {
		return json.Unmarshal(data, cfg)
	}

	groups := map[string]int{}
	if err := json.Unmarshal(data, &groups); err != nil {
		return err
	}
	cfg.Groups = groups
	return nil
}

func (c *GroupEmotionNSFWConfig) mode(chatID int64) int {
	mode := defaultGroupEmotionNSFWMode
	if c != nil {
		c.mu.RLock()
		defer c.mu.RUnlock()
		mode = c.Default
		if c.Groups != nil {
			if value, ok := c.Groups[strconv.FormatInt(chatID, 10)]; ok {
				mode = value
			}
		}
	}
	return clampGroupEmotionNSFWMode(mode)
}

func (c *GroupEmotionNSFWConfig) setGroupMode(chatID int64, mode int) error {
	if c == nil {
		return errors.New("group emotion nsfw config is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.path == "" {
		c.path = defaultGroupEmotionNSFWFile
	}
	if c.Groups == nil {
		c.Groups = map[string]int{}
	}
	c.Groups[strconv.FormatInt(chatID, 10)] = clampGroupEmotionNSFWMode(mode)
	return c.saveLocked()
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

func (c *GroupEmotionNSFWConfig) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
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
