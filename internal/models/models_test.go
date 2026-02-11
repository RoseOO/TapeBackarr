package models

import "testing"

func TestLTOTypeFromDensity(t *testing.T) {
	tests := []struct {
		name        string
		densityCode string
		wantType    string
		wantOK      bool
	}{
		{"LTO-1", "0x40", "LTO-1", true},
		{"LTO-2", "0x42", "LTO-2", true},
		{"LTO-3", "0x44", "LTO-3", true},
		{"LTO-4", "0x46", "LTO-4", true},
		{"LTO-5", "0x58", "LTO-5", true},
		{"LTO-6", "0x5a", "LTO-6", true},
		{"LTO-6 uppercase", "0x5A", "LTO-6", true},
		{"LTO-7", "0x5c", "LTO-7", true},
		{"LTO-7 Type M", "0x5d", "LTO-7", true},
		{"LTO-8", "0x5e", "LTO-8", true},
		{"LTO-9", "0x60", "LTO-9", true},
		{"LTO-10", "0x62", "LTO-10", true},
		{"unknown code", "0xff", "", false},
		{"empty string", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotOK := LTOTypeFromDensity(tt.densityCode)
			if gotType != tt.wantType || gotOK != tt.wantOK {
				t.Errorf("LTOTypeFromDensity(%q) = (%q, %v), want (%q, %v)",
					tt.densityCode, gotType, gotOK, tt.wantType, tt.wantOK)
			}
		})
	}
}

func TestDensityToLTOTypeCoversAllCapacities(t *testing.T) {
	// Verify that every LTO type returned by DensityToLTOType has a matching entry in LTOCapacities
	seen := make(map[string]bool)
	for code, ltoType := range DensityToLTOType {
		if _, ok := LTOCapacities[ltoType]; !ok {
			t.Errorf("DensityToLTOType[%q] = %q, but %q not found in LTOCapacities", code, ltoType, ltoType)
		}
		seen[ltoType] = true
	}
	// Every LTO type in LTOCapacities should be reachable via at least one density code
	for ltoType := range LTOCapacities {
		if !seen[ltoType] {
			t.Errorf("LTOCapacities has %q but no density code maps to it", ltoType)
		}
	}
}

func TestTapeFormatType(t *testing.T) {
	tests := []struct {
		name    string
		format  TapeFormatType
		wantStr string
	}{
		{"raw format", TapeFormatRaw, "raw"},
		{"ltfs format", TapeFormatLTFS, "ltfs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.format) != tt.wantStr {
				t.Errorf("expected %q, got %q", tt.wantStr, string(tt.format))
			}
		})
	}
}

func TestTapeFormatTypeDistinct(t *testing.T) {
	if TapeFormatRaw == TapeFormatLTFS {
		t.Error("TapeFormatRaw and TapeFormatLTFS should be distinct values")
	}
}

func TestCanUseLTFS(t *testing.T) {
	tests := []struct {
		name    string
		ltoType string
		want    bool
	}{
		{"LTO-1 not supported", "LTO-1", false},
		{"LTO-2 not supported", "LTO-2", false},
		{"LTO-3 not supported", "LTO-3", false},
		{"LTO-4 not supported", "LTO-4", false},
		{"LTO-5 supported", "LTO-5", true},
		{"LTO-6 supported", "LTO-6", true},
		{"LTO-7 supported", "LTO-7", true},
		{"LTO-8 supported", "LTO-8", true},
		{"LTO-9 supported", "LTO-9", true},
		{"LTO-10 supported", "LTO-10", true},
		{"empty string", "", false},
		{"invalid string", "DAT-72", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanUseLTFS(tt.ltoType); got != tt.want {
				t.Errorf("CanUseLTFS(%q) = %v, want %v", tt.ltoType, got, tt.want)
			}
		})
	}
}

func TestLTFSVendorLookup(t *testing.T) {
	tests := []struct {
		name        string
		vendor      string
		wantBackend LTFSBackend
		wantSupport bool
	}{
		{"IBM drives", "IBM", LTFSBackendLinTape, true},
		{"HP drives", "HP", LTFSBackendSG, true},
		{"HPE drives", "HPE", LTFSBackendSG, true},
		{"Tandberg drives", "TANDBERG", LTFSBackendSG, true},
		{"Tandberg lowercase", "tandberg", LTFSBackendSG, true},
		{"Overland drives", "OVERLAND", LTFSBackendSG, true},
		{"Quantum drives", "QUANTUM", LTFSBackendSG, true},
		{"Unknown vendor", "ACME", LTFSBackendSG, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := LTFSVendorLookup(tt.vendor)
			if info.Backend != tt.wantBackend {
				t.Errorf("LTFSVendorLookup(%q).Backend = %q, want %q", tt.vendor, info.Backend, tt.wantBackend)
			}
			if info.Supported != tt.wantSupport {
				t.Errorf("LTFSVendorLookup(%q).Supported = %v, want %v", tt.vendor, info.Supported, tt.wantSupport)
			}
		})
	}
}

func TestCheckLTFSCompat(t *testing.T) {
	tests := []struct {
		name       string
		vendor     string
		ltoType    string
		wantCompat bool
	}{
		{"Tandberg LTO-5", "TANDBERG", "LTO-5", true},
		{"Overland LTO-5", "OVERLAND", "LTO-5", true},
		{"IBM LTO-7", "IBM", "LTO-7", true},
		{"HP LTO-6", "HP", "LTO-6", true},
		{"LTO-4 not compatible", "HP", "LTO-4", false},
		{"no tape loaded", "HP", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckLTFSCompat(tt.vendor, tt.ltoType)
			if result.Compatible != tt.wantCompat {
				t.Errorf("CheckLTFSCompat(%q, %q).Compatible = %v, want %v (reason: %s)",
					tt.vendor, tt.ltoType, result.Compatible, tt.wantCompat, result.Reason)
			}
		})
	}
}
