package tape

import (
	"context"
	"crypto/rand"
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

func TestCopyFileEncryptedDecrypted(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	encPath := filepath.Join(tmpDir, "source.txt.enc")
	decPath := filepath.Join(tmpDir, "source_decrypted.txt")

	content := []byte("Hello, encrypted LTFS tape world! This is a test of per-file encryption.")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	// Generate a random 32-byte AES-256 key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Encrypt
	n, err := copyFileEncrypted(srcPath, encPath, key)
	if err != nil {
		t.Fatalf("copyFileEncrypted failed: %v", err)
	}
	if n != int64(len(content)) {
		t.Errorf("expected %d plaintext bytes, got %d", len(content), n)
	}

	// Verify encrypted file is different from plaintext
	encData, _ := os.ReadFile(encPath)
	if string(encData) == string(content) {
		t.Error("encrypted data should differ from plaintext")
	}

	// Decrypt
	n, err = copyFileDecrypted(encPath, decPath, key)
	if err != nil {
		t.Fatalf("copyFileDecrypted failed: %v", err)
	}
	if n != int64(len(content)) {
		t.Errorf("expected %d decrypted bytes, got %d", len(content), n)
	}

	// Verify decrypted content matches original
	decData, err := os.ReadFile(decPath)
	if err != nil {
		t.Fatalf("failed to read decrypted file: %v", err)
	}
	if string(decData) != string(content) {
		t.Errorf("decrypted content mismatch: expected %q, got %q", content, decData)
	}
}

func TestCopyFileEncryptedWrongKey(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	encPath := filepath.Join(tmpDir, "source.txt.enc")
	decPath := filepath.Join(tmpDir, "source_decrypted.txt")

	content := []byte("Secret data that should not be readable with the wrong key.")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	key := make([]byte, 32)
	rand.Read(key)

	if _, err := copyFileEncrypted(srcPath, encPath, key); err != nil {
		t.Fatalf("copyFileEncrypted failed: %v", err)
	}

	// Try to decrypt with a different key — should fail
	wrongKey := make([]byte, 32)
	rand.Read(wrongKey)

	_, err := copyFileDecrypted(encPath, decPath, wrongKey)
	if err == nil {
		t.Error("expected decryption to fail with wrong key")
	}
}

func TestWriteFilesEncryptedNotMounted(t *testing.T) {
	svc := NewLTFSService("/dev/nst0", "/tmp/ltfs-test-enc-unmounted-"+t.Name())
	key := make([]byte, 32)
	rand.Read(key)
	_, _, err := svc.WriteFilesEncrypted(context.Background(), "/tmp", nil, key, nil)
	if err == nil {
		t.Error("expected error when LTFS is not mounted")
	}
}

func TestRestoreFilesDecryptedNotMounted(t *testing.T) {
	svc := NewLTFSService("/dev/nst0", "/tmp/ltfs-test-dec-unmounted-"+t.Name())
	key := make([]byte, 32)
	rand.Read(key)
	_, _, err := svc.RestoreFilesDecrypted(context.Background(), "/tmp/dest", nil, key)
	if err == nil {
		t.Error("expected error when LTFS is not mounted")
	}
}

func TestEncryptedFileSuffix(t *testing.T) {
	if EncryptedFileSuffix != ".enc" {
		t.Errorf("unexpected encrypted file suffix: %s", EncryptedFileSuffix)
	}
}

func TestLtfsFormatSuccessful(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "success with LTFS15024I code",
			output: "LTFS15024I LTFS volume formatted successfully.",
			want:   true,
		},
		{
			name:   "success message in multi-line output",
			output: "LTFS15000I Starting mkltfs.\nLTFS15024I LTFS volume formatted successfully.\nDone.",
			want:   true,
		},
		{
			name:   "human readable success message",
			output: "Volume formatted successfully",
			want:   true,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
		{
			name:   "error output without success indicator",
			output: "LTFS15013E Cannot format: device is busy",
			want:   false,
		},
		{
			name:   "partial match should not succeed",
			output: "LTFS15024 missing trailing I",
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ltfsFormatSuccessful(tt.output); got != tt.want {
				t.Errorf("ltfsFormatSuccessful(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}

func TestIsLabelReadFailure(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "cannot read volume",
			output: "LTFS11009E Cannot read volume: failed to read partition labels.",
			want:   true,
		},
		{
			name:   "failed to read partition labels",
			output: "LTFS11170E Failed to read label (-1012) from partition 0.",
			want:   true,
		},
		{
			name:   "cannot read ANSI label",
			output: "LTFS11174E Cannot read ANSI label: read failed (-20801).",
			want:   true,
		},
		{
			name:   "unrelated error",
			output: "LTFS30205I READ (0x08) returns -20801.",
			want:   false,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
		{
			name: "full mount failure output from issue",
			output: `LTFS30205I READ (0x08) returns -20801.
LTFS30263I READ returns End-of-Data (EOD) Detected (-20801) /dev/nst0.
LTFS12049E Cannot read: backend call failed (-20801).
LTFS11174E Cannot read ANSI label: read failed (-20801).
LTFS11170E Failed to read label (-1012) from partition 0.
LTFS11009E Cannot read volume: failed to read partition labels.
LTFS14013E Cannot mount the volume from device.`,
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLabelReadFailure(tt.output); got != tt.want {
				t.Errorf("isLabelReadFailure(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}

func TestIsSGIOError(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "SG_IO ioctl failure",
			output: "LTFS30200I Failed to execute SG_IO ioctl, opcode = 08 (22).",
			want:   true,
		},
		{
			name:   "ioctl error from backend",
			output: "LTFS30263I READ returns ioctl error (-21700) /dev/nst0.",
			want:   true,
		},
		{
			name:   "error code -21700",
			output: "LTFS12049E Cannot read: backend call failed (-21700).",
			want:   true,
		},
		{
			name: "full ltfsck failure output from issue",
			output: `ltfsck failed: LTFS16000I Starting ltfsck, LTFS version 2.5.0.0 (Prelim), log level 2.
LTFS11026I Performing a full medium consistency check.
LTFS30200I Failed to execute SG_IO ioctl, opcode = 08 (22).
LTFS30263I READ returns ioctl error (-21700) /dev/nst0.
LTFS12049E Cannot read: backend call failed (-21700).
LTFS11253E No index found in the medium.
LTFS16021E Volume is inconsistent and was not corrected.: exit status 4`,
			want: true,
		},
		{
			name:   "label read failure is not SG_IO error",
			output: "LTFS11009E Cannot read volume: failed to read partition labels.",
			want:   false,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
		{
			name:   "unrelated error",
			output: "LTFS15013E Cannot format: device is busy",
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSGIOError(tt.output); got != tt.want {
				t.Errorf("isSGIOError(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}
