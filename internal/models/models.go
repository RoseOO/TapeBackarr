package models

import (
	"time"
)

// UserRole represents user permission levels
type UserRole string

const (
	RoleAdmin    UserRole = "admin"
	RoleOperator UserRole = "operator"
	RoleReadOnly UserRole = "readonly"
)

// User represents a system user for authentication
type User struct {
	ID           int64     `json:"id" db:"id"`
	Username     string    `json:"username" db:"username"`
	PasswordHash string    `json:"-" db:"password_hash"`
	Role         UserRole  `json:"role" db:"role"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// TapePool represents a group of tapes with similar policies
type TapePool struct {
	ID            int64     `json:"id" db:"id"`
	Name          string    `json:"name" db:"name"`
	Description   string    `json:"description" db:"description"`
	RetentionDays int       `json:"retention_days" db:"retention_days"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at"`
}

// TapeStatus represents the state of a tape
type TapeStatus string

const (
	TapeStatusBlank   TapeStatus = "blank"
	TapeStatusActive  TapeStatus = "active"
	TapeStatusFull    TapeStatus = "full"
	TapeStatusRetired TapeStatus = "retired"
	TapeStatusOffsite TapeStatus = "offsite"
)

// Tape represents a physical tape media
type Tape struct {
	ID              int64      `json:"id" db:"id"`
	Barcode         string     `json:"barcode" db:"barcode"`
	Label           string     `json:"label" db:"label"`
	PoolID          *int64     `json:"pool_id" db:"pool_id"`
	Status          TapeStatus `json:"status" db:"status"`
	CapacityBytes   int64      `json:"capacity_bytes" db:"capacity_bytes"`
	UsedBytes       int64      `json:"used_bytes" db:"used_bytes"`
	WriteCount      int        `json:"write_count" db:"write_count"`
	LastWrittenAt   *time.Time `json:"last_written_at" db:"last_written_at"`
	OffsiteLocation string     `json:"offsite_location" db:"offsite_location"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// DriveStatus represents the state of a tape drive
type DriveStatus string

const (
	DriveStatusReady   DriveStatus = "ready"
	DriveStatusBusy    DriveStatus = "busy"
	DriveStatusOffline DriveStatus = "offline"
	DriveStatusError   DriveStatus = "error"
)

// TapeDrive represents a physical tape drive
type TapeDrive struct {
	ID            int64       `json:"id" db:"id"`
	DevicePath    string      `json:"device_path" db:"device_path"`
	SerialNumber  string      `json:"serial_number" db:"serial_number"`
	Model         string      `json:"model" db:"model"`
	Status        DriveStatus `json:"status" db:"status"`
	CurrentTapeID *int64      `json:"current_tape_id" db:"current_tape_id"`
	CreatedAt     time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at" db:"updated_at"`
}

// SourceType represents the type of backup source
type SourceType string

const (
	SourceTypeLocal SourceType = "local"
	SourceTypeSMB   SourceType = "smb"
	SourceTypeNFS   SourceType = "nfs"
)

// BackupSource represents a configured backup source
type BackupSource struct {
	ID              int64      `json:"id" db:"id"`
	Name            string     `json:"name" db:"name"`
	SourceType      SourceType `json:"source_type" db:"source_type"`
	Path            string     `json:"path" db:"path"`
	IncludePatterns string     `json:"include_patterns" db:"include_patterns"` // JSON array
	ExcludePatterns string     `json:"exclude_patterns" db:"exclude_patterns"` // JSON array
	Enabled         bool       `json:"enabled" db:"enabled"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// BackupType represents the type of backup
type BackupType string

const (
	BackupTypeFull        BackupType = "full"
	BackupTypeIncremental BackupType = "incremental"
)

// BackupJob represents a scheduled backup job
type BackupJob struct {
	ID            int64      `json:"id" db:"id"`
	Name          string     `json:"name" db:"name"`
	SourceID      int64      `json:"source_id" db:"source_id"`
	PoolID        int64      `json:"pool_id" db:"pool_id"`
	BackupType    BackupType `json:"backup_type" db:"backup_type"`
	ScheduleCron  string     `json:"schedule_cron" db:"schedule_cron"`
	RetentionDays int        `json:"retention_days" db:"retention_days"`
	Enabled       bool       `json:"enabled" db:"enabled"`
	LastRunAt     *time.Time `json:"last_run_at" db:"last_run_at"`
	NextRunAt     *time.Time `json:"next_run_at" db:"next_run_at"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
}

// BackupSetStatus represents the status of a backup set
type BackupSetStatus string

const (
	BackupSetStatusPending   BackupSetStatus = "pending"
	BackupSetStatusRunning   BackupSetStatus = "running"
	BackupSetStatusCompleted BackupSetStatus = "completed"
	BackupSetStatusFailed    BackupSetStatus = "failed"
	BackupSetStatusCancelled BackupSetStatus = "cancelled"
)

// BackupSet represents a single backup operation
type BackupSet struct {
	ID          int64           `json:"id" db:"id"`
	JobID       int64           `json:"job_id" db:"job_id"`
	TapeID      int64           `json:"tape_id" db:"tape_id"`
	BackupType  BackupType      `json:"backup_type" db:"backup_type"`
	StartTime   time.Time       `json:"start_time" db:"start_time"`
	EndTime     *time.Time      `json:"end_time" db:"end_time"`
	Status      BackupSetStatus `json:"status" db:"status"`
	FileCount   int64           `json:"file_count" db:"file_count"`
	TotalBytes  int64           `json:"total_bytes" db:"total_bytes"`
	StartBlock  int64           `json:"start_block" db:"start_block"`
	EndBlock    int64           `json:"end_block" db:"end_block"`
	Checksum    string          `json:"checksum" db:"checksum"`
	ParentSetID *int64          `json:"parent_set_id" db:"parent_set_id"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

// CatalogEntry represents a file in the backup catalog
type CatalogEntry struct {
	ID          int64     `json:"id" db:"id"`
	BackupSetID int64     `json:"backup_set_id" db:"backup_set_id"`
	FilePath    string    `json:"file_path" db:"file_path"`
	FileSize    int64     `json:"file_size" db:"file_size"`
	FileMode    int       `json:"file_mode" db:"file_mode"`
	ModTime     time.Time `json:"mod_time" db:"mod_time"`
	Checksum    string    `json:"checksum" db:"checksum"`
	BlockOffset int64     `json:"block_offset" db:"block_offset"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// ExecutionStatus represents the status of a job execution
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusCancelled ExecutionStatus = "cancelled"
	ExecutionStatusPaused    ExecutionStatus = "paused"
)

// JobExecution represents a single execution of a backup job
type JobExecution struct {
	ID             int64           `json:"id" db:"id"`
	JobID          int64           `json:"job_id" db:"job_id"`
	BackupSetID    *int64          `json:"backup_set_id" db:"backup_set_id"`
	Status         ExecutionStatus `json:"status" db:"status"`
	StartTime      *time.Time      `json:"start_time" db:"start_time"`
	EndTime        *time.Time      `json:"end_time" db:"end_time"`
	FilesProcessed int64           `json:"files_processed" db:"files_processed"`
	BytesProcessed int64           `json:"bytes_processed" db:"bytes_processed"`
	ErrorMessage   string          `json:"error_message" db:"error_message"`
	CanResume      bool            `json:"can_resume" db:"can_resume"`
	ResumeState    string          `json:"resume_state" db:"resume_state"` // JSON
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at" db:"updated_at"`
}

// AuditLog represents an audit trail entry
type AuditLog struct {
	ID           int64     `json:"id" db:"id"`
	UserID       *int64    `json:"user_id" db:"user_id"`
	Action       string    `json:"action" db:"action"`
	ResourceType string    `json:"resource_type" db:"resource_type"`
	ResourceID   *int64    `json:"resource_id" db:"resource_id"`
	Details      string    `json:"details" db:"details"` // JSON
	IPAddress    string    `json:"ip_address" db:"ip_address"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// Snapshot represents a filesystem snapshot for incremental backups
type Snapshot struct {
	ID           int64     `json:"id" db:"id"`
	SourceID     int64     `json:"source_id" db:"source_id"`
	BackupSetID  *int64    `json:"backup_set_id" db:"backup_set_id"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	FileCount    int64     `json:"file_count" db:"file_count"`
	TotalBytes   int64     `json:"total_bytes" db:"total_bytes"`
	SnapshotData []byte    `json:"snapshot_data" db:"snapshot_data"`
}
