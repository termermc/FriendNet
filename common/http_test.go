package common

import (
	"testing"
)

func TestParseHttpRange(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		fileSize int64
		offset   int64
		limit    int64
		valid    bool
	}{
		// Empty header
		{
			name:     "empty header",
			header:   "",
			fileSize: 1000,
			offset:   0,
			limit:    0,
			valid:    true,
		},
		// Valid standard ranges
		{
			name:     "standard range start to end",
			header:   "bytes=0-99",
			fileSize: 1000,
			offset:   0,
			limit:    100,
			valid:    true,
		},
		{
			name:     "standard range mid file",
			header:   "bytes=100-199",
			fileSize: 1000,
			offset:   100,
			limit:    100,
			valid:    true,
		},
		{
			name:     "standard range single byte",
			header:   "bytes=50-50",
			fileSize: 1000,
			offset:   50,
			limit:    1,
			valid:    true,
		},
		// Open-ended ranges
		{
			name:     "open-ended from start",
			header:   "bytes=0-",
			fileSize: 1000,
			offset:   0,
			limit:    1000,
			valid:    true,
		},
		{
			name:     "open-ended from middle",
			header:   "bytes=500-",
			fileSize: 1000,
			offset:   500,
			limit:    500,
			valid:    true,
		},
		// Suffix ranges
		{
			name:     "suffix range last 100 bytes",
			header:   "bytes=-100",
			fileSize: 1000,
			offset:   900,
			limit:    100,
			valid:    true,
		},
		{
			name:     "suffix range larger than file",
			header:   "bytes=-2000",
			fileSize: 1000,
			offset:   0,
			limit:    1000,
			valid:    true,
		},
		{
			name:     "suffix range single byte",
			header:   "bytes=-1",
			fileSize: 1000,
			offset:   999,
			limit:    1,
			valid:    true,
		},
		// Invalid formats
		{
			name:     "missing bytes prefix",
			header:   "100-199",
			fileSize: 1000,
			valid:    false,
		},
		{
			name:     "wrong prefix",
			header:   "words=0-99",
			fileSize: 1000,
			valid:    false,
		},
		{
			name:     "multiple ranges",
			header:   "bytes=0-99,200-299",
			fileSize: 1000,
			valid:    false,
		},
		{
			name:     "both start and end empty",
			header:   "bytes=-",
			fileSize: 1000,
			valid:    false,
		},
		{
			name:     "negative start",
			header:   "bytes=-10-100",
			fileSize: 1000,
			valid:    false,
		},
		{
			name:     "negative end",
			header:   "bytes=0--50",
			fileSize: 1000,
			valid:    false,
		},
		{
			name:     "end before start",
			header:   "bytes=500-100",
			fileSize: 1000,
			valid:    false,
		},
		{
			name:     "start at file size",
			header:   "bytes=1000-1099",
			fileSize: 1000,
			valid:    false,
		},
		{
			name:     "start beyond file size",
			header:   "bytes=1500-1600",
			fileSize: 1000,
			valid:    false,
		},
		{
			name:     "end at or beyond file size",
			header:   "bytes=900-1000",
			fileSize: 1000,
			valid:    false,
		},
		// Non-numeric values
		{
			name:     "non-numeric start",
			header:   "bytes=abc-100",
			fileSize: 1000,
			valid:    false,
		},
		{
			name:     "non-numeric end",
			header:   "bytes=0-xyz",
			fileSize: 1000,
			valid:    false,
		},
		// Edge cases
		{
			name:     "file size 0",
			header:   "bytes=0-0",
			fileSize: 0,
			valid:    false,
		},
		{
			name:     "file size 1, last byte",
			header:   "bytes=0-0",
			fileSize: 1,
			offset:   0,
			limit:    1,
			valid:    true,
		},
		{
			name:     "whitespace in range",
			header:   "bytes= 0 - 99 ",
			fileSize: 1000,
			offset:   0,
			limit:    100,
			valid:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset, limit, valid := ParseHttpRange(tt.header, tt.fileSize)

			if valid != tt.valid {
				t.Errorf("valid = %v, want %v", valid, tt.valid)
			}

			if valid && (offset != tt.offset || limit != tt.limit) {
				t.Errorf("got (%d, %d), want (%d, %d)", offset, limit, tt.offset, tt.limit)
			}
		})
	}
}
