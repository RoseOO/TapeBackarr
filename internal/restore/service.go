package restore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/RoseOO/TapeBackarr/internal/database"
	"github.com/RoseOO/TapeBackarr/internal/logging"
	"github.com/RoseOO/TapeBackarr/internal/models"
	"github.com/RoseOO/TapeBackarr/internal/tape"
)

// RestoreRequest represents a restore operation request
type RestoreRequest struct {
	BackupSetID     int64    `json:"backup_set_id"`
	FilePaths       []string `json:"file_paths,omitempty"`    // Empty means restore all
	DestPath        string   `json:"dest_path"`
	DestinationType string   `json:"destination_type"`        // local, smb, nfs
	Verify          bool     `json:"verify"`
	Overwrite       bool     `json:"overwrite"`
}

// RestoreResult represents the result of a restore operation
type RestoreResult struct {
	FilesRestored int64     `json:"files_restored"`
	BytesRestored int64     `json:"bytes_restored"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	Errors        []string  `json:"errors,omitempty"`
	Verified      bool      `json:"verified"`
}

// TapeRequirement describes a tape needed for restore
type TapeRequirement struct {
	Tape       models.Tape `json:"tape"`
	FileCount  int         `json:"file_count"`
	TotalBytes int64       `json:"total_bytes"`
	Order      int         `json:"order"` // Insertion order
}

// Service handles restore operations
type Service struct {
	db          *database.DB
	tapeService *tape.Service
	logger      *logging.Logger
	blockSize   int
}

// NewService creates a new restore service
func NewService(db *database.DB, tapeService *tape.Service, logger *logging.Logger, blockSize int) *Service {
	return &Service{
		db:          db,
		tapeService: tapeService,
		logger:      logger,
		blockSize:   blockSize,
	}
}

// GetRequiredTapes returns the tapes needed for a restore operation
func (s *Service) GetRequiredTapes(ctx context.Context, req *RestoreRequest) ([]TapeRequirement, error) {
	var requirements []TapeRequirement
	tapeMap := make(map[int64]*TapeRequirement)

	if len(req.FilePaths) == 0 {
		// Restore entire backup set
		row := s.db.QueryRow(`
			SELECT t.id, t.barcode, t.label, t.status, bs.file_count, bs.total_bytes
			FROM backup_sets bs
			JOIN tapes t ON bs.tape_id = t.id
			WHERE bs.id = ?
		`, req.BackupSetID)

		var t models.Tape
		var fileCount int64
		var totalBytes int64
		if err := row.Scan(&t.ID, &t.Barcode, &t.Label, &t.Status, &fileCount, &totalBytes); err != nil {
			return nil, fmt.Errorf("backup set not found: %w", err)
		}

		requirements = append(requirements, TapeRequirement{
			Tape:       t,
			FileCount:  int(fileCount),
			TotalBytes: totalBytes,
			Order:      1,
		})
	} else {
		// Restore specific files - find which tapes they're on
		for _, filePath := range req.FilePaths {
			rows, err := s.db.Query(`
				SELECT DISTINCT t.id, t.barcode, t.label, t.status, ce.file_size
				FROM catalog_entries ce
				JOIN backup_sets bs ON ce.backup_set_id = bs.id
				JOIN tapes t ON bs.tape_id = t.id
				WHERE ce.file_path = ? AND bs.id = ?
				ORDER BY bs.start_time DESC
				LIMIT 1
			`, filePath, req.BackupSetID)
			if err != nil {
				return nil, err
			}

			for rows.Next() {
				var t models.Tape
				var fileSize int64
				if err := rows.Scan(&t.ID, &t.Barcode, &t.Label, &t.Status, &fileSize); err != nil {
					continue
				}

				if existing, ok := tapeMap[t.ID]; ok {
					existing.FileCount++
					existing.TotalBytes += fileSize
				} else {
					tapeMap[t.ID] = &TapeRequirement{
						Tape:       t,
						FileCount:  1,
						TotalBytes: fileSize,
						Order:      len(tapeMap) + 1,
					}
				}
			}
			rows.Close()
		}

		for _, req := range tapeMap {
			requirements = append(requirements, *req)
		}
	}

	return requirements, nil
}

// Restore performs a restore operation
func (s *Service) Restore(ctx context.Context, req *RestoreRequest) (*RestoreResult, error) {
	result := &RestoreResult{
		StartTime: time.Now(),
	}

	s.logger.Info("Starting restore", map[string]interface{}{
		"backup_set_id": req.BackupSetID,
		"dest_path":     req.DestPath,
		"file_count":    len(req.FilePaths),
	})

	// Get backup set info
	var tapeID int64
	var startBlock int64
	err := s.db.QueryRow(`
		SELECT tape_id, COALESCE(start_block, 0) 
		FROM backup_sets 
		WHERE id = ?
	`, req.BackupSetID).Scan(&tapeID, &startBlock)
	if err != nil {
		return nil, fmt.Errorf("backup set not found: %w", err)
	}

	// Get tape device path
	var devicePath string
	err = s.db.QueryRow("SELECT device_path FROM tape_drives WHERE current_tape_id = ?", tapeID).Scan(&devicePath)
	if err != nil {
		return nil, fmt.Errorf("tape not loaded in any drive: %w", err)
	}

	// Ensure destination exists
	if err := os.MkdirAll(req.DestPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Position tape
	if startBlock > 0 {
		if err := s.tapeService.SeekToBlock(ctx, startBlock); err != nil {
			s.logger.Warn("Failed to seek to block, rewinding", map[string]interface{}{
				"error": err.Error(),
			})
			if err := s.tapeService.Rewind(ctx); err != nil {
				return nil, fmt.Errorf("failed to rewind tape: %w", err)
			}
		}
	} else {
		if err := s.tapeService.Rewind(ctx); err != nil {
			return nil, fmt.Errorf("failed to rewind tape: %w", err)
		}
		// Skip label
		s.tapeService.SeekToFileNumber(ctx, 1)
	}

	// Build tar extract command
	tarArgs := []string{
		"-x",                                     // Extract
		"-b", fmt.Sprintf("%d", s.blockSize/512), // Block size
		"-f", devicePath,                         // Input from tape
		"-C", req.DestPath,                       // Change to destination
	}

	if req.Overwrite {
		tarArgs = append(tarArgs, "--overwrite")
	} else {
		tarArgs = append(tarArgs, "--keep-old-files")
	}

	// Add specific files if requested
	if len(req.FilePaths) > 0 {
		tarArgs = append(tarArgs, req.FilePaths...)
	}

	cmd := exec.CommandContext(ctx, "tar", tarArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := fmt.Sprintf("tar extract failed: %s", string(output))
		result.Errors = append(result.Errors, errMsg)
		s.logger.Error("Restore failed", map[string]interface{}{
			"error": errMsg,
		})
		return result, fmt.Errorf("restore failed: %w", err)
	}

	// Count restored files
	if len(req.FilePaths) > 0 {
		for _, fp := range req.FilePaths {
			destFile := filepath.Join(req.DestPath, fp)
			if info, err := os.Stat(destFile); err == nil {
				result.FilesRestored++
				result.BytesRestored += info.Size()
			}
		}
	} else {
		// Count all files in destination
		filepath.Walk(req.DestPath, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				result.FilesRestored++
				result.BytesRestored += info.Size()
			}
			return nil
		})
	}

	// Verify if requested
	if req.Verify {
		s.logger.Info("Verifying restored files", nil)
		verifyErrors := s.verifyRestore(ctx, req.BackupSetID, req.DestPath, req.FilePaths)
		if len(verifyErrors) > 0 {
			result.Errors = append(result.Errors, verifyErrors...)
			result.Verified = false
		} else {
			result.Verified = true
		}
	}

	result.EndTime = time.Now()

	s.logger.Info("Restore completed", map[string]interface{}{
		"files_restored": result.FilesRestored,
		"bytes_restored": result.BytesRestored,
		"duration":       result.EndTime.Sub(result.StartTime).String(),
		"verified":       result.Verified,
	})

	// Log audit entry
	s.db.Exec(`
		INSERT INTO audit_logs (action, resource_type, resource_id, details)
		VALUES (?, ?, ?, ?)
	`, "restore", "backup_set", req.BackupSetID, fmt.Sprintf("Restored %d files to %s", result.FilesRestored, req.DestPath))

	return result, nil
}

// verifyRestore checks restored files against catalog checksums
func (s *Service) verifyRestore(ctx context.Context, backupSetID int64, destPath string, filePaths []string) []string {
	var errors []string

	query := `
		SELECT file_path, file_size, checksum 
		FROM catalog_entries 
		WHERE backup_set_id = ?
	`
	args := []interface{}{backupSetID}

	if len(filePaths) > 0 {
		placeholders := make([]string, len(filePaths))
		for i, fp := range filePaths {
			placeholders[i] = "?"
			args = append(args, fp)
		}
		query += fmt.Sprintf(" AND file_path IN (%s)", strings.Join(placeholders, ","))
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		errors = append(errors, fmt.Sprintf("failed to query catalog: %v", err))
		return errors
	}
	defer rows.Close()

	for rows.Next() {
		var filePath string
		var expectedSize int64
		var expectedChecksum string

		if err := rows.Scan(&filePath, &expectedSize, &expectedChecksum); err != nil {
			continue
		}

		destFile := filepath.Join(destPath, filePath)
		info, err := os.Stat(destFile)
		if err != nil {
			errors = append(errors, fmt.Sprintf("file not found: %s", filePath))
			continue
		}

		if info.Size() != expectedSize {
			errors = append(errors, fmt.Sprintf("size mismatch for %s: expected %d, got %d", filePath, expectedSize, info.Size()))
		}

		if expectedChecksum != "" {
			actualChecksum, err := calculateChecksum(destFile)
			if err != nil {
				errors = append(errors, fmt.Sprintf("failed to calculate checksum for %s: %v", filePath, err))
				continue
			}
			if actualChecksum != expectedChecksum {
				errors = append(errors, fmt.Sprintf("checksum mismatch for %s", filePath))
			}
		}
	}

	return errors
}

func calculateChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// BrowseCatalog returns files in a backup set, optionally filtered by path prefix
func (s *Service) BrowseCatalog(ctx context.Context, backupSetID int64, pathPrefix string, limit int) ([]models.CatalogEntry, error) {
	query := `
		SELECT id, backup_set_id, file_path, file_size, file_mode, mod_time, checksum, block_offset
		FROM catalog_entries
		WHERE backup_set_id = ?
	`
	args := []interface{}{backupSetID}

	if pathPrefix != "" {
		query += " AND file_path LIKE ?"
		args = append(args, pathPrefix+"%")
	}

	query += " ORDER BY file_path LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.CatalogEntry
	for rows.Next() {
		var e models.CatalogEntry
		if err := rows.Scan(&e.ID, &e.BackupSetID, &e.FilePath, &e.FileSize, &e.FileMode, &e.ModTime, &e.Checksum, &e.BlockOffset); err != nil {
			continue
		}
		entries = append(entries, e)
	}

	return entries, nil
}

// GetCatalogDirectories returns unique directory paths from catalog
func (s *Service) GetCatalogDirectories(ctx context.Context, backupSetID int64) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT 
			CASE 
				WHEN INSTR(file_path, '/') > 0 
				THEN SUBSTR(file_path, 1, INSTR(file_path, '/') - 1)
				ELSE file_path
			END as dir
		FROM catalog_entries
		WHERE backup_set_id = ?
		ORDER BY dir
	`, backupSetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dirs []string
	for rows.Next() {
		var dir string
		if err := rows.Scan(&dir); err != nil {
			continue
		}
		dirs = append(dirs, dir)
	}

	return dirs, nil
}
