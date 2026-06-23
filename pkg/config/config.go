package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

type Config struct {
	WebHookConfig WebHookConfig `toml:"webHookConfig"`
	QQConfig      QQConfig      `toml:"qq"`
	Log           Log           `toml:"log"`
	Ytdlp         Ytdlp         `toml:"ytdlp"`
	Ai            Ai            `toml:"ai"`
	Emotion       EmotionConfig `toml:"emotion"`
	Admin         AdminConfig   `toml:"admin"`
	Storage       StorageConfig `toml:"storage"`
	TencentConfig TencentConfig `toml:"tencent"`
}

type AdminConfig struct {
	ChatIDs []int64 `toml:"chatIDs"`
}

type WebHookConfig struct {
	Token    string `json:"token" toml:"token"`
	Address  string `json:"address" toml:"address"`
	Domain   string `json:"domain" toml:"domain"`
	Secret   string `json:"secret" toml:"secret"`
	CertFile string `json:"certFile" toml:"certFile"`
	KeyFile  string `json:"keyFile" toml:"keyFile"`
}

type QQConfig struct {
	Enable bool   `toml:"enable"`
	WSAddr string `toml:"wsAddr"`
	BotQQ  string `toml:"botQQ"`
	Token  string `toml:"token"`
}

type TencentConfig struct {
	SecretID  string `json:"secretID" toml:"secretID"`
	SecretKey string `json:"secretKey" toml:"secretKey"`
}

type Log struct {
	TimeFormat string `json:"timeFormat" toml:"timeFormat"`
	Path       string `json:"path" toml:"path"`
	Level      int    `json:"level" toml:"level"`
}

type Ytdlp struct {
	Enable bool   `json:"enable" tomel:"enable"`
	Path   string `json:"path" toml:"path"`
}

type EmotionConfig struct {
	Enable    bool   `json:"enable" toml:"enable"`
	APIBaseURL string `json:"apiBaseUrl" toml:"apiBaseUrl"`
	APIKey    string `json:"apiKey" toml:"apiKey"`
}

type Ai struct {
	Enable        bool   `json:"enable" tomel:"enable"`
	GeminiKey     string `json:"geminiKey" toml:"geminiKey"`
	GeminiModel   string `json:"geminiModel" toml:"geminiModel"`
	OpenAiKey     string `json:"openaiKey" toml:"openaiKey"`
	OpenAiModel   string `json:"openaiModel" toml:"openaiModel"`
	OpenAiBaseUrl string `json:"openaiBaseUrl" toml:"openaiBaseUrl"`
}

type StorageConfig struct {
	Enable   bool         `json:"enable" tomel:"enable"`
	Provider string       `mapstructure:"provider" yaml:"provider" toml:"provider"`
	SqlDB    *SqlDBConfig `mapstructure:"sqlite" yaml:"sqlite" toml:"sqlite"`
}

type SqlDBConfig struct {
	Path       string `mapstructure:"path" yaml:"path" toml:"path"`
	Name       string `mapstructure:"name" yaml:"name" toml:"name"`
	Quotations string `mapstructure:"quotations" yaml:"quotations" toml:"quotations"`
	Host       string `mapstructure:"host" yaml:"host" toml:"host"`
	Port       string `mapstructure:"port" yaml:"port" toml:"port"`
	User       string `mapstructure:"user" yaml:"user" toml:"user"`
	Password   string `mapstructure:"password" yaml:"password" toml:"password"`
	DBName     string `mapstructure:"dbname" yaml:"dbname" toml:"dbname"`
	Charset    string `mapstructure:"charset" yaml:"charset" toml:"charset"`
}

func Load(path string) (*Config, error) {
	cfg := new(Config)
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("load config %s: %w", path, err)
	}
	return cfg, nil
}
