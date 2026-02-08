package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds all application configuration
type Config struct {
	Server        ServerConfig        `json:"server"`
	Database      DatabaseConfig      `json:"database"`
	Tape          TapeConfig          `json:"tape"`
	Logging       LoggingConfig       `json:"logging"`
	Auth          AuthConfig          `json:"auth"`
	Notifications NotificationsConfig `json:"notifications"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Path string `json:"path"`
}

// DriveConfig holds configuration for a single tape drive
type DriveConfig struct {
	DevicePath  string `json:"device_path"`
	DisplayName string `json:"display_name"`
	Enabled     bool   `json:"enabled"`
}

// TapeConfig holds tape-related configuration
type TapeConfig struct {
	DefaultDevice    string        `json:"default_device"`
	Drives           []DriveConfig `json:"drives,omitempty"`
	BufferSizeMB     int           `json:"buffer_size_mb"`
	BlockSize        int           `json:"block_size"`
	WriteRetries     int           `json:"write_retries"`
	VerifyAfterWrite bool          `json:"verify_after_write"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level      string `json:"level"`
	Format     string `json:"format"` // "json" or "text"
	OutputPath string `json:"output_path"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret       string `json:"jwt_secret"`
	TokenExpiration int    `json:"token_expiration"` // hours
	SessionTimeout  int    `json:"session_timeout"`  // minutes
}

// NotificationsConfig holds notification configuration
type NotificationsConfig struct {
	Telegram TelegramConfig `json:"telegram"`
}

// TelegramConfig holds Telegram bot configuration
type TelegramConfig struct {
	Enabled  bool   `json:"enabled"`
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Database: DatabaseConfig{
			Path: "/var/lib/tapebackarr/tapebackarr.db",
		},
		Tape: TapeConfig{
			DefaultDevice:    "/dev/nst0",
			Drives:           []DriveConfig{
				{DevicePath: "/dev/nst0", DisplayName: "Primary LTO Drive", Enabled: true},
			},
			BufferSizeMB:     256,
			BlockSize:        65536,
			WriteRetries:     3,
			VerifyAfterWrite: true,
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "json",
			OutputPath: "/var/log/tapebackarr/tapebackarr.log",
		},
		Auth: AuthConfig{
			JWTSecret:       "", // Must be set in config file
			TokenExpiration: 24,
			SessionTimeout:  60,
		},
		Notifications: NotificationsConfig{
			Telegram: TelegramConfig{
				Enabled:  false,
				BotToken: "",
				ChatID:   "",
			},
		},
	}
}

// Load loads configuration from a JSON file
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config if file doesn't exist
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Save saves the configuration to a JSON file
func (c *Config) Save(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}
