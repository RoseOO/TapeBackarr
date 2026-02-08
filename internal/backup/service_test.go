package backup

import (
	"os"
	"path/filepath"
	"testing"
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
