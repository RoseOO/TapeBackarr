package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// TelegramConfig holds Telegram bot configuration
type TelegramConfig struct {
	Enabled  bool   `json:"enabled"`
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}

// NotificationType defines the type of notification
type NotificationType string

const (
	NotifyTapeChange      NotificationType = "tape_change"
	NotifyTapeFull        NotificationType = "tape_full"
	NotifyBackupStart     NotificationType = "backup_start"
	NotifyBackupComplete  NotificationType = "backup_complete"
	NotifyBackupFailed    NotificationType = "backup_failed"
	NotifyRestoreStart    NotificationType = "restore_start"
	NotifyRestoreComplete NotificationType = "restore_complete"
	NotifyDriveError      NotificationType = "drive_error"
	NotifyWrongTape       NotificationType = "wrong_tape"
)

// Notification represents a notification to be sent
type Notification struct {
	Type      NotificationType       `json:"type"`
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	Priority  string                 `json:"priority"` // low, normal, high, urgent
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// TelegramService provides Telegram notification functionality
type TelegramService struct {
	config     TelegramConfig
	httpClient *http.Client
}

// NewTelegramService creates a new Telegram notification service
func NewTelegramService(config TelegramConfig) *TelegramService {
	return &TelegramService{
		config: config,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// IsEnabled returns true if Telegram notifications are enabled
func (s *TelegramService) IsEnabled() bool {
	return s.config.Enabled && s.config.BotToken != "" && s.config.ChatID != ""
}

// SendTestMessage sends a test notification via Telegram to verify the configuration
func (s *TelegramService) SendTestMessage(ctx context.Context) error {
	return s.Send(ctx, &Notification{
		Type:      "test",
		Title:     "Test Notification",
		Message:   "This is a test message from TapeBackarr. Your Telegram notifications are working correctly!",
		Priority:  "normal",
		Timestamp: time.Now(),
	})
}

// Send sends a notification via Telegram
func (s *TelegramService) Send(ctx context.Context, notification *Notification) error {
	if !s.IsEnabled() {
		return nil
	}

	// Format message with emoji based on type
	emoji := s.getEmoji(notification.Type, notification.Priority)
	formattedMessage := s.formatMessage(emoji, notification)

	return s.sendMessage(ctx, formattedMessage)
}

// getEmoji returns an appropriate emoji for the notification type
func (s *TelegramService) getEmoji(notifType NotificationType, priority string) string {
	switch notifType {
	case NotifyTapeChange:
		return "📼"
	case NotifyTapeFull:
		return "📀"
	case NotifyBackupStart:
		return "▶️"
	case NotifyBackupComplete:
		return "✅"
	case NotifyBackupFailed:
		return "❌"
	case NotifyRestoreStart:
		return "🔄"
	case NotifyRestoreComplete:
		return "✅"
	case NotifyDriveError:
		return "🚨"
	case NotifyWrongTape:
		return "⚠️"
	default:
		if priority == "urgent" || priority == "high" {
			return "🔴"
		}
		return "📢"
	}
}

// formatMessage formats a notification for Telegram
func (s *TelegramService) formatMessage(emoji string, notification *Notification) string {
	var buf bytes.Buffer

	// Header with emoji
	buf.WriteString(fmt.Sprintf("%s *%s*\n\n", emoji, escapeMarkdown(notification.Title)))

	// Message body
	buf.WriteString(escapeMarkdown(notification.Message))

	// Add data fields if present
	if len(notification.Data) > 0 {
		buf.WriteString("\n\n*Details:*\n")
		for key, value := range notification.Data {
			buf.WriteString(fmt.Sprintf("• %s: `%v`\n", escapeMarkdown(key), value))
		}
	}

	// Timestamp
	buf.WriteString(fmt.Sprintf("\n\n_Sent at %s_", escapeMarkdown(notification.Timestamp.Format("2006-01-02 15:04:05"))))

	return buf.String()
}

// escapeMarkdown escapes special characters for Telegram MarkdownV2
func escapeMarkdown(s string) string {
	specialChars := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
	result := s
	for _, char := range specialChars {
		result = replaceAll(result, char, "\\"+char)
	}
	return result
}

// replaceAll replaces all occurrences of old with new in s
func replaceAll(s, old, new string) string {
	return string(bytes.ReplaceAll([]byte(s), []byte(old), []byte(new)))
}

// telegramMessage represents a Telegram API message
type telegramMessage struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode"`
}

// sendMessage sends a message to Telegram
func (s *TelegramService) sendMessage(ctx context.Context, text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.config.BotToken)

	msg := telegramMessage{
		ChatID:    s.config.ChatID,
		Text:      text,
		ParseMode: "MarkdownV2",
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			OK          bool   `json:"ok"`
			Description string `json:"description"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return fmt.Errorf("telegram API error: %s", errResp.Description)
	}

	return nil
}

// NotifyTapeChangeRequired sends a tape change notification
func (s *TelegramService) NotifyTapeChangeRequired(ctx context.Context, jobName string, currentTape string, reason string, nextTape string) error {
	msg := fmt.Sprintf("Job '%s' requires a tape change.\n\nCurrent tape: %s\nReason: %s", jobName, currentTape, reason)
	if nextTape != "" {
		msg += fmt.Sprintf("\n\n📌 Next tape needed: %s", nextTape)
	}
	msg += "\n\nPlease insert the required tape and acknowledge in the web interface."

	data := map[string]interface{}{
		"Job":         jobName,
		"CurrentTape": currentTape,
		"Reason":      reason,
	}
	if nextTape != "" {
		data["NextTape"] = nextTape
	}

	return s.Send(ctx, &Notification{
		Type:      NotifyTapeChange,
		Title:     "Tape Change Required",
		Message:   msg,
		Priority:  "high",
		Timestamp: time.Now(),
		Data:      data,
	})
}

// NotifyTapeFull sends a tape full notification
func (s *TelegramService) NotifyTapeFull(ctx context.Context, tapeLabel string, usedBytes int64, jobName string, nextTape string) error {
	usedGB := float64(usedBytes) / (1024 * 1024 * 1024)
	msg := fmt.Sprintf("Tape '%s' is full (%.2f GB used).\n\nJob: %s", tapeLabel, usedGB, jobName)
	if nextTape != "" {
		msg += fmt.Sprintf("\n\n📌 Next tape needed: %s", nextTape)
	}
	msg += "\n\nPlease insert the required tape to continue."

	data := map[string]interface{}{
		"Tape":   tapeLabel,
		"UsedGB": fmt.Sprintf("%.2f", usedGB),
		"Job":    jobName,
	}
	if nextTape != "" {
		data["NextTape"] = nextTape
	}

	return s.Send(ctx, &Notification{
		Type:      NotifyTapeFull,
		Title:     "Tape Full",
		Message:   msg,
		Priority:  "urgent",
		Timestamp: time.Now(),
		Data:      data,
	})
}

// NotifyBackupStarted sends a backup start notification
func (s *TelegramService) NotifyBackupStarted(ctx context.Context, jobName string, sourceCount int, backupType string) error {
	return s.Send(ctx, &Notification{
		Type:      NotifyBackupStart,
		Title:     "Backup Started",
		Message:   fmt.Sprintf("Backup job '%s' has started.\n\nType: %s\nSources: %d", jobName, backupType, sourceCount),
		Priority:  "normal",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"Job":     jobName,
			"Type":    backupType,
			"Sources": sourceCount,
		},
	})
}

// NotifyBackupCompleted sends a backup completion notification
func (s *TelegramService) NotifyBackupCompleted(ctx context.Context, jobName string, fileCount int64, totalBytes int64, duration time.Duration) error {
	sizeGB := float64(totalBytes) / (1024 * 1024 * 1024)
	return s.Send(ctx, &Notification{
		Type:      NotifyBackupComplete,
		Title:     "Backup Completed",
		Message:   fmt.Sprintf("Backup job '%s' completed successfully.\n\nFiles: %d\nSize: %.2f GB\nDuration: %s", jobName, fileCount, sizeGB, duration.Round(time.Second)),
		Priority:  "normal",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"Job":      jobName,
			"Files":    fileCount,
			"SizeGB":   fmt.Sprintf("%.2f", sizeGB),
			"Duration": duration.Round(time.Second).String(),
		},
	})
}

// NotifyBackupFailed sends a backup failure notification
func (s *TelegramService) NotifyBackupFailed(ctx context.Context, jobName string, errorMsg string) error {
	return s.Send(ctx, &Notification{
		Type:      NotifyBackupFailed,
		Title:     "Backup Failed",
		Message:   fmt.Sprintf("Backup job '%s' failed!\n\nError: %s\n\nPlease check the logs and tape status.", jobName, errorMsg),
		Priority:  "urgent",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"Job":   jobName,
			"Error": errorMsg,
		},
	})
}

// NotifyDriveError sends a drive error notification
func (s *TelegramService) NotifyDriveError(ctx context.Context, devicePath string, errorMsg string) error {
	return s.Send(ctx, &Notification{
		Type:      NotifyDriveError,
		Title:     "Drive Error",
		Message:   fmt.Sprintf("Tape drive error detected!\n\nDevice: %s\nError: %s\n\nPlease check the drive status.", devicePath, errorMsg),
		Priority:  "urgent",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"Device": devicePath,
			"Error":  errorMsg,
		},
	})
}

// NotifyWrongTapeInserted sends a wrong tape notification
func (s *TelegramService) NotifyWrongTapeInserted(ctx context.Context, expectedLabel string, actualLabel string) error {
	return s.Send(ctx, &Notification{
		Type:      NotifyWrongTape,
		Title:     "Wrong Tape Inserted",
		Message:   fmt.Sprintf("The inserted tape does not match the expected tape.\n\nExpected: %s\nActual: %s\n\nPlease insert the correct tape.", expectedLabel, actualLabel),
		Priority:  "high",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"Expected": expectedLabel,
			"Actual":   actualLabel,
		},
	})
}

// CommandHandler is called when a Telegram command is received
type CommandHandler func(command string, args string) string

// RegisterCommands registers bot commands with Telegram's BotFather API
func (s *TelegramService) RegisterCommands(ctx context.Context) error {
	if !s.IsEnabled() {
		return nil
	}

	commands := []map[string]string{
		{"command": "status", "description": "Show current system status, loaded tape, and running jobs"},
		{"command": "jobs", "description": "List backup jobs and their status"},
		{"command": "tapes", "description": "List tapes and their status"},
		{"command": "drives", "description": "Show tape drive status"},
		{"command": "active", "description": "Show active/running backup operations"},
		{"command": "help", "description": "Show available commands"},
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/setMyCommands", s.config.BotToken)
	body, _ := json.Marshal(map[string]interface{}{"commands": commands})
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// StartCommandPolling starts polling for Telegram commands and dispatches to the handler
func (s *TelegramService) StartCommandPolling(ctx context.Context, handler CommandHandler) {
	if !s.IsEnabled() {
		return
	}

	go func() {
		offset := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			updates, err := s.getUpdates(ctx, offset, 30)
			if err != nil {
				time.Sleep(5 * time.Second)
				continue
			}

			for _, update := range updates {
				if update.Message != nil && update.Message.Text != "" {
					text := update.Message.Text
					// Parse command
					if len(text) > 0 && text[0] == '/' {
						parts := splitFirst(text[1:], " ")
						cmd := parts[0]
						args := ""
						if len(parts) > 1 {
							args = parts[1]
						}
						// Only respond if from the configured chat
						chatIDStr := fmt.Sprintf("%d", update.Message.Chat.ID)
						if chatIDStr == s.config.ChatID {
							response := handler(cmd, args)
							if response != "" {
								s.sendPlainMessage(ctx, response)
							}
						}
					}
				}
				offset = update.UpdateID + 1
			}
		}
	}()
}

type telegramUpdate struct {
	UpdateID int                      `json:"update_id"`
	Message  *telegramIncomingMessage `json:"message"`
}

type telegramIncomingMessage struct {
	Text string `json:"text"`
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
}

func (s *TelegramService) getUpdates(ctx context.Context, offset, timeout int) ([]telegramUpdate, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=%d", s.config.BotToken, offset, timeout)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool             `json:"ok"`
		Result []telegramUpdate `json:"result"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Result, nil
}

func (s *TelegramService) sendPlainMessage(ctx context.Context, text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.config.BotToken)
	msg := map[string]string{
		"chat_id": s.config.ChatID,
		"text":    text,
	}
	body, _ := json.Marshal(msg)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func splitFirst(s string, sep string) []string {
	idx := -1
	for i := 0; i < len(s); i++ {
		if string(s[i]) == sep {
			idx = i
			break
		}
	}
	if idx < 0 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}
