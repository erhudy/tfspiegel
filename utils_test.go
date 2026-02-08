package main

import "testing"

func TestStringInSlice(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		slice    []string
		expected bool
	}{
		{
			name:     "found",
			s:        "b",
			slice:    []string{"a", "b", "c"},
			expected: true,
		},
		{
			name:     "not found",
			s:        "d",
			slice:    []string{"a", "b", "c"},
			expected: false,
		},
		{
			name:     "empty slice",
			s:        "a",
			slice:    []string{},
			expected: false,
		},
		{
			name:     "empty string present",
			s:        "",
			slice:    []string{"a", "", "c"},
			expected: true,
		},
		{
			name:     "empty string absent",
			s:        "",
			slice:    []string{"a", "b", "c"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringInSlice(tt.s, tt.slice)
			if result != tt.expected {
				t.Errorf("StringInSlice(%q, %v) = %v, want %v", tt.s, tt.slice, result, tt.expected)
			}
		})
	}
}
