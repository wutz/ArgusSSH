package server

import (
	"encoding/binary"
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

func TestParsePtyRequest(t *testing.T) {
	tests := []struct {
		name       string
		payload    []byte
		wantTerm   string
		wantWidth  uint32
		wantHeight uint32
	}{
		{
			name: "xterm 80x24",
			payload: func() []byte {
				term := "xterm"
				buf := make([]byte, 4+len(term)+8)
				binary.BigEndian.PutUint32(buf[0:4], uint32(len(term)))
				copy(buf[4:4+len(term)], term)
				offset := 4 + len(term)
				binary.BigEndian.PutUint32(buf[offset:offset+4], 80)
				binary.BigEndian.PutUint32(buf[offset+4:offset+8], 24)
				return buf
			}(),
			wantTerm:   "xterm",
			wantWidth:  80,
			wantHeight: 24,
		},
		{
			name: "xterm-256color 120x40",
			payload: func() []byte {
				term := "xterm-256color"
				buf := make([]byte, 4+len(term)+8)
				binary.BigEndian.PutUint32(buf[0:4], uint32(len(term)))
				copy(buf[4:4+len(term)], term)
				offset := 4 + len(term)
				binary.BigEndian.PutUint32(buf[offset:offset+4], 120)
				binary.BigEndian.PutUint32(buf[offset+4:offset+8], 40)
				return buf
			}(),
			wantTerm:   "xterm-256color",
			wantWidth:  120,
			wantHeight: 40,
		},
		{
			name:       "empty payload",
			payload:    []byte{},
			wantTerm:   "",
			wantWidth:  80,
			wantHeight: 24,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term, width, height := parsePtyRequest(tt.payload)
			if term != tt.wantTerm {
				t.Errorf("term = %q, want %q", term, tt.wantTerm)
			}
			if width != tt.wantWidth {
				t.Errorf("width = %d, want %d", width, tt.wantWidth)
			}
			if height != tt.wantHeight {
				t.Errorf("height = %d, want %d", height, tt.wantHeight)
			}
		})
	}
}

func TestScanCRLFLine(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		atEOF       bool
		wantAdvance int
		wantToken   string
	}{
		{"LF", []byte("hello\nworld"), false, 6, "hello"},
		{"CR", []byte("hello\rworld"), false, 6, "hello"},
		{"CRLF", []byte("hello\r\nworld"), false, 7, "hello"},
		{"no newline", []byte("hello"), false, 0, ""},
		{"EOF", []byte("hello"), true, 5, "hello"},
		{"empty LF", []byte("\n"), false, 1, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			advance, token, _ := scanCRLFLine(tt.data, tt.atEOF)
			if advance != tt.wantAdvance {
				t.Errorf("advance = %d, want %d", advance, tt.wantAdvance)
			}
			got := ""
			if token != nil {
				got = string(token)
			}
			if got != tt.wantToken {
				t.Errorf("token = %q, want %q", got, tt.wantToken)
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
