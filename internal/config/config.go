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
	Proxmox       ProxmoxConfig       `json:"proxmox,omitempty"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	StaticDir string `json:"static_dir"`
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
	PipelineDepthMB  int           `json:"pipeline_depth_mb"`
	WriteRetries     int           `json:"write_retries"`
	VerifyAfterWrite bool          `json:"verify_after_write"`
	// LTFS enables the Linear Tape File System format for tape operations.
	// When enabled, tapes are formatted with LTFS and files are written as a
	// standard POSIX filesystem instead of tar archives. This makes each tape
	// self-describing and readable with any LTFS-compatible tool.
	// Requires LTO-5 or later drives and LTFS software (mkltfs, ltfs).
	EnableLTFS     bool   `json:"enable_ltfs"`
	LTFSMountPoint string `json:"ltfs_mount_point,omitempty"`
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
	Email    EmailConfig    `json:"email"`
}

// TelegramConfig holds Telegram bot configuration
type TelegramConfig struct {
	Enabled  bool   `json:"enabled"`
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

// EmailConfig holds SMTP email configuration
type EmailConfig struct {
	Enabled    bool   `json:"enabled"`
	SMTPHost   string `json:"smtp_host"`
	SMTPPort   int    `json:"smtp_port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	FromEmail  string `json:"from_email"`
	FromName   string `json:"from_name"`
	ToEmails   string `json:"to_emails"` // Comma-separated list
	UseTLS     bool   `json:"use_tls"`
	SkipVerify bool   `json:"skip_verify"`
}

// ProxmoxConfig holds Proxmox VE connection configuration
type ProxmoxConfig struct {
	Enabled       bool   `json:"enabled"`
	Host          string `json:"host"`            // Proxmox host/IP
	Port          int    `json:"port"`            // API port (default 8006)
	SkipTLSVerify bool   `json:"skip_tls_verify"` // Skip SSL certificate verification
	// Auth option 1: Username/Password
	Username string `json:"username,omitempty"` // e.g., "root"
	Password string `json:"password,omitempty"` // User password
	Realm    string `json:"realm,omitempty"`    // e.g., "pam" or "pve"
	// Auth option 2: API Token (recommended)
	TokenID     string `json:"token_id,omitempty"`     // Format: user@realm!tokenname
	TokenSecret string `json:"token_secret,omitempty"` // API token secret
	// Backup settings
	DefaultMode     string `json:"default_mode"`     // snapshot, suspend, or stop
	DefaultCompress string `json:"default_compress"` // zstd, lzo, gzip, or empty
	TempDir         string `json:"temp_dir"`         // Temp directory for backup operations
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:      "0.0.0.0",
			Port:      8080,
			StaticDir: "/opt/tapebackarr/static",
		},
		Database: DatabaseConfig{
			Path: "/var/lib/tapebackarr/tapebackarr.db",
		},
		Tape: TapeConfig{
			DefaultDevice: "/dev/nst0",
			Drives: []DriveConfig{
				{DevicePath: "/dev/nst0", DisplayName: "Primary LTO Drive", Enabled: true},
			},
			BufferSizeMB:     2048,
			BlockSize:        1048576,
			PipelineDepthMB:  64,
			WriteRetries:     3,
			VerifyAfterWrite: true,
			EnableLTFS:       false,
			LTFSMountPoint:   "/mnt/ltfs",
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
			Email: EmailConfig{
				Enabled:    false,
				SMTPHost:   "",
				SMTPPort:   587,
				Username:   "",
				Password:   "",
				FromEmail:  "",
				FromName:   "TapeBackarr",
				ToEmails:   "",
				UseTLS:     true,
				SkipVerify: false,
			},
		},
		Proxmox: ProxmoxConfig{
			Enabled:         false,
			Host:            "",
			Port:            8006,
			SkipTLSVerify:   true,
			Realm:           "pam",
			DefaultMode:     "snapshot",
			DefaultCompress: "zstd",
			TempDir:         "/var/lib/tapebackarr/proxmox-tmp",
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
