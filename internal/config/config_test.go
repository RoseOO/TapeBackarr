package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}

	if cfg.Server.StaticDir != "/opt/tapebackarr/static" {
		t.Errorf("expected static_dir /opt/tapebackarr/static, got %s", cfg.Server.StaticDir)
	}

	if cfg.Tape.DefaultDevice != "/dev/nst0" {
		t.Errorf("expected device /dev/nst0, got %s", cfg.Tape.DefaultDevice)
	}

	if cfg.Tape.BlockSize != 1048576 {
		t.Errorf("expected block size 1048576, got %d", cfg.Tape.BlockSize)
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	cfg, err := Load("/non/existent/path.json")
	if err != nil {
		t.Fatalf("expected no error for non-existent file, got %v", err)
	}

	// Should return default config
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Create config
	cfg := DefaultConfig()
	cfg.Server.Port = 9999
	cfg.Auth.JWTSecret = "test-secret"

	// Save
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	// Load
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Server.Port != 9999 {
		t.Errorf("expected port 9999, got %d", loaded.Server.Port)
	}

	if loaded.Auth.JWTSecret != "test-secret" {
		t.Errorf("expected jwt secret 'test-secret', got %s", loaded.Auth.JWTSecret)
	}
}

func TestDefaultConfigLTFSFields(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Tape.EnableLTFS != false {
		t.Error("expected EnableLTFS to default to false")
	}
	if cfg.Tape.LTFSMountPoint != "/mnt/ltfs" {
		t.Errorf("expected LTFSMountPoint /mnt/ltfs, got %s", cfg.Tape.LTFSMountPoint)
	}
}

func TestSaveAndLoadLTFSConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := DefaultConfig()
	cfg.Tape.EnableLTFS = true
	cfg.Tape.LTFSMountPoint = "/mnt/custom-ltfs"

	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if !loaded.Tape.EnableLTFS {
		t.Error("expected EnableLTFS to be true after load")
	}
	if loaded.Tape.LTFSMountPoint != "/mnt/custom-ltfs" {
		t.Errorf("expected LTFSMountPoint /mnt/custom-ltfs, got %s", loaded.Tape.LTFSMountPoint)
	}
}
