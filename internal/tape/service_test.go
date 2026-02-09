package tape

import (
	"testing"
)

func TestParseTapeInfoStats(t *testing.T) {
	svc := NewService("/dev/nst0", 65536)

	tests := []struct {
		name     string
		output   string
		wantFunc func(t *testing.T, stats *DriveStatisticsData)
	}{
		{
			name: "parse total loads",
			output: `Product Type: Tape Drive
Vendor: IBM
Total Loads: 1234
Total Written: 567890123456
Total Read: 987654321012
Write Errors: 3
Read Errors: 5
CleaningRequired: yes
PowerOnHours: 5678
`,
			wantFunc: func(t *testing.T, stats *DriveStatisticsData) {
				if stats.TotalLoadCount != 1234 {
					t.Errorf("expected TotalLoadCount 1234, got %d", stats.TotalLoadCount)
				}
				if stats.TotalBytesWritten != 567890123456 {
					t.Errorf("expected TotalBytesWritten 567890123456, got %d", stats.TotalBytesWritten)
				}
				if stats.TotalBytesRead != 987654321012 {
					t.Errorf("expected TotalBytesRead 987654321012, got %d", stats.TotalBytesRead)
				}
				if stats.WriteErrors != 3 {
					t.Errorf("expected WriteErrors 3, got %d", stats.WriteErrors)
				}
				if stats.ReadErrors != 5 {
					t.Errorf("expected ReadErrors 5, got %d", stats.ReadErrors)
				}
				if !stats.CleaningRequired {
					t.Error("expected CleaningRequired to be true")
				}
				if stats.PowerOnHours != 5678 {
					t.Errorf("expected PowerOnHours 5678, got %d", stats.PowerOnHours)
				}
			},
		},
		{
			name: "parse cleaning not required",
			output: `CleaningRequired: no
`,
			wantFunc: func(t *testing.T, stats *DriveStatisticsData) {
				if stats.CleaningRequired {
					t.Error("expected CleaningRequired to be false")
				}
			},
		},
		{
			name:   "empty output",
			output: "",
			wantFunc: func(t *testing.T, stats *DriveStatisticsData) {
				if stats.TotalLoadCount != 0 {
					t.Errorf("expected TotalLoadCount 0, got %d", stats.TotalLoadCount)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &DriveStatisticsData{}
			svc.parseTapeInfoStats(tt.output, stats)
			tt.wantFunc(t, stats)
		})
	}
}

func TestParseSgLogsStats(t *testing.T) {
	svc := NewService("/dev/nst0", 65536)

	tests := []struct {
		name     string
		output   string
		wantFunc func(t *testing.T, stats *DriveStatisticsData)
	}{
		{
			name: "parse sequential access page",
			output: `Sequential access device page
  Bytes written = 1234567890
  Bytes read = 9876543210
  Load count = 42
  tape motion hours = 123.5
  power on hours = 9999
`,
			wantFunc: func(t *testing.T, stats *DriveStatisticsData) {
				if stats.TotalBytesWritten != 1234567890 {
					t.Errorf("expected TotalBytesWritten 1234567890, got %d", stats.TotalBytesWritten)
				}
				if stats.TotalBytesRead != 9876543210 {
					t.Errorf("expected TotalBytesRead 9876543210, got %d", stats.TotalBytesRead)
				}
				if stats.TotalLoadCount != 42 {
					t.Errorf("expected TotalLoadCount 42, got %d", stats.TotalLoadCount)
				}
				if stats.TapeMotionHours != 123.5 {
					t.Errorf("expected TapeMotionHours 123.5, got %f", stats.TapeMotionHours)
				}
				if stats.PowerOnHours != 9999 {
					t.Errorf("expected PowerOnHours 9999, got %d", stats.PowerOnHours)
				}
			},
		},
		{
			name: "cleaning required detection",
			output: `Sequential access device page
  cleaning required = 1
`,
			wantFunc: func(t *testing.T, stats *DriveStatisticsData) {
				if !stats.CleaningRequired {
					t.Error("expected CleaningRequired to be true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &DriveStatisticsData{}
			svc.parseSgLogsStats(tt.output, stats)
			tt.wantFunc(t, stats)
		})
	}
}

func TestParseErrorCounters(t *testing.T) {
	svc := NewService("/dev/nst0", 65536)

	tests := []struct {
		name    string
		output  string
		isWrite bool
		wantErr int64
	}{
		{
			name: "write error counters",
			output: `Write error counter page
  total errors = 7
`,
			isWrite: true,
			wantErr: 7,
		},
		{
			name: "read error counters",
			output: `Read error counter page
  Total errors = 3
`,
			isWrite: false,
			wantErr: 3,
		},
		{
			name: "uncorrected errors",
			output: `Write error counter page
  uncorrected = 2
`,
			isWrite: true,
			wantErr: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &DriveStatisticsData{}
			svc.parseErrorCounters(tt.output, stats, tt.isWrite)
			if tt.isWrite {
				if stats.WriteErrors != tt.wantErr {
					t.Errorf("expected WriteErrors %d, got %d", tt.wantErr, stats.WriteErrors)
				}
			} else {
				if stats.ReadErrors != tt.wantErr {
					t.Errorf("expected ReadErrors %d, got %d", tt.wantErr, stats.ReadErrors)
				}
			}
		})
	}
}

func TestExtractSgLogsValue(t *testing.T) {
	tests := []struct {
		line string
		want int64
	}{
		{"  Bytes written = 1234567890", 1234567890},
		{"  Load count = 42", 42},
		{"  total errors = 0", 0},
		{"no equals sign", 0},
		{"  empty = ", 0},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := extractSgLogsValue(tt.line)
			if got != tt.want {
				t.Errorf("extractSgLogsValue(%q) = %d, want %d", tt.line, got, tt.want)
			}
		})
	}
}

func TestExtractSgLogsFloat(t *testing.T) {
	tests := []struct {
		line string
		want float64
	}{
		{"  tape motion hours = 123.5", 123.5},
		{"  power on hours = 9999", 9999.0},
		{"no equals sign", 0},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := extractSgLogsFloat(tt.line)
			if got != tt.want {
				t.Errorf("extractSgLogsFloat(%q) = %f, want %f", tt.line, got, tt.want)
			}
		})
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
