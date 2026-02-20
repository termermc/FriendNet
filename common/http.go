package common

import (
	"strconv"
	"strings"
)

// ParseHttpRange parses an HTTP Range header string.
// If fileSize < 0, the function panics.
func ParseHttpRange(header string, fileSize int64) (offset int64, limit int64, valid bool) {
	if fileSize < 0 {
		panic("BUG: parseRange: fileSize < 0")
	}

	if header == "" {
		return 0, 0, true
	}

	// Range header format: "bytes=start-end".
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, false
	}

	rangeSpec := strings.TrimPrefix(header, "bytes=")

	// Reject multiple ranges.
	if strings.Contains(rangeSpec, ",") {
		return 0, 0, false
	}

	parts := strings.Split(rangeSpec, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}

	start := strings.TrimSpace(parts[0])
	end := strings.TrimSpace(parts[1])

	// Both empty is invalid.
	if start == "" && end == "" {
		return 0, 0, false
	}

	// Suffix range: "bytes=-500".
	if start == "" {
		suffix, err := strconv.ParseInt(end, 10, 64)
		if err != nil || suffix < 0 {
			return 0, 0, false
		}

		offset = fileSize - suffix
		if offset < 0 {
			offset = 0
		}
		return offset, fileSize - offset, true
	}

	startNum, err := strconv.ParseInt(start, 10, 64)
	if err != nil || startNum < 0 {
		return 0, 0, false
	}

	if startNum >= fileSize {
		return 0, 0, false
	}

	if end == "" {
		// Open-ended range: "bytes=100-".
		return startNum, fileSize - startNum, true
	}

	endNum, err := strconv.ParseInt(end, 10, 64)
	if err != nil || endNum < 0 {
		return 0, 0, false
	}

	if endNum < startNum || endNum >= fileSize {
		return 0, 0, false
	}

	// limit is inclusive end position, so add 1.
	limit = endNum - startNum + 1
	return startNum, limit, true
}
