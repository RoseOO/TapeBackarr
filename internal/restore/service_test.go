package restore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RoseOO/TapeBackarr/internal/database"
	"github.com/RoseOO/TapeBackarr/internal/models"
)

func setupTestDB(t *testing.T) *database.DB {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}
	return db
}

func setupTestData(t *testing.T, db *database.DB) int64 {
	// Insert a tape pool
	_, err := db.Exec(`INSERT INTO tape_pools (name, description, retention_days) VALUES (?, ?, ?)`,
		"test_pool", "Test pool", 30)
	if err != nil {
		t.Fatalf("failed to insert tape pool: %v", err)
	}

	// Insert a tape
	result, err := db.Exec(`INSERT INTO tapes (barcode, label, pool_id, status, capacity_bytes, used_bytes) VALUES (?, ?, ?, ?, ?, ?)`,
		"TEST001", "Test Tape", 1, "active", 1000000000, 0)
	if err != nil {
		t.Fatalf("failed to insert tape: %v", err)
	}
	tapeID, _ := result.LastInsertId()

	// Insert a backup source
	_, err = db.Exec(`INSERT INTO backup_sources (name, source_type, path, enabled) VALUES (?, ?, ?, ?)`,
		"test_source", "local", "/test/source", true)
	if err != nil {
		t.Fatalf("failed to insert backup source: %v", err)
	}

	// Insert a backup job
	_, err = db.Exec(`INSERT INTO backup_jobs (name, source_id, pool_id, backup_type, schedule_cron, retention_days, enabled) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"test_job", 1, 1, "full", "0 2 * * *", 30, true)
	if err != nil {
		t.Fatalf("failed to insert backup job: %v", err)
	}

	// Insert a backup set
	result, err = db.Exec(`INSERT INTO backup_sets (job_id, tape_id, backup_type, start_time, status, file_count, total_bytes) VALUES (?, ?, ?, datetime('now'), ?, ?, ?)`,
		1, tapeID, "full", "completed", 5, 5000)
	if err != nil {
		t.Fatalf("failed to insert backup set: %v", err)
	}
	backupSetID, _ := result.LastInsertId()

	// Insert catalog entries with folder structure
	testFiles := []struct {
		path     string
		size     int64
		checksum string
	}{
		{"documents/report.pdf", 1000, "abc123def456789012345678901234567890123456789012345678901234"},
		{"documents/notes.txt", 500, "def456abc123789012345678901234567890123456789012345678901234"},
		{"documents/subfolder/data.csv", 800, "ghi789abc123def456012345678901234567890123456789012345678901"},
		{"documents/subfolder/deep/config.json", 200, "jkl012abc123def456789012345678901234567890123456789012345678"},
		{"images/photo.jpg", 2500, "mno345abc123def456789012345678901234567890123456789012345678"},
	}

	for _, f := range testFiles {
		_, err := db.Exec(`INSERT INTO catalog_entries (backup_set_id, file_path, file_size, file_mode, mod_time, checksum) VALUES (?, ?, ?, ?, datetime('now'), ?)`,
			backupSetID, f.path, f.size, 0644, f.checksum)
		if err != nil {
			t.Fatalf("failed to insert catalog entry: %v", err)
		}
	}

	return backupSetID
}

func TestGetFilesInFolders(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	backupSetID := setupTestData(t, db)

	svc := &Service{db: db}

	tests := []struct {
		name          string
		folderPaths   []string
		expectedCount int
	}{
		{
			name:          "single folder",
			folderPaths:   []string{"documents"},
			expectedCount: 4, // report.pdf, notes.txt, subfolder/data.csv, subfolder/deep/config.json
		},
		{
			name:          "subfolder only",
			folderPaths:   []string{"documents/subfolder"},
			expectedCount: 2, // data.csv, deep/config.json
		},
		{
			name:          "deep subfolder",
			folderPaths:   []string{"documents/subfolder/deep"},
			expectedCount: 1, // config.json
		},
		{
			name:          "multiple folders",
			folderPaths:   []string{"documents", "images"},
			expectedCount: 5, // all files
		},
		{
			name:          "non-existent folder",
			folderPaths:   []string{"nonexistent"},
			expectedCount: 0,
		},
		{
			name:          "empty folder list",
			folderPaths:   []string{},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, err := svc.getFilesInFolders(context.Background(), backupSetID, tt.folderPaths)
			if err != nil {
				t.Fatalf("getFilesInFolders failed: %v", err)
			}
			if len(files) != tt.expectedCount {
				t.Errorf("expected %d files, got %d: %v", tt.expectedCount, len(files), files)
			}
		})
	}
}

func TestGetFolderContents(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	backupSetID := setupTestData(t, db)

	svc := &Service{db: db}

	tests := []struct {
		name                string
		folderPath          string
		expectedFileCount   int
		expectedFolderCount int
	}{
		{
			name:                "documents folder",
			folderPath:          "documents",
			expectedFileCount:   2, // report.pdf, notes.txt
			expectedFolderCount: 1, // subfolder
		},
		{
			name:                "subfolder",
			folderPath:          "documents/subfolder",
			expectedFileCount:   1, // data.csv
			expectedFolderCount: 1, // deep
		},
		{
			name:                "deep folder",
			folderPath:          "documents/subfolder/deep",
			expectedFileCount:   1, // config.json
			expectedFolderCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, subfolders, err := svc.GetFolderContents(context.Background(), backupSetID, tt.folderPath)
			if err != nil {
				t.Fatalf("GetFolderContents failed: %v", err)
			}
			if len(files) != tt.expectedFileCount {
				t.Errorf("expected %d files, got %d", tt.expectedFileCount, len(files))
			}
			if len(subfolders) != tt.expectedFolderCount {
				t.Errorf("expected %d subfolders, got %d", tt.expectedFolderCount, len(subfolders))
			}
		})
	}
}

func TestBrowseCatalog(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	backupSetID := setupTestData(t, db)

	svc := &Service{db: db}

	t.Run("returns all entries with no limit", func(t *testing.T) {
		entries, err := svc.BrowseCatalog(context.Background(), backupSetID, "", 0, 0)
		if err != nil {
			t.Fatalf("BrowseCatalog failed: %v", err)
		}
		if len(entries) != 5 {
			t.Errorf("expected 5 entries, got %d", len(entries))
		}
	})

	t.Run("returns all entries with high limit", func(t *testing.T) {
		entries, err := svc.BrowseCatalog(context.Background(), backupSetID, "", 100, 0)
		if err != nil {
			t.Fatalf("BrowseCatalog failed: %v", err)
		}
		if len(entries) != 5 {
			t.Errorf("expected 5 entries, got %d", len(entries))
		}
	})

	t.Run("respects limit", func(t *testing.T) {
		entries, err := svc.BrowseCatalog(context.Background(), backupSetID, "", 2, 0)
		if err != nil {
			t.Fatalf("BrowseCatalog failed: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries, got %d", len(entries))
		}
	})

	t.Run("respects offset", func(t *testing.T) {
		all, err := svc.BrowseCatalog(context.Background(), backupSetID, "", 0, 0)
		if err != nil {
			t.Fatalf("BrowseCatalog failed: %v", err)
		}
		entries, err := svc.BrowseCatalog(context.Background(), backupSetID, "", 2, 2)
		if err != nil {
			t.Fatalf("BrowseCatalog failed: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries, got %d", len(entries))
		}
		if len(all) >= 3 && entries[0].FilePath != all[2].FilePath {
			t.Errorf("offset not applied correctly: expected %s, got %s", all[2].FilePath, entries[0].FilePath)
		}
	})

	t.Run("filters by prefix", func(t *testing.T) {
		entries, err := svc.BrowseCatalog(context.Background(), backupSetID, "documents/", 0, 0)
		if err != nil {
			t.Fatalf("BrowseCatalog failed: %v", err)
		}
		if len(entries) != 4 {
			t.Errorf("expected 4 entries, got %d", len(entries))
		}
	})

	t.Run("returns empty slice for non-existent set", func(t *testing.T) {
		entries, err := svc.BrowseCatalog(context.Background(), 99999, "", 0, 0)
		if err != nil {
			t.Fatalf("BrowseCatalog failed: %v", err)
		}
		if entries == nil {
			t.Error("expected non-nil empty slice, got nil")
		}
		if len(entries) != 0 {
			t.Errorf("expected 0 entries, got %d", len(entries))
		}
	})

	t.Run("handles NULL nullable columns", func(t *testing.T) {
		// Insert entries with NULL file_mode, mod_time, checksum, and block_offset
		_, err := db.Exec(`INSERT INTO catalog_entries (backup_set_id, file_path, file_size) VALUES (?, ?, ?)`,
			backupSetID, "nulltest/file1.dat", 4096)
		if err != nil {
			t.Fatalf("failed to insert catalog entry with NULLs: %v", err)
		}
		_, err = db.Exec(`INSERT INTO catalog_entries (backup_set_id, file_path, file_size) VALUES (?, ?, ?)`,
			backupSetID, "nulltest/file2.dat", 8192)
		if err != nil {
			t.Fatalf("failed to insert catalog entry with NULLs: %v", err)
		}

		entries, err := svc.BrowseCatalog(context.Background(), backupSetID, "nulltest/", 0, 0)
		if err != nil {
			t.Fatalf("BrowseCatalog failed: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries with NULL columns, got %d", len(entries))
		}
		for _, e := range entries {
			if e.FileSize == 0 {
				t.Errorf("expected non-zero file size for %s", e.FilePath)
			}
		}
	})
}

func TestRestoreRequestFolderPaths(t *testing.T) {
	// Test that RestoreRequest properly supports folder paths
	req := RestoreRequest{
		BackupSetID:     1,
		FilePaths:       []string{"file1.txt", "file2.txt"},
		FolderPaths:     []string{"documents", "images/photos"},
		DestPath:        "/restore/dest",
		DestinationType: "local",
		Verify:          true,
		Overwrite:       false,
	}

	if len(req.FolderPaths) != 2 {
		t.Errorf("expected 2 folder paths, got %d", len(req.FolderPaths))
	}

	if req.FolderPaths[0] != "documents" {
		t.Errorf("expected first folder 'documents', got '%s'", req.FolderPaths[0])
	}
}

func TestCalculateChecksum(t *testing.T) {
	// Create a test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Write test content
	testContent := []byte("Hello, World!")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Calculate checksum
	checksum, err := calculateChecksum(testFile)
	if err != nil {
		t.Fatalf("calculateChecksum failed: %v", err)
	}

	// Verify checksum is not empty and is valid SHA256 format (64 hex chars)
	if len(checksum) != 64 {
		t.Errorf("expected 64 character SHA256 hash, got %d characters", len(checksum))
	}

	// Verify checksum is consistent
	checksum2, err := calculateChecksum(testFile)
	if err != nil {
		t.Fatalf("calculateChecksum failed on second call: %v", err)
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

func TestRestoreResultFoldersRestored(t *testing.T) {
	// Test that RestoreResult includes folders_restored field
	result := RestoreResult{
		FilesRestored:   10,
		BytesRestored:   5000,
		FoldersRestored: 3,
		Verified:        true,
	}

	if result.FoldersRestored != 3 {
		t.Errorf("expected 3 folders restored, got %d", result.FoldersRestored)
	}
}

func TestBuildDecompressionCmdGzip(t *testing.T) {
	ctx := context.Background()

	cmd, err := buildDecompressionCmd(ctx, models.CompressionGzip)
	if err != nil {
		t.Fatalf("buildDecompressionCmd failed: %v", err)
	}

	args := cmd.Args
	// Should use either pigz or gzip depending on availability
	if args[0] != "pigz" && args[0] != "gzip" {
		t.Errorf("expected pigz or gzip, got %s", args[0])
	}

	// Must include -d (decompress)
	foundD := false
	for _, a := range args {
		if a == "-d" {
			foundD = true
			break
		}
	}
	if !foundD {
		t.Errorf("expected -d flag for decompression, got args: %v", args)
	}
}

func TestBuildDecompressionCmdZstd(t *testing.T) {
	ctx := context.Background()

	cmd, err := buildDecompressionCmd(ctx, models.CompressionZstd)
	if err != nil {
		t.Fatalf("buildDecompressionCmd failed: %v", err)
	}

	if cmd.Args[0] != "zstd" {
		t.Errorf("expected zstd, got %s", cmd.Args[0])
	}

	// Must include -d (decompress) and -T0 (multi-threaded)
	foundD, foundT := false, false
	for _, a := range cmd.Args {
		if a == "-d" {
			foundD = true
		}
		if a == "-T0" {
			foundT = true
		}
	}
	if !foundD {
		t.Errorf("expected -d flag for decompression, got args: %v", cmd.Args)
	}
	if !foundT {
		t.Errorf("expected -T0 flag for multi-threaded decompression, got args: %v", cmd.Args)
	}
}

func TestBuildDecompressionCmdUnsupported(t *testing.T) {
	ctx := context.Background()

	_, err := buildDecompressionCmd(ctx, models.CompressionType("lz4"))
	if err == nil {
		t.Error("expected error for unsupported compression type")
	}
}

func TestBuildDecompressionCmdNone(t *testing.T) {
	ctx := context.Background()

	_, err := buildDecompressionCmd(ctx, models.CompressionNone)
	if err == nil {
		t.Error("expected error for CompressionNone")
	}
}

func TestRestorePipeline(t *testing.T) {
	tests := []struct {
		name          string
		encrypted     bool
		encryptionKey string
		compressed    bool
		wantPipeline  string
		wantErr       bool
	}{
		{
			name:         "standard unencrypted uncompressed",
			encrypted:    false,
			compressed:   false,
			wantPipeline: "standard",
		},
		{
			name:          "encrypted-only (no compression)",
			encrypted:     true,
			encryptionKey: "secret",
			compressed:    false,
			wantPipeline:  "encrypted-only",
		},
		{
			name:          "compressed-only (no encryption)",
			encrypted:     false,
			compressed:    true,
			wantPipeline:  "compressed-only",
		},
		{
			name:          "encrypted and compressed",
			encrypted:     true,
			encryptionKey: "secret",
			compressed:    true,
			wantPipeline:  "encrypted+compressed",
		},
		{
			name:      "encrypted but key missing",
			encrypted: true,
			// encryptionKey intentionally empty
			compressed: false,
			wantErr:    true,
		},
		{
			name:      "encrypted and compressed but key missing",
			encrypted: true,
			// encryptionKey intentionally empty
			compressed: true,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := restorePipeline(tt.encrypted, tt.encryptionKey, tt.compressed)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pipeline != tt.wantPipeline {
				t.Errorf("expected pipeline %q, got %q", tt.wantPipeline, pipeline)
			}
		})
	}
}
