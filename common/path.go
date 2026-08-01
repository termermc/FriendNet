package common

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
	PathErrCodeReferencesParent  PathErrCode = `path references parent directory`
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

// ToSegments returns the segments of the protocol path.
// If the path is "/", returns an empty slice.
//
// For example, "/foo/bar" -> []string{"foo", "bar"}
//
// If calling on a zero value, panics.
// If unsure, call ProtoPath.IsZero first.
func (u ProtoPath) ToSegments() []string {
	if u.IsZero() {
		panic("tried to call ToSegments() on a zero ProtoPath")
	}

	if u.string == "/" {
		return []string{}
	}

	return strings.Split(u.string[1:], "/")
}

// Name returns the last segment of the path.
// If the path is "/", returns "".
//
// For example, "/foo/bar" -> "bar"
//
// If calling on a zero value, panics.
// If unsure, call ProtoPath.IsZero first.
func (u ProtoPath) Name() string {
	if u.IsZero() {
		panic("tried to call Name() on a zero ProtoPath")
	}

	slashIdx := strings.LastIndex(u.string, "/")
	return u.string[slashIdx+1:]
}

// IsRoot returns whether the path is "/".
func (u ProtoPath) IsRoot() bool {
	return u == RootProtoPath
}

// ZeroProtoPath is the zero value of ProtoPath.
// It is invalid.
var ZeroProtoPath = ProtoPath{}

// RootProtoPath is the root path "/".
var RootProtoPath = UncheckedCreateProtoPath("/")

// ValidatePath validates a protocol path and returns a PathError if it is invalid.
// This function expects an already correctly normalized path.
//
// To normalize a path before validating it, use NormalizePath.
func ValidatePath(path string) (ProtoPath, error) {
	if path == "" {
		return ProtoPath{}, NewPathError(PathErrCodeBlank, path)
	}
	if path == "/" {
		return UncheckedCreateProtoPath("/"), nil
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

// SegmentsToPath converts a slice of path segments into a protocol path.
// Each segment must be valid, according to the rules in the protocol's `README.md` file.
// Returns a PathError if any segment is invalid.
func SegmentsToPath(segments []string) (ProtoPath, error) {
	if len(segments) == 0 {
		return UncheckedCreateProtoPath("/"), nil
	}

	outStr := "/" + strings.Join(segments, "/")

	for _, segment := range segments {
		if !utf8.ValidString(segment) {
			return ZeroProtoPath, NewPathError(PathErrCodeInvalidUtf8, outStr)
		}
		if strings.IndexByte(segment, '\x00') != -1 {
			return ZeroProtoPath, NewPathError(PathErrCodeNullByte, outStr)
		}

		if segment == "." || segment == ".." {
			return ZeroProtoPath, NewPathError(PathErrCodePathContainsDots, outStr)
		}
	}

	return UncheckedCreateProtoPath(outStr), nil
}

// NormalizePath normalizes a path before validating it.
// If you have a path you already expect to be valid, use ValidatePath instead.
//
// For example, "/foo/../bar" -> "/bar".
//
// Returns a PathError if the path references a parent directory or the path cannot be validated.
func NormalizePath(path string) (ProtoPath, error) {
	parts := strings.Split(path, "/")
	segments := make([]string, 0, len(parts))

	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			if len(segments) == 0 {
				return ZeroProtoPath, NewPathError(PathErrCodeReferencesParent, path)
			}

			segments = segments[:len(segments)-1]

			continue
		}
		segments = append(segments, part)
	}

	return ValidatePath("/" + strings.Join(segments, "/"))
}

// JoinPaths joins multiple paths together.
// If no paths are provided, returns the root path "/".
func JoinPaths(paths ...ProtoPath) ProtoPath {
	if len(paths) == 0 {
		return RootProtoPath
	}

	var sb strings.Builder
	for _, path := range paths {
		sb.Grow(len(path.String()))
	}
	for _, path := range paths {
		if path.IsRoot() {
			continue
		}

		sb.WriteString(path.String())
	}
	return UncheckedCreateProtoPath(sb.String())
}
