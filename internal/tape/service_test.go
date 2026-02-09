package tape

import (
	"encoding/json"
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
		name     string
		output   string
		wantRead int64
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
