package api

import "testing"

func TestIsValidHexColor(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"#fff", true},
		{"#FFF", true},
		{"#3B82F6", true},
		{"#3b82f6", true},
		{"#AABBCCDD", true},  // 8-digit (with alpha)
		{"#ABC", true},       // 3-digit shorthand
		{"#ABCD", true},      // 4-digit shorthand (with alpha)
		{"", false},          // empty
		{"3B82F6", false},    // missing #
		{"#GGG", false},     // invalid hex chars
		{"#12", false},      // too short
		{"#12345", false},   // wrong length (5)
		{"#1234567", false}, // wrong length (7)
		{"#", false},        // just hash
		{"rgb(0,0,0)", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidHexColor(tt.input)
			if got != tt.want {
				t.Errorf("isValidHexColor(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
