package common

import (
	"fmt"
	"strconv"
)

// ParseGoOctalLiteral parses strings like "0755" (or "755") into the
// same numeric value you'd get from a Go octal literal: oct := 0755.
//
// It rejects non-octal digits and returns the parsed value as a uint32.
func ParseGoOctalLiteral(s string) (uint32, error) {
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}

	// Accept optional leading 0, but require all digits be 0-7.
	if s[0] == '0' {
		s = s[1:]
		if s == "" {
			// "0" => 0
			return 0, nil
		}
	}

	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '7' {
			return 0, fmt.Errorf("invalid octal digit %q at index %d", s[i], i)
		}
	}

	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, err
	}
	return uint32(v), nil
}
