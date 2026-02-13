package tape

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewTapeTOC(t *testing.T) {
	toc := NewTapeTOC("TAPE001", "uuid-1234", "default")

	if toc.Magic != tocMagic {
		t.Errorf("expected magic %q, got %q", tocMagic, toc.Magic)
	}
	if toc.Version != tocVersion {
		t.Errorf("expected version %d, got %d", tocVersion, toc.Version)
	}
	if toc.TapeLabel != "TAPE001" {
		t.Errorf("expected tape label 'TAPE001', got %q", toc.TapeLabel)
	}
	if toc.TapeUUID != "uuid-1234" {
		t.Errorf("expected tape UUID 'uuid-1234', got %q", toc.TapeUUID)
	}
	if toc.Pool != "default" {
		t.Errorf("expected pool 'default', got %q", toc.Pool)
	}
	if len(toc.BackupSets) != 0 {
		t.Errorf("expected 0 backup sets, got %d", len(toc.BackupSets))
	}
	if toc.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestMarshalUnmarshalTOC(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	toc := &TapeTOC{
		Magic:     tocMagic,
		Version:   tocVersion,
		TapeLabel: "WEEKLY-001",
		TapeUUID:  "abc-def-123",
		Pool:      "weekly",
		CreatedAt: now,
		BackupSets: []TOCBackupSet{
			{
				FileNumber:      1,
				JobName:         "nightly-full",
				BackupType:      "full",
				StartTime:       now.Add(-1 * time.Hour),
				EndTime:         now,
				FileCount:       3,
				TotalBytes:      15000,
				Encrypted:       false,
				Compressed:      true,
				CompressionType: "gzip",
				Files: []TOCFileEntry{
					{Path: "documents/report.pdf", Size: 5000, Mode: 0644, ModTime: now.Format(time.RFC3339), Checksum: "abc123"},
					{Path: "documents/notes.txt", Size: 2000, Mode: 0644, ModTime: now.Format(time.RFC3339), Checksum: "def456"},
					{Path: "images/photo.jpg", Size: 8000, Mode: 0644, ModTime: now.Format(time.RFC3339), Checksum: "ghi789"},
				},
			},
		},
	}

	// Marshal
	data, err := MarshalTOC(toc)
	if err != nil {
		t.Fatalf("MarshalTOC failed: %v", err)
	}

	// Unmarshal
	decoded, err := UnmarshalTOC(data)
	if err != nil {
		t.Fatalf("UnmarshalTOC failed: %v", err)
	}

	if decoded.Magic != tocMagic {
		t.Errorf("expected magic %q, got %q", tocMagic, decoded.Magic)
	}
	if decoded.TapeLabel != "WEEKLY-001" {
		t.Errorf("expected tape label 'WEEKLY-001', got %q", decoded.TapeLabel)
	}
	if decoded.TapeUUID != "abc-def-123" {
		t.Errorf("expected UUID 'abc-def-123', got %q", decoded.TapeUUID)
	}
	if decoded.Pool != "weekly" {
		t.Errorf("expected pool 'weekly', got %q", decoded.Pool)
	}
	if len(decoded.BackupSets) != 1 {
		t.Fatalf("expected 1 backup set, got %d", len(decoded.BackupSets))
	}

	bs := decoded.BackupSets[0]
	if bs.FileNumber != 1 {
		t.Errorf("expected file number 1, got %d", bs.FileNumber)
	}
	if bs.JobName != "nightly-full" {
		t.Errorf("expected job name 'nightly-full', got %q", bs.JobName)
	}
	if bs.FileCount != 3 {
		t.Errorf("expected 3 files, got %d", bs.FileCount)
	}
	if bs.TotalBytes != 15000 {
		t.Errorf("expected 15000 bytes, got %d", bs.TotalBytes)
	}
	if bs.Encrypted {
		t.Error("expected encrypted to be false")
	}
	if !bs.Compressed {
		t.Error("expected compressed to be true")
	}
	if bs.CompressionType != "gzip" {
		t.Errorf("expected compression type 'gzip', got %q", bs.CompressionType)
	}
	if len(bs.Files) != 3 {
		t.Fatalf("expected 3 file entries, got %d", len(bs.Files))
	}
	if bs.Files[0].Path != "documents/report.pdf" {
		t.Errorf("expected first file path 'documents/report.pdf', got %q", bs.Files[0].Path)
	}
	if bs.Files[0].Size != 5000 {
		t.Errorf("expected first file size 5000, got %d", bs.Files[0].Size)
	}
	if bs.Files[0].Checksum != "abc123" {
		t.Errorf("expected first file checksum 'abc123', got %q", bs.Files[0].Checksum)
	}
}

func TestUnmarshalTOCInvalidMagic(t *testing.T) {
	data := []byte(`{"magic":"INVALID","version":1}`)
	_, err := UnmarshalTOC(data)
	if err == nil {
		t.Error("expected error for invalid magic, got nil")
	}
}

func TestUnmarshalTOCInvalidJSON(t *testing.T) {
	data := []byte(`not json at all`)
	_, err := UnmarshalTOC(data)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestUnmarshalTOCWithNullPadding(t *testing.T) {
	toc := NewTapeTOC("TAPE001", "uuid-1234", "pool1")
	data, err := MarshalTOC(toc)
	if err != nil {
		t.Fatalf("MarshalTOC failed: %v", err)
	}

	// Add null padding (simulates reading from tape with block padding)
	padded := make([]byte, len(data)+1024)
	copy(padded, data)

	// Trim nulls and unmarshal (as ReadTOC does)
	trimmed := padded
	for i := len(trimmed) - 1; i >= 0; i-- {
		if trimmed[i] != 0 {
			trimmed = trimmed[:i+1]
			break
		}
	}

	decoded, err := UnmarshalTOC(trimmed)
	if err != nil {
		t.Fatalf("UnmarshalTOC with padding failed: %v", err)
	}
	if decoded.TapeLabel != "TAPE001" {
		t.Errorf("expected tape label 'TAPE001', got %q", decoded.TapeLabel)
	}
}

func TestTOCMultipleBackupSets(t *testing.T) {
	now := time.Now()
	toc := NewTapeTOC("MULTI-001", "uuid-multi", "default")
	toc.BackupSets = []TOCBackupSet{
		{
			FileNumber: 1,
			JobName:    "job-1",
			BackupType: "full",
			StartTime:  now.Add(-2 * time.Hour),
			EndTime:    now.Add(-1 * time.Hour),
			FileCount:  10,
			TotalBytes: 50000,
			Files: []TOCFileEntry{
				{Path: "file1.txt", Size: 5000},
			},
		},
		{
			FileNumber: 2,
			JobName:    "job-2",
			BackupType: "incremental",
			StartTime:  now.Add(-1 * time.Hour),
			EndTime:    now,
			FileCount:  5,
			TotalBytes: 10000,
			Encrypted:  true,
			Files: []TOCFileEntry{
				{Path: "file2.txt", Size: 2000},
			},
		},
	}

	data, err := MarshalTOC(toc)
	if err != nil {
		t.Fatalf("MarshalTOC failed: %v", err)
	}

	decoded, err := UnmarshalTOC(data)
	if err != nil {
		t.Fatalf("UnmarshalTOC failed: %v", err)
	}

	if len(decoded.BackupSets) != 2 {
		t.Fatalf("expected 2 backup sets, got %d", len(decoded.BackupSets))
	}
	if decoded.BackupSets[0].JobName != "job-1" {
		t.Errorf("expected first job name 'job-1', got %q", decoded.BackupSets[0].JobName)
	}
	if decoded.BackupSets[1].JobName != "job-2" {
		t.Errorf("expected second job name 'job-2', got %q", decoded.BackupSets[1].JobName)
	}
	if !decoded.BackupSets[1].Encrypted {
		t.Error("expected second backup set to be encrypted")
	}
}

func TestTOCEmptyFiles(t *testing.T) {
	toc := NewTapeTOC("EMPTY-001", "uuid-empty", "pool")
	toc.BackupSets = []TOCBackupSet{
		{
			FileNumber: 1,
			BackupType: "full",
			FileCount:  0,
			TotalBytes: 0,
			Files:      []TOCFileEntry{},
		},
	}

	data, err := MarshalTOC(toc)
	if err != nil {
		t.Fatalf("MarshalTOC failed: %v", err)
	}

	decoded, err := UnmarshalTOC(data)
	if err != nil {
		t.Fatalf("UnmarshalTOC failed: %v", err)
	}

	if len(decoded.BackupSets[0].Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(decoded.BackupSets[0].Files))
	}
}

func TestTOCJSONStructure(t *testing.T) {
	toc := NewTapeTOC("TAPE001", "uuid-1234", "default")
	toc.BackupSets = []TOCBackupSet{
		{
			FileNumber: 1,
			BackupType: "full",
			FileCount:  1,
			TotalBytes: 100,
			Files: []TOCFileEntry{
				{Path: "test.txt", Size: 100},
			},
		},
	}

	data, err := MarshalTOC(toc)
	if err != nil {
		t.Fatalf("MarshalTOC failed: %v", err)
	}

	// Verify it's valid JSON
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("TOC is not valid JSON: %v", err)
	}

	// Verify key fields exist
	if raw["magic"] != tocMagic {
		t.Errorf("expected magic field %q, got %v", tocMagic, raw["magic"])
	}
	if raw["version"] != float64(tocVersion) {
		t.Errorf("expected version %d, got %v", tocVersion, raw["version"])
	}
}

func TestLabelBlockFormat(t *testing.T) {
	// Verify that the label block is exactly 512 bytes and properly formatted.
	// This matters because the label is written with dd bs=512 count=1.
	fields := []string{labelMagic, "TAPE-001", "uuid-1234", "pool-a", "1234567890"}
	labelStr := strings.Join(fields, labelDelimiter)

	padded := make([]byte, 512)
	copy(padded, []byte(labelStr))

	if len(padded) != 512 {
		t.Fatalf("expected padded block length 512, got %d", len(padded))
	}

	// The label text should be at the beginning
	if !strings.HasPrefix(string(padded), "TAPEBACKARR|TAPE-001|") {
		t.Errorf("padded block does not start with expected label prefix")
	}

	// Remainder should be null bytes
	for i := len(labelStr); i < 512; i++ {
		if padded[i] != 0 {
			t.Errorf("expected null byte at position %d, got %d", i, padded[i])
			break
		}
	}

	// Simulate reading back: trim nulls and parse
	raw := strings.TrimRight(string(padded), "\x00")
	parts := strings.Split(raw, labelDelimiter)
	if len(parts) < 5 {
		t.Fatalf("expected at least 5 fields, got %d", len(parts))
	}
	if parts[0] != labelMagic {
		t.Errorf("expected magic %q, got %q", labelMagic, parts[0])
	}
	if parts[1] != "TAPE-001" {
		t.Errorf("expected label 'TAPE-001', got %q", parts[1])
	}
	if parts[2] != "uuid-1234" {
		t.Errorf("expected UUID 'uuid-1234', got %q", parts[2])
	}
	if parts[3] != "pool-a" {
		t.Errorf("expected pool 'pool-a', got %q", parts[3])
	}
}

func TestServiceBlockSizeField(t *testing.T) {
	// Verify that the Service stores and returns the configured block size.
	// WriteTapeLabel and ReadTapeLabel use SetBlockSize(0) before the dd
	// operation and defer restoration of s.blockSize afterwards.
	svc := NewService("/dev/nst0", 65536)
	if svc.GetBlockSize() != 65536 {
		t.Errorf("expected block size 65536, got %d", svc.GetBlockSize())
	}

	svc2 := NewService("/dev/nst0", 0)
	if svc2.GetBlockSize() != 0 {
		t.Errorf("expected block size 0, got %d", svc2.GetBlockSize())
	}
}

func TestTapeLabelDataFields(t *testing.T) {
	label := TapeLabelData{
		Label:                    "TEST-001",
		UUID:                     "uuid-test",
		Pool:                     "default",
		Timestamp:                1234567890,
		EncryptionKeyFingerprint: "abc123",
		CompressionType:          "gzip",
	}

	if label.Label != "TEST-001" {
		t.Errorf("expected label 'TEST-001', got %q", label.Label)
	}
	if label.UUID != "uuid-test" {
		t.Errorf("expected UUID 'uuid-test', got %q", label.UUID)
	}
}

func TestTapeLabelDataFormatType(t *testing.T) {
	tests := []struct {
		name       string
		formatType string
		wantEmpty  bool
	}{
		{"raw format", "raw", false},
		{"ltfs format", "ltfs", false},
		{"empty format defaults to zero value", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label := TapeLabelData{
				Label:      "TAPE-001",
				UUID:       "uuid-123",
				FormatType: tt.formatType,
			}
			if tt.wantEmpty && label.FormatType != "" {
				t.Errorf("expected empty FormatType, got %q", label.FormatType)
			}
			if !tt.wantEmpty && label.FormatType != tt.formatType {
				t.Errorf("expected FormatType %q, got %q", tt.formatType, label.FormatType)
			}
		})
	}
}

func TestTapeLabelDataFormatTypeJSON(t *testing.T) {
	label := TapeLabelData{
		Label:      "TAPE-001",
		UUID:       "uuid-123",
		FormatType: "ltfs",
	}

	data, err := json.Marshal(label)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	if !strings.Contains(string(data), `"format_type":"ltfs"`) {
		t.Errorf("JSON should contain format_type field, got: %s", string(data))
	}

	// Verify omitempty: empty format_type should not appear in JSON
	label.FormatType = ""
	data, err = json.Marshal(label)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	if strings.Contains(string(data), `"format_type"`) {
		t.Errorf("JSON should omit empty format_type field, got: %s", string(data))
	}
}

func TestLabelCacheOperations(t *testing.T) {
	cache := NewLabelCache()

	// Test Get on empty cache
	if entry := cache.Get("/dev/nst0", 5*time.Minute); entry != nil {
		t.Error("expected nil from empty cache")
	}

	// Test Set and Get
	label := &TapeLabelData{Label: "TEST-001", UUID: "uuid-1"}
	cache.Set("/dev/nst0", label, true)

	entry := cache.Get("/dev/nst0", 5*time.Minute)
	if entry == nil {
		t.Fatal("expected non-nil cache entry")
	}
	if entry.Label.Label != "TEST-001" {
		t.Errorf("expected label 'TEST-001', got %q", entry.Label.Label)
	}

	// Test Invalidate
	cache.Invalidate("/dev/nst0")
	if entry := cache.Get("/dev/nst0", 5*time.Minute); entry != nil {
		t.Error("expected nil after invalidation")
	}

	// Test InvalidateAll
	cache.Set("/dev/nst0", label, true)
	cache.Set("/dev/nst1", label, true)
	cache.InvalidateAll()
	if entry := cache.Get("/dev/nst0", 5*time.Minute); entry != nil {
		t.Error("expected nil after InvalidateAll")
	}
}

func TestLabelCacheExpiry(t *testing.T) {
	cache := NewLabelCache()
	label := &TapeLabelData{Label: "TEST-001"}
	cache.Set("/dev/nst0", label, true)

	// Should expire with very short maxAge
	if entry := cache.Get("/dev/nst0", 0); entry != nil {
		t.Error("expected nil for expired cache entry")
	}
}

func TestParseTemperaturePage(t *testing.T) {
	svc := NewService("/dev/nst0", 65536)

	tests := []struct {
		name   string
		output string
		wantC  int64
	}{
		{
			name: "parse current temperature",
			output: `Temperature page  [0xd]
  Current temperature = 42 C
  Reference temperature = <not available>
`,
			wantC: 42,
		},
		{
			name:   "empty output",
			output: "",
			wantC:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &DriveStatisticsData{}
			svc.parseTemperaturePage(tt.output, stats)
			if stats.TemperatureC != tt.wantC {
				t.Errorf("expected TemperatureC %d, got %d", tt.wantC, stats.TemperatureC)
			}
		})
	}
}

func TestParseDeviceStatisticsPage(t *testing.T) {
	svc := NewService("/dev/nst0", 65536)

	tests := []struct {
		name     string
		output   string
		wantFunc func(t *testing.T, stats *DriveStatisticsData)
	}{
		{
			name: "parse device statistics page",
			output: `Device statistics page (ssc-3 and adc)
  Lifetime media loads: 932
  Lifetime cleaning operations: 1
  Lifetime power on hours: 102613
  Lifetime media motion (head) hours: 4241
  Lifetime power cycles: 29
  Hard write errors: 0
  Hard read errors: 0
`,
			wantFunc: func(t *testing.T, stats *DriveStatisticsData) {
				if stats.TotalLoadCount != 932 {
					t.Errorf("expected TotalLoadCount 932, got %d", stats.TotalLoadCount)
				}
				if stats.PowerOnHours != 102613 {
					t.Errorf("expected PowerOnHours 102613, got %d", stats.PowerOnHours)
				}
				if stats.LifetimePowerCycles != 29 {
					t.Errorf("expected LifetimePowerCycles 29, got %d", stats.LifetimePowerCycles)
				}
			},
		},
		{
			name: "parse hard errors",
			output: `Device statistics page
  Hard write errors: 5
  Hard read errors: 3
`,
			wantFunc: func(t *testing.T, stats *DriveStatisticsData) {
				if stats.WriteErrors != 5 {
					t.Errorf("expected WriteErrors 5, got %d", stats.WriteErrors)
				}
				if stats.ReadErrors != 3 {
					t.Errorf("expected ReadErrors 3, got %d", stats.ReadErrors)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &DriveStatisticsData{}
			svc.parseDeviceStatisticsPage(tt.output, stats)
			tt.wantFunc(t, stats)
		})
	}
}

func TestParseDataCompressionPage(t *testing.T) {
	svc := NewService("/dev/nst0", 65536)

	tests := []struct {
		name      string
		output    string
		wantRead  int64
		wantWrite int64
	}{
		{
			name: "parse compression ratios",
			output: `Data compression page  (ssc-4) [0x1b]
  Read compression ratio x100: 530
  Write compression ratio x100: 250
`,
			wantRead:  530,
			wantWrite: 250,
		},
		{
			name: "zero compression",
			output: `Data compression page
  Read compression ratio x100: 0
  Write compression ratio x100: 0
`,
			wantRead:  0,
			wantWrite: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &DriveStatisticsData{}
			svc.parseDataCompressionPage(tt.output, stats)
			if stats.ReadCompressionPct != tt.wantRead {
				t.Errorf("expected ReadCompressionPct %d, got %d", tt.wantRead, stats.ReadCompressionPct)
			}
			if stats.WriteCompressionPct != tt.wantWrite {
				t.Errorf("expected WriteCompressionPct %d, got %d", tt.wantWrite, stats.WriteCompressionPct)
			}
		})
	}
}

func TestParseTapeAlertPage(t *testing.T) {
	svc := NewService("/dev/nst0", 65536)

	tests := []struct {
		name      string
		output    string
		wantFlags string
	}{
		{
			name: "no active alerts",
			output: `Tape alert page (ssc-3) [0x2e]
  Read warning: 0
  Write warning: 0
  Hard error: 0
  Media life: 0
  Cleaning required: 0
`,
			wantFlags: "",
		},
		{
			name: "active alerts",
			output: `Tape alert page (ssc-3) [0x2e]
  Read warning: 0
  Write warning: 1
  Hard error: 0
  Media life: 1
  Cleaning required: 0
  Reserved (30h): 0
  Obsolete (28h): 0
`,
			wantFlags: "Write warning,Media life",
		},
		{
			name:      "empty output",
			output:    "",
			wantFlags: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &DriveStatisticsData{}
			svc.parseTapeAlertPage(tt.output, stats)
			if stats.TapeAlertFlags != tt.wantFlags {
				t.Errorf("expected TapeAlertFlags %q, got %q", tt.wantFlags, stats.TapeAlertFlags)
			}
		})
	}
}

func TestDefaultOperationTimeout(t *testing.T) {
	// Verify the timeout constant is set to a reasonable value
	if DefaultOperationTimeout < 10*time.Second {
		t.Errorf("DefaultOperationTimeout too short: %v, should be at least 10s", DefaultOperationTimeout)
	}
	if DefaultOperationTimeout > 120*time.Second {
		t.Errorf("DefaultOperationTimeout too long: %v, should be at most 120s", DefaultOperationTimeout)
	}
}

func TestErrOperationTimeoutExists(t *testing.T) {
	// Verify the error type is properly defined
	if ErrOperationTimeout == nil {
		t.Error("ErrOperationTimeout should not be nil")
	}
	if ErrOperationTimeout.Error() != "tape operation timed out" {
		t.Errorf("unexpected error message: %q", ErrOperationTimeout.Error())
	}
}

func TestParseHardwareEncryptionStatus(t *testing.T) {
	svc := NewService("/dev/nst0", 65536)

	tests := []struct {
		name          string
		output        string
		wantSupported bool
		wantEnabled   bool
		wantMode      string
		wantAlgorithm string
	}{
		{
			name: "encryption on with AES",
			output: `Drive encryption capable
Drive encryption mode: encrypt on
Algorithm: AES-256-GCM
`,
			wantSupported: true,
			wantEnabled:   true,
			wantMode:      "on",
			wantAlgorithm: "AES-256-GCM",
		},
		{
			name: "encryption off",
			output: `Drive encryption capable
Drive encryption mode: off
`,
			wantSupported: true,
			wantEnabled:   false,
			wantMode:      "off",
		},
		{
			name: "mixed mode",
			output: `Drive encryption capable
Encryption mode: mixed
Algorithm: AES-256-GCM
`,
			wantSupported: true,
			wantEnabled:   true,
			wantMode:      "mixed",
			wantAlgorithm: "AES-256-GCM",
		},
		{
			name: "not capable",
			output: `Drive encryption not capable
`,
			wantSupported: false,
			wantEnabled:   false,
			wantMode:      "off",
		},
		{
			name:          "empty output",
			output:        "",
			wantSupported: false,
			wantEnabled:   false,
			wantMode:      "off",
		},
		{
			name: "rawread mode",
			output: `Drive encryption capable
Drive encryption mode: raw read
`,
			wantSupported: true,
			wantEnabled:   false,
			wantMode:      "rawread",
		},
		{
			name: "disabled mode",
			output: `Drive encryption capable
Drive encryption: disabled
`,
			wantSupported: true,
			wantEnabled:   false,
			wantMode:      "off",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &HardwareEncryptionStatus{Mode: "off"}
			svc.parseHardwareEncryptionStatus(tt.output, status)
			if status.Supported != tt.wantSupported {
				t.Errorf("Supported = %v, want %v", status.Supported, tt.wantSupported)
			}
			if status.Enabled != tt.wantEnabled {
				t.Errorf("Enabled = %v, want %v", status.Enabled, tt.wantEnabled)
			}
			if status.Mode != tt.wantMode {
				t.Errorf("Mode = %q, want %q", status.Mode, tt.wantMode)
			}
			if tt.wantAlgorithm != "" && status.Algorithm != tt.wantAlgorithm {
				t.Errorf("Algorithm = %q, want %q", status.Algorithm, tt.wantAlgorithm)
			}
		})
	}
}

func TestSetHardwareEncryptionInvalidKeySize(t *testing.T) {
	svc := NewService("/dev/nst0", 65536)

	// Too short
	err := svc.SetHardwareEncryption(context.Background(), make([]byte, 16))
	if err == nil {
		t.Error("expected error for 16-byte key")
	}

	// Too long
	err = svc.SetHardwareEncryption(context.Background(), make([]byte, 64))
	if err == nil {
		t.Error("expected error for 64-byte key")
	}

	// Empty
	err = svc.SetHardwareEncryption(context.Background(), []byte{})
	if err == nil {
		t.Error("expected error for empty key")
	}
}

func TestHardwareEncryptionStatusDefaults(t *testing.T) {
	status := &HardwareEncryptionStatus{
		Mode: "off",
	}
	if status.Supported {
		t.Error("expected Supported to be false by default")
	}
	if status.Enabled {
		t.Error("expected Enabled to be false by default")
	}
	if status.Mode != "off" {
		t.Errorf("expected Mode to be 'off', got %q", status.Mode)
	}
}

func TestGetDeviceLockSameDevice(t *testing.T) {
	mu1 := getDeviceLock("/dev/nst0")
	mu2 := getDeviceLock("/dev/nst0")
	if mu1 != mu2 {
		t.Error("expected same mutex for same device path")
	}
}

func TestGetDeviceLockDifferentDevices(t *testing.T) {
	mu1 := getDeviceLock("/dev/nst0")
	mu2 := getDeviceLock("/dev/nst1")
	if mu1 == mu2 {
		t.Error("expected different mutexes for different device paths")
	}
}

func TestServiceSharesDeviceLock(t *testing.T) {
	svc1 := NewService("/dev/nst99", 65536)
	svc2 := NewServiceForDevice("/dev/nst99", 65536)
	if svc1.deviceMu != svc2.deviceMu {
		t.Error("expected services for the same device to share the same mutex")
	}
}

func TestServiceDifferentDeviceLock(t *testing.T) {
	svc1 := NewService("/dev/nst98", 65536)
	svc2 := NewServiceForDevice("/dev/nst97", 65536)
	if svc1.deviceMu == svc2.deviceMu {
		t.Error("expected services for different devices to have different mutexes")
	}
}
