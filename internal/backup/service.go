package backup

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/RoseOO/TapeBackarr/internal/database"
	"github.com/RoseOO/TapeBackarr/internal/encryption"
	"github.com/RoseOO/TapeBackarr/internal/logging"
	"github.com/RoseOO/TapeBackarr/internal/models"
	"github.com/RoseOO/TapeBackarr/internal/tape"
)

// FileInfo represents a file in the backup set
type FileInfo struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	Mode    int       `json:"mode"`
	ModTime time.Time `json:"mod_time"`
	Hash    string    `json:"hash,omitempty"`
}

// JobProgress tracks the progress of a running backup job
type JobProgress struct {
	JobID                     int64     `json:"job_id"`
	JobName                   string    `json:"job_name"`
	BackupSetID               int64     `json:"backup_set_id"`
	Phase                     string    `json:"phase"`
	Message                   string    `json:"message"`
	Status                    string    `json:"status"` // running, paused, cancelled
	FileCount                 int64     `json:"file_count"`
	TotalFiles                int64     `json:"total_files"`
	TotalBytes                int64     `json:"total_bytes"`
	BytesWritten              int64     `json:"bytes_written"`
	WriteSpeed                float64   `json:"write_speed"` // bytes per second (recent average)
	TapeLabel                 string    `json:"tape_label"`
	TapeCapacityBytes         int64     `json:"tape_capacity_bytes"`
	TapeUsedBytes             int64     `json:"tape_used_bytes"` // used before this backup
	DevicePath                string    `json:"device_path"`
	EstimatedSecondsRemaining float64   `json:"estimated_seconds_remaining"`
	StartTime                 time.Time `json:"start_time"`
	UpdatedAt                 time.Time `json:"updated_at"`
	LogLines                  []string  `json:"log_lines"`
}

// EventCallback is called when backup progress events occur (for SSE/console)
type EventCallback func(eventType, category, title, message string)

// countingReader wraps an io.Reader and counts bytes read through it
type countingReader struct {
	reader   io.Reader
	count    int64
	mu       sync.Mutex
	callback func(bytesRead int64)
	paused   *int32 // atomic: 0=running, 1=paused
}

func (cr *countingReader) Read(p []byte) (int, error) {
	// Check pause state
	for cr.paused != nil && atomic.LoadInt32(cr.paused) == 1 {
		time.Sleep(100 * time.Millisecond)
	}
	n, err := cr.reader.Read(p)
	if n > 0 {
		cr.mu.Lock()
		cr.count += int64(n)
		total := cr.count
		cr.mu.Unlock()
		if cr.callback != nil {
			cr.callback(total)
		}
	}
	return n, err
}

func (cr *countingReader) bytesRead() int64 {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	return cr.count
}

// Service handles backup operations
type Service struct {
	db            *database.DB
	tapeService   *tape.Service
	logger        *logging.Logger
	blockSize     int
	mu            sync.Mutex
	activeJobs    map[int64]*JobProgress
	cancelFuncs   map[int64]context.CancelFunc
	pauseFlags    map[int64]*int32
	EventCallback EventCallback
}

// NewService creates a new backup service
func NewService(db *database.DB, tapeService *tape.Service, logger *logging.Logger, blockSize int) *Service {
	return &Service{
		db:          db,
		tapeService: tapeService,
		logger:      logger,
		blockSize:   blockSize,
		activeJobs:  make(map[int64]*JobProgress),
		cancelFuncs: make(map[int64]context.CancelFunc),
		pauseFlags:  make(map[int64]*int32),
	}
}

// GetActiveJobs returns all currently running backup jobs with progress
func (s *Service) GetActiveJobs() []*JobProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs := make([]*JobProgress, 0, len(s.activeJobs))
	for _, j := range s.activeJobs {
		jobs = append(jobs, j)
	}
	return jobs
}

// CancelJob cancels a running backup job
func (s *Service) CancelJob(jobID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel, ok := s.cancelFuncs[jobID]; ok {
		if p, ok := s.activeJobs[jobID]; ok {
			p.Status = "cancelled"
			p.Phase = "cancelled"
			p.Message = "Job cancelled by user"
			p.UpdatedAt = time.Now()
			p.LogLines = append(p.LogLines, fmt.Sprintf("[%s] Job cancelled by user", time.Now().Format("15:04:05")))
		}
		cancel()
		return true
	}
	return false
}

// PauseJob pauses a running backup job
func (s *Service) PauseJob(jobID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if flag, ok := s.pauseFlags[jobID]; ok {
		atomic.StoreInt32(flag, 1)
		if p, ok := s.activeJobs[jobID]; ok {
			p.Status = "paused"
			p.Message = "Job paused by user"
			p.UpdatedAt = time.Now()
			p.LogLines = append(p.LogLines, fmt.Sprintf("[%s] Job paused by user", time.Now().Format("15:04:05")))
		}
		return true
	}
	return false
}

// ResumeJob resumes a paused backup job
func (s *Service) ResumeJob(jobID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if flag, ok := s.pauseFlags[jobID]; ok {
		atomic.StoreInt32(flag, 0)
		if p, ok := s.activeJobs[jobID]; ok {
			p.Status = "running"
			p.Message = "Job resumed by user"
			p.UpdatedAt = time.Now()
			p.LogLines = append(p.LogLines, fmt.Sprintf("[%s] Job resumed by user", time.Now().Format("15:04:05")))
		}
		return true
	}
	return false
}

// emitEvent sends an event to the EventCallback if configured
func (s *Service) emitEvent(eventType, category, title, message string) {
	if s.EventCallback != nil {
		s.EventCallback(eventType, category, title, message)
	}
}

func (s *Service) updateProgress(jobID int64, phase, message string) {
	s.mu.Lock()
	if p, ok := s.activeJobs[jobID]; ok {
		p.Phase = phase
		p.Message = message
		p.UpdatedAt = time.Now()
		p.LogLines = append(p.LogLines, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), message))
		// Keep last 100 log lines
		if len(p.LogLines) > 100 {
			p.LogLines = p.LogLines[len(p.LogLines)-100:]
		}
		// Update write speed and ETA
		elapsed := time.Since(p.StartTime).Seconds()
		if elapsed > 0 && p.BytesWritten > 0 {
			p.WriteSpeed = float64(p.BytesWritten) / elapsed
			remainingBytes := p.TotalBytes - p.BytesWritten
			if p.WriteSpeed > 0 && remainingBytes > 0 {
				p.EstimatedSecondsRemaining = float64(remainingBytes) / p.WriteSpeed
			}
		}
	}
	s.mu.Unlock()

	// Emit event to system console
	eventType := "info"
	if phase == "failed" || phase == "cancelled" {
		eventType = "error"
	} else if phase == "completed" {
		eventType = "success"
	}
	s.emitEvent(eventType, "backup", fmt.Sprintf("Backup: %s", phase), message)
}

// ScanSource scans a backup source and returns file information using concurrent directory traversal
func (s *Service) ScanSource(ctx context.Context, source *models.BackupSource) ([]FileInfo, error) {
	// Parse include/exclude patterns
	var includePatterns, excludePatterns []string
	if source.IncludePatterns != "" {
		json.Unmarshal([]byte(source.IncludePatterns), &includePatterns)
	}
	if source.ExcludePatterns != "" {
		json.Unmarshal([]byte(source.ExcludePatterns), &excludePatterns)
	}

	numWorkers := runtime.NumCPU()
	if numWorkers < 4 {
		numWorkers = 4
	}

	var (
		files    []FileInfo
		filesMu  sync.Mutex
		dirWg    sync.WaitGroup
		workerWg sync.WaitGroup
		dirs     = make(chan string, numWorkers*4)
	)

	// matchFile checks if a file path matches the include/exclude patterns
	matchFile := func(path string) bool {
		relPath, _ := filepath.Rel(source.Path, path)
		baseName := filepath.Base(path)

		// Check exclude patterns
		for _, pattern := range excludePatterns {
			if matched, _ := filepath.Match(pattern, relPath); matched {
				return false
			}
			if matched, _ := filepath.Match(pattern, baseName); matched {
				return false
			}
		}

		// Check include patterns (if any)
		if len(includePatterns) > 0 {
			for _, pattern := range includePatterns {
				if matched, _ := filepath.Match(pattern, relPath); matched {
					return true
				}
				if matched, _ := filepath.Match(pattern, baseName); matched {
					return true
				}
			}
			return false
		}

		return true
	}

	// processDir reads a single directory and enqueues subdirectories for workers
	var processDir func(string)
	processDir = func(dirPath string) {
		defer dirWg.Done()

		select {
		case <-ctx.Done():
			return
		default:
		}

		entries, err := os.ReadDir(dirPath)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("Error accessing path", map[string]interface{}{
					"path":  dirPath,
					"error": err.Error(),
				})
			}
			return
		}

		var localFiles []FileInfo
		for _, entry := range entries {
			path := filepath.Join(dirPath, entry.Name())

			if entry.IsDir() {
				dirWg.Add(1)
				select {
				case dirs <- path:
					// Sent to a worker
				default:
					// Channel full, process inline to avoid deadlock
					processDir(path)
				}
				continue
			}

			if !matchFile(path) {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}

			localFiles = append(localFiles, FileInfo{
				Path:    path,
				Size:    info.Size(),
				Mode:    int(info.Mode()),
				ModTime: info.ModTime(),
			})
		}

		if len(localFiles) > 0 {
			filesMu.Lock()
			files = append(files, localFiles...)
			filesMu.Unlock()
		}
	}

	// Seed root directory
	dirWg.Add(1)
	dirs <- source.Path

	// Close channel when all directories are processed
	go func() {
		dirWg.Wait()
		close(dirs)
	}()

	// Start workers
	for i := 0; i < numWorkers; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for dir := range dirs {
				processDir(dir)
			}
		}()
	}

	workerWg.Wait()

	return files, ctx.Err()
}

// CompareWithSnapshot compares current files with a previous snapshot for incremental backup
func (s *Service) CompareWithSnapshot(ctx context.Context, currentFiles []FileInfo, snapshotData []byte) ([]FileInfo, error) {
	if len(snapshotData) == 0 {
		return currentFiles, nil
	}

	var previousFiles []FileInfo
	if err := json.Unmarshal(snapshotData, &previousFiles); err != nil {
		return nil, fmt.Errorf("failed to parse snapshot: %w", err)
	}

	// Create a map of previous files
	prevMap := make(map[string]FileInfo)
	for _, f := range previousFiles {
		prevMap[f.Path] = f
	}

	// Find changed files
	var changedFiles []FileInfo
	for _, current := range currentFiles {
		prev, exists := prevMap[current.Path]
		if !exists {
			// New file
			changedFiles = append(changedFiles, current)
		} else if current.ModTime.After(prev.ModTime) || current.Size != prev.Size {
			// Modified file
			changedFiles = append(changedFiles, current)
		}
	}

	return changedFiles, nil
}

// StreamToTape streams files directly to tape using tar
func (s *Service) StreamToTape(ctx context.Context, sourcePath string, files []FileInfo, devicePath string, progressCb func(bytesWritten int64), pauseFlag *int32) error {
	if len(files) == 0 {
		return nil
	}

	// Create a file list for tar
	fileListPath := fmt.Sprintf("/tmp/tapebackarr-filelist-%d.txt", time.Now().UnixNano())
	fileList, err := os.Create(fileListPath)
	if err != nil {
		return fmt.Errorf("failed to create file list: %w", err)
	}
	defer os.Remove(fileListPath)

	for _, f := range files {
		// Write relative path to file list
		relPath, _ := filepath.Rel(sourcePath, f.Path)
		fmt.Fprintln(fileList, relPath)
	}
	fileList.Close()

	// Build tar command with streaming to tape
	// Using mbuffer for buffering if available, otherwise direct
	tarArgs := []string{
		"-c",                                     // Create archive
		"-b", fmt.Sprintf("%d", s.blockSize/512), // Block size in 512-byte units
		"-C", sourcePath, // Change to source directory
		"-T", fileListPath, // Read files from list
	}

	var cmd *exec.Cmd

	// Check if mbuffer is available
	_, mbufferErr := exec.LookPath("mbuffer")
	if mbufferErr == nil {
		// Use mbuffer for better streaming performance
		tarCmd := exec.CommandContext(ctx, "tar", tarArgs...)
		mbufferCmd := exec.CommandContext(ctx, "mbuffer", "-s", fmt.Sprintf("%d", s.blockSize), "-m", "256M", "-o", devicePath)

		// Pipe tar output through counting reader to mbuffer
		tarCmd.Dir = sourcePath
		pipe, err := tarCmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("failed to create pipe: %w", err)
		}

		cr := &countingReader{reader: pipe, callback: progressCb, paused: pauseFlag}
		mbufferCmd.Stdin = cr

		if err := tarCmd.Start(); err != nil {
			return fmt.Errorf("failed to start tar: %w", err)
		}
		if err := mbufferCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			return fmt.Errorf("failed to start mbuffer: %w", err)
		}

		tarErr := tarCmd.Wait()
		mbufferErr := mbufferCmd.Wait()

		if ctx.Err() != nil {
			return fmt.Errorf("backup cancelled: %w", ctx.Err())
		}
		if tarErr != nil {
			return fmt.Errorf("tar failed: %w", tarErr)
		}
		if mbufferErr != nil {
			return fmt.Errorf("mbuffer failed: %w", mbufferErr)
		}
	} else {
		// Direct tar to tape
		tarArgs = append(tarArgs, "-f", devicePath)
		cmd = exec.CommandContext(ctx, "tar", tarArgs...)
		cmd.Dir = sourcePath

		output, err := cmd.CombinedOutput()
		if ctx.Err() != nil {
			return fmt.Errorf("backup cancelled: %w", ctx.Err())
		}
		if err != nil {
			return fmt.Errorf("tar failed: %s", string(output))
		}
	}

	return nil
}

// StreamToTapeEncrypted streams files directly to tape with encryption using openssl
func (s *Service) StreamToTapeEncrypted(ctx context.Context, sourcePath string, files []FileInfo, devicePath string, encryptionKey string, progressCb func(bytesWritten int64), pauseFlag *int32) error {
	if len(files) == 0 {
		return nil
	}

	// Create a file list for tar
	fileListPath := fmt.Sprintf("/tmp/tapebackarr-filelist-%d.txt", time.Now().UnixNano())
	fileList, err := os.Create(fileListPath)
	if err != nil {
		return fmt.Errorf("failed to create file list: %w", err)
	}
	defer os.Remove(fileListPath)

	for _, f := range files {
		relPath, _ := filepath.Rel(sourcePath, f.Path)
		fmt.Fprintln(fileList, relPath)
	}
	fileList.Close()

	// Build tar command
	tarArgs := []string{
		"-c",
		"-b", fmt.Sprintf("%d", s.blockSize/512),
		"-C", sourcePath,
		"-T", fileListPath,
	}

	// Create pipeline: tar -> openssl enc -> tape device
	// Using openssl for encryption (widely available, standard tool)
	tarCmd := exec.CommandContext(ctx, "tar", tarArgs...)
	tarCmd.Dir = sourcePath

	// openssl enc with AES-256-GCM and the key passed via stdin-derived password
	// Using -pbkdf2 for key derivation and -pass for the key
	opensslCmd := exec.CommandContext(ctx, "openssl", "enc",
		"-aes-256-cbc", // Using CBC as GCM is not widely supported in openssl enc
		"-salt",
		"-pbkdf2",
		"-iter", "100000",
		"-pass", "pass:"+encryptionKey,
	)

	// Check if mbuffer is available
	_, mbufferErr := exec.LookPath("mbuffer")

	// Set up the pipeline with counting reader for progress tracking
	tarPipe, err := tarCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create tar pipe: %w", err)
	}
	cr := &countingReader{reader: tarPipe, callback: progressCb, paused: pauseFlag}
	opensslCmd.Stdin = cr

	if mbufferErr == nil {
		// Use mbuffer for buffering before writing to tape
		mbufferCmd := exec.CommandContext(ctx, "mbuffer", "-s", fmt.Sprintf("%d", s.blockSize), "-m", "256M", "-o", devicePath)

		opensslPipe, err := opensslCmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("failed to create openssl pipe: %w", err)
		}
		mbufferCmd.Stdin = opensslPipe

		// Start the pipeline
		if err := tarCmd.Start(); err != nil {
			return fmt.Errorf("failed to start tar: %w", err)
		}
		if err := opensslCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			return fmt.Errorf("failed to start openssl: %w", err)
		}
		if err := mbufferCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			opensslCmd.Process.Kill()
			return fmt.Errorf("failed to start mbuffer: %w", err)
		}

		// Wait for all commands
		tarErr := tarCmd.Wait()
		opensslErr := opensslCmd.Wait()
		mbufferErr := mbufferCmd.Wait()

		if ctx.Err() != nil {
			return fmt.Errorf("backup cancelled: %w", ctx.Err())
		}
		if tarErr != nil {
			return fmt.Errorf("tar failed: %w", tarErr)
		}
		if opensslErr != nil {
			return fmt.Errorf("openssl encryption failed: %w", opensslErr)
		}
		if mbufferErr != nil {
			return fmt.Errorf("mbuffer failed: %w", mbufferErr)
		}
	} else {
		// Direct to tape device
		tapeFile, err := os.OpenFile(devicePath, os.O_WRONLY, 0)
		if err != nil {
			return fmt.Errorf("failed to open tape device: %w", err)
		}
		defer tapeFile.Close()

		opensslCmd.Stdout = tapeFile

		if err := tarCmd.Start(); err != nil {
			return fmt.Errorf("failed to start tar: %w", err)
		}
		if err := opensslCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			return fmt.Errorf("failed to start openssl: %w", err)
		}

		tarErr := tarCmd.Wait()
		opensslErr := opensslCmd.Wait()

		if ctx.Err() != nil {
			return fmt.Errorf("backup cancelled: %w", ctx.Err())
		}
		if tarErr != nil {
			return fmt.Errorf("tar failed: %w", tarErr)
		}
		if opensslErr != nil {
			return fmt.Errorf("openssl encryption failed: %w", opensslErr)
		}
	}

	return nil
}

// GetEncryptionKey retrieves the base64 encryption key for a given key ID
func (s *Service) GetEncryptionKey(ctx context.Context, keyID int64) (string, error) {
	var keyData string
	err := s.db.QueryRow("SELECT key_data FROM encryption_keys WHERE id = ?", keyID).Scan(&keyData)
	if err != nil {
		return "", fmt.Errorf("failed to get encryption key: %w", err)
	}
	return keyData, nil
}

// Compile-time check to ensure encryption package is used
var _ = encryption.AlgorithmAES256GCM
var _ = base64.StdEncoding

// CalculateChecksum calculates SHA256 checksum of a file
func (s *Service) CalculateChecksum(path string) (string, error) {
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

// CreateSnapshot creates a snapshot of the current file state
func (s *Service) CreateSnapshot(files []FileInfo) ([]byte, error) {
	return json.Marshal(files)
}

// RunBackup executes a full backup job
func (s *Service) RunBackup(ctx context.Context, job *models.BackupJob, source *models.BackupSource, tapeID int64, backupType models.BackupType) (*models.BackupSet, error) {
	startTime := time.Now()

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	var pauseFlag int32

	// Look up tape info for progress display
	var tapeLabel string
	var tapeCapacity, tapeUsed int64
	if err := s.db.QueryRow("SELECT label, capacity_bytes, used_bytes FROM tapes WHERE id = ?", tapeID).Scan(&tapeLabel, &tapeCapacity, &tapeUsed); err != nil {
		s.logger.Warn("Could not look up tape info for progress display", map[string]interface{}{
			"tape_id": tapeID,
			"error":   err.Error(),
		})
	}

	// Register active job progress
	s.mu.Lock()
	s.activeJobs[job.ID] = &JobProgress{
		JobID:             job.ID,
		JobName:           job.Name,
		Phase:             "initializing",
		Status:            "running",
		Message:           "Starting backup job...",
		TapeLabel:         tapeLabel,
		TapeCapacityBytes: tapeCapacity,
		TapeUsedBytes:     tapeUsed,
		StartTime:         startTime,
		UpdatedAt:         startTime,
		LogLines:          []string{fmt.Sprintf("[%s] Starting backup job: %s", startTime.Format("15:04:05"), job.Name)},
	}
	s.cancelFuncs[job.ID] = cancel
	s.pauseFlags[job.ID] = &pauseFlag
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.activeJobs, job.ID)
		delete(s.cancelFuncs, job.ID)
		delete(s.pauseFlags, job.ID)
		s.mu.Unlock()
		cancel()
	}()

	s.emitEvent("info", "backup", "Backup Started", fmt.Sprintf("Starting backup job: %s (tape: %s)", job.Name, tapeLabel))
	s.logger.Info("Starting backup job", map[string]interface{}{
		"job_id":      job.ID,
		"job_name":    job.Name,
		"source_path": source.Path,
		"backup_type": backupType,
		"tape_label":  tapeLabel,
	})

	// Create backup set record
	result, err := s.db.Exec(`
		INSERT INTO backup_sets (job_id, tape_id, backup_type, start_time, status)
		VALUES (?, ?, ?, ?, ?)
	`, job.ID, tapeID, backupType, startTime, models.BackupSetStatusRunning)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup set: %w", err)
	}

	backupSetID, _ := result.LastInsertId()
	s.mu.Lock()
	if p, ok := s.activeJobs[job.ID]; ok {
		p.BackupSetID = backupSetID
	}
	s.mu.Unlock()

	// Mark drive as busy
	s.db.Exec("UPDATE tape_drives SET status = 'busy' WHERE current_tape_id = ?", tapeID)
	defer s.db.Exec("UPDATE tape_drives SET status = 'ready' WHERE current_tape_id = ?", tapeID)

	// Scan source
	s.updateProgress(job.ID, "scanning", fmt.Sprintf("Scanning source: %s", source.Path))
	s.logger.Info("Scanning source", map[string]interface{}{"path": source.Path})
	files, err := s.ScanSource(ctx, source)
	if err != nil {
		s.updateProgress(job.ID, "failed", fmt.Sprintf("Failed to scan source: %s", err.Error()))
		s.updateBackupSetStatus(backupSetID, models.BackupSetStatusFailed, err.Error())
		return nil, fmt.Errorf("failed to scan source: %w", err)
	}

	s.updateProgress(job.ID, "scanning", fmt.Sprintf("Scan complete: found %d files", len(files)))
	s.logger.Info("Scan complete", map[string]interface{}{
		"file_count": len(files),
	})

	// For incremental backup, compare with previous snapshot
	if backupType == models.BackupTypeIncremental {
		var snapshotData []byte
		err := s.db.QueryRow(`
			SELECT snapshot_data FROM snapshots 
			WHERE source_id = ? 
			ORDER BY created_at DESC LIMIT 1
		`, source.ID).Scan(&snapshotData)

		if err == nil && len(snapshotData) > 0 {
			files, err = s.CompareWithSnapshot(ctx, files, snapshotData)
			if err != nil {
				s.logger.Warn("Failed to compare with snapshot, doing full backup", map[string]interface{}{
					"error": err.Error(),
				})
			} else {
				s.logger.Info("Incremental backup", map[string]interface{}{
					"changed_files": len(files),
				})
			}
		}
	}

	// Calculate total size
	var totalBytes int64
	for _, f := range files {
		totalBytes += f.Size
	}

	// Update progress with file/byte totals
	s.mu.Lock()
	if p, ok := s.activeJobs[job.ID]; ok {
		p.TotalFiles = int64(len(files))
		p.TotalBytes = totalBytes
	}
	s.mu.Unlock()

	// Get tape device path
	var devicePath string
	err = s.db.QueryRow("SELECT device_path FROM tape_drives WHERE current_tape_id = ?", tapeID).Scan(&devicePath)
	if err != nil {
		s.updateProgress(job.ID, "failed", "No drive found with specified tape")
		s.updateBackupSetStatus(backupSetID, models.BackupSetStatusFailed, "no drive found with tape")
		return nil, fmt.Errorf("no drive found with specified tape: %w", err)
	}

	// Update device path in progress
	s.mu.Lock()
	if p, ok := s.activeJobs[job.ID]; ok {
		p.DevicePath = devicePath
	}
	s.mu.Unlock()

	// Progress callback for real-time byte tracking
	progressCb := func(bytesWritten int64) {
		s.mu.Lock()
		if p, ok := s.activeJobs[job.ID]; ok {
			p.BytesWritten = bytesWritten
			elapsed := time.Since(p.StartTime).Seconds()
			if elapsed > 0 {
				p.WriteSpeed = float64(bytesWritten) / elapsed
				remainingBytes := p.TotalBytes - bytesWritten
				if p.WriteSpeed > 0 && remainingBytes > 0 {
					p.EstimatedSecondsRemaining = float64(remainingBytes) / p.WriteSpeed
				} else {
					p.EstimatedSecondsRemaining = 0
				}
			}
			p.UpdatedAt = time.Now()
		}
		s.mu.Unlock()
	}

	// Position tape past the label before writing data.
	// The tape label occupies file position 0 followed by a file mark.
	// We must seek to file position 1 so that backup data does not overwrite the label.
	s.updateProgress(job.ID, "positioning", "Positioning tape past label...")
	driveSvc := tape.NewServiceForDevice(devicePath, s.tapeService.GetBlockSize())
	if err := driveSvc.Rewind(ctx); err != nil {
		s.logger.Warn("Failed to rewind tape before positioning", map[string]interface{}{"error": err.Error()})
	}
	if err := driveSvc.SeekToFileNumber(ctx, 1); err != nil {
		// If seek fails (e.g. no file mark yet on a blank tape), rewind and write a label first
		s.logger.Warn("Failed to seek past label, tape may be unlabeled", map[string]interface{}{"error": err.Error()})
		if rewindErr := driveSvc.Rewind(ctx); rewindErr != nil {
			s.updateProgress(job.ID, "failed", "Failed to position tape: "+rewindErr.Error())
			s.updateBackupSetStatus(backupSetID, models.BackupSetStatusFailed, "failed to position tape")
			return nil, fmt.Errorf("failed to rewind tape: %w", rewindErr)
		}
	}

	// Stream to tape
	s.updateProgress(job.ID, "streaming", fmt.Sprintf("Streaming %d files (%d bytes) to tape device %s", len(files), totalBytes, devicePath))
	s.logger.Info("Streaming to tape", map[string]interface{}{
		"device":      devicePath,
		"file_count":  len(files),
		"total_bytes": totalBytes,
		"encrypted":   job.EncryptionEnabled,
	})

	var encrypted bool
	var encryptionKeyID *int64

	if job.EncryptionEnabled && job.EncryptionKeyID != nil {
		// Get encryption key
		encKey, err := s.GetEncryptionKey(ctx, *job.EncryptionKeyID)
		if err != nil {
			s.updateProgress(job.ID, "failed", "Encryption key not found: "+err.Error())
			s.updateBackupSetStatus(backupSetID, models.BackupSetStatusFailed, "encryption key not found: "+err.Error())
			return nil, fmt.Errorf("failed to get encryption key: %w", err)
		}

		s.updateProgress(job.ID, "streaming", "Encrypting and streaming to tape...")
		s.logger.Info("Encrypting backup with key", map[string]interface{}{
			"key_id": *job.EncryptionKeyID,
		})

		if err := s.StreamToTapeEncrypted(ctx, source.Path, files, devicePath, encKey, progressCb, &pauseFlag); err != nil {
			s.updateProgress(job.ID, "failed", "Stream failed: "+err.Error())
			s.updateBackupSetStatus(backupSetID, models.BackupSetStatusFailed, err.Error())
			s.emitEvent("error", "backup", "Backup Failed", fmt.Sprintf("Job %s failed: %s", job.Name, err.Error()))
			return nil, fmt.Errorf("failed to stream encrypted backup to tape: %w", err)
		}
		encrypted = true
		encryptionKeyID = job.EncryptionKeyID
	} else {
		if err := s.StreamToTape(ctx, source.Path, files, devicePath, progressCb, &pauseFlag); err != nil {
			s.updateProgress(job.ID, "failed", "Stream failed: "+err.Error())
			s.updateBackupSetStatus(backupSetID, models.BackupSetStatusFailed, err.Error())
			s.emitEvent("error", "backup", "Backup Failed", fmt.Sprintf("Job %s failed: %s", job.Name, err.Error()))
			return nil, fmt.Errorf("failed to stream to tape: %w", err)
		}
	}

	s.updateProgress(job.ID, "cataloging", "Writing file mark...")
	// Write file mark
	if err := s.tapeService.WriteFileMark(ctx); err != nil {
		s.logger.Warn("Failed to write file mark", map[string]interface{}{"error": err.Error()})
	}

	// Update catalog entries with checksums for data integrity
	s.updateProgress(job.ID, "cataloging", fmt.Sprintf("Cataloging %d files and calculating checksums...", len(files)))
	s.logger.Info("Calculating file checksums for data integrity", map[string]interface{}{
		"file_count": len(files),
	})
	for i, f := range files {
		relPath, _ := filepath.Rel(source.Path, f.Path)

		// Update file counter in progress
		s.mu.Lock()
		if p, ok := s.activeJobs[job.ID]; ok {
			p.FileCount = int64(i + 1)
		}
		s.mu.Unlock()

		// Calculate SHA256 checksum for data integrity verification
		checksum, checksumErr := s.CalculateChecksum(f.Path)
		if checksumErr != nil {
			s.logger.Warn("Failed to calculate checksum", map[string]interface{}{
				"file":  relPath,
				"error": checksumErr.Error(),
			})
			checksum = "" // Store empty checksum if calculation fails
		}

		_, err := s.db.Exec(`
			INSERT INTO catalog_entries (backup_set_id, file_path, file_size, file_mode, mod_time, checksum)
			VALUES (?, ?, ?, ?, ?, ?)
		`, backupSetID, relPath, f.Size, f.Mode, f.ModTime, checksum)
		if err != nil {
			s.logger.Warn("Failed to insert catalog entry", map[string]interface{}{
				"file":  relPath,
				"error": err.Error(),
			})
		}
	}

	// Save snapshot for future incremental backups
	snapshotData, _ := s.CreateSnapshot(files)
	s.db.Exec(`
		INSERT INTO snapshots (source_id, backup_set_id, file_count, total_bytes, snapshot_data)
		VALUES (?, ?, ?, ?, ?)
	`, source.ID, backupSetID, len(files), totalBytes, snapshotData)

	// Update backup set status
	endTime := time.Now()
	s.db.Exec(`
		UPDATE backup_sets SET 
			end_time = ?, 
			status = ?, 
			file_count = ?, 
			total_bytes = ?,
			encrypted = ?,
			encryption_key_id = ?
		WHERE id = ?
	`, endTime, models.BackupSetStatusCompleted, len(files), totalBytes, encrypted, encryptionKeyID, backupSetID)

	// Update tape usage
	s.db.Exec(`
		UPDATE tapes SET 
			used_bytes = used_bytes + ?, 
			write_count = write_count + 1,
			last_written_at = ?,
			status = CASE WHEN status = 'blank' THEN 'active' ELSE status END
		WHERE id = ?
	`, totalBytes, endTime, tapeID)

	// Update tape with encryption key info for library visibility
	if encrypted && encryptionKeyID != nil {
		var keyFingerprint, keyName string
		if err := s.db.QueryRow("SELECT key_fingerprint, name FROM encryption_keys WHERE id = ?", *encryptionKeyID).Scan(&keyFingerprint, &keyName); err != nil {
			s.logger.Warn("Failed to look up encryption key for tape tracking", map[string]interface{}{
				"encryption_key_id": *encryptionKeyID,
				"error":             err.Error(),
			})
		} else if keyFingerprint != "" {
			if _, err := s.db.Exec("UPDATE tapes SET encryption_key_fingerprint = ?, encryption_key_name = ? WHERE id = ?", keyFingerprint, keyName, tapeID); err != nil {
				s.logger.Warn("Failed to update tape encryption tracking", map[string]interface{}{
					"tape_id": tapeID,
					"error":   err.Error(),
				})
			}
		}
	}

	// Update job last run
	s.db.Exec("UPDATE backup_jobs SET last_run_at = ? WHERE id = ?", endTime, job.ID)

	s.updateProgress(job.ID, "completed", fmt.Sprintf("Backup completed: %d files, %d bytes in %s", len(files), totalBytes, endTime.Sub(startTime).String()))
	s.emitEvent("success", "backup", "Backup Completed", fmt.Sprintf("Job %s completed: %d files, %d bytes in %s", job.Name, len(files), totalBytes, endTime.Sub(startTime).String()))
	s.logger.Info("Backup completed", map[string]interface{}{
		"job_id":      job.ID,
		"file_count":  len(files),
		"total_bytes": totalBytes,
		"duration":    endTime.Sub(startTime).String(),
		"encrypted":   encrypted,
	})

	return &models.BackupSet{
		ID:              backupSetID,
		JobID:           job.ID,
		TapeID:          tapeID,
		BackupType:      backupType,
		StartTime:       startTime,
		EndTime:         &endTime,
		Status:          models.BackupSetStatusCompleted,
		FileCount:       int64(len(files)),
		TotalBytes:      totalBytes,
		Encrypted:       encrypted,
		EncryptionKeyID: encryptionKeyID,
	}, nil
}

func (s *Service) updateBackupSetStatus(id int64, status models.BackupSetStatus, errorMsg string) {
	s.db.Exec(`
		UPDATE backup_sets SET status = ?, updated_at = CURRENT_TIMESTAMP 
		WHERE id = ?
	`, status, id)

	if errorMsg != "" {
		s.logger.Error("Backup failed", map[string]interface{}{
			"backup_set_id": id,
			"error":         errorMsg,
		})
	}
}

// ListBackupSets returns backup sets with optional filters
func (s *Service) ListBackupSets(ctx context.Context, jobID *int64, limit int) ([]models.BackupSet, error) {
	query := "SELECT id, job_id, tape_id, backup_type, start_time, end_time, status, file_count, total_bytes FROM backup_sets"
	var args []interface{}

	if jobID != nil {
		query += " WHERE job_id = ?"
		args = append(args, *jobID)
	}

	query += " ORDER BY start_time DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sets []models.BackupSet
	for rows.Next() {
		var bs models.BackupSet
		if err := rows.Scan(&bs.ID, &bs.JobID, &bs.TapeID, &bs.BackupType, &bs.StartTime, &bs.EndTime, &bs.Status, &bs.FileCount, &bs.TotalBytes); err != nil {
			return nil, err
		}
		sets = append(sets, bs)
	}

	return sets, nil
}

// SearchCatalog searches the catalog for files matching a pattern
func (s *Service) SearchCatalog(ctx context.Context, pattern string, limit int) ([]models.CatalogEntry, error) {
	// Replace * with % for SQL LIKE
	sqlPattern := strings.ReplaceAll(pattern, "*", "%")

	rows, err := s.db.Query(`
		SELECT id, backup_set_id, file_path, file_size, file_mode, mod_time, checksum, block_offset
		FROM catalog_entries
		WHERE file_path LIKE ?
		ORDER BY file_path
		LIMIT ?
	`, sqlPattern, limit)
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

// GetTapesForRestore returns the tapes needed to restore specified files
func (s *Service) GetTapesForRestore(ctx context.Context, filePaths []string) ([]models.Tape, error) {
	if len(filePaths) == 0 {
		return nil, nil
	}

	// Build query with placeholders
	placeholders := make([]string, len(filePaths))
	args := make([]interface{}, len(filePaths))
	for i, path := range filePaths {
		placeholders[i] = "?"
		args[i] = path
	}

	query := fmt.Sprintf(`
		SELECT DISTINCT t.id, t.barcode, t.label, t.status
		FROM tapes t
		JOIN backup_sets bs ON t.id = bs.tape_id
		JOIN catalog_entries ce ON bs.id = ce.backup_set_id
		WHERE ce.file_path IN (%s)
		ORDER BY bs.start_time DESC
	`, strings.Join(placeholders, ","))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tapes []models.Tape
	for rows.Next() {
		var t models.Tape
		if err := rows.Scan(&t.ID, &t.Barcode, &t.Label, &t.Status); err != nil {
			continue
		}
		tapes = append(tapes, t)
	}

	return tapes, nil
}

// DummyScanner for when we need to suppress some output
type DummyScanner struct{}

func (d *DummyScanner) Scan() bool   { return false }
func (d *DummyScanner) Text() string { return "" }

var _ interface{ Scan() bool } = &DummyScanner{}
var _ = bufio.Scanner{}
