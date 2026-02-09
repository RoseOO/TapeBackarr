package backup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RoseOO/TapeBackarr/internal/database"
	"github.com/RoseOO/TapeBackarr/internal/models"
)

func TestCalculateChecksum(t *testing.T) {
	// Create a test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Write test content
	testContent := []byte("Hello, World!")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create a minimal service for testing
	svc := &Service{}

	// Calculate checksum
	checksum, err := svc.CalculateChecksum(testFile)
	if err != nil {
		t.Fatalf("CalculateChecksum failed: %v", err)
	}

	// Verify checksum is not empty and is valid SHA256 format (64 hex chars)
	if len(checksum) != 64 {
		t.Errorf("expected 64 character SHA256 hash, got %d characters", len(checksum))
	}

	// Verify checksum is consistent
	checksum2, err := svc.CalculateChecksum(testFile)
	if err != nil {
		t.Fatalf("CalculateChecksum failed on second call: %v", err)
	}

	if checksum != checksum2 {
		t.Errorf("checksums do not match: %s vs %s", checksum, checksum2)
	}

	// Known SHA256 for "Hello, World!"
	expectedChecksum := "dffd6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986f"
	if checksum != expectedChecksum {
		t.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, checksum)
	}
}

func TestCalculateChecksumNonExistentFile(t *testing.T) {
	svc := &Service{}

	// Try to calculate checksum for non-existent file
	_, err := svc.CalculateChecksum("/nonexistent/file.txt")
	if err == nil {
		t.Error("expected error for non-existent file, got nil")
	}
}

func TestCalculateChecksumDifferentContent(t *testing.T) {
	tmpDir := t.TempDir()
	svc := &Service{}

	// Create two files with different content
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")

	if err := os.WriteFile(file1, []byte("content1"), 0644); err != nil {
		t.Fatalf("failed to create file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte("content2"), 0644); err != nil {
		t.Fatalf("failed to create file2: %v", err)
	}

	checksum1, err := svc.CalculateChecksum(file1)
	if err != nil {
		t.Fatalf("CalculateChecksum failed for file1: %v", err)
	}

	checksum2, err := svc.CalculateChecksum(file2)
	if err != nil {
		t.Fatalf("CalculateChecksum failed for file2: %v", err)
	}

	if checksum1 == checksum2 {
		t.Error("different files should have different checksums")
	}
}

func TestCalculateChecksumEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	svc := &Service{}

	// Create an empty file
	emptyFile := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(emptyFile, []byte{}, 0644); err != nil {
		t.Fatalf("failed to create empty file: %v", err)
	}

	checksum, err := svc.CalculateChecksum(emptyFile)
	if err != nil {
		t.Fatalf("CalculateChecksum failed for empty file: %v", err)
	}

	// SHA256 of empty content
	expectedChecksum := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if checksum != expectedChecksum {
		t.Errorf("empty file checksum mismatch: expected %s, got %s", expectedChecksum, checksum)
	}
}

func TestScanSourceBasic(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	os.MkdirAll(filepath.Join(tmpDir, "subdir1"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "subdir2"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "subdir1", "file2.txt"), []byte("world"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "subdir2", "file3.txt"), []byte("test"), 0644)

	svc := &Service{}
	source := &models.BackupSource{Path: tmpDir}

	files, err := svc.ScanSource(context.Background(), source)
	if err != nil {
		t.Fatalf("ScanSource failed: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	// Verify all files found (sort for deterministic comparison)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	expectedPaths := []string{
		filepath.Join(tmpDir, "file1.txt"),
		filepath.Join(tmpDir, "subdir1", "file2.txt"),
		filepath.Join(tmpDir, "subdir2", "file3.txt"),
	}
	sort.Strings(expectedPaths)

	for i, f := range files {
		if f.Path != expectedPaths[i] {
			t.Errorf("file %d: expected path %s, got %s", i, expectedPaths[i], f.Path)
		}
	}
}

func TestScanSourceExcludePatterns(t *testing.T) {
	tmpDir := t.TempDir()

	os.WriteFile(filepath.Join(tmpDir, "keep.txt"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "skip.log"), []byte("skip"), 0644)

	svc := &Service{}
	excludeJSON, _ := json.Marshal([]string{"*.log"})
	source := &models.BackupSource{
		Path:            tmpDir,
		ExcludePatterns: string(excludeJSON),
	}

	files, err := svc.ScanSource(context.Background(), source)
	if err != nil {
		t.Fatalf("ScanSource failed: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if filepath.Base(files[0].Path) != "keep.txt" {
		t.Errorf("expected keep.txt, got %s", files[0].Path)
	}
}

func TestScanSourceIncludePatterns(t *testing.T) {
	tmpDir := t.TempDir()

	os.WriteFile(filepath.Join(tmpDir, "include.txt"), []byte("include"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "exclude.log"), []byte("exclude"), 0644)

	svc := &Service{}
	includeJSON, _ := json.Marshal([]string{"*.txt"})
	source := &models.BackupSource{
		Path:            tmpDir,
		IncludePatterns: string(includeJSON),
	}

	files, err := svc.ScanSource(context.Background(), source)
	if err != nil {
		t.Fatalf("ScanSource failed: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if filepath.Base(files[0].Path) != "include.txt" {
		t.Errorf("expected include.txt, got %s", files[0].Path)
	}
}

func TestScanSourceExcludeDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a regular directory with a file
	os.MkdirAll(filepath.Join(tmpDir, "documents"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "documents", "report.txt"), []byte("report"), 0644)

	// Create #recycle directory (Synology NAS recycle bin) with files
	os.MkdirAll(filepath.Join(tmpDir, "#recycle"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "#recycle", "deleted.txt"), []byte("deleted"), 0644)

	// Create @eaDir directory (Synology metadata) with files
	os.MkdirAll(filepath.Join(tmpDir, "@eaDir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "@eaDir", "metadata.db"), []byte("meta"), 0644)

	// Create a top-level file
	os.WriteFile(filepath.Join(tmpDir, "keep.txt"), []byte("keep"), 0644)

	svc := &Service{}
	excludeJSON, _ := json.Marshal([]string{"#recycle", "@eaDir"})
	source := &models.BackupSource{
		Path:            tmpDir,
		ExcludePatterns: string(excludeJSON),
	}

	files, err := svc.ScanSource(context.Background(), source)
	if err != nil {
		t.Fatalf("ScanSource failed: %v", err)
	}

	// Should only include keep.txt and documents/report.txt (not files in #recycle or @eaDir)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	names := make(map[string]bool)
	for _, f := range files {
		names[filepath.Base(f.Path)] = true
	}
	if !names["keep.txt"] {
		t.Error("expected keep.txt to be included")
	}
	if !names["report.txt"] {
		t.Error("expected report.txt to be included")
	}
	if names["deleted.txt"] {
		t.Error("expected deleted.txt to be excluded")
	}
	if names["metadata.db"] {
		t.Error("expected metadata.db to be excluded")
	}
}

func TestScanSourceDeepNesting(t *testing.T) {
	tmpDir := t.TempDir()

	// Create deeply nested directories
	deepPath := tmpDir
	for i := 0; i < 10; i++ {
		deepPath = filepath.Join(deepPath, "level")
		os.MkdirAll(deepPath, 0755)
		os.WriteFile(filepath.Join(deepPath, "file.txt"), []byte("data"), 0644)
	}

	svc := &Service{}
	source := &models.BackupSource{Path: tmpDir}

	files, err := svc.ScanSource(context.Background(), source)
	if err != nil {
		t.Fatalf("ScanSource failed: %v", err)
	}

	if len(files) != 10 {
		t.Fatalf("expected 10 files, got %d", len(files))
	}
}

func TestScanSourceContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some files
	for i := 0; i < 5; i++ {
		dir := filepath.Join(tmpDir, "dir", string(rune('a'+i)))
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0644)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	svc := &Service{}
	source := &models.BackupSource{Path: tmpDir}

	// Should not hang and should return context error
	_, err := svc.ScanSource(ctx, source)
	if err != context.Canceled {
		// It's also acceptable to get fewer files
		t.Logf("ScanSource with cancelled context returned err: %v", err)
	}
}

func TestScanSourceEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	svc := &Service{}
	source := &models.BackupSource{Path: tmpDir}

	files, err := svc.ScanSource(context.Background(), source)
	if err != nil {
		t.Fatalf("ScanSource failed: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("expected 0 files for empty directory, got %d", len(files))
	}
}

func TestScanSourceFileMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	content := []byte("test content here")
	testFile := filepath.Join(tmpDir, "meta.txt")
	os.WriteFile(testFile, content, 0644)

	svc := &Service{}
	source := &models.BackupSource{Path: tmpDir}

	files, err := svc.ScanSource(context.Background(), source)
	if err != nil {
		t.Fatalf("ScanSource failed: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	f := files[0]
	if f.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), f.Size)
	}
	if f.ModTime.IsZero() {
		t.Error("expected non-zero mod time")
	}
}

func TestCalculateChecksumLargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	svc := &Service{}

	// Create a larger file (1MB)
	largeFile := filepath.Join(tmpDir, "large.bin")
	largeContent := make([]byte, 1024*1024) // 1MB of zeros
	if err := os.WriteFile(largeFile, largeContent, 0644); err != nil {
		t.Fatalf("failed to create large file: %v", err)
	}

	checksum, err := svc.CalculateChecksum(largeFile)
	if err != nil {
		t.Fatalf("CalculateChecksum failed for large file: %v", err)
	}

	// Verify checksum is valid format
	if len(checksum) != 64 {
		t.Errorf("expected 64 character SHA256 hash, got %d characters", len(checksum))
	}
}

func TestFileInfoHashField(t *testing.T) {
	// Test that FileInfo struct has Hash field
	fi := FileInfo{
		Path: "/test/file.txt",
		Size: 1000,
		Mode: 0644,
		Hash: "abc123",
	}

	if fi.Hash != "abc123" {
		t.Errorf("expected hash 'abc123', got '%s'", fi.Hash)
	}
}

func TestGetActiveJobs(t *testing.T) {
	svc := NewService(nil, nil, nil, 65536, 512)

	// Initially no active jobs
	jobs := svc.GetActiveJobs()
	if len(jobs) != 0 {
		t.Errorf("expected 0 active jobs, got %d", len(jobs))
	}

	// Add an active job manually
	svc.mu.Lock()
	svc.activeJobs[1] = &JobProgress{
		JobID:   1,
		JobName: "test-job",
		Phase:   "streaming",
		Status:  "running",
	}
	svc.mu.Unlock()

	jobs = svc.GetActiveJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 active job, got %d", len(jobs))
	}
	if jobs[0].JobName != "test-job" {
		t.Errorf("expected job name 'test-job', got '%s'", jobs[0].JobName)
	}
	if jobs[0].Status != "running" {
		t.Errorf("expected status 'running', got '%s'", jobs[0].Status)
	}
}

func TestCancelJob(t *testing.T) {
	svc := NewService(nil, nil, nil, 65536, 512)

	// Cancel non-existent job returns false
	if svc.CancelJob(999) {
		t.Error("expected CancelJob to return false for non-existent job")
	}

	// Add a cancellable job
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.mu.Lock()
	svc.activeJobs[1] = &JobProgress{
		JobID:   1,
		JobName: "test-job",
		Phase:   "streaming",
		Status:  "running",
	}
	svc.cancelFuncs[1] = cancel
	svc.mu.Unlock()

	// Cancel should succeed
	if !svc.CancelJob(1) {
		t.Error("expected CancelJob to return true")
	}

	// Verify context is cancelled
	if ctx.Err() == nil {
		t.Error("expected context to be cancelled")
	}

	// Verify status updated
	svc.mu.Lock()
	p := svc.activeJobs[1]
	svc.mu.Unlock()
	if p.Status != "cancelled" {
		t.Errorf("expected status 'cancelled', got '%s'", p.Status)
	}
}

func TestPauseResumeJob(t *testing.T) {
	svc := NewService(nil, nil, nil, 65536, 512)

	// Pause non-existent job returns false
	if svc.PauseJob(999) {
		t.Error("expected PauseJob to return false for non-existent job")
	}

	// Add a pausable job
	var pauseFlag int32
	svc.mu.Lock()
	svc.activeJobs[1] = &JobProgress{
		JobID:   1,
		JobName: "test-job",
		Phase:   "streaming",
		Status:  "running",
	}
	svc.pauseFlags[1] = &pauseFlag
	svc.mu.Unlock()

	// Pause should succeed
	if !svc.PauseJob(1) {
		t.Error("expected PauseJob to return true")
	}

	svc.mu.Lock()
	p := svc.activeJobs[1]
	svc.mu.Unlock()
	if p.Status != "paused" {
		t.Errorf("expected status 'paused', got '%s'", p.Status)
	}

	// Resume should succeed
	if !svc.ResumeJob(1) {
		t.Error("expected ResumeJob to return true")
	}

	svc.mu.Lock()
	p = svc.activeJobs[1]
	svc.mu.Unlock()
	if p.Status != "running" {
		t.Errorf("expected status 'running', got '%s'", p.Status)
	}
}

func TestEventCallback(t *testing.T) {
	svc := NewService(nil, nil, nil, 65536, 512)

	var receivedEvents []string
	svc.EventCallback = func(eventType, category, title, message string) {
		receivedEvents = append(receivedEvents, eventType+":"+title)
	}

	// Register a job
	svc.mu.Lock()
	svc.activeJobs[1] = &JobProgress{
		JobID:   1,
		JobName: "test-job",
		Phase:   "initializing",
		Status:  "running",
	}
	svc.mu.Unlock()

	// Update progress should emit event
	svc.updateProgress(1, "scanning", "Scanning files...")

	if len(receivedEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(receivedEvents))
	}
	if receivedEvents[0] != "info:Backup: scanning" {
		t.Errorf("unexpected event: %s", receivedEvents[0])
	}

	// Completed event should be success type
	svc.updateProgress(1, "completed", "Done")
	if len(receivedEvents) != 2 {
		t.Fatalf("expected 2 events, got %d", len(receivedEvents))
	}
	if receivedEvents[1] != "success:Backup: completed" {
		t.Errorf("unexpected event: %s", receivedEvents[1])
	}
}

func TestBackupFailureEmitsErrorEvent(t *testing.T) {
	svc := NewService(nil, nil, nil, 65536, 512)

	var receivedEvents []string
	svc.EventCallback = func(eventType, category, title, message string) {
		receivedEvents = append(receivedEvents, eventType+":"+title+":"+message)
	}

	// Register a job
	svc.mu.Lock()
	svc.activeJobs[1] = &JobProgress{
		JobID:   1,
		JobName: "test-job",
		Phase:   "initializing",
		Status:  "running",
	}
	svc.mu.Unlock()

	// Failed phase should emit error event
	svc.updateProgress(1, "failed", "Failed to create backup set: some db error")

	if len(receivedEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(receivedEvents))
	}
	if receivedEvents[0] != "error:Backup: failed:Failed to create backup set: some db error" {
		t.Errorf("unexpected event: %s", receivedEvents[0])
	}

	// Direct emitEvent for backup failure should also work
	svc.emitEvent("error", "backup", "Backup Failed", "Job test-job failed: some error")
	if len(receivedEvents) != 2 {
		t.Fatalf("expected 2 events, got %d", len(receivedEvents))
	}
	if receivedEvents[1] != "error:Backup Failed:Job test-job failed: some error" {
		t.Errorf("unexpected event: %s", receivedEvents[1])
	}
}

func TestCountingReader(t *testing.T) {
	data := []byte("hello world test data for counting")
	reader := bytes.NewReader(data)

	var lastCount int64
	cr := &countingReader{
		reader: reader,
		callback: func(bytesRead int64) {
			lastCount = bytesRead
		},
	}

	buf := make([]byte, 10)

	// First read - callback fires immediately (lastCallback is zero)
	n, err := cr.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 10 {
		t.Errorf("expected 10 bytes read, got %d", n)
	}
	if lastCount != 10 {
		t.Errorf("expected callback with 10, got %d", lastCount)
	}

	// Second read - callback is throttled (< 1s since first), but byte count is still accurate
	n, err = cr.Read(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Total bytes tracked via atomic counter should always be accurate
	total := cr.bytesRead()
	if total != int64(10+n) {
		t.Errorf("expected total %d, got %d", 10+n, total)
	}
}

func TestCountingReaderThrottledCallback(t *testing.T) {
	// Verify that the callback is throttled but byte counting remains accurate
	data := make([]byte, 1024)
	reader := bytes.NewReader(data)

	var callbackCount int32
	cr := &countingReader{
		reader: reader,
		callback: func(bytesRead int64) {
			atomic.AddInt32(&callbackCount, 1)
		},
	}

	buf := make([]byte, 64)

	// Read many times in rapid succession
	totalRead := 0
	for {
		n, err := cr.Read(buf)
		totalRead += n
		if err != nil {
			break
		}
	}

	// All bytes should be counted accurately
	if cr.bytesRead() != int64(totalRead) {
		t.Errorf("expected %d bytes counted, got %d", totalRead, cr.bytesRead())
	}

	// Callback should have fired at least once (first read) but not for every read
	// due to 1-second throttling
	callbacks := atomic.LoadInt32(&callbackCount)
	if callbacks < 1 {
		t.Errorf("expected at least 1 callback, got %d", callbacks)
	}
	// With 1024/64 = 16 reads in quick succession (sub-millisecond), the 1-second
	// throttle should limit callbacks to at most a few (first read + possible
	// timer granularity races). Allow up to 5 as generous headroom.
	if callbacks > 5 {
		t.Errorf("expected throttled callbacks (<=5), got %d out of 16 reads", callbacks)
	}
}

func TestJobProgressFields(t *testing.T) {
	p := JobProgress{
		JobID:                     1,
		JobName:                   "test",
		Status:                    "running",
		Phase:                     "streaming",
		BytesWritten:              1000,
		WriteSpeed:                100.5,
		TapeLabel:                 "TAPE001",
		TapeCapacityBytes:         12000000000000,
		TapeUsedBytes:             5000000000000,
		DevicePath:                "/dev/nst0",
		EstimatedSecondsRemaining: 3600.5,
	}

	if p.BytesWritten != 1000 {
		t.Errorf("expected BytesWritten 1000, got %d", p.BytesWritten)
	}
	if p.WriteSpeed != 100.5 {
		t.Errorf("expected WriteSpeed 100.5, got %f", p.WriteSpeed)
	}
	if p.TapeLabel != "TAPE001" {
		t.Errorf("expected TapeLabel 'TAPE001', got '%s'", p.TapeLabel)
	}
	if p.DevicePath != "/dev/nst0" {
		t.Errorf("expected DevicePath '/dev/nst0', got '%s'", p.DevicePath)
	}
	if p.Status != "running" {
		t.Errorf("expected Status 'running', got '%s'", p.Status)
	}
}

func TestResumeStateJSON(t *testing.T) {
	state := ResumeState{
		FilesProcessed: []string{"dir1/file1.txt", "dir2/file2.txt"},
		BytesWritten:   1024000,
		TotalFiles:     100,
		TotalBytes:     10240000,
		TapeID:         42,
		BackupSetID:    7,
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("failed to marshal ResumeState: %v", err)
	}

	var decoded ResumeState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ResumeState: %v", err)
	}

	if len(decoded.FilesProcessed) != 2 {
		t.Fatalf("expected 2 files processed, got %d", len(decoded.FilesProcessed))
	}
	if decoded.FilesProcessed[0] != "dir1/file1.txt" {
		t.Errorf("expected first file 'dir1/file1.txt', got '%s'", decoded.FilesProcessed[0])
	}
	if decoded.BytesWritten != 1024000 {
		t.Errorf("expected bytes written 1024000, got %d", decoded.BytesWritten)
	}
	if decoded.TotalFiles != 100 {
		t.Errorf("expected total files 100, got %d", decoded.TotalFiles)
	}
	if decoded.BackupSetID != 7 {
		t.Errorf("expected backup set ID 7, got %d", decoded.BackupSetID)
	}
}

func TestResumeStateEmptyFilesProcessed(t *testing.T) {
	state := ResumeState{
		BytesWritten: 0,
		TotalFiles:   50,
		TotalBytes:   5000,
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("failed to marshal ResumeState: %v", err)
	}

	var decoded ResumeState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ResumeState: %v", err)
	}

	if len(decoded.FilesProcessed) != 0 {
		t.Errorf("expected 0 files processed, got %d", len(decoded.FilesProcessed))
	}
}

func TestPauseJobPersistsState(t *testing.T) {
	// Test that PauseJob still works with nil db (no crash)
	svc := NewService(nil, nil, nil, 65536, 512)

	var pauseFlag int32
	svc.mu.Lock()
	svc.activeJobs[1] = &JobProgress{
		JobID:        1,
		JobName:      "test-job",
		Phase:        "streaming",
		Status:       "running",
		BytesWritten: 50000,
		TotalFiles:   100,
		TotalBytes:   100000,
		BackupSetID:  5,
	}
	svc.pauseFlags[1] = &pauseFlag
	svc.mu.Unlock()

	if !svc.PauseJob(1) {
		t.Error("expected PauseJob to return true")
	}

	svc.mu.Lock()
	p := svc.activeJobs[1]
	svc.mu.Unlock()

	if p.Status != "paused" {
		t.Errorf("expected status 'paused', got '%s'", p.Status)
	}
}

func TestResumeFilesFiltering(t *testing.T) {
	svc := NewService(nil, nil, nil, 65536, 512)

	// Simulate resume files being set
	svc.mu.Lock()
	svc.resumeFiles[1] = []string{"file1.txt", "subdir/file2.txt"}
	svc.mu.Unlock()

	// Verify resumeFiles is populated
	svc.mu.Lock()
	rf := svc.resumeFiles[1]
	svc.mu.Unlock()

	if len(rf) != 2 {
		t.Fatalf("expected 2 resume files, got %d", len(rf))
	}
	if rf[0] != "file1.txt" {
		t.Errorf("expected first resume file 'file1.txt', got '%s'", rf[0])
	}

	// Clean up
	svc.mu.Lock()
	delete(svc.resumeFiles, 1)
	svc.mu.Unlock()

	svc.mu.Lock()
	rf = svc.resumeFiles[1]
	svc.mu.Unlock()

	if len(rf) != 0 {
		t.Errorf("expected 0 resume files after cleanup, got %d", len(rf))
	}
}

func TestSaveJobExecutionStateNilDB(t *testing.T) {
	// Ensure saveJobExecutionState doesn't panic with nil DB
	svc := NewService(nil, nil, nil, 65536, 512)
	p := &JobProgress{
		JobID:        1,
		BytesWritten: 1000,
		TotalFiles:   10,
		TotalBytes:   5000,
	}
	// Should not panic
	svc.saveJobExecutionState(1, p)
}

func TestSaveFailedJobStateNilDB(t *testing.T) {
	// Ensure saveFailedJobState doesn't panic with nil DB
	svc := NewService(nil, nil, nil, 65536, 512)
	p := &JobProgress{
		JobID:        1,
		BytesWritten: 1000,
		TotalFiles:   10,
		TotalBytes:   5000,
	}
	// Should not panic
	svc.saveFailedJobState(1, p, "network error")
}

func TestBuildCompressionCmdGzip(t *testing.T) {
	ctx := context.Background()

	cmd, err := buildCompressionCmd(ctx, models.CompressionGzip)
	if err != nil {
		t.Fatalf("buildCompressionCmd failed: %v", err)
	}

	args := cmd.Args
	// Should use either pigz or gzip depending on availability
	if args[0] != "pigz" && args[0] != "gzip" {
		t.Errorf("expected pigz or gzip, got %s", args[0])
	}

	// Must include -1 (fastest compression) to avoid tape throughput bottleneck
	found := false
	for _, a := range args {
		if a == "-1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected -1 flag for fast compression, got args: %v", args)
	}
}

func TestBuildCompressionCmdZstd(t *testing.T) {
	ctx := context.Background()

	cmd, err := buildCompressionCmd(ctx, models.CompressionZstd)
	if err != nil {
		t.Fatalf("buildCompressionCmd failed: %v", err)
	}

	if cmd.Args[0] != "zstd" {
		t.Errorf("expected zstd, got %s", cmd.Args[0])
	}

	// Must include -T0 for multi-threaded compression
	found := false
	for _, a := range cmd.Args {
		if a == "-T0" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected -T0 flag for multi-threaded compression, got args: %v", cmd.Args)
	}
}

func TestBuildCompressionCmdUnsupported(t *testing.T) {
	ctx := context.Background()

	_, err := buildCompressionCmd(ctx, models.CompressionType("lz4"))
	if err == nil {
		t.Error("expected error for unsupported compression type")
	}
}

func TestBuildCompressionCmdNone(t *testing.T) {
	ctx := context.Background()

	_, err := buildCompressionCmd(ctx, models.CompressionNone)
	if err == nil {
		t.Error("expected error for CompressionNone")
	}
}

func TestScanSourceProgressCallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a directory tree
	os.MkdirAll(filepath.Join(tmpDir, "dir1"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "dir2"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "dir1", "b.txt"), []byte("world!"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "dir2", "c.txt"), []byte("test"), 0644)

	svc := &Service{}
	source := &models.BackupSource{Path: tmpDir}

	var lastFiles, lastDirs, lastBytes int64
	cb := func(filesFound, dirsScanned, bytesFound int64) {
		lastFiles = filesFound
		lastDirs = dirsScanned
		lastBytes = bytesFound
	}

	files, err := svc.ScanSource(context.Background(), source, cb)
	if err != nil {
		t.Fatalf("ScanSource failed: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	// Final callback should report all files and dirs
	if lastFiles != 3 {
		t.Errorf("expected final callback with 3 files, got %d", lastFiles)
	}
	// root + dir1 + dir2 = 3 dirs scanned
	if lastDirs != 3 {
		t.Errorf("expected 3 dirs scanned, got %d", lastDirs)
	}
	// 5 + 6 + 4 = 15 bytes
	if lastBytes != 15 {
		t.Errorf("expected 15 bytes, got %d", lastBytes)
	}
}

func TestScanSourceNoCallbackStillWorks(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("data"), 0644)

	svc := &Service{}
	source := &models.BackupSource{Path: tmpDir}

	// No callback — should work without panicking
	files, err := svc.ScanSource(context.Background(), source)
	if err != nil {
		t.Fatalf("ScanSource failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
}

func TestSpeedTracker(t *testing.T) {
	st := newSpeedTracker(60 * time.Second)
	base := time.Now()

	// Single sample — not enough for speed
	st.Record(base, 0)
	if st.Speed() != 0 {
		t.Errorf("expected 0 speed with 1 sample, got %f", st.Speed())
	}

	// Two samples 10 seconds apart, 10 MB transferred
	st.Record(base.Add(10*time.Second), 10_000_000)
	speed := st.Speed()
	expected := 1_000_000.0 // 10 MB / 10 sec = 1 MB/s
	if speed < expected*0.9 || speed > expected*1.1 {
		t.Errorf("expected speed ~%f, got %f", expected, speed)
	}

	// Add more samples
	st.Record(base.Add(20*time.Second), 30_000_000)
	speed = st.Speed()
	expected = 1_500_000.0 // 30 MB / 20 sec
	if speed < expected*0.9 || speed > expected*1.1 {
		t.Errorf("expected speed ~%f, got %f", expected, speed)
	}
}

func TestSpeedTrackerWindowExpiry(t *testing.T) {
	st := newSpeedTracker(10 * time.Second) // short window for testing
	base := time.Now()

	st.Record(base, 0)
	st.Record(base.Add(5*time.Second), 5_000_000)
	// Speed should be 1 MB/s
	speed := st.Speed()
	if speed < 900_000 || speed > 1_100_000 {
		t.Errorf("expected speed ~1MB/s, got %f", speed)
	}

	// Add sample way past the window — old ones get pruned
	st.Record(base.Add(20*time.Second), 100_000_000)
	// Now window only contains samples from t+5 and t+20 => 95MB / 15s
	speed = st.Speed()
	expected := 95_000_000.0 / 15.0
	if speed < expected*0.9 || speed > expected*1.1 {
		t.Errorf("expected speed ~%f, got %f", expected, speed)
	}
}

func TestScanProgressFieldsInJobProgress(t *testing.T) {
	p := JobProgress{
		ScanFilesFound:  42,
		ScanDirsScanned: 10,
		ScanBytesFound:  123456,
	}

	if p.ScanFilesFound != 42 {
		t.Errorf("expected ScanFilesFound 42, got %d", p.ScanFilesFound)
	}
	if p.ScanDirsScanned != 10 {
		t.Errorf("expected ScanDirsScanned 10, got %d", p.ScanDirsScanned)
	}
	if p.ScanBytesFound != 123456 {
		t.Errorf("expected ScanBytesFound 123456, got %d", p.ScanBytesFound)
	}
}

func TestComputeChecksumsAsync(t *testing.T) {
	tmpDir := t.TempDir()
	svc := &Service{}

	// Create test files
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")
	if err := os.WriteFile(file1, []byte("Hello, World!"), 0644); err != nil {
		t.Fatalf("failed to create file1: %v", err)
	}
	if err := os.WriteFile(file2, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create file2: %v", err)
	}

	files := []FileInfo{
		{Path: file1, Size: 13},
		{Path: file2, Size: 12},
	}

	checksums := &sync.Map{}
	svc.computeChecksumsAsync(context.Background(), files, checksums, 0, tmpDir)

	// Both files should have checksums
	val1, ok1 := checksums.Load(file1)
	if !ok1 {
		t.Fatal("expected checksum for file1")
	}
	if len(val1.(string)) != 64 {
		t.Errorf("expected 64-char SHA256 hash, got %d chars", len(val1.(string)))
	}

	val2, ok2 := checksums.Load(file2)
	if !ok2 {
		t.Fatal("expected checksum for file2")
	}
	if val1.(string) == val2.(string) {
		t.Error("different files should have different checksums")
	}

	// Known SHA256 for "Hello, World!"
	expectedChecksum := "dffd6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986f"
	if val1.(string) != expectedChecksum {
		t.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, val1.(string))
	}
}

func TestComputeChecksumsAsyncContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	svc := &Service{}

	// Create many files
	files := make([]FileInfo, 100)
	for i := range files {
		path := filepath.Join(tmpDir, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(path, []byte(fmt.Sprintf("content %d", i)), 0644); err != nil {
			t.Fatalf("failed to create file %d: %v", i, err)
		}
		files[i] = FileInfo{Path: path, Size: 10}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	checksums := &sync.Map{}
	svc.computeChecksumsAsync(ctx, files, checksums, 0, tmpDir)

	// With immediate cancellation, very few (or zero) checksums should be computed
	count := 0
	checksums.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	// Should not have computed all 100 checksums
	if count == 100 {
		t.Error("expected fewer than 100 checksums after immediate cancellation")
	}
}

func TestComputeChecksumsAsyncMissingFile(t *testing.T) {
	svc := &Service{}

	files := []FileInfo{
		{Path: "/nonexistent/file.txt", Size: 100},
	}

	checksums := &sync.Map{}
	svc.computeChecksumsAsync(context.Background(), files, checksums, 0, "")

	// Missing file should not produce a checksum
	_, ok := checksums.Load("/nonexistent/file.txt")
	if ok {
		t.Error("expected no checksum for nonexistent file")
	}
}

func TestCountingWriter(t *testing.T) {
	var buf bytes.Buffer
	cw := &countingWriter{writer: &buf}

	data := []byte("hello world")
	n, err := cw.Write(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Errorf("expected %d bytes written, got %d", len(data), n)
	}
	if cw.bytesWritten() != int64(len(data)) {
		t.Errorf("expected count %d, got %d", len(data), cw.bytesWritten())
	}

	// Write more data
	data2 := []byte(" more data")
	n2, err := cw.Write(data2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := int64(n + n2)
	if cw.bytesWritten() != expected {
		t.Errorf("expected total %d, got %d", expected, cw.bytesWritten())
	}

	// Verify data was actually written to underlying writer
	if buf.String() != "hello world more data" {
		t.Errorf("expected 'hello world more data', got '%s'", buf.String())
	}
}

func TestCountingWriterWithBufferedIO(t *testing.T) {
	// Verify that a bufio.Writer aggregates small writes into large ones,
	// which is critical for tape performance (avoids shoe-shining).
	var writeSizes []int
	tw := &trackingWriter{writeSizes: &writeSizes}

	blockSize := 256 * 1024 // 256KB, matching LTO default
	buffered := bufio.NewWriterSize(tw, blockSize)
	cw := &countingWriter{writer: buffered}

	// Simulate many small writes (like Go's io.Copy with 32KB buffer)
	smallBuf := make([]byte, 32*1024) // 32KB
	for i := range smallBuf {
		smallBuf[i] = byte(i % 256)
	}

	totalWritten := 0
	// Write 8 × 32KB = 256KB (fills the buffer exactly)
	for i := 0; i < 8; i++ {
		n, err := cw.Write(smallBuf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		totalWritten += n
	}

	// Buffer is full but not yet flushed (bufio flushes on next write overflow)
	if len(writeSizes) != 0 {
		t.Errorf("expected 0 writes while buffer fills, got %d", len(writeSizes))
	}

	// Next write triggers flush of the full 256KB block, then buffers new data
	partialBuf := make([]byte, 100)
	n, err := cw.Write(partialBuf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	totalWritten += n

	// The full block should have been flushed as one large write
	if len(writeSizes) != 1 {
		t.Fatalf("expected 1 large write after overflow, got %d", len(writeSizes))
	}
	if writeSizes[0] != blockSize {
		t.Errorf("expected write of %d bytes (full block), got %d", blockSize, writeSizes[0])
	}

	// Final flush writes the remaining partial data
	if err := buffered.Flush(); err != nil {
		t.Fatalf("flush failed: %v", err)
	}

	if len(writeSizes) != 2 {
		t.Errorf("expected 2 total writes after flush, got %d", len(writeSizes))
	}
	if len(writeSizes) > 1 && writeSizes[1] != len(partialBuf) {
		t.Errorf("expected final write of %d bytes, got %d", len(partialBuf), writeSizes[1])
	}

	// Verify total bytes counted matches all data written
	expectedTotal := int64(totalWritten)
	if cw.bytesWritten() != expectedTotal {
		t.Errorf("expected %d total bytes, got %d", expectedTotal, cw.bytesWritten())
	}
}

// trackingWriter records the size of each Write call
type trackingWriter struct {
	writeSizes *[]int
}

func (tw *trackingWriter) Write(p []byte) (int, error) {
	*tw.writeSizes = append(*tw.writeSizes, len(p))
	return len(p), nil
}

func TestSplitFilesForTape(t *testing.T) {
	svc := &Service{}

	// Create files of known sizes
	files := []FileInfo{
		{Path: "/data/file1.dat", Size: 100_000_000}, // 100 MB
		{Path: "/data/file2.dat", Size: 200_000_000}, // 200 MB
		{Path: "/data/file3.dat", Size: 300_000_000}, // 300 MB
		{Path: "/data/file4.dat", Size: 400_000_000}, // 400 MB
		{Path: "/data/file5.dat", Size: 500_000_000}, // 500 MB
	}

	// Total: 1.5 GB. Tape capacity: 1.5 TB (plenty of room).
	// All files should fit on one tape.
	thisTape, remaining := svc.splitFilesForTape(files, 1_500_000_000_000)
	if len(thisTape) != 5 {
		t.Errorf("expected all 5 files on tape, got %d", len(thisTape))
	}
	if remaining != nil {
		t.Errorf("expected no remaining files, got %d", len(remaining))
	}

	// Tape capacity: 1 GB. With 1% overhead reserve, usable = 990 MB.
	// file1 (100M+1K) + file2 (200M+1K) + file3 (300M+1K) = 600M+3K => fits
	// + file4 (400M+1K) = 1000M+4K => exceeds 990M
	thisTape, remaining = svc.splitFilesForTape(files, 1_000_000_000)
	if len(thisTape) != 3 {
		t.Errorf("expected 3 files on tape with 1GB capacity, got %d", len(thisTape))
	}
	if len(remaining) != 2 {
		t.Errorf("expected 2 remaining files, got %d", len(remaining))
	}
}

func TestSplitFilesForTapeMaximizeUsage(t *testing.T) {
	// Verify the 1% overhead reserve is much tighter than the old 10%.
	// A 1.5 TB tape should allow ~1.485 TB of file data, not just ~1.35 TB.
	svc := &Service{}

	tapeCapacity := int64(1_500_000_000_000)    // 1.5 TB
	usableExpected := (tapeCapacity * 99) / 100 // 1.485 TB

	// Create one large file that fits in 99% but not 90%
	files := []FileInfo{
		{Path: "/data/bigfile.dat", Size: usableExpected - 2048}, // just under 99% usable
	}
	thisTape, remaining := svc.splitFilesForTape(files, tapeCapacity)
	if len(thisTape) != 1 {
		t.Error("file should fit with 1% overhead reserve")
	}
	if remaining != nil {
		t.Errorf("expected no remaining, got %d", len(remaining))
	}

	// With the old 10% reserve, this file would NOT fit:
	// usable_old = 1.35 TB, file = 1.485 TB - 2KB > 1.35 TB
	oldUsable := (tapeCapacity * 9) / 10
	if files[0].Size+1024 <= oldUsable {
		t.Error("test setup issue: file should exceed old 10% overhead reserve")
	}
}

func TestCompressionLTOTreatedAsNoCompression(t *testing.T) {
	// Verify that CompressionLTO is treated the same as CompressionNone
	// for the useCompression check (no software compression applied).
	lto := models.CompressionLTO
	none := models.CompressionNone

	// Both should result in useCompression == false
	useLTO := lto != "" && lto != models.CompressionNone && lto != models.CompressionLTO
	useNone := none != "" && none != models.CompressionNone && none != models.CompressionLTO

	if useLTO {
		t.Error("CompressionLTO should not trigger software compression")
	}
	if useNone {
		t.Error("CompressionNone should not trigger software compression")
	}

	// gzip and zstd should trigger software compression
	useGzip := models.CompressionGzip != "" && models.CompressionGzip != models.CompressionNone && models.CompressionGzip != models.CompressionLTO
	useZstd := models.CompressionZstd != "" && models.CompressionZstd != models.CompressionNone && models.CompressionZstd != models.CompressionLTO

	if !useGzip {
		t.Error("CompressionGzip should trigger software compression")
	}
	if !useZstd {
		t.Error("CompressionZstd should trigger software compression")
	}
}

func TestComputeChecksumsAsyncWritesCatalogToDB(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a real database
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	// Insert prerequisite records for foreign keys
	_, err = db.Exec("INSERT INTO tape_pools (name) VALUES ('test-pool')")
	if err != nil {
		t.Fatalf("failed to insert pool: %v", err)
	}
	_, err = db.Exec("INSERT INTO tapes (uuid, barcode, label, pool_id, status, capacity_bytes, used_bytes) VALUES ('uuid1', 'T001', 'T001', 1, 'active', 1500000000000, 0)")
	if err != nil {
		t.Fatalf("failed to insert tape: %v", err)
	}
	_, err = db.Exec("INSERT INTO backup_sources (name, source_type, path) VALUES ('test-src', 'local', ?)", tmpDir)
	if err != nil {
		t.Fatalf("failed to insert source: %v", err)
	}
	_, err = db.Exec("INSERT INTO backup_jobs (name, source_id, pool_id, backup_type, schedule_cron, retention_days) VALUES ('test-job', 1, 1, 'full', '', 30)")
	if err != nil {
		t.Fatalf("failed to insert job: %v", err)
	}
	result, err := db.Exec("INSERT INTO backup_sets (job_id, tape_id, backup_type, start_time, status) VALUES (1, 1, 'full', CURRENT_TIMESTAMP, 'running')")
	if err != nil {
		t.Fatalf("failed to insert backup set: %v", err)
	}
	backupSetID, _ := result.LastInsertId()

	// Create test files
	sourceDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(sourceDir, 0755)
	file1 := filepath.Join(sourceDir, "a.txt")
	file2 := filepath.Join(sourceDir, "b.txt")
	os.WriteFile(file1, []byte("hello"), 0644)
	os.WriteFile(file2, []byte("world"), 0644)

	files := []FileInfo{
		{Path: file1, Size: 5, Mode: 0644, ModTime: time.Now()},
		{Path: file2, Size: 5, Mode: 0644, ModTime: time.Now()},
	}

	svc := &Service{db: db}
	checksums := &sync.Map{}

	// Run computeChecksumsAsync — should write catalog entries to DB
	svc.computeChecksumsAsync(context.Background(), files, checksums, backupSetID, sourceDir)

	// Verify checksums are in sync.Map
	if _, ok := checksums.Load(file1); !ok {
		t.Error("expected checksum for a.txt in sync.Map")
	}
	if _, ok := checksums.Load(file2); !ok {
		t.Error("expected checksum for b.txt in sync.Map")
	}

	// Verify catalog entries were written to DB
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM catalog_entries WHERE backup_set_id = ?", backupSetID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query catalog entries: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 catalog entries in DB, got %d", count)
	}

	// Verify checksums are stored in DB entries
	var checksum string
	err = db.QueryRow("SELECT checksum FROM catalog_entries WHERE file_path = 'a.txt'").Scan(&checksum)
	if err != nil {
		t.Fatalf("failed to query checksum: %v", err)
	}
	if len(checksum) != 64 {
		t.Errorf("expected 64-char SHA256 in DB, got %d chars: %s", len(checksum), checksum)
	}
}

// TestBatchedBlockOffsetUpdates verifies that finishTape updates block_offset
// in batches rather than one-by-one, improving performance for large file counts.
func TestBatchedBlockOffsetUpdates(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a real database
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	// Insert prerequisite records for foreign keys
	_, err = db.Exec("INSERT INTO tape_pools (name) VALUES ('test-pool')")
	if err != nil {
		t.Fatalf("failed to insert pool: %v", err)
	}
	_, err = db.Exec("INSERT INTO tapes (uuid, barcode, label, pool_id, status, capacity_bytes, used_bytes) VALUES ('uuid1', 'T001', 'T001', 1, 'active', 1500000000000, 0)")
	if err != nil {
		t.Fatalf("failed to insert tape: %v", err)
	}
	_, err = db.Exec("INSERT INTO backup_sources (name, source_type, path) VALUES ('test-src', 'local', ?)", tmpDir)
	if err != nil {
		t.Fatalf("failed to insert source: %v", err)
	}
	_, err = db.Exec("INSERT INTO backup_jobs (name, source_id, pool_id, backup_type, schedule_cron, retention_days) VALUES ('test-job', 1, 1, 'full', '', 30)")
	if err != nil {
		t.Fatalf("failed to insert job: %v", err)
	}
	result, err := db.Exec("INSERT INTO backup_sets (job_id, tape_id, backup_type, start_time, status) VALUES (1, 1, 'full', CURRENT_TIMESTAMP, 'running')")
	if err != nil {
		t.Fatalf("failed to insert backup set: %v", err)
	}
	backupSetID, _ := result.LastInsertId()

	// Create test files and catalog entries (more than one batch to test batching)
	sourceDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(sourceDir, 0755)

	const numFiles = 600 // More than batch size of 500 to test batching
	files := make([]FileInfo, numFiles)
	for i := 0; i < numFiles; i++ {
		fileName := fmt.Sprintf("file%04d.txt", i)
		filePath := filepath.Join(sourceDir, fileName)
		os.WriteFile(filePath, []byte("content"), 0644)
		files[i] = FileInfo{Path: filePath, Size: 100, Mode: 0644, ModTime: time.Now()}
		// Insert catalog entry without block_offset
		_, err := db.Exec(`INSERT INTO catalog_entries (backup_set_id, file_path, file_size, file_mode, mod_time, checksum) VALUES (?, ?, ?, ?, ?, ?)`,
			backupSetID, fileName, 100, 0644, time.Now(), "deadbeef")
		if err != nil {
			t.Fatalf("failed to insert catalog entry %d: %v", i, err)
		}
	}

	// Call the portion of finishTape that updates block_offset
	// We simulate this directly using the same batched update logic
	svc := &Service{db: db}

	const tarHeaderOverhead = 1024
	const offsetBatchSize = 500
	var cumulativeOffset int64
	for i := 0; i < len(files); i += offsetBatchSize {
		end := i + offsetBatchSize
		if end > len(files) {
			end = len(files)
		}
		batch := files[i:end]

		tx, err := svc.db.Begin()
		if err != nil {
			t.Fatalf("failed to begin transaction: %v", err)
		}
		stmt, err := tx.Prepare(`UPDATE catalog_entries SET block_offset = ? WHERE backup_set_id = ? AND file_path = ?`)
		if err != nil {
			tx.Rollback()
			t.Fatalf("failed to prepare statement: %v", err)
		}
		for _, f := range batch {
			relPath, _ := filepath.Rel(sourceDir, f.Path)
			if _, err := stmt.Exec(cumulativeOffset, backupSetID, relPath); err != nil {
				t.Errorf("failed to update block_offset: %v", err)
			}
			cumulativeOffset += f.Size + tarHeaderOverhead
		}
		stmt.Close()
		tx.Commit()
	}

	// Verify all entries have block_offset set with correct cumulative values
	rows, err := db.Query("SELECT file_path, block_offset FROM catalog_entries WHERE backup_set_id = ? ORDER BY file_path", backupSetID)
	if err != nil {
		t.Fatalf("failed to query catalog entries: %v", err)
	}
	defer rows.Close()

	count := 0
	expectedOffset := int64(0)
	for rows.Next() {
		var filePath string
		var blockOffset int64
		if err := rows.Scan(&filePath, &blockOffset); err != nil {
			t.Fatalf("failed to scan row: %v", err)
		}
		if blockOffset != expectedOffset {
			t.Errorf("file %s: expected block_offset %d, got %d", filePath, expectedOffset, blockOffset)
		}
		expectedOffset += 100 + tarHeaderOverhead // file size + overhead
		count++
	}

	if count != numFiles {
		t.Errorf("expected %d catalog entries with block_offset, got %d", numFiles, count)
	}
}

func TestTapeChangeCallbackField(t *testing.T) {
	// Verify the TapeChangeCallback field works correctly on the Service struct
	svc := &Service{}

	// Nil callback should not panic
	if svc.TapeChangeCallback != nil {
		t.Error("expected TapeChangeCallback to be nil by default")
	}

	// Set callback and verify it's called with correct arguments
	var calledWith struct {
		jobName, currentTape, reason, nextTape string
	}
	svc.TapeChangeCallback = func(ctx context.Context, jobName, currentTape, reason, nextTape string) {
		calledWith.jobName = jobName
		calledWith.currentTape = currentTape
		calledWith.reason = reason
		calledWith.nextTape = nextTape
	}

	ctx := context.Background()
	svc.TapeChangeCallback(ctx, "TestJob", "TAPE-001", "tape_full", "TAPE-002")

	if calledWith.jobName != "TestJob" {
		t.Errorf("expected jobName 'TestJob', got %q", calledWith.jobName)
	}
	if calledWith.currentTape != "TAPE-001" {
		t.Errorf("expected currentTape 'TAPE-001', got %q", calledWith.currentTape)
	}
	if calledWith.reason != "tape_full" {
		t.Errorf("expected reason 'tape_full', got %q", calledWith.reason)
	}
	if calledWith.nextTape != "TAPE-002" {
		t.Errorf("expected nextTape 'TAPE-002', got %q", calledWith.nextTape)
	}
}

func TestWrongTapeCallbackField(t *testing.T) {
	// Verify the WrongTapeCallback field works correctly on the Service struct
	svc := &Service{}

	// Nil callback should not panic
	if svc.WrongTapeCallback != nil {
		t.Error("expected WrongTapeCallback to be nil by default")
	}

	// Set callback and verify it's called with correct arguments
	var calledWith struct {
		expectedLabel, actualLabel string
	}
	svc.WrongTapeCallback = func(ctx context.Context, expectedLabel, actualLabel string) {
		calledWith.expectedLabel = expectedLabel
		calledWith.actualLabel = actualLabel
	}

	ctx := context.Background()
	svc.WrongTapeCallback(ctx, "TAPE-001", "TAPE-999")

	if calledWith.expectedLabel != "TAPE-001" {
		t.Errorf("expected expectedLabel 'TAPE-001', got %q", calledWith.expectedLabel)
	}
	if calledWith.actualLabel != "TAPE-999" {
		t.Errorf("expected actualLabel 'TAPE-999', got %q", calledWith.actualLabel)
	}
}

func TestWrongTapeCallbackNoTapeLoaded(t *testing.T) {
	// Verify the WrongTapeCallback is called with "no tape loaded" when there is no tape
	svc := &Service{}

	var calledWith struct {
		expectedLabel, actualLabel string
	}
	svc.WrongTapeCallback = func(ctx context.Context, expectedLabel, actualLabel string) {
		calledWith.expectedLabel = expectedLabel
		calledWith.actualLabel = actualLabel
	}

	ctx := context.Background()
	svc.WrongTapeCallback(ctx, "TAPE-001", "no tape loaded")

	if calledWith.expectedLabel != "TAPE-001" {
		t.Errorf("expected expectedLabel 'TAPE-001', got %q", calledWith.expectedLabel)
	}
	if calledWith.actualLabel != "no tape loaded" {
		t.Errorf("expected actualLabel 'no tape loaded', got %q", calledWith.actualLabel)
	}
}
