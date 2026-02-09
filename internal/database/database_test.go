package database

import (
	"path/filepath"
	"testing"
)

func TestNewDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Test that we can ping the database
	if err := db.Ping(); err != nil {
		t.Fatalf("failed to ping database: %v", err)
	}
}

func TestMigrate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Verify tables exist
	tables := []string{
		"users",
		"tape_pools",
		"tapes",
		"tape_drives",
		"backup_sources",
		"backup_jobs",
		"backup_sets",
		"catalog_entries",
		"job_executions",
		"audit_logs",
		"snapshots",
	}

	for _, table := range tables {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil {
			t.Fatalf("failed to check table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("expected table %s to exist", table)
		}
	}

	// Verify default data
	var adminCount int
	err = db.QueryRow("SELECT COUNT(*) FROM users WHERE username='admin'").Scan(&adminCount)
	if err != nil {
		t.Fatalf("failed to check admin user: %v", err)
	}
	if adminCount != 1 {
		t.Error("expected default admin user to exist")
	}

	var poolCount int
	err = db.QueryRow("SELECT COUNT(*) FROM tape_pools").Scan(&poolCount)
	if err != nil {
		t.Fatalf("failed to count pools: %v", err)
	}
	if poolCount != 4 {
		t.Errorf("expected 4 default pools, got %d", poolCount)
	}
}

func TestBusyTimeoutConfigured(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Verify busy_timeout is set (should be 5000ms)
	var busyTimeout int
	err = db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout)
	if err != nil {
		t.Fatalf("failed to query busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("expected busy_timeout 5000, got %d", busyTimeout)
	}

	// Verify WAL mode is enabled
	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected journal_mode 'wal', got '%s'", journalMode)
	}
}

func TestConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Create a test table
	_, err = db.Exec("CREATE TABLE test_concurrent (id INTEGER PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	// Run concurrent inserts - should not get SQLITE_BUSY with proper settings
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			_, err := db.Exec("INSERT INTO test_concurrent (value) VALUES (?)", i)
			errs <- err
		}(i)
	}

	for i := 0; i < 10; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent insert %d failed: %v", i, err)
		}
	}

	// Verify all rows inserted
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_concurrent").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != 10 {
		t.Errorf("expected 10 rows, got %d", count)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Run migrations multiple times - should be idempotent
	for i := 0; i < 3; i++ {
		if err := db.Migrate(); err != nil {
			t.Fatalf("failed to run migrations (attempt %d): %v", i+1, err)
		}
	}

	// Verify still only one admin
	var adminCount int
	err = db.QueryRow("SELECT COUNT(*) FROM users WHERE username='admin'").Scan(&adminCount)
	if err != nil {
		t.Fatalf("failed to check admin user: %v", err)
	}
	if adminCount != 1 {
		t.Errorf("expected 1 admin user, got %d", adminCount)
	}
}
