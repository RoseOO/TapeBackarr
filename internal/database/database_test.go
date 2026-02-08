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
