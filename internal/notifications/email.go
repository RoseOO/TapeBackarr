package notifications

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

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

// EmailService provides email notification functionality
type EmailService struct {
	config EmailConfig
}

// NewEmailService creates a new email notification service
func NewEmailService(config EmailConfig) *EmailService {
	if config.SMTPPort == 0 {
		config.SMTPPort = 587 // Default SMTP port
	}
	if config.FromName == "" {
		config.FromName = "TapeBackarr"
	}
	return &EmailService{
		config: config,
	}
}

// IsEnabled returns true if email notifications are enabled
func (s *EmailService) IsEnabled() bool {
	return s.config.Enabled && s.config.SMTPHost != "" && s.config.ToEmails != ""
}

// Send sends a notification via email
func (s *EmailService) Send(ctx context.Context, notification *Notification) error {
	if !s.IsEnabled() {
		return nil
	}

	subject := s.formatSubject(notification)
	body := s.formatBody(notification)

	return s.sendEmail(ctx, subject, body)
}

// formatSubject formats the email subject
func (s *EmailService) formatSubject(notification *Notification) string {
	prefix := "[TapeBackarr]"
	switch notification.Priority {
	case "urgent":
		prefix = "[TapeBackarr] 🚨 URGENT:"
	case "high":
		prefix = "[TapeBackarr] ⚠️"
	}
	return fmt.Sprintf("%s %s", prefix, notification.Title)
}

// formatBody formats the email body as HTML
func (s *EmailService) formatBody(notification *Notification) string {
	var buf bytes.Buffer

	// HTML header
	buf.WriteString(`<!DOCTYPE html>
<html>
<head>
<style>
body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
.container { max-width: 600px; margin: 0 auto; padding: 20px; }
.header { background-color: #2c3e50; color: white; padding: 15px; border-radius: 5px 5px 0 0; }
.content { background-color: #f9f9f9; padding: 20px; border: 1px solid #ddd; }
.details { background-color: #fff; padding: 15px; margin-top: 15px; border-left: 4px solid #3498db; }
.footer { font-size: 12px; color: #666; margin-top: 20px; padding-top: 10px; border-top: 1px solid #ddd; }
.urgent { border-left-color: #e74c3c; }
.high { border-left-color: #f39c12; }
.success { border-left-color: #27ae60; }
</style>
</head>
<body>
<div class="container">
`)

	// Header with priority color
	headerColor := "#2c3e50"
	switch notification.Priority {
	case "urgent":
		headerColor = "#e74c3c"
	case "high":
		headerColor = "#f39c12"
	}
	buf.WriteString(fmt.Sprintf(`<div class="header" style="background-color: %s;">
<h2 style="margin: 0;">%s</h2>
</div>
`, headerColor, escapeHTML(notification.Title)))

	// Content
	buf.WriteString(`<div class="content">`)
	buf.WriteString(fmt.Sprintf("<p>%s</p>", escapeHTML(notification.Message)))

	// Details section
	if len(notification.Data) > 0 {
		detailsClass := "details"
		switch notification.Priority {
		case "urgent":
			detailsClass = "details urgent"
		case "high":
			detailsClass = "details high"
		}
		if notification.Type == NotifyBackupComplete || notification.Type == NotifyRestoreComplete {
			detailsClass = "details success"
		}

		buf.WriteString(fmt.Sprintf(`<div class="%s">
<h3 style="margin-top: 0;">Details</h3>
<table style="width: 100%%;">
`, detailsClass))

		for key, value := range notification.Data {
			buf.WriteString(fmt.Sprintf(`<tr>
<td style="font-weight: bold; padding: 5px 10px 5px 0;">%s:</td>
<td style="padding: 5px 0;">%v</td>
</tr>
`, escapeHTML(key), value))
		}

		buf.WriteString(`</table>
</div>`)
	}

	buf.WriteString(`</div>`)

	// Footer
	buf.WriteString(fmt.Sprintf(`<div class="footer">
<p>This notification was sent by TapeBackarr at %s</p>
<p>Access the web interface to manage your tape backup system.</p>
</div>
</div>
</body>
</html>
`, notification.Timestamp.Format("2006-01-02 15:04:05 MST")))

	return buf.String()
}

// escapeHTML escapes special HTML characters
func escapeHTML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}

// sendEmail sends an email via SMTP
func (s *EmailService) sendEmail(ctx context.Context, subject, body string) error {
	// Parse recipients
	recipients := strings.Split(s.config.ToEmails, ",")
	for i, r := range recipients {
		recipients[i] = strings.TrimSpace(r)
	}

	// Build email message
	from := s.config.FromEmail
	if s.config.FromName != "" {
		from = fmt.Sprintf("%s <%s>", s.config.FromName, s.config.FromEmail)
	}

	var msg bytes.Buffer
	msg.WriteString(fmt.Sprintf("From: %s\r\n", from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(recipients, ", ")))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	// SMTP address
	addr := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)

	// Create auth if credentials provided
	var auth smtp.Auth
	if s.config.Username != "" && s.config.Password != "" {
		auth = smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.SMTPHost)
	}

	// Send email
	if s.config.UseTLS {
		return s.sendEmailTLS(addr, auth, s.config.FromEmail, recipients, msg.Bytes())
	}

	return smtp.SendMail(addr, auth, s.config.FromEmail, recipients, msg.Bytes())
}

// sendEmailTLS sends email using TLS connection
func (s *EmailService) sendEmailTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	// TLS configuration
	tlsConfig := &tls.Config{
		ServerName:         s.config.SMTPHost,
		InsecureSkipVerify: s.config.SkipVerify,
	}

	// Connect to SMTP server
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	// Create SMTP client
	client, err := smtp.NewClient(conn, s.config.SMTPHost)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// Authenticate if credentials provided
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}

	// Set sender
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Set recipients
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", recipient, err)
		}
	}

	// Send message body
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %w", err)
	}

	if _, err := writer.Write(msg); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	return client.Quit()
}

// NotifyTapeChangeRequired sends a tape change notification via email
func (s *EmailService) NotifyTapeChangeRequired(ctx context.Context, jobName string, currentTape string, reason string) error {
	return s.Send(ctx, &Notification{
		Type:      NotifyTapeChange,
		Title:     "Tape Change Required",
		Message:   fmt.Sprintf("Job '%s' requires a tape change. Current tape: %s. Reason: %s. Please insert a new tape and acknowledge in the web interface.", jobName, currentTape, reason),
		Priority:  "high",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"Job":          jobName,
			"Current Tape": currentTape,
			"Reason":       reason,
		},
	})
}

// NotifyBackupCompleted sends a backup completion notification via email
func (s *EmailService) NotifyBackupCompleted(ctx context.Context, jobName string, fileCount int64, totalBytes int64, duration time.Duration) error {
	sizeGB := float64(totalBytes) / (1024 * 1024 * 1024)
	return s.Send(ctx, &Notification{
		Type:      NotifyBackupComplete,
		Title:     "Backup Completed Successfully",
		Message:   fmt.Sprintf("Backup job '%s' completed successfully.", jobName),
		Priority:  "normal",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"Job":      jobName,
			"Files":    fileCount,
			"Size":     fmt.Sprintf("%.2f GB", sizeGB),
			"Duration": duration.Round(time.Second).String(),
		},
	})
}

// NotifyBackupFailed sends a backup failure notification via email
func (s *EmailService) NotifyBackupFailed(ctx context.Context, jobName string, errorMsg string) error {
	return s.Send(ctx, &Notification{
		Type:      NotifyBackupFailed,
		Title:     "Backup Failed",
		Message:   fmt.Sprintf("Backup job '%s' failed! Please check the logs and tape status.", jobName),
		Priority:  "urgent",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"Job":   jobName,
			"Error": errorMsg,
		},
	})
}

// NotifyDriveError sends a drive error notification via email
func (s *EmailService) NotifyDriveError(ctx context.Context, devicePath string, errorMsg string) error {
	return s.Send(ctx, &Notification{
		Type:      NotifyDriveError,
		Title:     "Tape Drive Error",
		Message:   fmt.Sprintf("A tape drive error has been detected on device %s. Please check the drive status.", devicePath),
		Priority:  "urgent",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"Device": devicePath,
			"Error":  errorMsg,
		},
	})
}
