package tape

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewLTFSService(t *testing.T) {
	svc := NewLTFSService("/dev/nst0", "/mnt/ltfs")
	if svc.DevicePath() != "/dev/nst0" {
		t.Errorf("expected device path /dev/nst0, got %s", svc.DevicePath())
	}
	if svc.MountPoint() != "/mnt/ltfs" {
		t.Errorf("expected mount point /mnt/ltfs, got %s", svc.MountPoint())
	}
}

func TestNewLTFSServiceDefaultMountPoint(t *testing.T) {
	svc := NewLTFSService("/dev/nst1", "")
	if svc.MountPoint() != LTFSDefaultMountPoint {
		t.Errorf("expected default mount point %s, got %s", LTFSDefaultMountPoint, svc.MountPoint())
	}
}

func TestLTFSServiceIsMountedFalse(t *testing.T) {
	// Using a non-existent mount point should not be mounted
	svc := NewLTFSService("/dev/nst0", "/tmp/ltfs-test-nonexistent-"+t.Name())
	if svc.IsMounted() {
		t.Error("expected IsMounted to return false for non-existent mount point")
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")

	content := []byte("Hello, LTFS tape world!")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	n, err := copyFile(srcPath, dstPath)
	if err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	if n != int64(len(content)) {
		t.Errorf("expected %d bytes copied, got %d", len(content), n)
	}

	// Verify content
	readBack, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if string(readBack) != string(content) {
		t.Errorf("content mismatch: expected %q, got %q", content, readBack)
	}
}

func TestCopyFileLargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "large.bin")
	dstPath := filepath.Join(tmpDir, "large_copy.bin")

	// Create a file larger than the 1MB buffer to test multi-block copy
	size := 2*1024*1024 + 123 // 2MB + 123 bytes
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(srcPath, data, 0644); err != nil {
		t.Fatalf("failed to write large source file: %v", err)
	}

	n, err := copyFile(srcPath, dstPath)
	if err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	if n != int64(size) {
		t.Errorf("expected %d bytes copied, got %d", size, n)
	}

	readBack, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if len(readBack) != size {
		t.Errorf("size mismatch: expected %d, got %d", size, len(readBack))
	}
}

func TestCopyFileNonExistentSource(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := copyFile("/nonexistent/file.txt", filepath.Join(tmpDir, "out.txt"))
	if err == nil {
		t.Error("expected error for non-existent source")
	}
}

func TestCopyFileInvalidDest(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "src.txt")
	if err := os.WriteFile(srcPath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	_, err := copyFile(srcPath, "/nonexistent/dir/out.txt")
	if err == nil {
		t.Error("expected error for invalid destination directory")
	}
}

func TestLTFSWriteFilesNotMounted(t *testing.T) {
	svc := NewLTFSService("/dev/nst0", "/tmp/ltfs-test-unmounted-"+t.Name())
	_, _, err := svc.WriteFiles(context.Background(), "/tmp", nil, nil)
	if err == nil {
		t.Error("expected error when LTFS is not mounted")
	}
}

func TestLTFSRestoreFilesNotMounted(t *testing.T) {
	svc := NewLTFSService("/dev/nst0", "/tmp/ltfs-test-unmounted-"+t.Name())
	_, _, err := svc.RestoreFiles(context.Background(), "/tmp/dest", nil)
	if err == nil {
		t.Error("expected error when LTFS is not mounted")
	}
}

func TestLTFSListFilesNotMounted(t *testing.T) {
	svc := NewLTFSService("/dev/nst0", "/tmp/ltfs-test-unmounted-"+t.Name())
	_, err := svc.ListFiles(context.Background())
	if err == nil {
		t.Error("expected error when LTFS is not mounted")
	}
}

func TestLTFSReadFileNotMounted(t *testing.T) {
	svc := NewLTFSService("/dev/nst0", "/tmp/ltfs-test-unmounted-"+t.Name())
	_, err := svc.ReadFile(context.Background(), "test.txt")
	if err == nil {
		t.Error("expected error when LTFS is not mounted")
	}
}

func TestLTFSReadLTFSLabelNotMounted(t *testing.T) {
	svc := NewLTFSService("/dev/nst0", "/tmp/ltfs-test-unmounted-"+t.Name())
	_, err := svc.ReadLTFSLabel(context.Background())
	if err == nil {
		t.Error("expected error when LTFS is not mounted")
	}
}

func TestLTFSGetVolumeInfoNotMounted(t *testing.T) {
	svc := NewLTFSService("/dev/nst0", "/tmp/ltfs-test-not-mounted-"+t.Name())
	info := svc.GetVolumeInfo(context.Background())
	if info == nil {
		t.Fatal("expected non-nil VolumeInfo")
	}
	if info.Mounted {
		t.Error("expected Mounted=false")
	}
	if info.DevicePath != "/dev/nst0" {
		t.Errorf("expected device path /dev/nst0, got %s", info.DevicePath)
	}
}

func TestLTFSLabelStruct(t *testing.T) {
	label := LTFSLabel{
		Magic:     "TAPEBACKARR_LTFS",
		Version:   1,
		Label:     "TAPE01",
		UUID:      "abc-123",
		Pool:      "default",
		Format:    "ltfs",
		CreatedAt: "2025-01-01T00:00:00Z",
	}

	if label.Magic != "TAPEBACKARR_LTFS" {
		t.Errorf("unexpected magic: %s", label.Magic)
	}
	if label.Format != "ltfs" {
		t.Errorf("unexpected format: %s", label.Format)
	}
}

func TestLTFSDefaultMountPoint(t *testing.T) {
	if LTFSDefaultMountPoint != "/mnt/ltfs" {
		t.Errorf("unexpected default mount point: %s", LTFSDefaultMountPoint)
	}
}

func TestLTFSFileEntry(t *testing.T) {
	entry := LTFSFileEntry{
		Path: "documents/test.txt",
		Size: 1024,
		Mode: 0644,
	}

	if entry.Path != "documents/test.txt" {
		t.Errorf("unexpected path: %s", entry.Path)
	}
	if entry.Size != 1024 {
		t.Errorf("unexpected size: %d", entry.Size)
	}
}

func TestLTFSVolumeInfo(t *testing.T) {
	info := LTFSVolumeInfo{
		MountPoint: "/mnt/ltfs",
		DevicePath: "/dev/nst0",
		Mounted:    true,
		VolumeName: "BACKUP01",
		UsedBytes:  1000000,
		AvailBytes: 5000000000,
	}

	if info.VolumeName != "BACKUP01" {
		t.Errorf("unexpected volume name: %s", info.VolumeName)
	}
	if info.UsedBytes != 1000000 {
		t.Errorf("unexpected used bytes: %d", info.UsedBytes)
	}
}
