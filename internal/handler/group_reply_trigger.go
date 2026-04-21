package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/rs/zerolog/log"
)

const (
	defaultGroupReplyTriggerFile = "./data/group_reply_trigger.json"
	defaultGroupReplyMultiplier  = 1.0
	minGroupReplyMultiplier      = 0.0
	maxGroupReplyMultiplier      = 10.0
	randomGroupReplyBaseRate     = 0.003
)

type GroupReplyTriggerConfig struct {
	mu      sync.RWMutex       `json:"-"`
	path    string             `json:"-"`
	Default float64            `json:"default"`
	Groups  map[string]float64 `json:"groups"`
}

func NewGroupReplyTriggerConfig(paths ...string) *GroupReplyTriggerConfig {
	path := defaultGroupReplyTriggerFile
	if len(paths) > 0 && paths[0] != "" {
		path = paths[0]
	}
	cfg := &GroupReplyTriggerConfig{
		path:    path,
		Default: defaultGroupReplyMultiplier,
		Groups:  map[string]float64{},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Warn().Err(err).Str("path", path).Msg("failed to read group reply trigger config; using defaults")
		}
		return cfg
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return cfg
	}

	if err := decodeGroupReplyTriggerConfig(data, cfg); err != nil {
		log.Warn().Err(err).Str("path", path).Msg("failed to parse group reply trigger config; using defaults")
		return &GroupReplyTriggerConfig{
			path:    path,
			Default: defaultGroupReplyMultiplier,
			Groups:  map[string]float64{},
		}
	}
	cfg.Default = clampGroupReplyMultiplier(cfg.Default)
	if cfg.Groups == nil {
		cfg.Groups = map[string]float64{}
	}
	for chatID, multiplier := range cfg.Groups {
		cfg.Groups[chatID] = clampGroupReplyMultiplier(multiplier)
	}
	log.Info().Str("path", path).Int("groups", len(cfg.Groups)).Float64("default", cfg.Default).Msg("loaded group reply trigger config")
	return cfg
}

func decodeGroupReplyTriggerConfig(data []byte, cfg *GroupReplyTriggerConfig) error {
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

	groups := map[string]float64{}
	if err := json.Unmarshal(data, &groups); err != nil {
		return err
	}
	cfg.Groups = groups
	return nil
}

func (c *GroupReplyTriggerConfig) rate(chatID int64) float64 {
	multiplier := defaultGroupReplyMultiplier
	if c != nil {
		c.mu.RLock()
		defer c.mu.RUnlock()
		multiplier = c.Default
		if c.Groups != nil {
			if value, ok := c.Groups[strconv.FormatInt(chatID, 10)]; ok {
				multiplier = value
			}
		}
	}
	return clampPercentage(randomGroupReplyBaseRate * clampGroupReplyMultiplier(multiplier))
}

func (c *GroupReplyTriggerConfig) setGroupMultiplier(chatID int64, multiplier float64) error {
	if c == nil {
		return errors.New("group reply trigger config is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.path == "" {
		c.path = defaultGroupReplyTriggerFile
	}
	if c.Groups == nil {
		c.Groups = map[string]float64{}
	}
	c.Groups[strconv.FormatInt(chatID, 10)] = clampGroupReplyMultiplier(multiplier)
	return c.saveLocked()
}

func (c *GroupReplyTriggerConfig) saveLocked() error {
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
