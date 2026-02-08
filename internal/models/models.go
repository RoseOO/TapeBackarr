package models

import (
	"strings"
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
	ID               int64     `json:"id" db:"id"`
	Name             string    `json:"name" db:"name"`
	Description      string    `json:"description" db:"description"`
	RetentionDays    int       `json:"retention_days" db:"retention_days"`
	AllowReuse       bool      `json:"allow_reuse" db:"allow_reuse"`
	AllocationPolicy string    `json:"allocation_policy" db:"allocation_policy"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

// TapeStatus represents the state of a tape
type TapeStatus string

const (
	TapeStatusBlank    TapeStatus = "blank"
	TapeStatusActive   TapeStatus = "active"
	TapeStatusFull     TapeStatus = "full"
	TapeStatusExpired  TapeStatus = "expired"
	TapeStatusRetired  TapeStatus = "retired"
	TapeStatusExported TapeStatus = "exported"
)

// LTOCapacities maps LTO generation to native capacity in bytes
var LTOCapacities = map[string]int64{
	"LTO-1":  100000000000,   // 100 GB
	"LTO-2":  200000000000,   // 200 GB
	"LTO-3":  400000000000,   // 400 GB
	"LTO-4":  800000000000,   // 800 GB
	"LTO-5":  1500000000000,  // 1.5 TB
	"LTO-6":  2500000000000,  // 2.5 TB
	"LTO-7":  6000000000000,  // 6 TB
	"LTO-8":  12000000000000, // 12 TB
	"LTO-9":  18000000000000, // 18 TB
	"LTO-10": 36000000000000, // 36 TB (expected)
}

// DensityToLTOType maps SCSI density codes to LTO generation strings
var DensityToLTOType = map[string]string{
	"0x40": "LTO-1",
	"0x42": "LTO-2",
	"0x44": "LTO-3",
	"0x46": "LTO-4",
	"0x58": "LTO-5",
	"0x5a": "LTO-6",
	"0x5c": "LTO-7",
	"0x5d": "LTO-7",  // LTO-7 Type M (M8)
	"0x5e": "LTO-8",
	"0x60": "LTO-9",
	"0x62": "LTO-10",
}

// LTOTypeFromDensity returns the LTO type for a given density code.
// The density code should be a hex string like "0x58".
// Returns the LTO type string and true if found, or empty string and false.
func LTOTypeFromDensity(densityCode string) (string, bool) {
	ltoType, ok := DensityToLTOType[strings.ToLower(densityCode)]
	return ltoType, ok
}

// UnknownTapeInfo represents a tape loaded in a drive that is not in the database
type UnknownTapeInfo struct {
	Label     string `json:"label"`
	UUID      string `json:"uuid"`
	Pool      string `json:"pool"`
	Timestamp int64  `json:"timestamp"`
}

// Tape represents a physical tape media
type Tape struct {
	ID              int64      `json:"id" db:"id"`
	UUID            string     `json:"uuid" db:"uuid"`
	Barcode         string     `json:"barcode" db:"barcode"`
	Label           string     `json:"label" db:"label"`
	LTOType         string     `json:"lto_type" db:"lto_type"`
	PoolID          *int64     `json:"pool_id" db:"pool_id"`
	Status          TapeStatus `json:"status" db:"status"`
	CapacityBytes   int64      `json:"capacity_bytes" db:"capacity_bytes"`
	UsedBytes       int64      `json:"used_bytes" db:"used_bytes"`
	WriteCount      int        `json:"write_count" db:"write_count"`
	LastWrittenAt   *time.Time `json:"last_written_at" db:"last_written_at"`
	OffsiteLocation string     `json:"offsite_location" db:"offsite_location"`
	ExportTime      *time.Time `json:"export_time" db:"export_time"`
	ImportTime      *time.Time `json:"import_time" db:"import_time"`
	LabeledAt       *time.Time `json:"labeled_at" db:"labeled_at"`
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
	DisplayName   string      `json:"display_name" db:"display_name"`
	Vendor        string      `json:"vendor" db:"vendor"`
	SerialNumber  string      `json:"serial_number" db:"serial_number"`
	Model         string      `json:"model" db:"model"`
	Status        DriveStatus `json:"status" db:"status"`
	CurrentTapeID *int64      `json:"current_tape_id" db:"current_tape_id"`
	CurrentTape   string           `json:"current_tape" db:"-"`
	UnknownTape   *UnknownTapeInfo `json:"unknown_tape,omitempty" db:"-"`
	Enabled       bool             `json:"enabled" db:"enabled"`
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
	ID                int64      `json:"id" db:"id"`
	Name              string     `json:"name" db:"name"`
	SourceID          int64      `json:"source_id" db:"source_id"`
	PoolID            int64      `json:"pool_id" db:"pool_id"`
	BackupType        BackupType `json:"backup_type" db:"backup_type"`
	ScheduleCron      string     `json:"schedule_cron" db:"schedule_cron"`
	RetentionDays     int        `json:"retention_days" db:"retention_days"`
	Enabled           bool       `json:"enabled" db:"enabled"`
	EncryptionEnabled bool       `json:"encryption_enabled" db:"encryption_enabled"`
	EncryptionKeyID   *int64     `json:"encryption_key_id" db:"encryption_key_id"`
	LastRunAt         *time.Time `json:"last_run_at" db:"last_run_at"`
	NextRunAt         *time.Time `json:"next_run_at" db:"next_run_at"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
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
	ID              int64           `json:"id" db:"id"`
	JobID           int64           `json:"job_id" db:"job_id"`
	TapeID          int64           `json:"tape_id" db:"tape_id"`
	BackupType      BackupType      `json:"backup_type" db:"backup_type"`
	StartTime       time.Time       `json:"start_time" db:"start_time"`
	EndTime         *time.Time      `json:"end_time" db:"end_time"`
	Status          BackupSetStatus `json:"status" db:"status"`
	FileCount       int64           `json:"file_count" db:"file_count"`
	TotalBytes      int64           `json:"total_bytes" db:"total_bytes"`
	StartBlock      int64           `json:"start_block" db:"start_block"`
	EndBlock        int64           `json:"end_block" db:"end_block"`
	Checksum        string          `json:"checksum" db:"checksum"`
	Encrypted       bool            `json:"encrypted" db:"encrypted"`
	EncryptionKeyID *int64          `json:"encryption_key_id" db:"encryption_key_id"`
	ParentSetID     *int64          `json:"parent_set_id" db:"parent_set_id"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at" db:"updated_at"`
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

// TapeSpanningSet represents a backup that spans multiple tapes
type TapeSpanningSet struct {
	ID          int64     `json:"id" db:"id"`
	BackupSetID int64     `json:"backup_set_id" db:"backup_set_id"`
	TotalTapes  int       `json:"total_tapes" db:"total_tapes"`
	TotalBytes  int64     `json:"total_bytes" db:"total_bytes"`
	Status      string    `json:"status" db:"status"` // in_progress, completed, failed
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// TapeSpanningMember represents a single tape in a spanning set
type TapeSpanningMember struct {
	ID              int64     `json:"id" db:"id"`
	SpanningSetID   int64     `json:"spanning_set_id" db:"spanning_set_id"`
	TapeID          int64     `json:"tape_id" db:"tape_id"`
	SequenceNumber  int       `json:"sequence_number" db:"sequence_number"`
	StartBlock      int64     `json:"start_block" db:"start_block"`
	EndBlock        int64     `json:"end_block" db:"end_block"`
	BytesWritten    int64     `json:"bytes_written" db:"bytes_written"`
	FilesStartIndex int64     `json:"files_start_index" db:"files_start_index"`
	FilesEndIndex   int64     `json:"files_end_index" db:"files_end_index"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

// TapeChangeRequest represents a pending tape change request
type TapeChangeRequest struct {
	ID             int64      `json:"id" db:"id"`
	JobExecutionID int64      `json:"job_execution_id" db:"job_execution_id"`
	SpanningSetID  *int64     `json:"spanning_set_id" db:"spanning_set_id"`
	CurrentTapeID  int64      `json:"current_tape_id" db:"current_tape_id"`
	Reason         string     `json:"reason" db:"reason"` // tape_full, tape_error
	Status         string     `json:"status" db:"status"` // pending, acknowledged, completed, cancelled
	RequestedAt    time.Time  `json:"requested_at" db:"requested_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at" db:"acknowledged_at"`
	NewTapeID      *int64     `json:"new_tape_id" db:"new_tape_id"`
}

// DatabaseBackup represents a backup of the TapeBackarr database to tape
type DatabaseBackup struct {
	ID           int64     `json:"id" db:"id"`
	TapeID       int64     `json:"tape_id" db:"tape_id"`
	BackupTime   time.Time `json:"backup_time" db:"backup_time"`
	FileSize     int64     `json:"file_size" db:"file_size"`
	Checksum     string    `json:"checksum" db:"checksum"`
	BlockOffset  int64     `json:"block_offset" db:"block_offset"`
	Status       string    `json:"status" db:"status"` // pending, completed, failed
	ErrorMessage string    `json:"error_message,omitempty" db:"error_message"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// RestoreDestinationType represents the type of restore destination
type RestoreDestinationType string

const (
	RestoreDestLocal RestoreDestinationType = "local"
	RestoreDestSMB   RestoreDestinationType = "smb"
	RestoreDestNFS   RestoreDestinationType = "nfs"
)

// RestoreOperation represents a tracked restore operation
type RestoreOperation struct {
	ID              int64                  `json:"id" db:"id"`
	BackupSetID     *int64                 `json:"backup_set_id" db:"backup_set_id"`
	DestinationType RestoreDestinationType `json:"destination_type" db:"destination_type"`
	DestinationPath string                 `json:"destination_path" db:"destination_path"`
	FilesRequested  int64                  `json:"files_requested" db:"files_requested"`
	FilesRestored   int64                  `json:"files_restored" db:"files_restored"`
	BytesRestored   int64                  `json:"bytes_restored" db:"bytes_restored"`
	Status          string                 `json:"status" db:"status"` // pending, running, completed, failed
	ErrorMessage    string                 `json:"error_message,omitempty" db:"error_message"`
	VerifyEnabled   bool                   `json:"verify_enabled" db:"verify_enabled"`
	VerifyPassed    *bool                  `json:"verify_passed" db:"verify_passed"`
	StartedAt       *time.Time             `json:"started_at" db:"started_at"`
	CompletedAt     *time.Time             `json:"completed_at" db:"completed_at"`
	CreatedAt       time.Time              `json:"created_at" db:"created_at"`
}
