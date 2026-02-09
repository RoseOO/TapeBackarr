package proxmox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/RoseOO/TapeBackarr/internal/cmdutil"
	"github.com/RoseOO/TapeBackarr/internal/database"
	"github.com/RoseOO/TapeBackarr/internal/logging"
	"github.com/RoseOO/TapeBackarr/internal/tape"
)

// RestoreRequest represents a request to restore a Proxmox backup
type RestoreRequest struct {
	BackupID   int64  `json:"backup_id"`
	TargetNode string `json:"target_node"`           // Node to restore to (can be different from original)
	TargetVMID int    `json:"target_vmid,omitempty"` // New VMID (0 = use original)
	TargetName string `json:"target_name,omitempty"` // New name (empty = use original)
	Storage    string `json:"storage"`               // Target storage for disks
	StartAfter bool   `json:"start_after"`           // Start the guest after restore
	Overwrite  bool   `json:"overwrite"`             // Overwrite if VMID exists
	RestoreRAM bool   `json:"restore_ram"`           // Restore RAM state (if available)
	DriveID    *int64 `json:"drive_id,omitempty"`    // Tape drive to use for restore
}

// RestoreResult represents the result of a restore operation
type RestoreResult struct {
	RestoreID     int64     `json:"restore_id"`
	BackupID      int64     `json:"backup_id"`
	SourceNode    string    `json:"source_node"`
	TargetNode    string    `json:"target_node"`
	SourceVMID    int       `json:"source_vmid"`
	TargetVMID    int       `json:"target_vmid"`
	GuestType     GuestType `json:"guest_type"`
	GuestName     string    `json:"guest_name"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	TotalBytes    int64     `json:"total_bytes"`
	Status        string    `json:"status"`
	Error         string    `json:"error,omitempty"`
	ConfigApplied bool      `json:"config_applied"`
}

// RestoreService handles Proxmox restore operations
type RestoreService struct {
	client      *Client
	db          *database.DB
	tapeService *tape.Service
	logger      *logging.Logger
	blockSize   int
	tmpDir      string
}

// NewRestoreService creates a new Proxmox restore service
func NewRestoreService(client *Client, db *database.DB, tapeService *tape.Service, logger *logging.Logger, blockSize int) *RestoreService {
	return &RestoreService{
		client:      client,
		db:          db,
		tapeService: tapeService,
		logger:      logger,
		blockSize:   blockSize,
		tmpDir:      "/var/lib/tapebackarr/proxmox-tmp",
	}
}

// SetTempDir sets the temporary directory for restore operations
func (s *RestoreService) SetTempDir(dir string) {
	s.tmpDir = dir
}

// RestoreGuest restores a Proxmox VM or LXC from tape
func (s *RestoreService) RestoreGuest(ctx context.Context, req *RestoreRequest) (*RestoreResult, error) {
	startTime := time.Now()
	result := &RestoreResult{
		BackupID:  req.BackupID,
		StartTime: startTime,
		Status:    "running",
	}

	// Get backup details from database
	var backup struct {
		Node       string
		VMID       int
		GuestType  GuestType
		GuestName  string
		TapeID     int64
		TotalBytes int64
		ConfigData []byte
	}

	err := s.db.QueryRow(`
		SELECT node, vmid, guest_type, guest_name, tape_id, total_bytes, config_data
		FROM proxmox_backups
		WHERE id = ?
	`, req.BackupID).Scan(&backup.Node, &backup.VMID, &backup.GuestType,
		&backup.GuestName, &backup.TapeID, &backup.TotalBytes, &backup.ConfigData)
	if err != nil {
		result.Status = "failed"
		result.Error = "backup not found"
		return result, fmt.Errorf("backup not found: %w", err)
	}

	result.SourceNode = backup.Node
	result.SourceVMID = backup.VMID
	result.GuestType = backup.GuestType
	result.GuestName = backup.GuestName
	result.TotalBytes = backup.TotalBytes

	// Set target node and VMID
	if req.TargetNode == "" {
		req.TargetNode = backup.Node
	}
	result.TargetNode = req.TargetNode

	if req.TargetVMID == 0 {
		req.TargetVMID = backup.VMID
	}
	result.TargetVMID = req.TargetVMID

	s.logger.Info("Starting Proxmox restore", map[string]interface{}{
		"backup_id":   req.BackupID,
		"source_node": backup.Node,
		"target_node": req.TargetNode,
		"source_vmid": backup.VMID,
		"target_vmid": req.TargetVMID,
		"guest_type":  backup.GuestType,
	})

	// Check if tape is loaded - use explicit drive if provided
	var devicePath string
	if req.DriveID != nil {
		err = s.db.QueryRow(
			"SELECT device_path FROM tape_drives WHERE id = ? AND enabled = 1",
			*req.DriveID,
		).Scan(&devicePath)
		if err != nil {
			result.Status = "failed"
			result.Error = "drive not found or not enabled"
			return result, fmt.Errorf("drive not found or not enabled: %w", err)
		}
	} else {
		err = s.db.QueryRow(`
			SELECT device_path FROM tape_drives WHERE current_tape_id = ?
		`, backup.TapeID).Scan(&devicePath)
		if err != nil {
			result.Status = "failed"
			result.Error = "required tape not loaded"
			return result, fmt.Errorf("tape not loaded: %w", err)
		}
	}

	// Create a drive-specific tape service for all tape operations
	driveSvc := tape.NewServiceForDevice(devicePath, s.blockSize)

	// Wait for tape to be physically ready
	if err := driveSvc.WaitForTape(ctx, 30*time.Second); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("tape not ready: %v", err)
		return result, err
	}

	// Verify the correct tape is loaded by reading its label
	var expectedLabel string
	if err := s.db.QueryRow("SELECT label FROM tapes WHERE id = ?", backup.TapeID).Scan(&expectedLabel); err == nil && expectedLabel != "" {
		s.logger.Info("Verifying tape label", map[string]interface{}{
			"expected_label": expectedLabel,
			"device_path":    devicePath,
		})
		label, err := driveSvc.ReadTapeLabel(ctx)
		if err != nil {
			result.Status = "failed"
			result.Error = fmt.Sprintf("failed to read tape label: %v", err)
			return result, err
		}
		if label == nil || label.Label != expectedLabel {
			actualLabel := ""
			if label != nil {
				actualLabel = label.Label
			}
			result.Status = "failed"
			result.Error = fmt.Sprintf("wrong tape loaded: expected %s, got %s", expectedLabel, actualLabel)
			return result, fmt.Errorf("wrong tape loaded: expected %s, got %s", expectedLabel, actualLabel)
		}
		s.logger.Info("Correct tape verified", map[string]interface{}{
			"label": expectedLabel,
		})
	}

	// Create restore record
	dbResult, err := s.db.Exec(`
		INSERT INTO proxmox_restores (
			backup_id, source_node, target_node, source_vmid, target_vmid,
			guest_type, guest_name, status, start_time
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.BackupID, backup.Node, req.TargetNode, backup.VMID, req.TargetVMID,
		backup.GuestType, backup.GuestName, "running", startTime)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to create restore record: %v", err)
		return result, err
	}
	restoreID, _ := dbResult.LastInsertId()
	result.RestoreID = restoreID

	// Ensure temp directory exists
	if err := os.MkdirAll(s.tmpDir, 0755); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to create temp dir: %v", err)
		s.updateRestoreStatus(restoreID, "failed", result.Error)
		return result, err
	}

	// Position tape past the label to the backup data
	if err := driveSvc.SeekToFileNumber(ctx, 1); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to seek past tape label: %v", err)
		s.updateRestoreStatus(restoreID, "failed", result.Error)
		return result, err
	}

	// Extract backup from tape to temp directory
	tmpBackupPath := filepath.Join(s.tmpDir, fmt.Sprintf("restore-%d-%d", req.BackupID, time.Now().UnixNano()))
	if err := os.MkdirAll(tmpBackupPath, 0755); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to create temp backup dir: %v", err)
		s.updateRestoreStatus(restoreID, "failed", result.Error)
		return result, err
	}
	defer os.RemoveAll(tmpBackupPath)

	// Extract from tape
	if err := s.extractFromTape(ctx, devicePath, tmpBackupPath); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to extract from tape: %v", err)
		s.updateRestoreStatus(restoreID, "failed", result.Error)
		return result, err
	}

	// Find the vzdump archive in extracted files
	backupFile, err := s.findBackupFile(tmpBackupPath)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("backup file not found: %v", err)
		s.updateRestoreStatus(restoreID, "failed", result.Error)
		return result, err
	}

	// Perform the restore using qmrestore or pct restore
	if err := s.performRestore(ctx, req, backup.GuestType, backupFile); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		s.updateRestoreStatus(restoreID, "failed", result.Error)
		return result, err
	}

	// Apply saved configuration if available
	if len(backup.ConfigData) > 0 {
		var config map[string]interface{}
		if err := json.Unmarshal(backup.ConfigData, &config); err == nil {
			if err := s.applyConfig(ctx, req.TargetNode, req.TargetVMID, backup.GuestType, config); err != nil {
				s.logger.Warn("Failed to apply config", map[string]interface{}{"error": err.Error()})
			} else {
				result.ConfigApplied = true
			}
		}
	}

	// Start the guest if requested
	if req.StartAfter {
		if backup.GuestType == GuestTypeVM {
			s.client.StartVM(ctx, req.TargetNode, req.TargetVMID)
		} else {
			s.client.StartLXC(ctx, req.TargetNode, req.TargetVMID)
		}
	}

	result.EndTime = time.Now()
	result.Status = "completed"
	s.updateRestoreStatus(restoreID, "completed", "")

	s.logger.Info("Proxmox restore completed", map[string]interface{}{
		"restore_id":  restoreID,
		"target_vmid": req.TargetVMID,
		"duration":    result.EndTime.Sub(startTime).String(),
	})

	return result, nil
}

// extractFromTape extracts the backup archive from tape
func (s *RestoreService) extractFromTape(ctx context.Context, devicePath, destPath string) error {
	// First, skip the metadata file mark and extract metadata
	// Then extract the actual backup data

	// Extract to destination
	tarArgs := []string{
		"-x",
		"-b", fmt.Sprintf("%d", s.blockSize/512),
		"-f", devicePath,
		"-C", destPath,
	}

	cmd := exec.CommandContext(ctx, "tar", tarArgs...)
	var tarStderr bytes.Buffer
	cmd.Stderr = &tarStderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tar extract failed (%s)", cmdutil.ErrorDetail(err, &tarStderr))
	}

	return nil
}

// findBackupFile locates the vzdump archive in the extracted files
func (s *RestoreService) findBackupFile(path string) (string, error) {
	var backupFile string

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		// Look for vzdump files (typically .vma for QEMU, .tar for LXC)
		name := info.Name()
		if filepath.Ext(name) == ".vma" || filepath.Ext(name) == ".tar" ||
			filepath.Ext(name) == ".zst" || filepath.Ext(name) == ".lzo" ||
			filepath.Ext(name) == ".gz" {
			backupFile = filePath
			return filepath.SkipAll
		}
		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return "", err
	}
	if backupFile == "" {
		return "", fmt.Errorf("no backup archive found in extracted files")
	}

	return backupFile, nil
}

// performRestore executes qmrestore or pct restore
func (s *RestoreService) performRestore(ctx context.Context, req *RestoreRequest, guestType GuestType, backupFile string) error {
	var cmd *exec.Cmd

	if guestType == GuestTypeVM {
		// Use qmrestore for VMs
		args := []string{
			backupFile,
			fmt.Sprintf("%d", req.TargetVMID),
		}
		if req.Storage != "" {
			args = append(args, "--storage", req.Storage)
		}
		if req.Overwrite {
			args = append(args, "--force", "1")
		}
		cmd = exec.CommandContext(ctx, "qmrestore", args...)
	} else {
		// Use pct restore for LXC
		args := []string{
			"restore",
			fmt.Sprintf("%d", req.TargetVMID),
			backupFile,
		}
		if req.Storage != "" {
			args = append(args, "--storage", req.Storage)
		}
		if req.Overwrite {
			args = append(args, "--force", "1")
		}
		cmd = exec.CommandContext(ctx, "pct", args...)
	}

	var cmdStderr bytes.Buffer
	cmd.Stderr = &cmdStderr
	if err := cmd.Run(); err != nil {
		cmdName := "qmrestore"
		if guestType != GuestTypeVM {
			cmdName = "pct restore"
		}
		return fmt.Errorf("%s failed (%s)", cmdName, cmdutil.ErrorDetail(err, &cmdStderr))
	}

	return nil
}

// applyConfig applies saved configuration to a restored guest
func (s *RestoreService) applyConfig(ctx context.Context, node string, vmid int, guestType GuestType, config map[string]interface{}) error {
	// For now, we just log that we would apply the config
	// Full implementation would use the Proxmox API to set each config option
	s.logger.Info("Would apply saved configuration", map[string]interface{}{
		"node":   node,
		"vmid":   vmid,
		"config": config,
	})
	return nil
}

// updateRestoreStatus updates the status of a restore in the database
func (s *RestoreService) updateRestoreStatus(restoreID int64, status, errorMsg string) {
	if status == "completed" {
		s.db.Exec(`
			UPDATE proxmox_restores 
			SET status = ?, end_time = CURRENT_TIMESTAMP
			WHERE id = ?
		`, status, restoreID)
	} else {
		s.db.Exec(`
			UPDATE proxmox_restores 
			SET status = ?, error_message = ?, end_time = CURRENT_TIMESTAMP
			WHERE id = ?
		`, status, errorMsg, restoreID)
	}
}

// GetRequiredTapes returns the tapes needed for a restore operation
func (s *RestoreService) GetRequiredTapes(ctx context.Context, backupID int64) ([]TapeRequirement, error) {
	var tapeID int64
	var tapeBarcode, tapeLabel, tapeStatus string
	var totalBytes int64

	err := s.db.QueryRow(`
		SELECT pb.tape_id, t.barcode, t.label, t.status, pb.total_bytes
		FROM proxmox_backups pb
		JOIN tapes t ON pb.tape_id = t.id
		WHERE pb.id = ?
	`, backupID).Scan(&tapeID, &tapeBarcode, &tapeLabel, &tapeStatus, &totalBytes)
	if err != nil {
		return nil, fmt.Errorf("backup not found: %w", err)
	}

	return []TapeRequirement{
		{
			TapeID:     tapeID,
			Barcode:    tapeBarcode,
			Label:      tapeLabel,
			Status:     tapeStatus,
			TotalBytes: totalBytes,
			Order:      1,
		},
	}, nil
}

// TapeRequirement describes a tape needed for restore
type TapeRequirement struct {
	TapeID     int64  `json:"tape_id"`
	Barcode    string `json:"barcode"`
	Label      string `json:"label"`
	Status     string `json:"status"`
	TotalBytes int64  `json:"total_bytes"`
	Order      int    `json:"order"`
}

// ListRestores returns all Proxmox restores from the database
func (s *RestoreService) ListRestores(ctx context.Context, limit int) ([]RestoreResult, error) {
	rows, err := s.db.Query(`
		SELECT id, backup_id, source_node, target_node, source_vmid, target_vmid,
			   guest_type, guest_name, start_time, end_time, status, error_message
		FROM proxmox_restores
		ORDER BY start_time DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var restores []RestoreResult
	for rows.Next() {
		var r RestoreResult
		var endTime *time.Time
		var errorMsg *string
		if err := rows.Scan(&r.RestoreID, &r.BackupID, &r.SourceNode, &r.TargetNode,
			&r.SourceVMID, &r.TargetVMID, &r.GuestType, &r.GuestName,
			&r.StartTime, &endTime, &r.Status, &errorMsg); err != nil {
			continue
		}
		if endTime != nil {
			r.EndTime = *endTime
		}
		if errorMsg != nil {
			r.Error = *errorMsg
		}
		restores = append(restores, r)
	}

	return restores, nil
}

// StreamRestoreFromReader restores from an io.Reader (for tape)
func (s *RestoreService) StreamRestoreFromReader(ctx context.Context, r io.Reader, destPath string) error {
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}

	cmd := exec.CommandContext(ctx, "tar", "-x", "-C", destPath)
	cmd.Stdin = r

	var tarStderr bytes.Buffer
	cmd.Stderr = &tarStderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tar extract failed (%s)", cmdutil.ErrorDetail(err, &tarStderr))
	}

	return nil
}
