package fsys

import (
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// FilenameReplacer is a function that replaces invalid characters in filenames.
type FilenameReplacer func(string) string

// ReplaceFilename replaces characters in filenames according to the mapping and returns the resulting string.
func (r FilenameReplacer) ReplaceFilename(filename string) string {
	return r(filename)
}

// ReplacePath replaces characters in each segment of the specified path according to the mapping and returns the
// resulting string.
func (r FilenameReplacer) ReplacePath(path string) string {
	sep := string(filepath.Separator)
	parts := strings.Split(path, sep)
	for i, part := range parts {
		if part == "" {
			continue
		}

		parts[i] = r.ReplaceFilename(part)
	}
	return strings.Join(parts, sep)
}

// GetFilenameReplacerForPath returns the appropriate filename filter for files written inside the specified path.
// The filter selected may be overly broad (such as applying NTFS rules to APFS), but it will try to never be
// insufficient (such as Ext4 rules for FAT).
func GetFilenameReplacerForPath(path string) (FilenameReplacer, error) {
	return getFilenameReplacerForPath(path)
}

// StrictReplacer is a FilenameReplacer that implements the most strict rules.
// It is suitable for NTFS, FAT(32,16) and APFS.
// It also handles Windows-specific restrictions, like device names.
var StrictReplacer FilenameReplacer = func(str string) string {
	var b strings.Builder
	// Allocate slightly larger than the input string length.
	// This is to allow space for potential Unicode replacements that are bigger than their ASCII equivalents.
	b.Grow(int(float64(len(str)) * 1.25))

	for _, r := range str {
		switch r {
		// Always-invalid / reserved across Windows (NTFS/FAT) and/or POSIX path rules
		case 0: // NUL
			b.WriteRune('␀') // SYMBOL FOR NULL
		case '/':
			b.WriteRune('／') // FULLWIDTH SOLIDUS
		case '\\':
			b.WriteRune('＼') // FULLWIDTH REVERSE SOLIDUS
		case ':':
			b.WriteRune('꞉') // MODIFIER LETTER COLON (good visual colon substitute)
		case '*':
			b.WriteRune('∗') // ASTERISK OPERATOR
		case '?':
			b.WriteRune('？') // FULLWIDTH QUESTION MARK
		case '"':
			b.WriteRune('＂') // FULLWIDTH QUOTATION MARK
		case '<':
			b.WriteRune('‹') // SINGLE LEFT-POINTING ANGLE QUOTATION MARK
		case '>':
			b.WriteRune('›') // SINGLE RIGHT-POINTING ANGLE QUOTATION MARK
		case '|':
			b.WriteRune('｜') // FULLWIDTH VERTICAL LINE

		// Control chars (Windows disallows U+0000..U+001F; FAT similarly problematic).
		// Map to Unicode control pictures for readability where possible.
		case 0x01:
			b.WriteRune('␁')
		case 0x02:
			b.WriteRune('␂')
		case 0x03:
			b.WriteRune('␃')
		case 0x04:
			b.WriteRune('␄')
		case 0x05:
			b.WriteRune('␅')
		case 0x06:
			b.WriteRune('␆')
		case 0x07:
			b.WriteRune('␇')
		case 0x08:
			b.WriteRune('␈')
		case 0x09:
			b.WriteRune('␉')
		case 0x0A:
			b.WriteRune('␊')
		case 0x0B:
			b.WriteRune('␋')
		case 0x0C:
			b.WriteRune('␌')
		case 0x0D:
			b.WriteRune('␍')
		case 0x0E:
			b.WriteRune('␎')
		case 0x0F:
			b.WriteRune('␏')
		case 0x10:
			b.WriteRune('␐')
		case 0x11:
			b.WriteRune('␑')
		case 0x12:
			b.WriteRune('␒')
		case 0x13:
			b.WriteRune('␓')
		case 0x14:
			b.WriteRune('␔')
		case 0x15:
			b.WriteRune('␕')
		case 0x16:
			b.WriteRune('␖')
		case 0x17:
			b.WriteRune('␗')
		case 0x18:
			b.WriteRune('␘')
		case 0x19:
			b.WriteRune('␙')
		case 0x1A:
			b.WriteRune('␚')
		case 0x1B:
			b.WriteRune('␛')
		case 0x1C:
			b.WriteRune('␜')
		case 0x1D:
			b.WriteRune('␝')
		case 0x1E:
			b.WriteRune('␞')
		case 0x1F:
			b.WriteRune('␟')

		// DEL (not always forbidden by the same APIs, but often undesirable).
		case 0x7F:
			b.WriteRune('␡') // SYMBOL FOR DELETE

		default:
			// Replace invalid UTF-8 rune if it ever appears (Go uses RuneError).
			// This keeps the output stable rather than emitting U+FFFD.
			if r == utf8.RuneError {
				b.WriteRune('�')
				continue
			}
			b.WriteRune(r)
		}
	}

	// Windows/FAT practical restriction: no trailing space or period.
	out := b.String()
	out = strings.TrimRight(out, " .")

	// Avoid empty result.
	if out == "" {
		out = "_"
	}

	// Avoid Windows reserved device names (case-insensitive),
	// even with extensions. E.g., "CON", "con.txt", "LPT1." etc.
	// (Still "most strict" for NTFS/FAT compatibility.)
	if isWindowsReservedDeviceName(out) {
		out = "_" + out
	}

	return out
}

func isWindowsReservedDeviceName(name string) bool {
	// Check the base name up to first dot, and ignore trailing spaces/dots
	// (Windows normalizes those away).
	n := strings.TrimRight(name, " .")
	base := n
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	base = strings.ToUpper(base)

	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	case "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9":
		return true
	case "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}
