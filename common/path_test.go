package common

import "testing"

//goland:noinspection ALL
func TestPathContains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		parent   string
		path     string
		wantBool bool
		wantIdx  int
	}{
		// --- valid ---
		{
			name:     "valid_root_whatever",
			parent:   "/",
			path:     "/this/is/anything",
			wantBool: true,
			wantIdx:  0,
		},
		{
			name:     "valid_first_degree",
			parent:   "/foo/",
			path:     "/foo/bar",
			wantBool: true,
			wantIdx:  1,
		},
		{
			name:     "valid_second_degree",
			parent:   "/foo/",
			path:     "/foo/bar/qux",
			wantBool: true,
			wantIdx:  1,
		},
		// --- invalid ---
		{
			name:     "invalid_longer_parent",
			parent:   "/foo/bar/qux",
			path:     "/foo/",
			wantBool: false,
			wantIdx:  0,
		},
		{
			name:     "invalid_wrong_first_degree",
			parent:   "/qux/",
			path:     "/foo/bar/qux",
			wantBool: false,
			wantIdx:  0,
		},
		{
			name:     "invalid_wrong_second_degree",
			parent:   "/qux/foo",
			path:     "/foo/bar/qux",
			wantBool: false,
			wantIdx:  0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parent, err := NormalizePath(tt.parent)
			if err != nil {
				t.Fatalf("PathContains parent normalize fail: %v", err)
			}

			path, err := NormalizePath(tt.path)
			if err != nil {
				t.Fatalf("PathContains path normalize fail: %v", err)
			}

			res, idx := PathContains(parent, path)

			if res != tt.wantBool {
				t.Fatalf("expected result %t, got %t", tt.wantBool, res)
			}

			if idx != tt.wantIdx {
				t.Fatalf("expected parent path cutoff index %d, got %d", tt.wantIdx, idx)
			}
		})
	}
}

//goland:noinspection ALL
func TestValidatePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		wantErr  bool
		wantCode PathErrCode
	}{
		// --- valid ---
		{
			name:    "valid_root_only",
			path:    "/",
			wantErr: false,
		},
		{
			name:    "valid_single_segment",
			path:    "/foo",
			wantErr: false,
		},
		{
			name:    "valid_multiple_segments_with_spaces_and_unicode",
			path:    "/shared music/Kevin MacLeod/Monkeys Spinning Monkeys.mp3",
			wantErr: false,
		},
		{
			name:    "valid_allows_tilde_literal",
			path:    "/~/not-an-alias",
			wantErr: false,
		},
		{
			name:    "valid_allows_dot_in_segment_not_equal_dot",
			path:    "/foo./.bar/..baz/baz..",
			wantErr: false,
		},
		{
			name:    "valid_allows_empty_segment_only_for_root",
			path:    "/",
			wantErr: false,
		},

		// --- invalid UTF-8 ---
		{
			name:     "invalid_utf8",
			path:     string([]byte{0xff, 0xfe, '/'}),
			wantErr:  true,
			wantCode: PathErrCodeInvalidUtf8,
		},

		// --- null byte ---
		{
			name:     "contains_null_byte",
			path:     "/foo\x00bar",
			wantErr:  true,
			wantCode: PathErrCodeNullByte,
		},

		// --- absolute path rule ---
		{
			name:     "not_absolute_missing_leading_slash",
			path:     "song.mp3",
			wantErr:  true,
			wantCode: PathErrCodeNotAbsolute,
		},
		{
			name:     "not_absolute_starts_with_backslash",
			path:     `\pics\dogs`,
			wantErr:  true,
			wantCode: PathErrCodeNotAbsolute,
		},
		{
			name:     "empty_string",
			path:     "",
			wantErr:  true,
			wantCode: PathErrCodeBlank,
		},
		{
			name:     "not_absolute_space_string",
			path:     " ",
			wantErr:  true,
			wantCode: PathErrCodeNotAbsolute,
		},

		// --- ends with slash ---
		{
			name:     "ends_with_slash_simple",
			path:     "/foo/",
			wantErr:  true,
			wantCode: PathErrCodePathEndsWithSlash,
		},
		{
			name:     "ends_with_slash_nested",
			path:     "/foo/bar/",
			wantErr:  true,
			wantCode: PathErrCodePathEndsWithSlash,
		},

		// --- duplicate slashes ---
		{
			name:     "duplicate_slash_middle",
			path:     "/pics//cats",
			wantErr:  true,
			wantCode: PathErrCodeDuplicateSlash,
		},
		{
			name:     "duplicate_slash_near_start",
			path:     "//foo",
			wantErr:  true,
			wantCode: PathErrCodeDuplicateSlash,
		},

		// --- dot segments ---
		{
			name:     "contains_dot_segment",
			path:     "/foo/./bar",
			wantErr:  true,
			wantCode: PathErrCodePathContainsDots,
		},
		{
			name:     "contains_dotdot_segment",
			path:     "/foo/../bar",
			wantErr:  true,
			wantCode: PathErrCodePathContainsDots,
		},
		{
			name:     "contains_dot_segment_at_end",
			path:     "/foo/.",
			wantErr:  true,
			wantCode: PathErrCodePathContainsDots,
		},
		{
			name:     "contains_dotdot_segment_at_end",
			path:     "/foo/..",
			wantErr:  true,
			wantCode: PathErrCodePathContainsDots,
		},
		{
			name:     "dot_segment_immediately_after_root",
			path:     "/./foo",
			wantErr:  true,
			wantCode: PathErrCodePathContainsDots,
		},
		{
			name:     "dotdot_segment_immediately_after_root",
			path:     "/../foo",
			wantErr:  true,
			wantCode: PathErrCodePathContainsDots,
		},

		// --- precedence / first error encountered ---
		{
			// Both "duplicate slash" and "ends with slash" could be argued,
			// but the implementation returns PathEndsWithSlash when it sees
			// a slash at len(path)-1.
			name:     "error_precedence_double_slash_at_end_returns_ends_with_slash",
			path:     "/foo//",
			wantErr:  true,
			wantCode: PathErrCodePathEndsWithSlash,
		},
		{
			// Contains null byte and is not absolute. Implementation checks
			// UTF-8 + null byte before absolute, so null byte wins.
			name:     "error_precedence_null_byte_before_not_absolute",
			path:     "foo\x00bar",
			wantErr:  true,
			wantCode: PathErrCodeNullByte,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ValidatePath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				pe, ok := err.(*PathError)
				if !ok {
					t.Fatalf("expected *PathError, got %T (%v)", err, err)
				}
				if pe.Code != tt.wantCode {
					t.Fatalf("expected code %q, got %q (err=%v)", tt.wantCode, pe.Code, err)
				}
				if pe.Path != tt.path {
					t.Fatalf("expected path %q on error, got %q", tt.path, pe.Path)
				}
			} else {
				if err != nil {
					t.Fatalf("expected nil error, got %T (%v)", err, err)
				}
			}
		})
	}
}
