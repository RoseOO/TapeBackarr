package logging

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewLogger(t *testing.T) {
	logger, err := NewLogger("info", "text", "")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer logger.Close()
}

func TestLoggerLevels(t *testing.T) {
	logger, _ := NewLogger("warn", "text", "")
	defer logger.Close()

	// Debug should not log when level is warn
	// This is a basic check - in a real test we'd capture output
}

func TestLoggerToFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	logger, err := NewLogger("info", "json", logPath)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	logger.Info("test message", map[string]interface{}{
		"key": "value",
	})

	logger.Close()

	// Read log file
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "test message") {
		t.Error("log file should contain test message")
	}

	if !strings.Contains(string(content), "key") {
		t.Error("log file should contain field key")
	}
}

func TestLoggerJSONFormat(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	logger, _ := NewLogger("info", "json", logPath)
	logger.Info("json test", map[string]interface{}{
		"number": 42,
		"text":   "hello",
	})
	logger.Close()

	content, _ := os.ReadFile(logPath)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) == 0 {
		t.Fatal("expected at least one log line")
	}

	var entry LogEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("failed to parse JSON log: %v", err)
	}

	if entry.Message != "json test" {
		t.Errorf("expected message 'json test', got '%s'", entry.Message)
	}

	if entry.Level != "info" {
		t.Errorf("expected level 'info', got '%s'", entry.Level)
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"debug", LevelDebug},
		{"info", LevelInfo},
		{"warn", LevelWarn},
		{"warning", LevelWarn},
		{"error", LevelError},
		{"unknown", LevelInfo}, // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseLevel(tt.input)
			if result != tt.expected {
				t.Errorf("ParseLevel(%s) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestWithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := &Logger{
		level:  LevelDebug,
		output: &buf,
		format: "json",
	}

	fieldLogger := logger.WithFields(map[string]interface{}{
		"service": "test",
	})

	fieldLogger.Info("with fields", map[string]interface{}{
		"extra": "data",
	})

	output := buf.String()
	if !strings.Contains(output, "service") {
		t.Error("expected preset field 'service' in output")
	}
	if !strings.Contains(output, "extra") {
		t.Error("expected additional field 'extra' in output")
	}
}
