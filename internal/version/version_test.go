package version

import (
	"testing"
)

func TestCompare(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
	}{
		{"v1.5.0", "v1.5.0", 0},
		{"1.5.0", "v1.5.0", 0},
		{"v1.5.0", "v1.6.0", -1},
		{"v1.6.0", "v1.5.0", 1},
		{"v1.5.1", "v1.5.0", 1},
		{"v2.0.0", "v1.9.9", 1},
		{"v1.5.0-dev", "v1.5.0", -1},
		{"v1.5.0", "v1.5.0-rc1", 1},
	}

	for _, tt := range tests {
		got := Compare(tt.v1, tt.v2)
		if got != tt.expected {
			t.Errorf("Compare(%q, %q) = %d; want %d", tt.v1, tt.v2, got, tt.expected)
		}
	}
}

func TestIsNewer(t *testing.T) {
	if !IsNewer("v1.6.0", "v1.5.0") {
		t.Errorf("expected v1.6.0 to be newer than v1.5.0")
	}
	if IsNewer("v1.5.0", "v1.5.0") {
		t.Errorf("expected v1.5.0 to not be newer than v1.5.0")
	}
	if IsNewer("v1.4.0", "v1.5.0") {
		t.Errorf("expected v1.4.0 to not be newer than v1.5.0")
	}
}
