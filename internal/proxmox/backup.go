package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/RoseOO/TapeBackarr/internal/database"
	"github.com/RoseOO/TapeBackarr/internal/logging"
	"github.com/RoseOO/TapeBackarr/internal/tape"
)

// GuestType represents the type of Proxmox guest
type GuestType string

const (
	GuestTypeVM  GuestType = "qemu"
	GuestTypeLXC GuestType = "lxc"
)

// BackupMode represents the vzdump backup mode
type BackupMode string

const (
	BackupModeSnapshot BackupMode = "snapshot"
	BackupModeSuspend  BackupMode = "suspend"
	BackupModeStop     BackupMode = "stop"
)

// ProxmoxBackupRequest represents a request to backup a Proxmox guest
type ProxmoxBackupRequest struct {
	Node       string     `json:"node"`
	VMID       int        `json:"vmid"`
	GuestType  GuestType  `json:"guest_type"`
	GuestName  string     `json:"guest_name"`
	BackupMode BackupMode `json:"backup_mode"`
	Compress   string     `json:"compress"` // zstd, lzo, gzip, or empty
	TapeID     int64      `json:"tape_id"`
	Notes      string     `json:"notes,omitempty"`
}

// ProxmoxBackupResult represents the result of a Proxmox backup
type ProxmoxBackupResult struct {
	BackupID    int64     `json:"backup_id"`
	Node        string    `json:"node"`
	VMID        int       `json:"vmid"`
	GuestType   GuestType `json:"guest_type"`
	GuestName   string    `json:"guest_name"`
	TapeID      int64     `json:"tape_id"`
	TapeBarcode string    `json:"tape_barcode"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	TotalBytes  int64     `json:"total_bytes"`
	Status      string    `json:"status"`
	ConfigSaved bool      `json:"config_saved"`
	Error       string    `json:"error,omitempty"`
}

// ProxmoxBackupMetadata stores metadata about a Proxmox backup for restore
type ProxmoxBackupMetadata struct {
	BackupID       int64                  `json:"backup_id"`
	BackupTime     time.Time              `json:"backup_time"`
	Node           string                 `json:"node"`
	VMID           int                    `json:"vmid"`
	GuestType      GuestType              `json:"guest_type"`
	GuestName      string                 `json:"guest_name"`
	BackupMode     BackupMode             `json:"backup_mode"`
	Compress       string                 `json:"compress"`
	TotalBytes     int64                  `json:"total_bytes"`
	VMConfig       map[string]interface{} `json:"vm_config,omitempty"`
	LXCConfig      map[string]interface{} `json:"lxc_config,omitempty"`
	TapeBlockStart int64                  `json:"tape_block_start"`
	TapeBlockEnd   int64                  `json:"tape_block_end"`
	Notes          string                 `json:"notes,omitempty"`
}

// BackupService handles Proxmox backup operations
type BackupService struct {
	client      *Client
	db          *database.DB
	tapeService *tape.Service
	logger      *logging.Logger
	blockSize   int
	tmpDir      string // Temporary directory for vzdump output before streaming
}

// NewBackupService creates a new Proxmox backup service
func NewBackupService(client *Client, db *database.DB, tapeService *tape.Service, logger *logging.Logger, blockSize int) *BackupService {
	return &BackupService{
		client:      client,
		db:          db,
		tapeService: tapeService,
		logger:      logger,
		blockSize:   blockSize,
		tmpDir:      "/var/lib/tapebackarr/proxmox-tmp",
	}
}

// SetTempDir sets the temporary directory for backup operations
func (s *BackupService) SetTempDir(dir string) {
	s.tmpDir = dir
}

// BackupGuest performs a backup of a VM or LXC container to tape
func (s *BackupService) BackupGuest(ctx context.Context, req *ProxmoxBackupRequest) (*ProxmoxBackupResult, error) {
	startTime := time.Now()
	result := &ProxmoxBackupResult{
		Node:      req.Node,
		VMID:      req.VMID,
		GuestType: req.GuestType,
		GuestName: req.GuestName,
		TapeID:    req.TapeID,
		StartTime: startTime,
		Status:    "running",
	}

	s.logger.Info("Starting Proxmox backup", map[string]interface{}{
		"node":       req.Node,
		"vmid":       req.VMID,
		"guest_type": req.GuestType,
		"guest_name": req.GuestName,
		"mode":       req.BackupMode,
	})

	// Get tape device and barcode
	var devicePath, tapeBarcode string
	err := s.db.QueryRow(`
		SELECT td.device_path, t.barcode 
		FROM tape_drives td 
		JOIN tapes t ON td.current_tape_id = t.id 
		WHERE td.current_tape_id = ?
	`, req.TapeID).Scan(&devicePath, &tapeBarcode)
	if err != nil {
		result.Status = "failed"
		result.Error = "tape not loaded in any drive"
		return result, fmt.Errorf("tape not loaded: %w", err)
	}
	result.TapeBarcode = tapeBarcode

	// Create database record for the backup
	dbResult, err := s.db.Exec(`
		INSERT INTO proxmox_backups (
			node, vmid, guest_type, guest_name, tape_id, backup_mode, 
			compress, status, start_time, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.Node, req.VMID, req.GuestType, req.GuestName, req.TapeID,
		req.BackupMode, req.Compress, "running", startTime, req.Notes)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to create backup record: %v", err)
		return result, err
	}
	backupID, _ := dbResult.LastInsertId()
	result.BackupID = backupID

	// Get guest configuration
	var configData map[string]interface{}
	if req.GuestType == GuestTypeVM {
		vmConfig, err := s.client.GetVMConfig(ctx, req.Node, req.VMID)
		if err != nil {
			s.logger.Warn("Failed to get VM config", map[string]interface{}{"error": err.Error()})
		} else {
			configData = vmConfig.RawConfig
			result.ConfigSaved = true
		}
	} else {
		lxcConfig, err := s.client.GetLXCConfig(ctx, req.Node, req.VMID)
		if err != nil {
			s.logger.Warn("Failed to get LXC config", map[string]interface{}{"error": err.Error()})
		} else {
			configData = lxcConfig.RawConfig
			result.ConfigSaved = true
		}
	}

	// Ensure temp directory exists
	if err := os.MkdirAll(s.tmpDir, 0755); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to create temp dir: %v", err)
		s.updateBackupStatus(backupID, "failed", result.Error, 0)
		return result, err
	}

	// Start vzdump backup
	vzdumpOptions := map[string]string{
		"mode":     string(req.BackupMode),
		"storage":  "", // We handle storage ourselves
		"stdout":   "1", // Output to stdout for streaming
	}
	if req.Compress != "" {
		vzdumpOptions["compress"] = req.Compress
	}

	// For tape backup, we need to capture vzdump output and stream to tape
	// vzdump doesn't support direct tape output, so we use a named pipe
	pipePath := filepath.Join(s.tmpDir, fmt.Sprintf("vzdump-pipe-%d-%d", req.VMID, time.Now().UnixNano()))
	
	// Create metadata for this backup
	metadata := &ProxmoxBackupMetadata{
		BackupID:   backupID,
		BackupTime: startTime,
		Node:       req.Node,
		VMID:       req.VMID,
		GuestType:  req.GuestType,
		GuestName:  req.GuestName,
		BackupMode: req.BackupMode,
		Compress:   req.Compress,
		Notes:      req.Notes,
	}
	if req.GuestType == GuestTypeVM {
		metadata.VMConfig = configData
	} else {
		metadata.LXCConfig = configData
	}

	// Write metadata to tape first
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to marshal metadata: %v", err)
		s.updateBackupStatus(backupID, "failed", result.Error, 0)
		return result, err
	}

	if err := s.writeMetadataToTape(ctx, devicePath, metadataBytes); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("failed to write metadata to tape: %v", err)
		s.updateBackupStatus(backupID, "failed", result.Error, 0)
		return result, err
	}

	// Execute vzdump and stream to tape
	totalBytes, err := s.executeVzdumpToTape(ctx, req, devicePath, pipePath)
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		s.updateBackupStatus(backupID, "failed", result.Error, 0)
		return result, err
	}

	result.TotalBytes = totalBytes
	result.EndTime = time.Now()
	result.Status = "completed"

	// Write file mark to separate this backup
	if err := s.tapeService.WriteFileMark(ctx); err != nil {
		s.logger.Warn("Failed to write file mark", map[string]interface{}{"error": err.Error()})
	}

	// Update database record
	s.updateBackupStatus(backupID, "completed", "", totalBytes)

	// Save config to database
	if len(configData) > 0 {
		configJSON, _ := json.Marshal(configData)
		s.db.Exec(`UPDATE proxmox_backups SET config_data = ? WHERE id = ?`, configJSON, backupID)
	}

	// Update tape usage
	s.db.Exec(`
		UPDATE tapes SET 
			used_bytes = used_bytes + ?, 
			write_count = write_count + 1,
			last_written_at = CURRENT_TIMESTAMP,
			status = CASE WHEN status = 'blank' THEN 'active' ELSE status END
		WHERE id = ?
	`, totalBytes, req.TapeID)

	s.logger.Info("Proxmox backup completed", map[string]interface{}{
		"backup_id":   backupID,
		"vmid":        req.VMID,
		"total_bytes": totalBytes,
		"duration":    result.EndTime.Sub(startTime).String(),
	})

	return result, nil
}

// executeVzdumpToTape runs vzdump and streams output to tape
func (s *BackupService) executeVzdumpToTape(ctx context.Context, req *ProxmoxBackupRequest, devicePath, pipePath string) (int64, error) {
	// Build vzdump command
	// vzdump outputs to stdout when using --stdout
	args := []string{
		fmt.Sprintf("%d", req.VMID),
		"--mode", string(req.BackupMode),
		"--stdout",
	}
	if req.Compress != "" {
		args = append(args, "--compress", req.Compress)
	}

	// For VM snapshots, we may need additional options
	if req.GuestType == GuestTypeVM && req.BackupMode == BackupModeSnapshot {
		// Use QEMU guest agent if available for consistent snapshots
		args = append(args, "--quiet")
	}

	s.logger.Info("Executing vzdump", map[string]interface{}{
		"vmid": req.VMID,
		"args": strings.Join(args, " "),
	})

	// Create vzdump command
	vzdumpCmd := exec.CommandContext(ctx, "vzdump", args...)
	vzdumpStdout, err := vzdumpCmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to create vzdump stdout pipe: %w", err)
	}

	// Create tar command to write to tape
	// We wrap the vzdump output in a tar archive for consistency with other backups
	tarArgs := []string{
		"-c",
		"-b", fmt.Sprintf("%d", s.blockSize/512),
		"-f", devicePath,
		"--label", fmt.Sprintf("proxmox-%s-%d-%s", req.GuestType, req.VMID, time.Now().Format("20060102-150405")),
		"-",
	}
	tarCmd := exec.CommandContext(ctx, "tar", tarArgs...)
	tarCmd.Stdin = vzdumpStdout

	// Start both commands
	if err := vzdumpCmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start vzdump: %w", err)
	}

	if err := tarCmd.Start(); err != nil {
		vzdumpCmd.Process.Kill()
		return 0, fmt.Errorf("failed to start tar: %w", err)
	}

	// Wait for vzdump to complete
	vzdumpErr := vzdumpCmd.Wait()
	
	// Close the pipe and wait for tar
	tarErr := tarCmd.Wait()

	if vzdumpErr != nil {
		return 0, fmt.Errorf("vzdump failed: %w", vzdumpErr)
	}
	if tarErr != nil {
		return 0, fmt.Errorf("tar to tape failed: %w", tarErr)
	}

	// Get approximate bytes written (vzdump doesn't report exact size)
	// We'll estimate based on file info or process stats
	return s.estimateBackupSize(req)
}

// estimateBackupSize estimates the backup size for a guest
func (s *BackupService) estimateBackupSize(req *ProxmoxBackupRequest) (int64, error) {
	// Query the guest for disk usage
	if req.GuestType == GuestTypeVM {
		vms, err := s.client.GetNodeVMs(context.Background(), req.Node)
		if err != nil {
			return 0, err
		}
		for _, vm := range vms {
			if vm.VMID == req.VMID {
				return vm.Disk, nil
			}
		}
	} else {
		lxcs, err := s.client.GetNodeLXCs(context.Background(), req.Node)
		if err != nil {
			return 0, err
		}
		for _, lxc := range lxcs {
			if lxc.VMID == req.VMID {
				return lxc.Disk, nil
			}
		}
	}
	return 0, nil
}

// writeMetadataToTape writes backup metadata as a separate tar archive
func (s *BackupService) writeMetadataToTape(ctx context.Context, devicePath string, metadata []byte) error {
	// Create a temporary file for metadata
	tmpFile, err := os.CreateTemp(s.tmpDir, "proxmox-metadata-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temp metadata file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(metadata); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	tmpFile.Close()

	// Write metadata to tape using tar
	tarArgs := []string{
		"-c",
		"-b", fmt.Sprintf("%d", s.blockSize/512),
		"-f", devicePath,
		"--label", "proxmox-metadata",
		"-C", filepath.Dir(tmpFile.Name()),
		filepath.Base(tmpFile.Name()),
	}

	cmd := exec.CommandContext(ctx, "tar", tarArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to write metadata to tape: %s", string(output))
	}

	// Write a file mark to separate metadata from data
	return s.tapeService.WriteFileMark(ctx)
}

// updateBackupStatus updates the status of a backup in the database
func (s *BackupService) updateBackupStatus(backupID int64, status, errorMsg string, totalBytes int64) {
	if status == "completed" {
		s.db.Exec(`
			UPDATE proxmox_backups 
			SET status = ?, end_time = CURRENT_TIMESTAMP, total_bytes = ?
			WHERE id = ?
		`, status, totalBytes, backupID)
	} else {
		s.db.Exec(`
			UPDATE proxmox_backups 
			SET status = ?, error_message = ?, end_time = CURRENT_TIMESTAMP
			WHERE id = ?
		`, status, errorMsg, backupID)
	}
}

// BackupAllGuests backs up all VMs and LXCs on a node or cluster
func (s *BackupService) BackupAllGuests(ctx context.Context, node string, tapeID int64, mode BackupMode, compress string) ([]*ProxmoxBackupResult, error) {
	var results []*ProxmoxBackupResult

	// Get nodes to backup
	var nodes []string
	if node != "" {
		nodes = []string{node}
	} else {
		// Get all nodes in cluster
		nodeList, err := s.client.GetNodes(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get nodes: %w", err)
		}
		for _, n := range nodeList {
			if n.Status == "online" {
				nodes = append(nodes, n.Node)
			}
		}
	}

	// Backup each node
	for _, nodeName := range nodes {
		// Backup VMs
		vms, err := s.client.GetNodeVMs(ctx, nodeName)
		if err != nil {
			s.logger.Warn("Failed to get VMs for node", map[string]interface{}{
				"node":  nodeName,
				"error": err.Error(),
			})
			continue
		}

		for _, vm := range vms {
			if vm.Template == 1 {
				continue // Skip templates
			}

			req := &ProxmoxBackupRequest{
				Node:       nodeName,
				VMID:       vm.VMID,
				GuestType:  GuestTypeVM,
				GuestName:  vm.Name,
				BackupMode: mode,
				Compress:   compress,
				TapeID:     tapeID,
			}

			result, err := s.BackupGuest(ctx, req)
			if err != nil {
				s.logger.Error("Failed to backup VM", map[string]interface{}{
					"vmid":  vm.VMID,
					"name":  vm.Name,
					"error": err.Error(),
				})
			}
			results = append(results, result)
		}

		// Backup LXCs
		lxcs, err := s.client.GetNodeLXCs(ctx, nodeName)
		if err != nil {
			s.logger.Warn("Failed to get LXCs for node", map[string]interface{}{
				"node":  nodeName,
				"error": err.Error(),
			})
			continue
		}

		for _, lxc := range lxcs {
			if lxc.Template == 1 {
				continue // Skip templates
			}

			req := &ProxmoxBackupRequest{
				Node:       nodeName,
				VMID:       lxc.VMID,
				GuestType:  GuestTypeLXC,
				GuestName:  lxc.Name,
				BackupMode: mode,
				Compress:   compress,
				TapeID:     tapeID,
			}

			result, err := s.BackupGuest(ctx, req)
			if err != nil {
				s.logger.Error("Failed to backup LXC", map[string]interface{}{
					"vmid":  lxc.VMID,
					"name":  lxc.Name,
					"error": err.Error(),
				})
			}
			results = append(results, result)
		}
	}

	return results, nil
}

// ListBackups returns all Proxmox backups from the database
func (s *BackupService) ListBackups(ctx context.Context, limit int) ([]ProxmoxBackupResult, error) {
	rows, err := s.db.Query(`
		SELECT pb.id, pb.node, pb.vmid, pb.guest_type, pb.guest_name, 
			   pb.tape_id, t.barcode, pb.start_time, pb.end_time, 
			   pb.total_bytes, pb.status, pb.config_data IS NOT NULL
		FROM proxmox_backups pb
		JOIN tapes t ON pb.tape_id = t.id
		ORDER BY pb.start_time DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []ProxmoxBackupResult
	for rows.Next() {
		var b ProxmoxBackupResult
		var endTime *time.Time
		if err := rows.Scan(&b.BackupID, &b.Node, &b.VMID, &b.GuestType, &b.GuestName,
			&b.TapeID, &b.TapeBarcode, &b.StartTime, &endTime, &b.TotalBytes,
			&b.Status, &b.ConfigSaved); err != nil {
			continue
		}
		if endTime != nil {
			b.EndTime = *endTime
		}
		backups = append(backups, b)
	}

	return backups, nil
}

// GetBackup returns details of a specific Proxmox backup
func (s *BackupService) GetBackup(ctx context.Context, backupID int64) (*ProxmoxBackupResult, error) {
	var b ProxmoxBackupResult
	var endTime *time.Time
	err := s.db.QueryRow(`
		SELECT pb.id, pb.node, pb.vmid, pb.guest_type, pb.guest_name, 
			   pb.tape_id, t.barcode, pb.start_time, pb.end_time, 
			   pb.total_bytes, pb.status, pb.config_data IS NOT NULL, pb.error_message
		FROM proxmox_backups pb
		JOIN tapes t ON pb.tape_id = t.id
		WHERE pb.id = ?
	`, backupID).Scan(&b.BackupID, &b.Node, &b.VMID, &b.GuestType, &b.GuestName,
		&b.TapeID, &b.TapeBarcode, &b.StartTime, &endTime, &b.TotalBytes,
		&b.Status, &b.ConfigSaved, &b.Error)
	if err != nil {
		return nil, err
	}
	if endTime != nil {
		b.EndTime = *endTime
	}
	return &b, nil
}

// StreamBackupToWriter streams a Proxmox backup directly to an io.Writer (for tape)
// This is an alternative method that uses vzdump's stdout mode
func (s *BackupService) StreamBackupToWriter(ctx context.Context, req *ProxmoxBackupRequest, w io.Writer) error {
	args := []string{
		fmt.Sprintf("%d", req.VMID),
		"--mode", string(req.BackupMode),
		"--stdout",
	}
	if req.Compress != "" {
		args = append(args, "--compress", req.Compress)
	}

	cmd := exec.CommandContext(ctx, "vzdump", args...)
	cmd.Stdout = w

	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vzdump failed: %s: %w", stderr.String(), err)
	}

	return nil
}
