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
