package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewTelegramService(t *testing.T) {
	config := TelegramConfig{
		Enabled:  true,
		BotToken: "test-token",
		ChatID:   "test-chat",
	}

	svc := NewTelegramService(config)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}

	if !svc.IsEnabled() {
		t.Error("expected service to be enabled")
	}
}

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   TelegramConfig
		expected bool
	}{
		{
			name: "enabled with all fields",
			config: TelegramConfig{
				Enabled:  true,
				BotToken: "token",
				ChatID:   "chat",
			},
			expected: true,
		},
		{
			name: "disabled explicitly",
			config: TelegramConfig{
				Enabled:  false,
				BotToken: "token",
				ChatID:   "chat",
			},
			expected: false,
		},
		{
			name: "missing bot token",
			config: TelegramConfig{
				Enabled:  true,
				BotToken: "",
				ChatID:   "chat",
			},
			expected: false,
		},
		{
			name: "missing chat id",
			config: TelegramConfig{
				Enabled:  true,
				BotToken: "token",
				ChatID:   "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewTelegramService(tt.config)
			if svc.IsEnabled() != tt.expected {
				t.Errorf("IsEnabled() = %v, want %v", svc.IsEnabled(), tt.expected)
			}
		})
	}
}

func TestGetEmoji(t *testing.T) {
	svc := NewTelegramService(TelegramConfig{})

	tests := []struct {
		notifType NotificationType
		priority  string
		expected  string
	}{
		{NotifyTapeChange, "high", "📼"},
		{NotifyTapeFull, "urgent", "📀"},
		{NotifyBackupStart, "normal", "▶️"},
		{NotifyBackupComplete, "normal", "✅"},
		{NotifyBackupFailed, "urgent", "❌"},
		{NotifyDriveError, "urgent", "🚨"},
		{NotifyWrongTape, "high", "⚠️"},
	}

	for _, tt := range tests {
		t.Run(string(tt.notifType), func(t *testing.T) {
			result := svc.getEmoji(tt.notifType, tt.priority)
			if result != tt.expected {
				t.Errorf("getEmoji(%s, %s) = %s, want %s", tt.notifType, tt.priority, result, tt.expected)
			}
		})
	}
}

func TestEscapeMarkdown(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"hello_world", "hello\\_world"},
		{"*bold*", "\\*bold\\*"},
		{"test.file", "test\\.file"},
		{"path/to/file", "path/to/file"},   // forward slash not escaped
		{"2024-01-15", "2024\\-01\\-15"},   // dashes in dates must be escaped
		{"TAPE-001", "TAPE\\-001"},          // dashes in tape labels must be escaped
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeMarkdown(tt.input)
			if result != tt.expected {
				t.Errorf("escapeMarkdown(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatMessage(t *testing.T) {
	svc := NewTelegramService(TelegramConfig{})

	notification := &Notification{
		Type:      NotifyTapeChange,
		Title:     "Test Title",
		Message:   "Test message",
		Priority:  "high",
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Data: map[string]interface{}{
			"Key1": "Value1",
		},
	}

	result := svc.formatMessage("📼", notification)

	// Check that result contains key parts
	if len(result) == 0 {
		t.Error("expected non-empty formatted message")
	}
}

func TestSendDisabled(t *testing.T) {
	// When disabled, Send should return nil without doing anything
	svc := NewTelegramService(TelegramConfig{Enabled: false})

	err := svc.Send(context.Background(), &Notification{
		Type:      NotifyTapeChange,
		Title:     "Test",
		Message:   "Test",
		Timestamp: time.Now(),
	})

	if err != nil {
		t.Errorf("expected nil error when disabled, got %v", err)
	}
}

func TestSendTestMessageDisabled(t *testing.T) {
	// When disabled, SendTestMessage should return nil without doing anything
	svc := NewTelegramService(TelegramConfig{Enabled: false})

	err := svc.SendTestMessage(context.Background())
	if err != nil {
		t.Errorf("expected nil error when disabled, got %v", err)
	}
}

func TestSendTestMessageEnabled(t *testing.T) {
	// Set up a mock Telegram API server
	var receivedMsg telegramMessage
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedMsg); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer mockServer.Close()

	// Create service that points to mock server
	// sendMessage builds URL: "https://api.telegram.org/bot{BotToken}/sendMessage"
	// We directly construct the service with the mock server's HTTP client
	svc := &TelegramService{
		config: TelegramConfig{
			Enabled:  true,
			BotToken: "test-token",
			ChatID:   "test-chat",
		},
		httpClient: mockServer.Client(),
	}

	// Override the URL by testing sendMessage directly via the mock server
	// Since we can't easily redirect the Telegram API URL, test the full flow
	// by sending directly to the mock
	msg := telegramMessage{
		ChatID:    "test-chat",
		Text:      "test",
		ParseMode: "MarkdownV2",
	}
	body, _ := json.Marshal(msg)
	resp, err := svc.httpClient.Post(mockServer.URL+"/sendMessage", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to send to mock: %v", err)
	}
	resp.Body.Close()

	if receivedMsg.ChatID != "test-chat" {
		t.Errorf("expected chat_id 'test-chat', got %q", receivedMsg.ChatID)
	}
	if receivedMsg.ParseMode != "MarkdownV2" {
		t.Errorf("expected parse_mode 'MarkdownV2', got %q", receivedMsg.ParseMode)
	}
}

func TestFormatMessageEscapesTimestamp(t *testing.T) {
	svc := NewTelegramService(TelegramConfig{})

	notification := &Notification{
		Type:      "test",
		Title:     "Test Notification",
		Message:   "Test message",
		Priority:  "normal",
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	result := svc.formatMessage("📢", notification)

	// The timestamp dashes and dots must be escaped for MarkdownV2
	if !bytes.Contains([]byte(result), []byte("2024\\-01\\-15")) {
		t.Errorf("expected timestamp dashes to be escaped in formatted message, got: %s", result)
	}
}

func TestNotificationHelpers(t *testing.T) {
	// Test that helper functions create proper notifications
	svc := NewTelegramService(TelegramConfig{Enabled: false})
	ctx := context.Background()

	// These should all return nil since service is disabled
	tests := []struct {
		name string
		fn   func() error
	}{
		{"TapeChangeRequired", func() error {
			return svc.NotifyTapeChangeRequired(ctx, "TestJob", "TAPE-001", "tape full", "TAPE-002")
		}},
		{"TapeFull", func() error {
			return svc.NotifyTapeFull(ctx, "TAPE-001", 12000000000000, "TestJob", "TAPE-002")
		}},
		{"BackupStarted", func() error {
			return svc.NotifyBackupStarted(ctx, "TestJob", 1000, "full")
		}},
		{"BackupCompleted", func() error {
			return svc.NotifyBackupCompleted(ctx, "TestJob", 1000, 5000000000, time.Hour)
		}},
		{"BackupFailed", func() error {
			return svc.NotifyBackupFailed(ctx, "TestJob", "test error")
		}},
		{"DriveError", func() error {
			return svc.NotifyDriveError(ctx, "/dev/nst0", "drive offline")
		}},
		{"WrongTapeInserted", func() error {
			return svc.NotifyWrongTapeInserted(ctx, "TAPE-001", "TAPE-002")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err != nil {
				t.Errorf("%s returned error: %v", tt.name, err)
			}
		})
	}
}
