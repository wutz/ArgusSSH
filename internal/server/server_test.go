package server

import (
	"testing"
)

func TestMatchCommand(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		cmdLine  string
		expected bool
	}{
		{
			name:     "exact match",
			pattern:  "echo hello",
			cmdLine:  "echo hello",
			expected: true,
		},
		{
			name:     "pattern prefix match",
			pattern:  "echo",
			cmdLine:  "echo hello world",
			expected: true,
		},
		{
			name:     "pattern with args matches longer command",
			pattern:  "docker logs",
			cmdLine:  "docker logs myapp",
			expected: true,
		},
		{
			name:     "pattern mismatch",
			pattern:  "cat",
			cmdLine:  "echo hello",
			expected: false,
		},
		{
			name:     "command shorter than pattern",
			pattern:  "docker logs myapp",
			cmdLine:  "docker logs",
			expected: false,
		},
		{
			name:     "different first arg",
			pattern:  "docker ps",
			cmdLine:  "docker logs",
			expected: false,
		},
		{
			name:     "empty pattern",
			pattern:  "",
			cmdLine:  "echo hello",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchCommand(tt.pattern, tt.cmdLine)
			if result != tt.expected {
				t.Errorf("matchCommand(%q, %q) = %v, want %v", tt.pattern, tt.cmdLine, result, tt.expected)
			}
		})
	}
}

func TestExtractCommand(t *testing.T) {
	tests := []struct {
		name     string
		payload  []byte
		expected string
	}{
		{
			name:     "valid command",
			payload:  []byte{0, 0, 0, 4, 'e', 'c', 'h', 'o'},
			expected: "echo",
		},
		{
			name:     "empty payload",
			payload:  []byte{},
			expected: "",
		},
		{
			name:     "truncated payload",
			payload:  []byte{0, 0, 0, 10, 'e', 'c'},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCommand(tt.payload)
			if result != tt.expected {
				t.Errorf("extractCommand() = %q, want %q", result, tt.expected)
			}
		})
	}
}
