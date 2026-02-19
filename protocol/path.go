package protocol

import (
	"strings"
	"unicode/utf8"
)

// PathErrCode is a path error code stored inside a PathError.
type PathErrCode string

const (
	PathErrCodeBlank             PathErrCode = "path is blank"
	PathErrCodeInvalidUtf8       PathErrCode = "path is invalid UTF-8"
	PathErrCodeNullByte          PathErrCode = "path contains null byte"
	PathErrCodeNotAbsolute       PathErrCode = `path is not absolute (does not start with "/")`
	PathErrCodePathContainsDots  PathErrCode = `path contains "." or ".." segment`
	PathErrCodePathEndsWithSlash PathErrCode = `path ends with "/"`
	PathErrCodeDuplicateSlash    PathErrCode = `path contains multiple consecutive slashes`
)

// PathError is an error returned when a path is invalid.
type PathError struct {
	Code PathErrCode
	Path string
}

func NewPathError(code PathErrCode, path string) *PathError {
	return &PathError{
		Code: code,
		Path: path,
	}
}

func (e *PathError) Error() string {
	return string(e.Code) + ": " + e.Path
}

// ProtoPath is a valid protocol path.
// Valid protocol paths adhere to the rules in the protocol's `README.md` file.
// To create one, use ValidatePath or UncheckedCreateProtoPath.
type ProtoPath struct {
	string
}

// UncheckedCreateProtoPath creates a ProtoPath from a string without validating it.
// Use ValidatePath to validate the path.
func UncheckedCreateProtoPath(path string) ProtoPath {
	return ProtoPath{path}
}

// IsZero returns whether the ProtoPath is a zero value.
// If so, it MUST NOT be used.
// Calling ProtoPath.String on a zero value will panic.
func (u ProtoPath) IsZero() bool {
	return u.string == ""
}

// String returns the string representation of the path.
// If calling on a zero value, panics.
// If unsure, call ProtoPath.IsZero first.
func (u ProtoPath) String() string {
	if u.IsZero() {
		panic("tried to call String() on a zero ProtoPath")
	}

	return u.string
}

// ZeroProtoPath is the zero value of ProtoPath.
// It is invalid.
var ZeroProtoPath = ProtoPath{}

// ValidatePath validates a protocol path and returns a PathError if it is invalid.
func ValidatePath(path string) (ProtoPath, error) {
	if path == "" {
		return ProtoPath{}, NewPathError(PathErrCodeBlank, path)
	}
	if path == "/" {
		return ZeroProtoPath, nil
	}

	if !utf8.ValidString(path) {
		return ZeroProtoPath, NewPathError(PathErrCodeInvalidUtf8, path)
	}
	if strings.IndexByte(path, '\x00') != -1 {
		return ZeroProtoPath, NewPathError(PathErrCodeNullByte, path)
	}

	idx := 0
	lastSlashIdx := -1
	for {
		slashIdx := strings.IndexRune(path[idx:], '/')

		var isLastIter bool
		if slashIdx == -1 {
			isLastIter = true

			// Pretend slash is at the end of the string so that we can process the last path segment.
			slashIdx = len(path)
		} else {
			isLastIter = false

			// Get real index into string.
			slashIdx += idx

			if slashIdx == len(path)-1 {
				return ZeroProtoPath, NewPathError(PathErrCodePathEndsWithSlash, path)
			}
		}

		if idx == 0 {
			// First iteration.

			// Make sure slash is at index 0.
			if slashIdx != 0 {
				return ZeroProtoPath, NewPathError(PathErrCodeNotAbsolute, path)
			}
		} else {
			// Later iteration.

			// Make sure last slash was not directly before this one.
			if slashIdx == lastSlashIdx+1 {
				return ZeroProtoPath, NewPathError(PathErrCodeDuplicateSlash, path)
			}

			lastSegment := path[lastSlashIdx+1 : slashIdx]
			if lastSegment == "." || lastSegment == ".." {
				return ZeroProtoPath, NewPathError(PathErrCodePathContainsDots, path)
			}
		}

		if isLastIter {
			break
		}

		lastSlashIdx = slashIdx
		idx = slashIdx + 1
	}

	return UncheckedCreateProtoPath(path), nil
}
