package config

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server          ServerConfig    `yaml:"server"`
	Database        DatabaseConfig  `yaml:"database"`
	DefaultChannels []string        `yaml:"default_channels"`
	RateLimit       RateLimitConfig `yaml:"rate_limit"`
	Retry           RetryConfig     `yaml:"retry"`
	Channels        []ChannelConfig `yaml:"channels"`
	Silence         SilenceConfig   `yaml:"silence"`
}

type ServerConfig struct {
	Listen        string `yaml:"listen"`
	MetricsListen string `yaml:"metrics_listen"`
	AdminToken    string `yaml:"admin_token"`
	DebugPort     string `yaml:"debug_port"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type RateLimitConfig struct {
	PerSourceLevel LimitRule `yaml:"per_source_level"`
	PerSource      LimitRule `yaml:"per_source"`
	Global         LimitRule `yaml:"global"`
}

type LimitRule struct {
	Window string `yaml:"window"`
	Max    int    `yaml:"max"`
}

func (r LimitRule) ParseWindow() time.Duration {
	d, err := time.ParseDuration(r.Window)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}

type RetryConfig struct {
	MaxAttempts int    `yaml:"max_attempts"`
	BaseDelay   string `yaml:"base_delay"`
	MaxDelay    string `yaml:"max_delay"`
}

func (r RetryConfig) ParseBaseDelay() time.Duration {
	d, err := time.ParseDuration(r.BaseDelay)
	if err != nil {
		return 2 * time.Second
	}
	return d
}

func (r RetryConfig) ParseMaxDelay() time.Duration {
	d, err := time.ParseDuration(r.MaxDelay)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

type ChannelConfig struct {
	Name    string         `yaml:"name"`
	Type    string         `yaml:"type"`
	Config  map[string]string `yaml:"config"`
	Filter  *RouteFilter   `yaml:"filter"`
	Enabled bool           `yaml:"enabled"`
}

type RouteFilter struct {
	Levels  []string          `yaml:"levels"`
	Tags    map[string]string `yaml:"tags"`
	Sources []string          `yaml:"sources"`
}

type SilenceConfig struct {
	Windows []SilenceWindow `yaml:"windows"`
}

type SilenceWindow struct {
	Start    string `yaml:"start"`
	End      string `yaml:"end"`
	Timezone string `yaml:"timezone"`
	Levels   []string `yaml:"levels"`
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Listen:        ":8080",
			MetricsListen: ":9090",
		},
		Database: DatabaseConfig{
			Path: "data/notifier.db",
		},
		DefaultChannels: []string{},
		RateLimit: RateLimitConfig{
			PerSourceLevel: LimitRule{Window: "5m", Max: 10},
			PerSource:      LimitRule{Window: "5m", Max: 50},
			Global:         LimitRule{Window: "1m", Max: 100},
		},
		Retry: RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   "2s",
			MaxDelay:    "30s",
		},
		Channels: []ChannelConfig{},
		Silence:  SilenceConfig{},
	}
}

type Manager struct {
	current atomic.Value
	path    string
}

func Load(path string) (*Manager, error) {
	m := &Manager{path: path}

	cfg := Default()
	if path != "" {
		if err := m.loadFile(cfg); err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	m.current.Store(cfg)
	return m, nil
}

func (c *Config) Validate() error {
	if c.Server.AdminToken == "" {
		return fmt.Errorf("server.admin_token is required and must not be empty")
	}
	if c.Server.AdminToken == "changeme" {
		return fmt.Errorf("server.admin_token must be changed from the default value 'changeme'")
	}
	return nil
}

func (m *Manager) loadFile(cfg *Config) error {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, cfg)
}

func (m *Manager) Get() *Config {
	return m.current.Load().(*Config)
}

func (m *Manager) Reload() error {
	cfg := Default()
	if err := m.loadFile(cfg); err != nil {
		return err
	}
	m.current.Store(cfg)
	return nil
}
