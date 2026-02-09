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
	JobID                         int64     `json:"job_id"`
	JobName                       string    `json:"job_name"`
	BackupSetID                   int64     `json:"backup_set_id"`
	Phase                         string    `json:"phase"`
	Message                       string    `json:"message"`
	Status                        string    `json:"status"` // running, paused, cancelled
	FileCount                     int64     `json:"file_count"`
	TotalFiles                    int64     `json:"total_files"`
	TotalBytes                    int64     `json:"total_bytes"`
	BytesWritten                  int64     `json:"bytes_written"`
	WriteSpeed                    float64   `json:"write_speed"` // bytes per second (recent average)
	TapeLabel                     string    `json:"tape_label"`
	TapeCapacityBytes             int64     `json:"tape_capacity_bytes"`
	TapeUsedBytes                 int64     `json:"tape_used_bytes"` // used before this backup
	DevicePath                    string    `json:"device_path"`
	EstimatedSecondsRemaining     float64   `json:"estimated_seconds_remaining"`
	TapeEstimatedSecondsRemaining float64   `json:"tape_estimated_seconds_remaining"`
	StartTime                     time.Time `json:"start_time"`
	UpdatedAt                     time.Time `json:"updated_at"`
	LogLines                      []string  `json:"log_lines"`
	// Scan progress fields (populated during the "scanning" phase)
	ScanFilesFound int64 `json:"scan_files_found"`
	ScanDirsScanned int64 `json:"scan_dirs_scanned"`
	ScanBytesFound int64 `json:"scan_bytes_found"`
}

// ScanProgressFunc is a callback invoked periodically during ScanSource
// to report scanning progress (files found, dirs scanned, bytes found).
type ScanProgressFunc func(filesFound, dirsScanned, bytesFound int64)

// speedSample is a timestamped byte counter snapshot.
type speedSample struct {
	time  time.Time
	bytes int64
}

// speedTracker computes a rolling-window average write speed.
type speedTracker struct {
	window  time.Duration
	samples []speedSample
}

func newSpeedTracker(window time.Duration) *speedTracker {
	return &speedTracker{window: window}
}

// Record adds a sample and discards entries older than the window.
func (st *speedTracker) Record(now time.Time, bytes int64) {
	st.samples = append(st.samples, speedSample{time: now, bytes: bytes})
	cutoff := now.Add(-st.window)
	i := 0
	for i < len(st.samples) && st.samples[i].time.Before(cutoff) {
		i++
	}
	// Keep the most recent pre-cutoff sample as a baseline for the delta.
	if i > 0 {
		st.samples = st.samples[i-1:]
	}
}

// Speed returns the average bytes/sec over the window.  Returns 0 when
// fewer than two samples exist or the time span is too short.
func (st *speedTracker) Speed() float64 {
	if len(st.samples) < 2 {
		return 0
	}
	first := st.samples[0]
	last := st.samples[len(st.samples)-1]
	dt := last.time.Sub(first.time).Seconds()
	if dt < 1.0 {
		return 0
	}
	return float64(last.bytes-first.bytes) / dt
}

// EventCallback is called when backup progress events occur (for SSE/console)
type EventCallback func(eventType, category, title, message string)

// TapeChangeCallback is called when a tape change is required during multi-tape spanning.
// It allows the caller to send notifications (e.g. Telegram) with the exact next tape label.
type TapeChangeCallback func(ctx context.Context, jobName, currentTape, reason, nextTape string)

// WrongTapeCallback is called when the wrong tape (or no tape) is found in the drive
// during backup tape verification. It notifies the operator to insert the correct tape.
type WrongTapeCallback func(ctx context.Context, expectedLabel, actualLabel string)

// buildCompressionCmd returns the exec.Cmd for the given compression type.
// For gzip it uses pigz (parallel gzip) with -1 (fastest) when available,
// falling back to gzip -1. For zstd it uses automatic multi-threading.
func buildCompressionCmd(ctx context.Context, compression models.CompressionType) (*exec.Cmd, error) {
	switch compression {
	case models.CompressionGzip:
		if _, err := exec.LookPath("pigz"); err == nil {
			return exec.CommandContext(ctx, "pigz", "-1", "-c"), nil
		}
		return exec.CommandContext(ctx, "gzip", "-1", "-c"), nil
	case models.CompressionZstd:
		return exec.CommandContext(ctx, "zstd", "-T0", "-c", "--no-progress"), nil
	default:
		return nil, fmt.Errorf("unsupported compression type: %s", compression)
	}
}

// countingReader wraps an io.Reader and counts bytes read through it.
// It uses atomic operations instead of a mutex for the byte counter to avoid
// lock contention in the hot data path, and throttles the progress callback
// to fire at most once per second to reduce overhead from mutex acquisition,
// time.Now() calls, and float math in the callback.
type countingReader struct {
	reader       io.Reader
	count        int64 // accessed atomically
	lastCallback int64 // unix nanoseconds of last callback, accessed atomically
	callback     func(bytesRead int64)
	paused       *int32 // atomic: 0=running, 1=paused
}

func (cr *countingReader) Read(p []byte) (int, error) {
	// Check pause state
	for cr.paused != nil && atomic.LoadInt32(cr.paused) == 1 {
		time.Sleep(100 * time.Millisecond)
	}
	n, err := cr.reader.Read(p)
	if n > 0 {
		total := atomic.AddInt64(&cr.count, int64(n))
		if cr.callback != nil {
			now := time.Now().UnixNano()
			last := atomic.LoadInt64(&cr.lastCallback)
			// Throttle callback to at most once per second to reduce overhead
			if now-last >= int64(time.Second) {
				if atomic.CompareAndSwapInt64(&cr.lastCallback, last, now) {
					cr.callback(total)
				}
			}
		}
	}
	return n, err
}

func (cr *countingReader) bytesRead() int64 {
	return atomic.LoadInt64(&cr.count)
}

// countingWriter wraps an io.Writer and counts bytes written through it.
type countingWriter struct {
	writer io.Writer
	count  int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.writer.Write(p)
	atomic.AddInt64(&cw.count, int64(n))
	return n, err
}

func (cw *countingWriter) bytesWritten() int64 {
	return atomic.LoadInt64(&cw.count)
}

// Service handles backup operations
type Service struct {
	db            *database.DB
	tapeService   *tape.Service
	logger        *logging.Logger
	blockSize     int
	bufferSizeMB  int
	mu            sync.Mutex
	activeJobs    map[int64]*JobProgress
	cancelFuncs   map[int64]context.CancelFunc
	pauseFlags    map[int64]*int32
	resumeFiles        map[int64][]string // files already processed for resume
	EventCallback      EventCallback
	TapeChangeCallback TapeChangeCallback
	WrongTapeCallback  WrongTapeCallback
}

// NewService creates a new backup service
func NewService(db *database.DB, tapeService *tape.Service, logger *logging.Logger, blockSize int, bufferSizeMB int) *Service {
	if bufferSizeMB <= 0 {
		bufferSizeMB = 512
	}
	return &Service{
		db:           db,
		tapeService:  tapeService,
		logger:       logger,
		blockSize:    blockSize,
		bufferSizeMB: bufferSizeMB,
		activeJobs:   make(map[int64]*JobProgress),
		cancelFuncs:  make(map[int64]context.CancelFunc),
		pauseFlags:   make(map[int64]*int32),
		resumeFiles:  make(map[int64][]string),
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

// InjectTestJob adds a job directly into activeJobs for testing purposes.
func (s *Service) InjectTestJob(jobID int64, p *JobProgress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeJobs[jobID] = p
}

// RemoveTestJob removes a job from activeJobs for testing cleanup.
func (s *Service) RemoveTestJob(jobID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.activeJobs, jobID)
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

// PauseJob pauses a running backup job and persists state to database for restart resilience
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

			// Persist pause state to database for server restart resilience
			s.saveJobExecutionState(jobID, p)
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

// ScanSource scans a backup source and returns file information using concurrent directory traversal.
// An optional progressCb is invoked periodically to report scanning progress.
func (s *Service) ScanSource(ctx context.Context, source *models.BackupSource, progressCb ...ScanProgressFunc) ([]FileInfo, error) {
	// Parse include/exclude patterns
	var includePatterns, excludePatterns []string
	if source.IncludePatterns != "" {
		json.Unmarshal([]byte(source.IncludePatterns), &includePatterns)
	}
	if source.ExcludePatterns != "" {
		json.Unmarshal([]byte(source.ExcludePatterns), &excludePatterns)
	}

	// Use many more workers than CPUs: on NAS paths the bottleneck is network
	// latency (each stat blocks ~1-5 ms), so extra goroutines keep many
	// I/O requests in flight at the same time.
	numWorkers := runtime.NumCPU() * 4
	if numWorkers < 16 {
		numWorkers = 16
	}

	var (
		files    []FileInfo
		filesMu  sync.Mutex
		dirWg    sync.WaitGroup
		workerWg sync.WaitGroup
		dirs     = make(chan string, numWorkers*8)
	)

	// Separate exclude patterns into exact names (fast map lookup) vs globs.
	// Most exclude patterns are plain directory/file names like "#recycle" or
	// ".snapshots" which don't contain any glob meta-characters.  We can check
	// those with a O(1) map lookup instead of calling filepath.Match per
	// pattern per entry.
	excludeExact := make(map[string]struct{})
	var excludeGlobs []string
	for _, p := range excludePatterns {
		if strings.ContainsAny(p, "*?[") {
			excludeGlobs = append(excludeGlobs, p)
		} else {
			excludeExact[p] = struct{}{}
		}
	}

	includeExact := make(map[string]struct{})
	var includeGlobs []string
	for _, p := range includePatterns {
		if strings.ContainsAny(p, "*?[") {
			includeGlobs = append(includeGlobs, p)
		} else {
			includeExact[p] = struct{}{}
		}
	}

	// Atomic counters for scan progress
	var filesFound int64
	var dirsScanned int64
	var bytesFound int64
	var lastProgressCb int64 // unix nanos, throttle to once per second

	// emitProgress fires the optional progress callback at most once per second.
	var cb ScanProgressFunc
	if len(progressCb) > 0 {
		cb = progressCb[0]
	}
	emitProgress := func() {
		if cb == nil {
			return
		}
		now := time.Now().UnixNano()
		last := atomic.LoadInt64(&lastProgressCb)
		if now-last >= int64(time.Second) {
			if atomic.CompareAndSwapInt64(&lastProgressCb, last, now) {
				cb(atomic.LoadInt64(&filesFound), atomic.LoadInt64(&dirsScanned), atomic.LoadInt64(&bytesFound))
			}
		}
	}

	// shouldExcludeDir checks if a directory path matches any exclude pattern.
	// Fast path: exact name lookup via map.  Slow path: glob matching.
	shouldExcludeDir := func(path string) bool {
		baseName := filepath.Base(path)

		// Fast exact-match check (covers most real-world excludes)
		if _, ok := excludeExact[baseName]; ok {
			return true
		}

		// Only compute relPath and run globs if there are glob patterns
		if len(excludeGlobs) > 0 {
			relPath, _ := filepath.Rel(source.Path, path)
			if _, ok := excludeExact[relPath]; ok {
				return true
			}
			for _, pattern := range excludeGlobs {
				if matched, _ := filepath.Match(pattern, relPath); matched {
					return true
				}
				if matched, _ := filepath.Match(pattern, baseName); matched {
					return true
				}
			}
		}
		return false
	}

	// matchFile checks if a file path matches the include/exclude patterns.
	// Uses fast exact-match maps before falling back to glob matching.
	matchFile := func(path string) bool {
		baseName := filepath.Base(path)

		// Fast exact-match exclude check
		if _, ok := excludeExact[baseName]; ok {
			return false
		}

		// Glob exclude check (only if glob patterns exist)
		if len(excludeGlobs) > 0 {
			relPath, _ := filepath.Rel(source.Path, path)
			if _, ok := excludeExact[relPath]; ok {
				return false
			}
			for _, pattern := range excludeGlobs {
				if matched, _ := filepath.Match(pattern, relPath); matched {
					return false
				}
				if matched, _ := filepath.Match(pattern, baseName); matched {
					return false
				}
			}
		}

		// Check include patterns (if any)
		if len(includePatterns) > 0 {
			// Fast exact-match include check
			if _, ok := includeExact[baseName]; ok {
				return true
			}
			if len(includeGlobs) > 0 {
				relPath, _ := filepath.Rel(source.Path, path)
				if _, ok := includeExact[relPath]; ok {
					return true
				}
				for _, pattern := range includeGlobs {
					if matched, _ := filepath.Match(pattern, relPath); matched {
						return true
					}
					if matched, _ := filepath.Match(pattern, baseName); matched {
						return true
					}
				}
			}
			return false
		}

		return true
	}

	// readDir reads directory entries without sorting (avoids O(n log n)
	// overhead of os.ReadDir on directories with many files).
	readDir := func(dirPath string) ([]os.DirEntry, error) {
		f, err := os.Open(dirPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return f.ReadDir(-1)
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

		entries, err := readDir(dirPath)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("Error accessing path", map[string]interface{}{
					"path":  dirPath,
					"error": err.Error(),
				})
			}
			return
		}

		atomic.AddInt64(&dirsScanned, 1)

		var localFiles []FileInfo
		for _, entry := range entries {
			path := filepath.Join(dirPath, entry.Name())

			if entry.IsDir() {
				if shouldExcludeDir(path) {
					continue
				}
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
			var batchBytes int64
			for _, f := range localFiles {
				batchBytes += f.Size
			}
			atomic.AddInt64(&filesFound, int64(len(localFiles)))
			atomic.AddInt64(&bytesFound, batchBytes)

			filesMu.Lock()
			files = append(files, localFiles...)
			filesMu.Unlock()
		}

		// Always emit progress after each directory so the UI stays up to
		// date even when traversing large trees with few files.
		emitProgress()
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

	// Emit final progress callback so the caller sees the completed totals.
	if cb != nil {
		cb(atomic.LoadInt64(&filesFound), atomic.LoadInt64(&dirsScanned), atomic.LoadInt64(&bytesFound))
	}

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
func (s *Service) StreamToTape(ctx context.Context, sourcePath string, files []FileInfo, devicePath string, progressCb func(bytesWritten int64), pauseFlag *int32) (int64, error) {
	if len(files) == 0 {
		return 0, nil
	}

	// Create a file list for tar
	fileListPath := fmt.Sprintf("/tmp/tapebackarr-filelist-%d.txt", time.Now().UnixNano())
	fileList, err := os.Create(fileListPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create file list: %w", err)
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
		mbufferCmd := exec.CommandContext(ctx, "mbuffer", "-s", fmt.Sprintf("%d", s.blockSize), "-m", fmt.Sprintf("%dM", s.bufferSizeMB), "-o", devicePath)

		// Pipe tar output through counting reader to mbuffer
		tarCmd.Dir = sourcePath
		pipe, err := tarCmd.StdoutPipe()
		if err != nil {
			return 0, fmt.Errorf("failed to create pipe: %w", err)
		}

		cr := &countingReader{reader: pipe, callback: progressCb, paused: pauseFlag}
		mbufferCmd.Stdin = cr

		if err := tarCmd.Start(); err != nil {
			return 0, fmt.Errorf("failed to start tar: %w", err)
		}
		if err := mbufferCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			return 0, fmt.Errorf("failed to start mbuffer: %w", err)
		}

		tarErr := tarCmd.Wait()
		mbufferErr := mbufferCmd.Wait()

		if ctx.Err() != nil {
			return 0, fmt.Errorf("backup cancelled: %w", ctx.Err())
		}
		if tarErr != nil {
			return 0, fmt.Errorf("tar failed: %w", tarErr)
		}
		if mbufferErr != nil {
			return 0, fmt.Errorf("mbuffer failed: %w", mbufferErr)
		}
		// For uncompressed streams the tar bytes equal tape bytes
		return cr.bytesRead(), nil
	} else {
		// Direct tar to tape — no countingReader in this path.
		// Returns 0 so finishTape falls back to totalBytes (correct for
		// uncompressed streams where raw file size ≈ tape usage).
		tarArgs = append(tarArgs, "-f", devicePath)
		cmd = exec.CommandContext(ctx, "tar", tarArgs...)
		cmd.Dir = sourcePath

		output, err := cmd.CombinedOutput()
		if ctx.Err() != nil {
			return 0, fmt.Errorf("backup cancelled: %w", ctx.Err())
		}
		if err != nil {
			return 0, fmt.Errorf("tar failed: %s", string(output))
		}
	}

	return 0, nil
}

// StreamToTapeEncrypted streams files directly to tape with encryption using openssl
func (s *Service) StreamToTapeEncrypted(ctx context.Context, sourcePath string, files []FileInfo, devicePath string, encryptionKey string, progressCb func(bytesWritten int64), pauseFlag *int32) (int64, error) {
	if len(files) == 0 {
		return 0, nil
	}

	// Create a file list for tar
	fileListPath := fmt.Sprintf("/tmp/tapebackarr-filelist-%d.txt", time.Now().UnixNano())
	fileList, err := os.Create(fileListPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create file list: %w", err)
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
		return 0, fmt.Errorf("failed to create tar pipe: %w", err)
	}
	cr := &countingReader{reader: tarPipe, callback: progressCb, paused: pauseFlag}
	opensslCmd.Stdin = cr

	if mbufferErr == nil {
		// Use mbuffer for buffering before writing to tape
		mbufferCmd := exec.CommandContext(ctx, "mbuffer", "-s", fmt.Sprintf("%d", s.blockSize), "-m", fmt.Sprintf("%dM", s.bufferSizeMB), "-o", devicePath)

		opensslPipe, err := opensslCmd.StdoutPipe()
		if err != nil {
			return 0, fmt.Errorf("failed to create openssl pipe: %w", err)
		}
		// Count actual encrypted bytes going to tape
		tapeCr := &countingReader{reader: opensslPipe}
		mbufferCmd.Stdin = tapeCr

		// Start the pipeline
		if err := tarCmd.Start(); err != nil {
			return 0, fmt.Errorf("failed to start tar: %w", err)
		}
		if err := opensslCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			return 0, fmt.Errorf("failed to start openssl: %w", err)
		}
		if err := mbufferCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			opensslCmd.Process.Kill()
			return 0, fmt.Errorf("failed to start mbuffer: %w", err)
		}

		// Wait for all commands
		tarErr := tarCmd.Wait()
		opensslErr := opensslCmd.Wait()
		mbufferErr := mbufferCmd.Wait()

		if ctx.Err() != nil {
			return 0, fmt.Errorf("backup cancelled: %w", ctx.Err())
		}
		if tarErr != nil {
			return 0, fmt.Errorf("tar failed: %w", tarErr)
		}
		if opensslErr != nil {
			return 0, fmt.Errorf("openssl encryption failed: %w", opensslErr)
		}
		if mbufferErr != nil {
			return 0, fmt.Errorf("mbuffer failed: %w", mbufferErr)
		}
		return tapeCr.bytesRead(), nil
	} else {
		// Direct to tape device
		tapeFile, err := os.OpenFile(devicePath, os.O_WRONLY, 0)
		if err != nil {
			return 0, fmt.Errorf("failed to open tape device: %w", err)
		}
		defer tapeFile.Close()

		tapeCw := &countingWriter{writer: tapeFile}
		opensslCmd.Stdout = tapeCw

		if err := tarCmd.Start(); err != nil {
			return 0, fmt.Errorf("failed to start tar: %w", err)
		}
		if err := opensslCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			return 0, fmt.Errorf("failed to start openssl: %w", err)
		}

		tarErr := tarCmd.Wait()
		opensslErr := opensslCmd.Wait()

		if ctx.Err() != nil {
			return 0, fmt.Errorf("backup cancelled: %w", ctx.Err())
		}
		if tarErr != nil {
			return 0, fmt.Errorf("tar failed: %w", tarErr)
		}
		if opensslErr != nil {
			return 0, fmt.Errorf("openssl encryption failed: %w", opensslErr)
		}
		return tapeCw.bytesWritten(), nil
	}
}

// StreamToTapeCompressed streams files to tape with compression
func (s *Service) StreamToTapeCompressed(ctx context.Context, sourcePath string, files []FileInfo, devicePath string, compression models.CompressionType, progressCb func(bytesWritten int64), pauseFlag *int32) (int64, error) {
	if len(files) == 0 {
		return 0, nil
	}

	// Create a file list for tar
	fileListPath := fmt.Sprintf("/tmp/tapebackarr-filelist-%d.txt", time.Now().UnixNano())
	fileList, err := os.Create(fileListPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create file list: %w", err)
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

	tarCmd := exec.CommandContext(ctx, "tar", tarArgs...)
	tarCmd.Dir = sourcePath

	compCmd, err := buildCompressionCmd(ctx, compression)
	if err != nil {
		return 0, err
	}

	// Set up pipeline: tar -> countingReader -> compression -> tape
	tarPipe, err := tarCmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to create tar pipe: %w", err)
	}
	cr := &countingReader{reader: tarPipe, callback: progressCb, paused: pauseFlag}
	compCmd.Stdin = cr

	// Check if mbuffer is available
	_, mbufferErr := exec.LookPath("mbuffer")

	if mbufferErr == nil {
		mbufferCmd := exec.CommandContext(ctx, "mbuffer", "-s", fmt.Sprintf("%d", s.blockSize), "-m", fmt.Sprintf("%dM", s.bufferSizeMB), "-o", devicePath)
		compPipe, err := compCmd.StdoutPipe()
		if err != nil {
			return 0, fmt.Errorf("failed to create compression pipe: %w", err)
		}
		// Count actual compressed bytes going to tape
		tapeCr := &countingReader{reader: compPipe}
		mbufferCmd.Stdin = tapeCr

		if err := tarCmd.Start(); err != nil {
			return 0, fmt.Errorf("failed to start tar: %w", err)
		}
		if err := compCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			return 0, fmt.Errorf("failed to start compression: %w", err)
		}
		if err := mbufferCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			compCmd.Process.Kill()
			return 0, fmt.Errorf("failed to start mbuffer: %w", err)
		}

		tarErr := tarCmd.Wait()
		compErr := compCmd.Wait()
		mbufErr := mbufferCmd.Wait()

		if ctx.Err() != nil {
			return 0, fmt.Errorf("backup cancelled: %w", ctx.Err())
		}
		if tarErr != nil {
			return 0, fmt.Errorf("tar failed: %w", tarErr)
		}
		if compErr != nil {
			return 0, fmt.Errorf("compression failed: %w", compErr)
		}
		if mbufErr != nil {
			return 0, fmt.Errorf("mbuffer failed: %w", mbufErr)
		}
		return tapeCr.bytesRead(), nil
	} else {
		tapeFile, err := os.OpenFile(devicePath, os.O_WRONLY, 0)
		if err != nil {
			return 0, fmt.Errorf("failed to open tape device: %w", err)
		}
		defer tapeFile.Close()

		tapeCw := &countingWriter{writer: tapeFile}
		compCmd.Stdout = tapeCw

		if err := tarCmd.Start(); err != nil {
			return 0, fmt.Errorf("failed to start tar: %w", err)
		}
		if err := compCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			return 0, fmt.Errorf("failed to start compression: %w", err)
		}

		tarErr := tarCmd.Wait()
		compErr := compCmd.Wait()

		if ctx.Err() != nil {
			return 0, fmt.Errorf("backup cancelled: %w", ctx.Err())
		}
		if tarErr != nil {
			return 0, fmt.Errorf("tar failed: %w", tarErr)
		}
		if compErr != nil {
			return 0, fmt.Errorf("compression failed: %w", compErr)
		}
		return tapeCw.bytesWritten(), nil
	}
}

// StreamToTapeCompressedEncrypted streams files to tape with both compression and encryption
func (s *Service) StreamToTapeCompressedEncrypted(ctx context.Context, sourcePath string, files []FileInfo, devicePath string, compression models.CompressionType, encryptionKey string, progressCb func(bytesWritten int64), pauseFlag *int32) (int64, error) {
	if len(files) == 0 {
		return 0, nil
	}

	fileListPath := fmt.Sprintf("/tmp/tapebackarr-filelist-%d.txt", time.Now().UnixNano())
	fileList, err := os.Create(fileListPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create file list: %w", err)
	}
	defer os.Remove(fileListPath)

	for _, f := range files {
		relPath, _ := filepath.Rel(sourcePath, f.Path)
		fmt.Fprintln(fileList, relPath)
	}
	fileList.Close()

	tarArgs := []string{
		"-c",
		"-b", fmt.Sprintf("%d", s.blockSize/512),
		"-C", sourcePath,
		"-T", fileListPath,
	}

	tarCmd := exec.CommandContext(ctx, "tar", tarArgs...)
	tarCmd.Dir = sourcePath

	compCmd, err := buildCompressionCmd(ctx, compression)
	if err != nil {
		return 0, err
	}

	opensslCmd := exec.CommandContext(ctx, "openssl", "enc",
		"-aes-256-cbc", "-salt", "-pbkdf2", "-iter", "100000",
		"-pass", "pass:"+encryptionKey,
	)

	// Pipeline: tar -> countingReader -> compress -> encrypt -> tape
	tarPipe, err := tarCmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to create tar pipe: %w", err)
	}
	cr := &countingReader{reader: tarPipe, callback: progressCb, paused: pauseFlag}
	compCmd.Stdin = cr

	compPipe, err := compCmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to create compression pipe: %w", err)
	}
	opensslCmd.Stdin = compPipe

	_, mbufferErr := exec.LookPath("mbuffer")

	if mbufferErr == nil {
		mbufferCmd := exec.CommandContext(ctx, "mbuffer", "-s", fmt.Sprintf("%d", s.blockSize), "-m", fmt.Sprintf("%dM", s.bufferSizeMB), "-o", devicePath)
		opensslPipe, err := opensslCmd.StdoutPipe()
		if err != nil {
			return 0, fmt.Errorf("failed to create openssl pipe: %w", err)
		}
		// Count actual compressed+encrypted bytes going to tape
		tapeCr := &countingReader{reader: opensslPipe}
		mbufferCmd.Stdin = tapeCr

		if err := tarCmd.Start(); err != nil {
			return 0, fmt.Errorf("failed to start tar: %w", err)
		}
		if err := compCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			return 0, fmt.Errorf("failed to start compression: %w", err)
		}
		if err := opensslCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			compCmd.Process.Kill()
			return 0, fmt.Errorf("failed to start openssl: %w", err)
		}
		if err := mbufferCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			compCmd.Process.Kill()
			opensslCmd.Process.Kill()
			return 0, fmt.Errorf("failed to start mbuffer: %w", err)
		}

		tarErr := tarCmd.Wait()
		compErr := compCmd.Wait()
		opensslErr := opensslCmd.Wait()
		mbufErr := mbufferCmd.Wait()

		if ctx.Err() != nil {
			return 0, fmt.Errorf("backup cancelled: %w", ctx.Err())
		}
		if tarErr != nil {
			return 0, fmt.Errorf("tar failed: %w", tarErr)
		}
		if compErr != nil {
			return 0, fmt.Errorf("compression failed: %w", compErr)
		}
		if opensslErr != nil {
			return 0, fmt.Errorf("openssl encryption failed: %w", opensslErr)
		}
		if mbufErr != nil {
			return 0, fmt.Errorf("mbuffer failed: %w", mbufErr)
		}
		return tapeCr.bytesRead(), nil
	} else {
		tapeFile, err := os.OpenFile(devicePath, os.O_WRONLY, 0)
		if err != nil {
			return 0, fmt.Errorf("failed to open tape device: %w", err)
		}
		defer tapeFile.Close()

		tapeCw := &countingWriter{writer: tapeFile}
		opensslCmd.Stdout = tapeCw

		if err := tarCmd.Start(); err != nil {
			return 0, fmt.Errorf("failed to start tar: %w", err)
		}
		if err := compCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			return 0, fmt.Errorf("failed to start compression: %w", err)
		}
		if err := opensslCmd.Start(); err != nil {
			tarCmd.Process.Kill()
			compCmd.Process.Kill()
			return 0, fmt.Errorf("failed to start openssl: %w", err)
		}

		tarErr := tarCmd.Wait()
		compErr := compCmd.Wait()
		opensslErr := opensslCmd.Wait()

		if ctx.Err() != nil {
			return 0, fmt.Errorf("backup cancelled: %w", ctx.Err())
		}
		if tarErr != nil {
			return 0, fmt.Errorf("tar failed: %w", tarErr)
		}
		if compErr != nil {
			return 0, fmt.Errorf("compression failed: %w", compErr)
		}
		if opensslErr != nil {
			return 0, fmt.Errorf("openssl encryption failed: %w", opensslErr)
		}
		return tapeCw.bytesWritten(), nil
	}
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

// computeChecksumsAsync computes SHA256 checksums for all files concurrently,
// storing results in the provided sync.Map (path -> checksum string) AND
// inserting catalog entries into the database as each batch of checksums
// completes. This builds the catalog incrementally during streaming rather
// than in one large batch after streaming finishes, preventing the "stuck in
// cataloging" state for large file counts. The TOC file list is written to
// tape separately at the end by finishTape.
func (s *Service) computeChecksumsAsync(ctx context.Context, files []FileInfo, checksums *sync.Map, backupSetID int64, sourcePath string) {
	numWorkers := runtime.NumCPU()
	if numWorkers < 4 {
		numWorkers = 4
	}

	type catalogEntry struct {
		relPath  string
		fi       FileInfo
		checksum string
	}

	// Channel for catalog entries; writer goroutine batches them into DB transactions
	entryCh := make(chan catalogEntry, numWorkers*2)

	// Writer goroutine: batches catalog inserts for efficient DB writes
	var writerWg sync.WaitGroup
	writerWg.Add(1)
	go func() {
		defer writerWg.Done()
		const batchSize = 500
		batch := make([]catalogEntry, 0, batchSize)

		flush := func() {
			if len(batch) == 0 || s.db == nil {
				batch = batch[:0]
				return
			}
			tx, err := s.db.Begin()
			if err != nil {
				batch = batch[:0]
				return
			}
			stmt, err := tx.Prepare(`
				INSERT INTO catalog_entries (backup_set_id, file_path, file_size, file_mode, mod_time, checksum)
				VALUES (?, ?, ?, ?, ?, ?)
			`)
			if err != nil {
				tx.Rollback()
				batch = batch[:0]
				return
			}
			for _, e := range batch {
				if _, err := stmt.Exec(backupSetID, e.relPath, e.fi.Size, e.fi.Mode, e.fi.ModTime, e.checksum); err != nil {
					s.logger.Warn("Failed to insert catalog entry", map[string]interface{}{
						"file":  e.relPath,
						"error": err.Error(),
					})
				}
			}
			stmt.Close()
			tx.Commit()
			batch = batch[:0]
		}

		for entry := range entryCh {
			batch = append(batch, entry)
			if len(batch) >= batchSize {
				flush()
			}
		}
		flush() // remaining entries
	}()

	// Checksum workers
	sem := make(chan struct{}, numWorkers)
	var wg sync.WaitGroup

	for _, f := range files {
		if ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			break
		case sem <- struct{}{}:
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(fi FileInfo) {
			defer wg.Done()
			defer func() { <-sem }()
			select {
			case <-ctx.Done():
				return
			default:
			}
			checksum, err := s.CalculateChecksum(fi.Path)
			if err == nil {
				checksums.Store(fi.Path, checksum)
			}
			relPath, relErr := filepath.Rel(sourcePath, fi.Path)
			if relErr != nil {
				relPath = fi.Path // fall back to absolute path
			}
			entryCh <- catalogEntry{relPath: relPath, fi: fi, checksum: checksum}
		}(f)
	}
	wg.Wait()
	close(entryCh)
	writerWg.Wait()
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
		s.updateProgress(job.ID, "failed", "Failed to create backup set: "+err.Error())
		s.emitEvent("error", "backup", "Backup Failed", fmt.Sprintf("Job %s failed: %s", job.Name, err.Error()))
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

	scanCb := func(filesFound, dirsScanned, bytesFound int64) {
		s.mu.Lock()
		if p, ok := s.activeJobs[job.ID]; ok {
			p.ScanFilesFound = filesFound
			p.ScanDirsScanned = dirsScanned
			p.ScanBytesFound = bytesFound
			p.UpdatedAt = time.Now()
		}
		s.mu.Unlock()
	}

	files, err := s.ScanSource(ctx, source, scanCb)
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

	// Filter out already-processed files when resuming from a checkpoint
	s.mu.Lock()
	resumeFiles := s.resumeFiles[job.ID]
	s.mu.Unlock()
	if len(resumeFiles) > 0 {
		processedSet := make(map[string]bool, len(resumeFiles))
		for _, f := range resumeFiles {
			processedSet[f] = true
		}
		var remaining []FileInfo
		for _, f := range files {
			relPath, err := filepath.Rel(source.Path, f.Path)
			if err != nil {
				// If we can't compute relative path, include the file to be safe
				remaining = append(remaining, f)
				continue
			}
			if !processedSet[relPath] {
				remaining = append(remaining, f)
			}
		}
		skipped := len(files) - len(remaining)
		s.updateProgress(job.ID, "scanning", fmt.Sprintf("Resuming: skipping %d already-processed files, %d remaining", skipped, len(remaining)))
		s.logger.Info("Resume checkpoint applied", map[string]interface{}{
			"skipped":   skipped,
			"remaining": len(remaining),
		})
		files = remaining
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

	// Progress callback for real-time byte tracking (1-minute rolling average)
	tracker := newSpeedTracker(60 * time.Second)
	progressCb := func(bytesWritten int64) {
		now := time.Now()
		s.mu.Lock()
		if p, ok := s.activeJobs[job.ID]; ok {
			p.BytesWritten = bytesWritten
			// Estimate file count from bytes written for live progress display.
			// Assumes linear relationship between bytes and files (approximate when file sizes vary).
			if p.TotalFiles > 0 && p.TotalBytes > 0 {
				estimatedFiles := int64(float64(p.TotalFiles) * float64(bytesWritten) / float64(p.TotalBytes))
				if estimatedFiles > p.TotalFiles {
					estimatedFiles = p.TotalFiles
				}
				p.FileCount = estimatedFiles
			}
			tracker.Record(now, bytesWritten)
			speed := tracker.Speed()
			if speed > 0 {
				p.WriteSpeed = speed
				remainingBytes := p.TotalBytes - bytesWritten
				if remainingBytes > 0 {
					p.EstimatedSecondsRemaining = float64(remainingBytes) / speed
				} else {
					p.EstimatedSecondsRemaining = 0
				}
				// Calculate per-tape ETA
				if p.TapeCapacityBytes > 0 {
					tapeRemaining := p.TapeCapacityBytes - p.TapeUsedBytes - bytesWritten
					if tapeRemaining > 0 {
						p.TapeEstimatedSecondsRemaining = float64(tapeRemaining) / speed
					} else {
						p.TapeEstimatedSecondsRemaining = 0
					}
				}
			}
			p.UpdatedAt = now
		}
		s.mu.Unlock()
	}

	// Verify tape label before writing
	s.updateProgress(job.ID, "positioning", "Verifying tape label...")
	driveSvc := tape.NewServiceForDevice(devicePath, s.tapeService.GetBlockSize())

	// Read expected tape info from DB
	var expectedLabel, expectedUUID string
	if err := s.db.QueryRow("SELECT label, uuid FROM tapes WHERE id = ?", tapeID).Scan(&expectedLabel, &expectedUUID); err != nil {
		s.updateProgress(job.ID, "failed", "Failed to look up tape info: "+err.Error())
		s.updateBackupSetStatus(backupSetID, models.BackupSetStatusFailed, "failed to look up tape info")
		return nil, fmt.Errorf("failed to look up tape info: %w", err)
	}

	// Read and verify the physical tape label, retrying until the correct tape is inserted.
	// Instead of failing immediately when the tape is missing or wrong, we notify the
	// operator and wait for them to insert the correct tape.
	const tapeRetryInterval = 10 * time.Second
	notifiedUser := false
	for {
		// Check if a tape is loaded in the drive
		tapeLoaded, loadErr := driveSvc.IsTapeLoaded(ctx)
		if loadErr != nil {
			s.logger.Warn("Error checking tape status, will retry", map[string]interface{}{
				"error": loadErr.Error(), "tape": expectedLabel,
			})
		}

		if loadErr != nil || !tapeLoaded {
			// No tape in drive — notify operator and wait
			waitMsg := fmt.Sprintf("No tape found in drive %s. Please insert tape %q to continue backup job %q.",
				devicePath, expectedLabel, job.Name)
			s.updateProgress(job.ID, "waiting", waitMsg)
			if !notifiedUser {
				s.emitEvent("warning", "backup", "Tape Required",
					fmt.Sprintf("Job %s: no tape in drive. Please insert tape %s.", job.Name, expectedLabel))
				if s.WrongTapeCallback != nil {
					s.WrongTapeCallback(ctx, expectedLabel, "no tape loaded")
				}
				notifiedUser = true
			}
			select {
			case <-ctx.Done():
				s.updateProgress(job.ID, "failed", "Backup cancelled while waiting for tape")
				s.updateBackupSetStatus(backupSetID, models.BackupSetStatusFailed, "cancelled waiting for tape")
				return nil, ctx.Err()
			case <-time.After(tapeRetryInterval):
				continue
			}
		}

		// Tape is loaded — try to read the label
		physicalLabel, readErr := driveSvc.ReadTapeLabel(ctx)
		if readErr != nil {
			s.logger.Warn("Failed to read tape label, will retry", map[string]interface{}{
				"error": readErr.Error(), "tape": expectedLabel,
			})
			waitMsg := fmt.Sprintf("Cannot read tape label in drive %s (error: %s). Please check tape %q is correctly inserted.",
				devicePath, readErr.Error(), expectedLabel)
			s.updateProgress(job.ID, "waiting", waitMsg)
			if !notifiedUser {
				s.emitEvent("warning", "backup", "Tape Read Error",
					fmt.Sprintf("Job %s: cannot read tape label. Please check tape %s.", job.Name, expectedLabel))
				if s.WrongTapeCallback != nil {
					s.WrongTapeCallback(ctx, expectedLabel, "unreadable")
				}
				notifiedUser = true
			}
			select {
			case <-ctx.Done():
				s.updateProgress(job.ID, "failed", "Backup cancelled while waiting for tape")
				s.updateBackupSetStatus(backupSetID, models.BackupSetStatusFailed, "cancelled waiting for tape")
				return nil, ctx.Err()
			case <-time.After(tapeRetryInterval):
				continue
			}
		}

		// Check if the label matches
		if physicalLabel != nil && physicalLabel.Label == expectedLabel && physicalLabel.UUID == expectedUUID {
			// Correct tape found — break out of retry loop
			break
		}

		// Wrong tape or unlabeled tape — notify and retry
		actualLabel := "unlabeled"
		if physicalLabel != nil {
			actualLabel = physicalLabel.Label
		}
		var waitMsg string
		if physicalLabel != nil && physicalLabel.Label == expectedLabel {
			waitMsg = fmt.Sprintf("Tape UUID mismatch: expected %q but drive has %q (label %q). Please insert the correct tape %q.",
				expectedUUID, physicalLabel.UUID, actualLabel, expectedLabel)
		} else {
			waitMsg = fmt.Sprintf("Wrong tape in drive: expected %q but found %q. Please insert the correct tape.",
				expectedLabel, actualLabel)
		}
		s.updateProgress(job.ID, "waiting", waitMsg)
		s.emitEvent("warning", "backup", "Wrong Tape Inserted",
			fmt.Sprintf("Job %s: %s", job.Name, waitMsg))
		if s.WrongTapeCallback != nil {
			s.WrongTapeCallback(ctx, expectedLabel, actualLabel)
		}
		notifiedUser = true

		select {
		case <-ctx.Done():
			s.updateProgress(job.ID, "failed", "Backup cancelled while waiting for correct tape")
			s.updateBackupSetStatus(backupSetID, models.BackupSetStatusFailed, "cancelled waiting for correct tape")
			return nil, ctx.Err()
		case <-time.After(tapeRetryInterval):
			continue
		}
	}

	s.updateProgress(job.ID, "positioning", "Tape label verified, positioning past label...")

	// Position tape past the label. ReadTapeLabel already rewound, so we seek forward.
	if err := driveSvc.SeekToFileNumber(ctx, 1); err != nil {
		errMsg := fmt.Sprintf("Failed to position tape past label: %s - please check the tape and try again", err.Error())
		s.logger.Error("Failed to seek past label on tape", map[string]interface{}{"error": err.Error(), "tape": expectedLabel})
		s.updateProgress(job.ID, "failed", errMsg)
		s.updateBackupSetStatus(backupSetID, models.BackupSetStatusFailed, errMsg)
		s.emitEvent("error", "backup", "Tape Positioning Failed", fmt.Sprintf("Job %s failed: %s", job.Name, errMsg))
		return nil, fmt.Errorf("failed to position tape past label: %w", err)
	}

	// Record the tape position so restore can seek directly to the data
	_, startBlock, posErr := driveSvc.GetTapePosition(ctx)
	if posErr != nil {
		s.logger.Warn("Could not read tape position for start_block", map[string]interface{}{"error": posErr.Error()})
	} else {
		if _, dbErr := s.db.Exec("UPDATE backup_sets SET start_block = ? WHERE id = ?", startBlock, backupSetID); dbErr != nil {
			s.logger.Warn("Failed to record start_block", map[string]interface{}{"error": dbErr.Error()})
		} else {
			s.logger.Info("Recorded backup start block", map[string]interface{}{
				"start_block":   startBlock,
				"backup_set_id": backupSetID,
			})
		}
	}

	// Determine encryption and compression settings
	var encrypted bool
	var encryptionKeyID *int64
	var compressed bool
	var compressionType models.CompressionType
	// CompressionLTO means "let the LTO drive handle compression" — no software
	// compression is applied. The drive performs block-level compression at full
	// streaming speed with no host CPU involvement.
	useCompression := job.Compression != "" &&
		job.Compression != models.CompressionNone &&
		job.Compression != models.CompressionLTO
	var encKey string

	if job.EncryptionEnabled && job.EncryptionKeyID != nil {
		key, err := s.GetEncryptionKey(ctx, *job.EncryptionKeyID)
		if err != nil {
			s.updateProgress(job.ID, "failed", "Encryption key not found: "+err.Error())
			s.updateBackupSetStatus(backupSetID, models.BackupSetStatusFailed, "encryption key not found: "+err.Error())
			return nil, fmt.Errorf("failed to get encryption key: %w", err)
		}
		encKey = key
		encrypted = true
		encryptionKeyID = job.EncryptionKeyID
	}
	if useCompression {
		compressed = true
		compressionType = job.Compression
	}

	// streamFailed is a helper to save state on stream failure for retry capability
	streamFailed := func(errMsg string) {
		s.mu.Lock()
		if p, ok := s.activeJobs[job.ID]; ok {
			s.saveFailedJobState(job.ID, p, errMsg)
		}
		s.mu.Unlock()
	}

	// streamBatch streams a batch of files to the tape device with the configured
	// encryption and compression settings. Returns actual bytes written to tape.
	streamBatch := func(batch []FileInfo) (int64, error) {
		var batchBytes int64
		for _, f := range batch {
			batchBytes += f.Size
		}

		if encrypted && useCompression {
			s.updateProgress(job.ID, "streaming", fmt.Sprintf("Compressing (%s), encrypting and streaming %d files to tape %s...", job.Compression, len(batch), expectedLabel))
			return s.StreamToTapeCompressedEncrypted(ctx, source.Path, batch, devicePath, job.Compression, encKey, progressCb, &pauseFlag)
		} else if encrypted {
			s.updateProgress(job.ID, "streaming", fmt.Sprintf("Encrypting and streaming %d files to tape %s...", len(batch), expectedLabel))
			return s.StreamToTapeEncrypted(ctx, source.Path, batch, devicePath, encKey, progressCb, &pauseFlag)
		} else if useCompression {
			s.updateProgress(job.ID, "streaming", fmt.Sprintf("Compressing (%s) and streaming %d files to tape %s...", job.Compression, len(batch), expectedLabel))
			return s.StreamToTapeCompressed(ctx, source.Path, batch, devicePath, job.Compression, progressCb, &pauseFlag)
		}
		s.updateProgress(job.ID, "streaming", fmt.Sprintf("Streaming %d files to tape %s...", len(batch), expectedLabel))
		return s.StreamToTape(ctx, source.Path, batch, devicePath, progressCb, &pauseFlag)
	}

	// Start concurrent checksum computation for all files. Checksums are
	// computed in parallel with tape streaming and catalog entries are written
	// to the database incrementally as they complete — no post-streaming
	// cataloging bottleneck. The TOC file list is written to tape at the end.
	fileChecksums := &sync.Map{}
	go s.computeChecksumsAsync(ctx, files, fileChecksums, backupSetID, source.Path)

	// Check if all files fit on the current tape
	remainingCapacity := tapeCapacity - tapeUsed
	_, overflow := s.splitFilesForTape(files, remainingCapacity)

	if overflow == nil {
		// --- Single tape path: all files fit on this tape ---
		s.updateProgress(job.ID, "streaming", fmt.Sprintf("Streaming %d files (%d bytes) to tape device %s", len(files), totalBytes, devicePath))
		s.logger.Info("Streaming to tape (single tape)", map[string]interface{}{
			"device":      devicePath,
			"file_count":  len(files),
			"total_bytes": totalBytes,
			"encrypted":   encrypted,
		})

		actualTapeBytes, err := streamBatch(files)
		if err != nil {
			s.updateProgress(job.ID, "failed", "Stream failed: "+err.Error())
			s.updateBackupSetStatus(backupSetID, models.BackupSetStatusFailed, err.Error())
			s.emitEvent("error", "backup", "Backup Failed", fmt.Sprintf("Job %s failed: %s", job.Name, err.Error()))
			streamFailed(err.Error())
			return nil, fmt.Errorf("failed to stream to tape: %w", err)
		}

		if err := s.finishTape(finishTapeParams{
			ctx: ctx, job: job, source: source,
			backupSetID: backupSetID, initialBackupSetID: backupSetID,
			tapeID: tapeID,
			tapeLabel: expectedLabel, tapeUUID: expectedUUID,
			driveSvc: driveSvc, files: files, totalBytes: totalBytes,
			actualTapeBytes: actualTapeBytes,
			backupType: backupType, encrypted: encrypted,
			encryptionKeyID: encryptionKeyID, compressed: compressed,
			compressionType: compressionType, startTime: startTime,
			checksums: fileChecksums,
		}); err != nil {
			s.logger.Warn("finishTape failed", map[string]interface{}{"error": err.Error()})
		}
	} else {
		// --- Multi-tape spanning path ---
		s.logger.Info("Backup requires multiple tapes", map[string]interface{}{
			"total_bytes":        totalBytes,
			"remaining_capacity": remainingCapacity,
			"file_count":         len(files),
		})
		s.emitEvent("info", "backup", "Multi-Tape Backup", fmt.Sprintf("Job %s requires multiple tapes — spanning enabled", job.Name))

		// Create spanning set record
		spanResult, err := s.db.Exec(`
			INSERT INTO tape_spanning_sets (job_id, total_bytes, total_files, status)
			VALUES (?, ?, ?, 'in_progress')
		`, job.ID, totalBytes, len(files))
		if err != nil {
			s.updateProgress(job.ID, "failed", "Failed to create spanning set: "+err.Error())
			s.updateBackupSetStatus(backupSetID, models.BackupSetStatusFailed, err.Error())
			return nil, fmt.Errorf("failed to create spanning set: %w", err)
		}
		spanningSetID, _ := spanResult.LastInsertId()
		usedTapeIDs := []int64{tapeID}

		remaining := files
		seqNum := 0
		currentTapeID := tapeID
		currentLabel := expectedLabel
		currentUUID := expectedUUID
		currentDriveSvc := driveSvc
		currentBackupSetID := backupSetID
		filesStartIndex := 0

		for len(remaining) > 0 {
			seqNum++

			// Refresh remaining capacity for the current tape
			var curCapacity, curUsed int64
			if err := s.db.QueryRow("SELECT capacity_bytes, used_bytes FROM tapes WHERE id = ?", currentTapeID).Scan(&curCapacity, &curUsed); err != nil {
				curCapacity = tapeCapacity
				curUsed = tapeUsed
			}
			curRemaining := curCapacity - curUsed

			batch, rest := s.splitFilesForTape(remaining, curRemaining)
			if len(batch) == 0 {
				// Tape has no usable capacity — need a new one immediately
				rest = remaining
				batch = nil
			}

			if batch != nil {
				var batchBytes int64
				for _, f := range batch {
					batchBytes += f.Size
				}

				// For tapes after the first, we need a new backup set
				if seqNum > 1 {
					setResult, err := s.db.Exec(`
						INSERT INTO backup_sets (job_id, tape_id, backup_type, start_time, status)
						VALUES (?, ?, ?, ?, ?)
					`, job.ID, currentTapeID, backupType, time.Now(), models.BackupSetStatusRunning)
					if err != nil {
						s.updateProgress(job.ID, "failed", "Failed to create backup set for tape "+currentLabel+": "+err.Error())
						s.db.Exec("UPDATE tape_spanning_sets SET status = 'failed' WHERE id = ?", spanningSetID)
						return nil, fmt.Errorf("failed to create backup set: %w", err)
					}
					currentBackupSetID, _ = setResult.LastInsertId()
				}

				s.logger.Info("Streaming batch to tape", map[string]interface{}{
					"tape_label":      currentLabel,
					"sequence":        seqNum,
					"batch_files":     len(batch),
					"batch_bytes":     batchBytes,
					"remaining_files": len(rest),
				})

				actualBatchBytes, err := streamBatch(batch)
				if err != nil {
					s.updateProgress(job.ID, "failed", "Stream failed on tape "+currentLabel+": "+err.Error())
					s.updateBackupSetStatus(currentBackupSetID, models.BackupSetStatusFailed, err.Error())
					s.emitEvent("error", "backup", "Backup Failed", fmt.Sprintf("Job %s failed on tape %s: %s", job.Name, currentLabel, err.Error()))
					s.db.Exec("UPDATE tape_spanning_sets SET status = 'failed' WHERE id = ?", spanningSetID)
					streamFailed(err.Error())
					return nil, fmt.Errorf("failed to stream to tape %s: %w", currentLabel, err)
				}

				// Finish this tape with its per-tape TOC
				if err := s.finishTape(finishTapeParams{
					ctx: ctx, job: job, source: source,
					backupSetID: currentBackupSetID, initialBackupSetID: backupSetID,
					tapeID: currentTapeID,
					tapeLabel: currentLabel, tapeUUID: currentUUID,
					pool: "", driveSvc: currentDriveSvc,
					files: batch, totalBytes: batchBytes,
					actualTapeBytes: actualBatchBytes,
					backupType: backupType, encrypted: encrypted,
					encryptionKeyID: encryptionKeyID, compressed: compressed,
					compressionType: compressionType, startTime: startTime,
					spanningSetID: spanningSetID, sequenceNumber: seqNum,
					checksums: fileChecksums,
				}); err != nil {
					s.logger.Warn("finishTape failed", map[string]interface{}{
						"tape_label": currentLabel,
						"error":      err.Error(),
					})
				}

				// Record spanning member
				s.db.Exec(`
					INSERT INTO tape_spanning_members (spanning_set_id, tape_id, backup_set_id, sequence_number, bytes_written, files_start_index, files_end_index)
					VALUES (?, ?, ?, ?, ?, ?, ?)
				`, spanningSetID, currentTapeID, currentBackupSetID, seqNum,
					batchBytes, filesStartIndex, filesStartIndex+len(batch)-1)
				filesStartIndex += len(batch)
			}

			remaining = rest
			if len(remaining) == 0 {
				break
			}

			// Auto-eject the completed tape so the operator can swap it
			if ejectErr := currentDriveSvc.Eject(ctx); ejectErr != nil {
				s.logger.Warn("Failed to auto-eject completed tape", map[string]interface{}{
					"tape_label": currentLabel,
					"error":      ejectErr.Error(),
				})
			}

			// Try to allocate the next tape from the pool
			nextTapeID, allocErr := s.allocateNextTape(ctx, job.PoolID, usedTapeIDs)
			var nextTapeLabel string
			if allocErr != nil {
				s.logger.Warn("Could not auto-allocate next tape", map[string]interface{}{"error": allocErr.Error()})
			} else {
				if err := s.db.QueryRow("SELECT label FROM tapes WHERE id = ?", nextTapeID).Scan(&nextTapeLabel); err != nil {
					s.logger.Warn("Could not look up next tape label", map[string]interface{}{"tape_id": nextTapeID, "error": err.Error()})
				}
			}

			// Need another tape — request a change
			if nextTapeLabel != "" {
				s.updateProgress(job.ID, "waiting", fmt.Sprintf("Tape %s complete. Waiting for next tape %s... (%d files remaining)", currentLabel, nextTapeLabel, len(remaining)))
				s.emitEvent("warning", "backup", "Tape Change Required",
					fmt.Sprintf("Job %s: tape %s is full. Please load tape %s. %d files remaining.", job.Name, currentLabel, nextTapeLabel, len(remaining)))
			} else {
				s.updateProgress(job.ID, "waiting", fmt.Sprintf("Tape %s complete. Waiting for next tape... (%d files remaining)", currentLabel, len(remaining)))
				s.emitEvent("warning", "backup", "Tape Change Required",
					fmt.Sprintf("Job %s: tape %s is full. Please load a new tape from the pool. %d files remaining.", job.Name, currentLabel, len(remaining)))
			}

			// Send notification (e.g. Telegram) about the tape change
			if s.TapeChangeCallback != nil {
				s.TapeChangeCallback(ctx, job.Name, currentLabel, "tape_full", nextTapeLabel)
			}

			reqID, err := s.createTapeChangeRequest(ctx, currentTapeID, spanningSetID, "tape_full")
			if err != nil {
				s.updateProgress(job.ID, "failed", "Failed to create tape change request: "+err.Error())
				s.db.Exec("UPDATE tape_spanning_sets SET status = 'failed' WHERE id = ?", spanningSetID)
				return nil, fmt.Errorf("failed to create tape change request: %w", err)
			}

			// If we auto-allocated a tape, pre-fill the request with it for the operator to confirm
			if allocErr == nil && nextTapeID > 0 {
				s.db.Exec("UPDATE tape_change_requests SET new_tape_id = ? WHERE id = ?", nextTapeID, reqID)
			}

			// Wait for operator to complete the tape change
			newTapeID, err := s.waitForTapeChange(ctx, reqID)
			if err != nil {
				s.updateProgress(job.ID, "failed", "Tape change failed: "+err.Error())
				s.db.Exec("UPDATE tape_spanning_sets SET status = 'failed' WHERE id = ?", spanningSetID)
				return nil, fmt.Errorf("tape change failed: %w", err)
			}

			// Set up the new tape
			currentTapeID = newTapeID
			usedTapeIDs = append(usedTapeIDs, currentTapeID)

			if err := s.db.QueryRow("SELECT label, uuid FROM tapes WHERE id = ?", currentTapeID).Scan(&currentLabel, &currentUUID); err != nil {
				s.updateProgress(job.ID, "failed", "Failed to look up new tape info: "+err.Error())
				s.db.Exec("UPDATE tape_spanning_sets SET status = 'failed' WHERE id = ?", spanningSetID)
				return nil, fmt.Errorf("failed to look up new tape: %w", err)
			}

			// Find the drive with the new tape loaded
			if err := s.db.QueryRow("SELECT device_path FROM tape_drives WHERE current_tape_id = ?", currentTapeID).Scan(&devicePath); err != nil {
				s.updateProgress(job.ID, "failed", "No drive found with new tape "+currentLabel)
				s.db.Exec("UPDATE tape_spanning_sets SET status = 'failed' WHERE id = ?", spanningSetID)
				return nil, fmt.Errorf("no drive found with new tape %s: %w", currentLabel, err)
			}
			currentDriveSvc = tape.NewServiceForDevice(devicePath, s.tapeService.GetBlockSize())

			// Verify and position the new tape
			physLabel, readErr := currentDriveSvc.ReadTapeLabel(ctx)
			if readErr != nil || physLabel == nil || physLabel.Label != currentLabel || physLabel.UUID != currentUUID {
				errMsg := fmt.Sprintf("new tape label verification failed for %s", currentLabel)
				s.updateProgress(job.ID, "failed", errMsg)
				s.db.Exec("UPDATE tape_spanning_sets SET status = 'failed' WHERE id = ?", spanningSetID)
				return nil, fmt.Errorf("%s", errMsg)
			}
			if err := currentDriveSvc.SeekToFileNumber(ctx, 1); err != nil {
				errMsg := fmt.Sprintf("failed to position new tape %s: %s", currentLabel, err.Error())
				s.updateProgress(job.ID, "failed", errMsg)
				s.db.Exec("UPDATE tape_spanning_sets SET status = 'failed' WHERE id = ?", spanningSetID)
				return nil, fmt.Errorf("%s", errMsg)
			}

			// Record the tape position so restore can seek directly to the data
			if _, spanStartBlock, posErr := currentDriveSvc.GetTapePosition(ctx); posErr == nil {
				if _, dbErr := s.db.Exec("UPDATE backup_sets SET start_block = ? WHERE id = ?", spanStartBlock, currentBackupSetID); dbErr != nil {
					s.logger.Warn("Failed to record start_block for spanning tape", map[string]interface{}{"error": dbErr.Error()})
				}
			}

			// Update progress for the new tape
			var newCapacity, newUsed int64
			s.db.QueryRow("SELECT capacity_bytes, used_bytes FROM tapes WHERE id = ?", currentTapeID).Scan(&newCapacity, &newUsed)
			s.mu.Lock()
			if p, ok := s.activeJobs[job.ID]; ok {
				p.TapeLabel = currentLabel
				p.TapeCapacityBytes = newCapacity
				p.TapeUsedBytes = newUsed
				p.DevicePath = devicePath
			}
			s.mu.Unlock()

			s.updateProgress(job.ID, "streaming", fmt.Sprintf("Continuing backup on tape %s (%d files remaining)", currentLabel, len(remaining)))
		}

		// Update spanning set as completed
		s.db.Exec("UPDATE tape_spanning_sets SET total_tapes = ?, status = 'completed', updated_at = CURRENT_TIMESTAMP WHERE id = ?",
			seqNum, spanningSetID)

		// Update TOC total_tapes on all member tapes (now that we know the final count).
		// This is informational — each tape already has its per-tape TOC written.
		s.logger.Info("Spanning backup completed", map[string]interface{}{
			"spanning_set_id": spanningSetID,
			"total_tapes":     seqNum,
			"total_files":     len(files),
			"total_bytes":     totalBytes,
		})
	}

	// Save snapshot for future incremental backups
	snapshotData, _ := s.CreateSnapshot(files)
	s.db.Exec(`
		INSERT INTO snapshots (source_id, backup_set_id, file_count, total_bytes, snapshot_data)
		VALUES (?, ?, ?, ?, ?)
	`, source.ID, backupSetID, len(files), totalBytes, snapshotData)

	// Update job last run
	endTime := time.Now()
	s.db.Exec("UPDATE backup_jobs SET last_run_at = ? WHERE id = ?", endTime, job.ID)

	s.updateProgress(job.ID, "completed", fmt.Sprintf("Backup completed: %d files, %d bytes in %s", len(files), totalBytes, endTime.Sub(startTime).String()))
	s.emitEvent("success", "backup", "Backup Completed", fmt.Sprintf("Job %s completed: %d files, %d bytes in %s", job.Name, len(files), totalBytes, endTime.Sub(startTime).String()))
	s.logger.Info("Backup completed", map[string]interface{}{
		"job_id":      job.ID,
		"file_count":  len(files),
		"total_bytes": totalBytes,
		"duration":    endTime.Sub(startTime).String(),
		"encrypted":   encrypted,
		"compressed":  compressed,
		"compression": compressionType,
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
		Compressed:      compressed,
		CompressionType: compressionType,
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

// finishTapeParams holds the parameters for finalizing a single tape during a backup.
type finishTapeParams struct {
	ctx                context.Context
	job                *models.BackupJob
	source             *models.BackupSource
	backupSetID        int64
	initialBackupSetID int64 // the backupSetID used during checksum computation; for multi-tape tape 2+, catalog entries are reassigned from this ID
	tapeID             int64
	tapeLabel          string
	tapeUUID           string
	pool               string
	driveSvc           *tape.Service
	files              []FileInfo // only the files written to this specific tape
	totalBytes         int64
	actualTapeBytes    int64 // actual bytes written to tape device (post-compression); 0 means use totalBytes
	backupType         models.BackupType
	encrypted          bool
	encryptionKeyID    *int64
	compressed         bool
	compressionType    models.CompressionType
	startTime          time.Time
	spanningSetID      int64 // 0 if not spanning
	sequenceNumber     int   // 1-based tape index within spanning set
	totalTapes         int   // 0 if not yet known (updated later)
	checksums          *sync.Map // pre-computed file checksums (path -> string), computed concurrently during streaming
}

// finishTape writes the per-tape TOC and updates catalog/tape records for
// the files written to this specific tape. Each tape in a multi-tape backup
// receives a self-describing TOC containing only its own files.
//
// Catalog entries (file checksums) are already written to the database
// incrementally by computeChecksumsAsync during streaming. finishTape only
// needs to reassign entries to the correct backup_set_id for multi-tape
// spanning (tape 2+), then write the TOC file list to tape.
func (s *Service) finishTape(p finishTapeParams) error {
	s.updateProgress(p.job.ID, "cataloging", "Writing file mark...")
	if err := p.driveSvc.WriteFileMark(p.ctx); err != nil {
		s.logger.Warn("Failed to write file mark", map[string]interface{}{"error": err.Error()})
	}

	// For multi-tape tape 2+, catalog entries were inserted with the initial
	// backup_set_id during checksum computation. Reassign them to this tape's
	// backup_set_id so restore knows which files are on which tape.
	if p.initialBackupSetID != 0 && p.initialBackupSetID != p.backupSetID && len(p.files) > 0 {
		s.updateProgress(p.job.ID, "cataloging", fmt.Sprintf("Reassigning %d catalog entries to tape %s...", len(p.files), p.tapeLabel))
		for _, f := range p.files {
			select {
			case <-p.ctx.Done():
				return p.ctx.Err()
			default:
			}
			relPath, relErr := filepath.Rel(p.source.Path, f.Path)
			if relErr != nil {
				relPath = f.Path
			}
			if _, err := s.db.Exec(`UPDATE catalog_entries SET backup_set_id = ? WHERE backup_set_id = ? AND file_path = ?`,
				p.backupSetID, p.initialBackupSetID, relPath); err != nil {
				s.logger.Warn("Failed to reassign catalog entry", map[string]interface{}{
					"file":  relPath,
					"error": err.Error(),
				})
			}
		}
	}

	// Write per-tape TOC containing only the files on this tape
	s.updateProgress(p.job.ID, "cataloging", fmt.Sprintf("Writing TOC for tape %s (%d files)...", p.tapeLabel, len(p.files)))
	tocData := tape.NewTapeTOC(p.tapeLabel, p.tapeUUID, p.pool)
	tocData.SpanningSetID = p.spanningSetID
	tocData.SequenceNumber = p.sequenceNumber
	tocData.TotalTapes = p.totalTapes

	tocBackupSet := tape.TOCBackupSet{
		FileNumber:      1,
		JobName:         p.job.Name,
		BackupType:      string(p.backupType),
		StartTime:       p.startTime,
		EndTime:         time.Now(),
		FileCount:       int64(len(p.files)),
		TotalBytes:      p.totalBytes,
		Encrypted:       p.encrypted,
		Compressed:      p.compressed,
		CompressionType: string(p.compressionType),
		Files:           make([]tape.TOCFileEntry, 0, len(p.files)),
	}
	for _, f := range p.files {
		// Respect context cancellation during TOC building
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()
		default:
		}

		relPath, _ := filepath.Rel(p.source.Path, f.Path)
		// Use pre-computed checksum directly instead of querying DB per-file
		var checksum string
		if p.checksums != nil {
			if val, ok := p.checksums.Load(f.Path); ok {
				checksum = val.(string)
			}
		}
		tocBackupSet.Files = append(tocBackupSet.Files, tape.TOCFileEntry{
			Path:     relPath,
			Size:     f.Size,
			Mode:     f.Mode,
			ModTime:  f.ModTime.Format(time.RFC3339),
			Checksum: checksum,
		})
	}
	tocData.BackupSets = append(tocData.BackupSets, tocBackupSet)
	if err := p.driveSvc.WriteTOC(p.ctx, tocData); err != nil {
		s.logger.Warn("Failed to write TOC to tape", map[string]interface{}{"error": err.Error()})
	} else {
		s.logger.Info("Per-tape TOC written", map[string]interface{}{
			"file_count":      len(p.files),
			"tape_label":      p.tapeLabel,
			"spanning_set_id": p.spanningSetID,
			"sequence_number": p.sequenceNumber,
		})
	}

	// Update backup set for this tape
	endTime := time.Now()
	s.db.Exec(`
		UPDATE backup_sets SET 
			end_time = ?, status = ?, file_count = ?, total_bytes = ?,
			encrypted = ?, encryption_key_id = ?, compressed = ?, compression_type = ?
		WHERE id = ?
	`, endTime, models.BackupSetStatusCompleted, len(p.files), p.totalBytes,
		p.encrypted, p.encryptionKeyID, p.compressed, p.compressionType, p.backupSetID)

	// Update tape usage — use actual bytes written to tape (post-compression)
	// when available, so compressed backups don't overestimate tape consumption.
	tapeUsageDelta := p.totalBytes
	if p.actualTapeBytes > 0 {
		tapeUsageDelta = p.actualTapeBytes
	}
	s.db.Exec(`
		UPDATE tapes SET 
			used_bytes = used_bytes + ?, write_count = write_count + 1,
			last_written_at = ?,
			status = CASE WHEN status = 'blank' THEN 'active' ELSE status END
		WHERE id = ?
	`, tapeUsageDelta, endTime, p.tapeID)

	// Track encryption key on tape if applicable
	if p.encrypted && p.encryptionKeyID != nil {
		var keyFingerprint, keyName string
		if err := s.db.QueryRow("SELECT key_fingerprint, name FROM encryption_keys WHERE id = ?", *p.encryptionKeyID).Scan(&keyFingerprint, &keyName); err != nil {
			s.logger.Warn("Failed to look up encryption key for tape tracking", map[string]interface{}{
				"encryption_key_id": *p.encryptionKeyID,
				"error":             err.Error(),
			})
		} else if keyFingerprint != "" {
			if _, err := s.db.Exec("UPDATE tapes SET encryption_key_fingerprint = ?, encryption_key_name = ? WHERE id = ?", keyFingerprint, keyName, p.tapeID); err != nil {
				s.logger.Warn("Failed to update tape encryption tracking", map[string]interface{}{
					"tape_id": p.tapeID,
					"error":   err.Error(),
				})
			}
		}
	}

	return nil
}

// splitFilesForTape partitions files into a batch that fits on the current tape
// and the remaining files for subsequent tapes. It reserves ~1% of remaining
// capacity for tar headers, file marks, and the TOC. The overhead is small:
// tar headers are ~1KB per file, file marks are negligible, and the TOC is a
// small JSON document. A 1% reserve is more than sufficient and avoids wasting
// significant tape capacity (e.g. 10% of a 1.5TB tape = 150GB unused).
func (s *Service) splitFilesForTape(files []FileInfo, remainingCapacity int64) (thisTape []FileInfo, remaining []FileInfo) {
	usableCapacity := (remainingCapacity * 99) / 100
	if usableCapacity <= 0 {
		return nil, files
	}

	var currentSize int64
	for i, f := range files {
		// Account for tar header overhead (~1KB per file including padding)
		fileWithOverhead := f.Size + 1024
		if currentSize+fileWithOverhead > usableCapacity && i > 0 {
			return files[:i], files[i:]
		}
		currentSize += fileWithOverhead
	}
	return files, nil
}

// allocateNextTape finds the next available tape in the given pool, excluding
// tapes already used in this backup. Returns the tape ID or an error.
func (s *Service) allocateNextTape(ctx context.Context, poolID int64, excludeTapeIDs []int64) (int64, error) {
	query := `
		SELECT id FROM tapes
		WHERE pool_id = ? AND status IN ('active', 'blank')
		AND (capacity_bytes - used_bytes) > 0
	`
	args := []interface{}{poolID}

	if len(excludeTapeIDs) > 0 {
		placeholders := make([]string, len(excludeTapeIDs))
		for i, id := range excludeTapeIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += " AND id NOT IN (" + strings.Join(placeholders, ",") + ")"
	}

	query += " ORDER BY used_bytes ASC LIMIT 1"

	var nextTapeID int64
	if err := s.db.QueryRow(query, args...).Scan(&nextTapeID); err != nil {
		return 0, fmt.Errorf("no available tape in pool: %w", err)
	}
	return nextTapeID, nil
}

// createTapeChangeRequest inserts a tape change request and notifies the operator.
func (s *Service) createTapeChangeRequest(ctx context.Context, currentTapeID int64, spanningSetID int64, reason string) (int64, error) {
	result, err := s.db.Exec(`
		INSERT INTO tape_change_requests (spanning_set_id, current_tape_id, reason, status, requested_at)
		VALUES (?, ?, ?, 'pending', CURRENT_TIMESTAMP)
	`, spanningSetID, currentTapeID, reason)
	if err != nil {
		return 0, fmt.Errorf("failed to create tape change request: %w", err)
	}
	id, _ := result.LastInsertId()
	return id, nil
}

// waitForTapeChange polls the tape_change_requests table until the request is
// completed by the operator loading a new tape. Returns the new tape ID.
func (s *Service) waitForTapeChange(ctx context.Context, requestID int64) (int64, error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Cancel the pending request
			s.db.Exec("UPDATE tape_change_requests SET status = 'cancelled' WHERE id = ? AND status = 'pending'", requestID)
			return 0, ctx.Err()
		case <-ticker.C:
			var status string
			var newTapeID *int64
			err := s.db.QueryRow("SELECT status, new_tape_id FROM tape_change_requests WHERE id = ?", requestID).Scan(&status, &newTapeID)
			if err != nil {
				return 0, fmt.Errorf("failed to check tape change request: %w", err)
			}
			if status == "completed" && newTapeID != nil {
				return *newTapeID, nil
			}
			if status == "cancelled" {
				return 0, fmt.Errorf("tape change request was cancelled")
			}
		}
	}
}

// ResumeState represents the persisted state of a paused/failed backup job for resume capability
type ResumeState struct {
	FilesProcessed []string `json:"files_processed"` // Relative paths of files already backed up
	BytesWritten   int64    `json:"bytes_written"`
	TotalFiles     int64    `json:"total_files"`
	TotalBytes     int64    `json:"total_bytes"`
	TapeID         int64    `json:"tape_id"`
	BackupSetID    int64    `json:"backup_set_id"`
}

// saveJobExecutionState persists the current job progress to the database so it can survive server restarts
func (s *Service) saveJobExecutionState(jobID int64, p *JobProgress) {
	if s.db == nil {
		return
	}

	// Build resume state from current progress
	state := ResumeState{
		BytesWritten: p.BytesWritten,
		TotalFiles:   p.TotalFiles,
		TotalBytes:   p.TotalBytes,
		BackupSetID:  p.BackupSetID,
	}

	// Collect processed files from catalog entries if we have a backup set ID
	if p.BackupSetID > 0 {
		rows, err := s.db.Query("SELECT file_path FROM catalog_entries WHERE backup_set_id = ?", p.BackupSetID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var filePath string
				if rows.Scan(&filePath) == nil {
					state.FilesProcessed = append(state.FilesProcessed, filePath)
				}
			}
		}
	}

	stateJSON, err := json.Marshal(state)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to marshal resume state", map[string]interface{}{
				"job_id": jobID,
				"error":  err.Error(),
			})
		}
		return
	}

	// Upsert into job_executions
	_, err = s.db.Exec(`
		INSERT INTO job_executions (job_id, backup_set_id, status, files_processed, bytes_processed, can_resume, resume_state)
		VALUES (?, ?, 'paused', ?, ?, 1, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = 'paused', files_processed = excluded.files_processed,
			bytes_processed = excluded.bytes_processed, can_resume = 1,
			resume_state = excluded.resume_state, updated_at = CURRENT_TIMESTAMP
	`, jobID, p.BackupSetID, p.FileCount, p.BytesWritten, string(stateJSON))
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to save job execution state", map[string]interface{}{
				"job_id": jobID,
				"error":  err.Error(),
			})
		}
	}
}

// saveFailedJobState saves state for a failed job so it can be retried
func (s *Service) saveFailedJobState(jobID int64, p *JobProgress, errorMessage string) {
	if s.db == nil {
		return
	}

	state := ResumeState{
		BytesWritten: p.BytesWritten,
		TotalFiles:   p.TotalFiles,
		TotalBytes:   p.TotalBytes,
		BackupSetID:  p.BackupSetID,
	}

	if p.BackupSetID > 0 {
		rows, err := s.db.Query("SELECT file_path FROM catalog_entries WHERE backup_set_id = ?", p.BackupSetID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var filePath string
				if rows.Scan(&filePath) == nil {
					state.FilesProcessed = append(state.FilesProcessed, filePath)
				}
			}
		}
	}

	stateJSON, err := json.Marshal(state)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to marshal failed job resume state", map[string]interface{}{
				"job_id": jobID,
				"error":  err.Error(),
			})
		}
		return
	}

	s.db.Exec(`
		INSERT INTO job_executions (job_id, backup_set_id, status, files_processed, bytes_processed, error_message, can_resume, resume_state)
		VALUES (?, ?, 'failed', ?, ?, ?, 1, ?)
	`, jobID, p.BackupSetID, p.FileCount, p.BytesWritten, errorMessage, string(stateJSON))
}

// RunBackupWithResume runs a backup that resumes from a previous checkpoint, skipping already-processed files
func (s *Service) RunBackupWithResume(ctx context.Context, job *models.BackupJob, source *models.BackupSource, tapeID int64, backupType models.BackupType, resumeStateJSON string) (*models.BackupSet, error) {
	var state ResumeState
	if err := json.Unmarshal([]byte(resumeStateJSON), &state); err != nil {
		// If resume state is invalid, fall back to full backup
		if s.logger != nil {
			s.logger.Warn("Invalid resume state, starting fresh backup", map[string]interface{}{
				"job_id": job.ID,
				"error":  err.Error(),
			})
		}
		return s.RunBackup(ctx, job, source, tapeID, backupType)
	}

	if len(state.FilesProcessed) == 0 {
		// No files were processed, just run normally
		return s.RunBackup(ctx, job, source, tapeID, backupType)
	}

	// Store the set of already-processed files for filtering
	s.mu.Lock()
	s.resumeFiles[job.ID] = state.FilesProcessed
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.resumeFiles, job.ID)
		s.mu.Unlock()
	}()

	s.emitEvent("info", "backup", "Backup Resuming", fmt.Sprintf("Resuming backup job: %s (skipping %d already-processed files)", job.Name, len(state.FilesProcessed)))

	return s.RunBackup(ctx, job, source, tapeID, backupType)
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
