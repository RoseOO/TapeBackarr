package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Level represents log severity
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "unknown"
	}
}

// ParseLevel converts a string to a Level
func ParseLevel(s string) Level {
	switch s {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// Logger provides structured logging
type Logger struct {
	mu       sync.Mutex
	level    Level
	output   io.Writer
	format   string // "json" or "text"
	file     *os.File
	filePath string
}

// NewLogger creates a new logger
func NewLogger(level string, format string, outputPath string) (*Logger, error) {
	l := &Logger{
		level:  ParseLevel(level),
		format: format,
		output: os.Stdout,
	}

	if outputPath != "" && outputPath != "-" {
		// Ensure directory exists
		dir := filepath.Dir(outputPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}

		f, err := os.OpenFile(outputPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}

		l.file = f
		l.filePath = outputPath
		// Write to both stdout and file
		l.output = io.MultiWriter(os.Stdout, f)
	}

	return l, nil
}

// Close closes the log file
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// log writes a log entry
func (l *Logger) log(level Level, message string, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level.String(),
		Message:   message,
		Fields:    fields,
	}

	var output string
	if l.format == "json" {
		data, _ := json.Marshal(entry)
		output = string(data)
	} else {
		output = fmt.Sprintf("%s [%s] %s", entry.Timestamp.Format(time.RFC3339), entry.Level, entry.Message)
		if len(fields) > 0 {
			for k, v := range fields {
				output += fmt.Sprintf(" %s=%v", k, v)
			}
		}
	}

	fmt.Fprintln(l.output, output)
}

// Debug logs a debug message
func (l *Logger) Debug(message string, fields map[string]interface{}) {
	l.log(LevelDebug, message, fields)
}

// Info logs an info message
func (l *Logger) Info(message string, fields map[string]interface{}) {
	l.log(LevelInfo, message, fields)
}

// Warn logs a warning message
func (l *Logger) Warn(message string, fields map[string]interface{}) {
	l.log(LevelWarn, message, fields)
}

// Error logs an error message
func (l *Logger) Error(message string, fields map[string]interface{}) {
	l.log(LevelError, message, fields)
}

// WithFields returns a child logger with default fields
func (l *Logger) WithFields(fields map[string]interface{}) *FieldLogger {
	return &FieldLogger{
		logger: l,
		fields: fields,
	}
}

// FieldLogger is a logger with preset fields
type FieldLogger struct {
	logger *Logger
	fields map[string]interface{}
}

func (fl *FieldLogger) mergeFields(additional map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})
	for k, v := range fl.fields {
		merged[k] = v
	}
	for k, v := range additional {
		merged[k] = v
	}
	return merged
}

// Debug logs a debug message
func (fl *FieldLogger) Debug(message string, fields map[string]interface{}) {
	fl.logger.log(LevelDebug, message, fl.mergeFields(fields))
}

// Info logs an info message
func (fl *FieldLogger) Info(message string, fields map[string]interface{}) {
	fl.logger.log(LevelInfo, message, fl.mergeFields(fields))
}

// Warn logs a warning message
func (fl *FieldLogger) Warn(message string, fields map[string]interface{}) {
	fl.logger.log(LevelWarn, message, fl.mergeFields(fields))
}

// Error logs an error message
func (fl *FieldLogger) Error(message string, fields map[string]interface{}) {
	fl.logger.log(LevelError, message, fl.mergeFields(fields))
}

// AuditLogger logs audit events to database
type AuditLogger struct {
	db interface {
		Exec(query string, args ...interface{}) (interface{}, error)
	}
	logger *Logger
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(db interface {
	Exec(query string, args ...interface{}) (interface{}, error)
}, logger *Logger) *AuditLogger {
	return &AuditLogger{
		db:     db,
		logger: logger,
	}
}

// Log logs an audit event
func (al *AuditLogger) Log(userID *int64, action, resourceType string, resourceID *int64, details, ipAddress string) error {
	detailsJSON, _ := json.Marshal(map[string]interface{}{
		"details": details,
	})

	_, err := al.db.Exec(`
		INSERT INTO audit_logs (user_id, action, resource_type, resource_id, details, ip_address)
		VALUES (?, ?, ?, ?, ?, ?)
	`, userID, action, resourceType, resourceID, string(detailsJSON), ipAddress)

	if err != nil {
		al.logger.Error("Failed to write audit log", map[string]interface{}{
			"error":         err.Error(),
			"action":        action,
			"resource_type": resourceType,
		})
		return err
	}

	al.logger.Debug("Audit log written", map[string]interface{}{
		"action":        action,
		"resource_type": resourceType,
		"resource_id":   resourceID,
	})

	return nil
}
