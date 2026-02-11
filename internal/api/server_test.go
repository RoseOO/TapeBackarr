package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RoseOO/TapeBackarr/internal/backup"
	"github.com/RoseOO/TapeBackarr/internal/database"
	"github.com/RoseOO/TapeBackarr/internal/logging"
	"github.com/RoseOO/TapeBackarr/internal/scheduler"
	"github.com/RoseOO/TapeBackarr/internal/tape"

	"github.com/go-chi/chi/v5"
)

func TestStaticFileServing(t *testing.T) {
	// Create a temp directory with static files
	staticDir := t.TempDir()

	// Create an index.html
	indexContent := []byte("<html><body>TapeBackarr</body></html>")
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), indexContent, 0644); err != nil {
		t.Fatalf("failed to create index.html: %v", err)
	}

	// Create a CSS file in a subdirectory
	cssDir := filepath.Join(staticDir, "_app", "css")
	if err := os.MkdirAll(cssDir, 0755); err != nil {
		t.Fatalf("failed to create css dir: %v", err)
	}
	cssContent := []byte("body { margin: 0; }")
	if err := os.WriteFile(filepath.Join(cssDir, "style.css"), cssContent, 0644); err != nil {
		t.Fatalf("failed to create style.css: %v", err)
	}

	// Create a minimal server with just the static file serving
	s := &Server{
		router:    chi.NewRouter(),
		staticDir: staticDir,
	}
	s.setupRoutes()

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "root serves index.html",
			path:       "/",
			wantStatus: http.StatusOK,
			wantBody:   "TapeBackarr",
		},
		{
			name:       "static CSS file",
			path:       "/_app/css/style.css",
			wantStatus: http.StatusOK,
			wantBody:   "body { margin: 0; }",
		},
		{
			name:       "SPA fallback for unknown path",
			path:       "/dashboard",
			wantStatus: http.StatusOK,
			wantBody:   "TapeBackarr",
		},
		{
			name:       "API route still returns 404",
			path:       "/api/v1/nonexistent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rr := httptest.NewRecorder()
			s.router.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rr.Code)
			}

			if tt.wantBody != "" {
				body := rr.Body.String()
				if !strings.Contains(body, tt.wantBody) {
					t.Errorf("expected body to contain %q, got %q", tt.wantBody, body)
				}
			}
		})
	}
}

func TestNoStaticDir(t *testing.T) {
	// Server with empty staticDir should return 404 for root
	s := &Server{
		router:    chi.NewRouter(),
		staticDir: "",
	}
	s.setupRoutes()

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for root with no static dir, got %d", rr.Code)
	}
}

func TestHandleDashboardPoolStorage(t *testing.T) {
	// Create a temp database with migrations applied
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	// Migrations already create DAILY (id=1), WEEKLY (id=2), MONTHLY (id=3), ARCHIVE (id=4)
	// Insert test tapes into the DAILY pool (id=1)
	_, err = db.Exec("INSERT INTO tapes (uuid, barcode, label, pool_id, status, capacity_bytes, used_bytes) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"uuid-1", "TAPE01", "TAPE01", 1, "active", int64(1500000000000), int64(500000000000))
	if err != nil {
		t.Fatalf("failed to insert tape: %v", err)
	}
	_, err = db.Exec("INSERT INTO tapes (uuid, barcode, label, pool_id, status, capacity_bytes, used_bytes) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"uuid-2", "TAPE02", "TAPE02", 1, "active", int64(1500000000000), int64(1200000000000))
	if err != nil {
		t.Fatalf("failed to insert tape: %v", err)
	}

	// Insert a tape into the ARCHIVE pool (id=4)
	_, err = db.Exec("INSERT INTO tapes (uuid, barcode, label, pool_id, status, capacity_bytes, used_bytes) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"uuid-3", "TAPE03", "TAPE03", 4, "full", int64(1500000000000), int64(1500000000000))
	if err != nil {
		t.Fatalf("failed to insert tape: %v", err)
	}

	// Create server with the test database and a dummy tape service
	tapeService := tape.NewService("/dev/null", 65536)
	s := &Server{
		router:      chi.NewRouter(),
		db:          db,
		tapeService: tapeService,
	}

	// Call the dashboard handler directly (bypassing auth middleware)
	req := httptest.NewRequest("GET", "/api/v1/dashboard", nil)
	rr := httptest.NewRecorder()
	s.handleDashboard(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var result struct {
		TotalTapes  int `json:"total_tapes"`
		PoolStorage []struct {
			ID                 int64  `json:"id"`
			Name               string `json:"name"`
			TapeCount          int    `json:"tape_count"`
			TotalCapacityBytes int64  `json:"total_capacity_bytes"`
			TotalUsedBytes     int64  `json:"total_used_bytes"`
			TotalFreeBytes     int64  `json:"total_free_bytes"`
		} `json:"pool_storage"`
	}

	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.TotalTapes != 3 {
		t.Errorf("expected 3 total tapes, got %d", result.TotalTapes)
	}

	// Migrations create 4 pools: ARCHIVE, DAILY, MONTHLY, WEEKLY (sorted by name)
	if len(result.PoolStorage) != 4 {
		t.Fatalf("expected 4 pools in pool_storage, got %d", len(result.PoolStorage))
	}

	// Find ARCHIVE and DAILY pools in the results
	var archive, daily *struct {
		ID                 int64  `json:"id"`
		Name               string `json:"name"`
		TapeCount          int    `json:"tape_count"`
		TotalCapacityBytes int64  `json:"total_capacity_bytes"`
		TotalUsedBytes     int64  `json:"total_used_bytes"`
		TotalFreeBytes     int64  `json:"total_free_bytes"`
	}
	for i := range result.PoolStorage {
		switch result.PoolStorage[i].Name {
		case "ARCHIVE":
			archive = &result.PoolStorage[i]
		case "DAILY":
			daily = &result.PoolStorage[i]
		}
	}

	if archive == nil {
		t.Fatal("ARCHIVE pool not found in pool_storage")
	}
	if archive.TapeCount != 1 {
		t.Errorf("expected ARCHIVE to have 1 tape, got %d", archive.TapeCount)
	}
	if archive.TotalCapacityBytes != 1500000000000 {
		t.Errorf("expected ARCHIVE capacity 1500000000000, got %d", archive.TotalCapacityBytes)
	}
	if archive.TotalUsedBytes != 1500000000000 {
		t.Errorf("expected ARCHIVE used 1500000000000, got %d", archive.TotalUsedBytes)
	}
	if archive.TotalFreeBytes != 0 {
		t.Errorf("expected ARCHIVE free 0, got %d", archive.TotalFreeBytes)
	}

	if daily == nil {
		t.Fatal("DAILY pool not found in pool_storage")
	}
	if daily.TapeCount != 2 {
		t.Errorf("expected DAILY to have 2 tapes, got %d", daily.TapeCount)
	}
	if daily.TotalCapacityBytes != 3000000000000 {
		t.Errorf("expected DAILY capacity 3000000000000, got %d", daily.TotalCapacityBytes)
	}
	if daily.TotalUsedBytes != 1700000000000 {
		t.Errorf("expected DAILY used 1700000000000, got %d", daily.TotalUsedBytes)
	}
	if daily.TotalFreeBytes != 1300000000000 {
		t.Errorf("expected DAILY free 1300000000000, got %d", daily.TotalFreeBytes)
	}
}

func TestHandleDashboardExtendedStats(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	// Insert a source
	_, err = db.Exec("INSERT INTO backup_sources (name, source_type, path) VALUES (?, ?, ?)", "TestSource", "smb", "/mnt/share")
	if err != nil {
		t.Fatalf("failed to insert source: %v", err)
	}

	// Insert a completed backup set with file_count
	_, err = db.Exec("INSERT INTO tapes (uuid, barcode, label, pool_id, status, capacity_bytes, used_bytes) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"uuid-ext", "TAPEEXT", "TAPEEXT", 1, "active", int64(1500000000000), int64(500000000000))
	if err != nil {
		t.Fatalf("failed to insert tape: %v", err)
	}
	_, err = db.Exec(`INSERT INTO backup_jobs (name, source_id, pool_id, backup_type, retention_days, enabled) VALUES (?, ?, ?, ?, ?, ?)`,
		"TestJob", 1, 1, "full", 30, true)
	if err != nil {
		t.Fatalf("failed to insert job: %v", err)
	}
	_, err = db.Exec(`INSERT INTO backup_sets (job_id, tape_id, backup_type, status, file_count, total_bytes, start_time, end_time)
		VALUES (?, ?, ?, ?, ?, ?, datetime('now', '-1 hour'), datetime('now'))`, 1, 1, "full", "completed", 42, int64(1000000))
	if err != nil {
		t.Fatalf("failed to insert backup set: %v", err)
	}

	tapeService := tape.NewService("/dev/null", 65536)
	s := &Server{
		router:      chi.NewRouter(),
		db:          db,
		tapeService: tapeService,
	}

	req := httptest.NewRequest("GET", "/api/v1/dashboard", nil)
	rr := httptest.NewRecorder()
	s.handleDashboard(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var result struct {
		TotalFilesCataloged int64   `json:"total_files_cataloged"`
		TotalSources        int     `json:"total_sources"`
		TotalEncryptionKeys int     `json:"total_encryption_keys"`
		TotalBackupSets     int     `json:"total_backup_sets"`
		LastBackupTime      *string `json:"last_backup_time"`
		OldestBackup        *string `json:"oldest_backup"`
	}

	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.TotalFilesCataloged != 42 {
		t.Errorf("expected 42 total files cataloged, got %d", result.TotalFilesCataloged)
	}
	if result.TotalSources != 1 {
		t.Errorf("expected 1 source, got %d", result.TotalSources)
	}
	if result.TotalEncryptionKeys != 0 {
		t.Errorf("expected 0 encryption keys, got %d", result.TotalEncryptionKeys)
	}
	if result.TotalBackupSets != 1 {
		t.Errorf("expected 1 backup set, got %d", result.TotalBackupSets)
	}
	if result.LastBackupTime == nil {
		t.Error("expected last_backup_time to be set")
	}
	if result.OldestBackup == nil {
		t.Error("expected oldest_backup to be set")
	}
}

func TestTelegramFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
		{1099511627776, "1.00 TB"},
		{500000000, "476.84 MB"},
		{1500000000000, "1.36 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := telegramFormatBytes(tt.input)
			if result != tt.expected {
				t.Errorf("telegramFormatBytes(%d) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTelegramFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{0, "0s"},
		{30 * time.Second, "30s"},
		{4*time.Minute + 25*time.Second, "4m 25s"},
		{1*time.Hour + 30*time.Minute + 5*time.Second, "1h 30m 5s"},
		{2 * time.Hour, "2h 0m 0s"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := telegramFormatDuration(tt.input)
			if result != tt.expected {
				t.Errorf("telegramFormatDuration(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTelegramDrivesCommand(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	// Insert a test drive with display_name
	_, err = db.Exec(`INSERT INTO tape_drives (device_path, display_name, status, enabled) VALUES (?, ?, ?, ?)`,
		"/dev/nst0", "Main Drive", "ready", 1)
	if err != nil {
		t.Fatalf("failed to insert drive: %v", err)
	}

	tapeService := tape.NewService("/dev/null", 65536)
	s := &Server{
		db:          db,
		tapeService: tapeService,
	}

	result := s.telegramDrivesCommand()

	if !strings.Contains(result, "Tape Drives") {
		t.Error("expected result to contain 'Tape Drives'")
	}
	if !strings.Contains(result, "Main Drive") {
		t.Errorf("expected result to contain 'Main Drive', got: %s", result)
	}
	if !strings.Contains(result, "/dev/nst0") {
		t.Errorf("expected result to contain '/dev/nst0', got: %s", result)
	}
	if !strings.Contains(result, "ready") {
		t.Errorf("expected result to contain 'ready', got: %s", result)
	}
}

func TestTelegramDrivesCommandNoDrives(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	tapeService := tape.NewService("/dev/null", 65536)
	s := &Server{
		db:          db,
		tapeService: tapeService,
	}

	result := s.telegramDrivesCommand()

	if !strings.Contains(result, "No drives configured") {
		t.Errorf("expected result to contain 'No drives configured', got: %s", result)
	}
}

// setupTestServerWithBackupSet creates a test server with a backup set in the given status.
// Returns the server, the backup set ID, and a cleanup function.
func setupTestServerWithBackupSet(t *testing.T, status string) (*Server, int64) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	logger, err := logging.NewLogger("warn", "text", "")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Insert a tape (pool_id=1 from migrations: DAILY)
	_, err = db.Exec("INSERT INTO tapes (uuid, barcode, label, pool_id, status, capacity_bytes, used_bytes) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"uuid-t1", "TEST01", "TEST01", 1, "active", int64(1500000000000), int64(0))
	if err != nil {
		t.Fatalf("failed to insert tape: %v", err)
	}

	// Insert a backup source
	_, err = db.Exec("INSERT INTO backup_sources (name, source_type, path) VALUES (?, ?, ?)", "test-source", "local", "/tmp/test")
	if err != nil {
		t.Fatalf("failed to insert backup source: %v", err)
	}

	// Insert a backup job
	_, err = db.Exec("INSERT INTO backup_jobs (name, source_id, pool_id, backup_type, schedule_cron, retention_days) VALUES (?, ?, ?, ?, ?, ?)",
		"test-job", 1, 1, "full", "0 0 * * *", 30)
	if err != nil {
		t.Fatalf("failed to insert backup job: %v", err)
	}

	// Insert a backup set with the given status
	now := time.Now()
	result, err := db.Exec("INSERT INTO backup_sets (job_id, tape_id, backup_type, start_time, status, file_count, total_bytes) VALUES (?, ?, ?, ?, ?, ?, ?)",
		1, 1, "full", now, status, 0, 0)
	if err != nil {
		t.Fatalf("failed to insert backup set: %v", err)
	}
	setID, _ := result.LastInsertId()

	tapeService := tape.NewService("/dev/null", 65536)
	r := chi.NewRouter()
	s := &Server{
		router:      r,
		db:          db,
		tapeService: tapeService,
		logger:      logger,
	}

	// Wire up just the backup set routes
	r.Delete("/api/v1/backup-sets/{id}", s.handleDeleteBackupSet)
	r.Post("/api/v1/backup-sets/{id}/cancel", s.handleCancelBackupSet)

	return s, setID
}

func TestDeleteBackupSetWithForeignKeys(t *testing.T) {
	s, setID := setupTestServerWithBackupSet(t, "failed")

	// Insert a job_execution referencing this backup set (this is the FK that was missed)
	_, err := s.db.Exec("INSERT INTO job_executions (job_id, backup_set_id, status) VALUES (?, ?, ?)", 1, setID, "failed")
	if err != nil {
		t.Fatalf("failed to insert job_execution: %v", err)
	}

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/backup-sets/%d", setID), nil)
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "deleted" {
		t.Errorf("expected status 'deleted', got %q", resp["status"])
	}

	// Verify backup set is gone
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM backup_sets WHERE id = ?", setID).Scan(&count)
	if count != 0 {
		t.Errorf("expected backup set to be deleted, but it still exists")
	}
}

func TestDeleteCancelledBackupSet(t *testing.T) {
	s, setID := setupTestServerWithBackupSet(t, "cancelled")

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/backup-sets/%d", setID), nil)
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "deleted" {
		t.Errorf("expected status 'deleted', got %q", resp["status"])
	}
}

func TestCancelRunningBackupSet(t *testing.T) {
	s, setID := setupTestServerWithBackupSet(t, "running")

	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/backup-sets/%d/cancel", setID), nil)
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "cancelled" {
		t.Errorf("expected status 'cancelled', got %q", resp["status"])
	}

	// Verify the backup set status was updated
	var status string
	s.db.QueryRow("SELECT status FROM backup_sets WHERE id = ?", setID).Scan(&status)
	if status != "cancelled" {
		t.Errorf("expected backup set status 'cancelled', got %q", status)
	}
}

func TestCancelCompletedBackupSetFails(t *testing.T) {
	s, setID := setupTestServerWithBackupSet(t, "completed")

	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/backup-sets/%d/cancel", setID), nil)
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteRunningBackupSetFails(t *testing.T) {
	s, setID := setupTestServerWithBackupSet(t, "running")

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/backup-sets/%d", setID), nil)
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestTelegramActiveCommandCatalogingPhase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	tapeService := tape.NewService("/dev/null", 65536)
	backupSvc := backup.NewService(db, tapeService, nil, 65536, 512, 0)

	// Inject an active job in "cataloging" phase
	backupSvc.InjectTestJob(1, &backup.JobProgress{
		JobID:             1,
		JobName:           "test-backup",
		Phase:             "cataloging",
		Status:            "running",
		TapeLabel:         "TAPE-001",
		DevicePath:        "/dev/nst0",
		BytesWritten:      1000000000,
		TotalBytes:        5000000000,
		TotalFiles:        100,
		FileCount:         42,
		TapeCapacityBytes: 1500000000000,
		TapeUsedBytes:     500000000000,
		StartTime:         time.Now().Add(-1 * time.Hour),
	})
	defer backupSvc.RemoveTestJob(1)

	s := &Server{
		db:            db,
		tapeService:   tapeService,
		backupService: backupSvc,
	}

	result := s.telegramActiveCommand()

	// Should contain the cataloging phase icon
	if !strings.Contains(result, "📋") {
		t.Errorf("expected cataloging icon '📋', got: %s", result)
	}
	if !strings.Contains(result, "Phase: cataloging") {
		t.Errorf("expected 'Phase: cataloging', got: %s", result)
	}
	// Should show cataloging-specific progress
	if !strings.Contains(result, "Cataloging 42/100 files") {
		t.Errorf("expected cataloging progress line, got: %s", result)
	}
	// Should NOT show stale speed during cataloging
	if strings.Contains(result, "Speed:") {
		t.Errorf("should not show stale speed during cataloging, got: %s", result)
	}
	// Should still have tape space info
	if !strings.Contains(result, "Tape Space:") {
		t.Errorf("expected tape space info, got: %s", result)
	}
}

func TestTelegramActiveCommandStreamingPhase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	tapeService := tape.NewService("/dev/null", 65536)
	backupSvc := backup.NewService(db, tapeService, nil, 65536, 512, 0)

	// Inject an active job in "streaming" phase
	backupSvc.InjectTestJob(1, &backup.JobProgress{
		JobID:             1,
		JobName:           "test-backup",
		Phase:             "streaming",
		Status:            "running",
		TapeLabel:         "TAPE-001",
		DevicePath:        "/dev/nst0",
		BytesWritten:      1000000000,
		TotalBytes:        5000000000,
		TotalFiles:        100,
		FileCount:         20,
		WriteSpeed:        100000000,
		TapeCapacityBytes: 1500000000000,
		TapeUsedBytes:     500000000000,
		StartTime:         time.Now().Add(-1 * time.Hour),
	})
	defer backupSvc.RemoveTestJob(1)

	s := &Server{
		db:            db,
		tapeService:   tapeService,
		backupService: backupSvc,
	}

	result := s.telegramActiveCommand()

	// Should contain the streaming phase icon
	if !strings.Contains(result, "📼") {
		t.Errorf("expected streaming icon '📼', got: %s", result)
	}
	// Should show speed during streaming
	if !strings.Contains(result, "Speed:") {
		t.Errorf("expected speed during streaming, got: %s", result)
	}
	// Should show files count during streaming
	if !strings.Contains(result, "Files: 20/100") {
		t.Errorf("expected files count, got: %s", result)
	}
}

func TestDeleteJobWithForeignKeys(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	logger, err := logging.NewLogger("warn", "text", "")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Insert a backup source, job, tape, backup set, and job execution
	_, err = db.Exec("INSERT INTO backup_sources (name, source_type, path) VALUES (?, ?, ?)", "src", "local", "/tmp")
	if err != nil {
		t.Fatalf("failed to insert source: %v", err)
	}
	_, err = db.Exec("INSERT INTO backup_jobs (name, source_id, pool_id, backup_type, retention_days, enabled) VALUES (?, ?, ?, ?, ?, ?)",
		"TestJob", 1, 1, "full", 30, true)
	if err != nil {
		t.Fatalf("failed to insert job: %v", err)
	}
	_, err = db.Exec("INSERT INTO tapes (uuid, barcode, label, pool_id, status, capacity_bytes, used_bytes) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"uuid-1", "T01", "T01", 1, "active", int64(1500000000000), int64(0))
	if err != nil {
		t.Fatalf("failed to insert tape: %v", err)
	}
	_, err = db.Exec("INSERT INTO backup_sets (job_id, tape_id, backup_type, start_time, status) VALUES (?, ?, ?, datetime('now'), ?)",
		1, 1, "full", "completed")
	if err != nil {
		t.Fatalf("failed to insert backup set: %v", err)
	}
	_, err = db.Exec("INSERT INTO job_executions (job_id, backup_set_id, status) VALUES (?, ?, ?)", 1, 1, "completed")
	if err != nil {
		t.Fatalf("failed to insert job execution: %v", err)
	}
	_, err = db.Exec("INSERT INTO catalog_entries (backup_set_id, file_path, file_size) VALUES (?, ?, ?)", 1, "/test/file.txt", 1024)
	if err != nil {
		t.Fatalf("failed to insert catalog entry: %v", err)
	}

	tapeService := tape.NewService("/dev/null", 65536)
	sched := scheduler.NewService(db, logger, nil)
	r := chi.NewRouter()
	s := &Server{
		router:      r,
		db:          db,
		tapeService: tapeService,
		logger:      logger,
		scheduler:   sched,
	}

	r.Delete("/api/v1/jobs/{id}", s.handleDeleteJob)

	req := httptest.NewRequest("DELETE", "/api/v1/jobs/1", nil)
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "deleted" {
		t.Errorf("expected status 'deleted', got %q", resp["status"])
	}

	// Verify all related records are gone
	var count int
	db.QueryRow("SELECT COUNT(*) FROM backup_jobs WHERE id = 1").Scan(&count)
	if count != 0 {
		t.Error("backup job should be deleted")
	}
	db.QueryRow("SELECT COUNT(*) FROM backup_sets WHERE job_id = 1").Scan(&count)
	if count != 0 {
		t.Error("backup sets should be deleted")
	}
	db.QueryRow("SELECT COUNT(*) FROM job_executions WHERE job_id = 1").Scan(&count)
	if count != 0 {
		t.Error("job executions should be deleted")
	}
	db.QueryRow("SELECT COUNT(*) FROM catalog_entries WHERE backup_set_id = 1").Scan(&count)
	if count != 0 {
		t.Error("catalog entries should be deleted")
	}
}

func TestDeleteProxmoxJobWithForeignKeys(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	logger, err := logging.NewLogger("warn", "text", "")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Insert a proxmox backup job and execution
	_, err = db.Exec("INSERT INTO proxmox_backup_jobs (name, pool_id, enabled) VALUES (?, ?, ?)", "PBSJob", 1, true)
	if err != nil {
		t.Fatalf("failed to insert proxmox job: %v", err)
	}
	_, err = db.Exec("INSERT INTO proxmox_job_executions (job_id, status) VALUES (?, ?)", 1, "completed")
	if err != nil {
		t.Fatalf("failed to insert proxmox execution: %v", err)
	}

	r := chi.NewRouter()
	s := &Server{
		router: r,
		db:     db,
		logger: logger,
	}

	r.Delete("/api/v1/proxmox/jobs/{id}", s.handleProxmoxDeleteJob)

	req := httptest.NewRequest("DELETE", "/api/v1/proxmox/jobs/1", nil)
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["message"] != "Proxmox backup job deleted" {
		t.Errorf("expected message 'Proxmox backup job deleted', got %q", resp["message"])
	}

	// Verify all related records are gone
	var count int
	db.QueryRow("SELECT COUNT(*) FROM proxmox_backup_jobs WHERE id = 1").Scan(&count)
	if count != 0 {
		t.Error("proxmox backup job should be deleted")
	}
	db.QueryRow("SELECT COUNT(*) FROM proxmox_job_executions WHERE job_id = 1").Scan(&count)
	if count != 0 {
		t.Error("proxmox job executions should be deleted")
	}
}

func TestDeleteJobWithParentSetFK(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	logger, err := logging.NewLogger("warn", "text", "")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Create source, job, tape
	if _, err := db.Exec("INSERT INTO backup_sources (name, source_type, path) VALUES (?, ?, ?)", "src", "local", "/tmp"); err != nil {
		t.Fatalf("failed to insert source: %v", err)
	}
	if _, err := db.Exec("INSERT INTO backup_jobs (name, source_id, pool_id, backup_type, retention_days, enabled) VALUES (?, ?, ?, ?, ?, ?)",
		"TestJob", 1, 1, "full", 30, true); err != nil {
		t.Fatalf("failed to insert job: %v", err)
	}
	if _, err := db.Exec("INSERT INTO tapes (uuid, barcode, label, pool_id, status, capacity_bytes, used_bytes) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"uuid-1", "T01", "T01", 1, "active", int64(1500000000000), int64(0)); err != nil {
		t.Fatalf("failed to insert tape: %v", err)
	}

	// Create parent backup set (full), then child (incremental) referencing it
	if _, err := db.Exec("INSERT INTO backup_sets (job_id, tape_id, backup_type, start_time, status) VALUES (?, ?, ?, datetime('now'), ?)",
		1, 1, "full", "completed"); err != nil {
		t.Fatalf("failed to insert parent backup set: %v", err)
	}
	if _, err := db.Exec("INSERT INTO backup_sets (job_id, tape_id, backup_type, start_time, status, parent_set_id) VALUES (?, ?, ?, datetime('now'), ?, ?)",
		1, 1, "incremental", "completed", 1); err != nil {
		t.Fatalf("failed to insert child backup set: %v", err)
	}

	tapeService := tape.NewService("/dev/null", 65536)
	sched := scheduler.NewService(db, logger, nil)
	r := chi.NewRouter()
	s := &Server{
		router:      r,
		db:          db,
		tapeService: tapeService,
		logger:      logger,
		scheduler:   sched,
	}

	r.Delete("/api/v1/jobs/{id}", s.handleDeleteJob)

	req := httptest.NewRequest("DELETE", "/api/v1/jobs/1", nil)
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM backup_sets WHERE job_id = 1").Scan(&count)
	if count != 0 {
		t.Error("backup sets with parent_set_id self-reference should be deleted")
	}
	db.QueryRow("SELECT COUNT(*) FROM backup_jobs WHERE id = 1").Scan(&count)
	if count != 0 {
		t.Error("backup job should be deleted")
	}
}

func TestDeleteJobWithSpanningSetFK(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	logger, err := logging.NewLogger("warn", "text", "")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Create source, job, tape
	if _, err := db.Exec("INSERT INTO backup_sources (name, source_type, path) VALUES (?, ?, ?)", "src", "local", "/tmp"); err != nil {
		t.Fatalf("failed to insert source: %v", err)
	}
	if _, err := db.Exec("INSERT INTO backup_jobs (name, source_id, pool_id, backup_type, retention_days, enabled) VALUES (?, ?, ?, ?, ?, ?)",
		"TestJob", 1, 1, "full", 30, true); err != nil {
		t.Fatalf("failed to insert job: %v", err)
	}
	if _, err := db.Exec("INSERT INTO tapes (uuid, barcode, label, pool_id, status, capacity_bytes, used_bytes) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"uuid-1", "T01", "T01", 1, "active", int64(1500000000000), int64(0)); err != nil {
		t.Fatalf("failed to insert tape: %v", err)
	}

	// Create backup set, spanning set, and tape_change_request referencing the spanning set
	if _, err := db.Exec("INSERT INTO backup_sets (job_id, tape_id, backup_type, start_time, status) VALUES (?, ?, ?, datetime('now'), ?)",
		1, 1, "full", "completed"); err != nil {
		t.Fatalf("failed to insert backup set: %v", err)
	}
	if _, err := db.Exec("INSERT INTO tape_spanning_sets (job_id, total_tapes, status) VALUES (?, ?, ?)", 1, 1, "completed"); err != nil {
		t.Fatalf("failed to insert spanning set: %v", err)
	}
	if _, err := db.Exec("INSERT INTO tape_change_requests (spanning_set_id, current_tape_id, reason, status) VALUES (?, ?, ?, ?)",
		1, 1, "tape_full", "completed"); err != nil {
		t.Fatalf("failed to insert tape change request: %v", err)
	}

	tapeService := tape.NewService("/dev/null", 65536)
	sched := scheduler.NewService(db, logger, nil)
	r := chi.NewRouter()
	s := &Server{
		router:      r,
		db:          db,
		tapeService: tapeService,
		logger:      logger,
		scheduler:   sched,
	}

	r.Delete("/api/v1/jobs/{id}", s.handleDeleteJob)

	req := httptest.NewRequest("DELETE", "/api/v1/jobs/1", nil)
	rr := httptest.NewRecorder()
	s.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM tape_spanning_sets WHERE job_id = 1").Scan(&count)
	if count != 0 {
		t.Error("tape spanning sets should be deleted")
	}
	db.QueryRow("SELECT COUNT(*) FROM tape_change_requests WHERE spanning_set_id = 1").Scan(&count)
	if count != 0 {
		t.Error("tape change requests should be deleted")
	}
	db.QueryRow("SELECT COUNT(*) FROM backup_jobs WHERE id = 1").Scan(&count)
	if count != 0 {
		t.Error("backup job should be deleted")
	}
}

func TestSelectTapeFromPoolPrefersDriveLoaded(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	// Create two active tapes in the same pool
	if _, err := db.Exec("INSERT INTO tapes (uuid, barcode, label, pool_id, status, capacity_bytes, used_bytes) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"uuid-a", "T01", "TapeA", 1, "active", int64(1500000000000), int64(100)); err != nil {
		t.Fatalf("failed to insert tapeA: %v", err)
	}
	if _, err := db.Exec("INSERT INTO tapes (uuid, barcode, label, pool_id, status, capacity_bytes, used_bytes) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"uuid-b", "T02", "TapeB", 1, "active", int64(1500000000000), int64(500)); err != nil {
		t.Fatalf("failed to insert tapeB: %v", err)
	}

	// Load TapeB into a drive (TapeA is not in any drive)
	if _, err := db.Exec("INSERT INTO tape_drives (device_path, display_name, status, enabled, current_tape_id) VALUES (?, ?, ?, ?, ?)",
		"/dev/nst0", "Drive0", "ready", 1, 2); err != nil {
		t.Fatalf("failed to insert drive: %v", err)
	}

	s := &Server{db: db}

	tapeID, tapeLabel, err := s.selectTapeFromPool(1, 30)
	if err != nil {
		t.Fatalf("selectTapeFromPool failed: %v", err)
	}

	// TapeB (id=2) should be selected because it's in a drive, even though TapeA has less used space
	if tapeID != 2 {
		t.Errorf("expected tape id 2 (loaded in drive), got %d", tapeID)
	}
	if tapeLabel != "TapeB" {
		t.Errorf("expected tape label 'TapeB', got %q", tapeLabel)
	}
}

func TestHandleDownloadDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	s := &Server{
		router: chi.NewRouter(),
		db:     db,
	}

	req := httptest.NewRequest("GET", "/api/v1/database-backup/download", nil)
	rr := httptest.NewRecorder()
	s.handleDownloadDatabase(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Check headers
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/octet-stream" {
		t.Errorf("expected Content-Type application/octet-stream, got %q", contentType)
	}

	disposition := rr.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "tapebackarr-backup-") {
		t.Errorf("expected Content-Disposition to contain backup filename, got %q", disposition)
	}
	if !strings.Contains(disposition, ".db") {
		t.Errorf("expected Content-Disposition to have .db extension, got %q", disposition)
	}

	// Verify the downloaded content is a valid SQLite database
	if rr.Body.Len() == 0 {
		t.Fatal("expected non-empty response body")
	}

	// SQLite files start with "SQLite format 3"
	header := rr.Body.String()[:16]
	if !strings.HasPrefix(header, "SQLite format 3") {
		t.Error("downloaded file does not appear to be a valid SQLite database")
	}
}

func TestHandleUploadDatabase(t *testing.T) {
	// Create source database to serve as the "uploaded" file
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "upload.db")
	srcDB, err := database.New(srcPath)
	if err != nil {
		t.Fatalf("failed to create source database: %v", err)
	}
	if err := srcDB.Migrate(); err != nil {
		t.Fatalf("failed to migrate source database: %v", err)
	}
	// Insert a marker to verify restore
	srcDB.Exec("INSERT INTO audit_logs (action, resource_type, resource_id, details) VALUES ('test_marker', 'test', 0, 'uploaded db marker')")
	srcDB.Close()

	// Create the server's database
	srvDir := t.TempDir()
	srvPath := filepath.Join(srvDir, "server.db")
	srvDB, err := database.New(srvPath)
	if err != nil {
		t.Fatalf("failed to create server database: %v", err)
	}
	defer func() {
		// The server's db reference may have changed after upload
	}()

	if err := srvDB.Migrate(); err != nil {
		t.Fatalf("failed to migrate server database: %v", err)
	}

	s := &Server{
		router: chi.NewRouter(),
		db:     srvDB,
	}

	// Read the source db file
	srcData, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("failed to read source db: %v", err)
	}

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("database", "upload.db")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(srcData)); err != nil {
		t.Fatalf("failed to write form file: %v", err)
	}
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/database-backup/upload", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	s.handleUploadDatabase(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result["status"] != "restored" {
		t.Errorf("expected status 'restored', got %q", result["status"])
	}

	// Verify the marker exists in the restored database
	var marker string
	err = s.db.QueryRow("SELECT details FROM audit_logs WHERE action = 'test_marker'").Scan(&marker)
	if err != nil {
		t.Fatalf("failed to find marker in restored database: %v", err)
	}
	if marker != "uploaded db marker" {
		t.Errorf("expected marker 'uploaded db marker', got %q", marker)
	}

	s.db.Close()
}

func TestHandleUploadDatabaseInvalidFile(t *testing.T) {
	srvDir := t.TempDir()
	srvPath := filepath.Join(srvDir, "server.db")
	srvDB, err := database.New(srvPath)
	if err != nil {
		t.Fatalf("failed to create server database: %v", err)
	}
	defer srvDB.Close()

	if err := srvDB.Migrate(); err != nil {
		t.Fatalf("failed to migrate server database: %v", err)
	}

	s := &Server{
		router: chi.NewRouter(),
		db:     srvDB,
	}

	// Create multipart form with invalid (non-SQLite) data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("database", "bad.db")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	part.Write([]byte("this is not a sqlite database"))
	writer.Close()

	req := httptest.NewRequest("POST", "/api/v1/database-backup/upload", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	s.handleUploadDatabase(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid database, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestMoveFile(t *testing.T) {
	t.Run("same directory", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "src.txt")
		dst := filepath.Join(dir, "dst.txt")
		content := []byte("hello world")

		if err := os.WriteFile(src, content, 0644); err != nil {
			t.Fatal(err)
		}

		if err := moveFile(src, dst); err != nil {
			t.Fatalf("moveFile failed: %v", err)
		}

		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("failed to read dst: %v", err)
		}
		if string(got) != string(content) {
			t.Errorf("content mismatch: got %q, want %q", got, content)
		}

		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Error("source file should have been removed")
		}
	})

	t.Run("different directories", func(t *testing.T) {
		srcDir := t.TempDir()
		dstDir := t.TempDir()
		src := filepath.Join(srcDir, "src.txt")
		dst := filepath.Join(dstDir, "dst.txt")
		content := []byte("cross directory move")

		if err := os.WriteFile(src, content, 0644); err != nil {
			t.Fatal(err)
		}

		if err := moveFile(src, dst); err != nil {
			t.Fatalf("moveFile failed: %v", err)
		}

		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("failed to read dst: %v", err)
		}
		if string(got) != string(content) {
			t.Errorf("content mismatch: got %q, want %q", got, content)
		}

		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Error("source file should have been removed")
		}
	})

	t.Run("nonexistent source", func(t *testing.T) {
		dir := t.TempDir()
		err := moveFile(filepath.Join(dir, "missing.txt"), filepath.Join(dir, "dst.txt"))
		if err == nil {
			t.Error("expected error for nonexistent source")
		}
	})
}
