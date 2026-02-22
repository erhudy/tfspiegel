package main

import "testing"

func TestStringInSlice(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		slice  []string
		expect bool
	}{
		{"found", "b", []string{"a", "b", "c"}, true},
		{"not found", "d", []string{"a", "b", "c"}, false},
		{"empty slice", "a", []string{}, false},
		{"empty string match", "", []string{"a", "", "b"}, true},
		{"empty string no match", "", []string{"a", "b"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StringInSlice(tt.s, tt.slice)
			if got != tt.expect {
				t.Errorf("StringInSlice(%q, %v) = %v, want %v", tt.s, tt.slice, got, tt.expect)
			}
		})
	}
}
