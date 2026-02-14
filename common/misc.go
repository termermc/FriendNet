package common

// StrPtr returns a pointer to the specified string.
func StrPtr(str string) *string {
	return &str
}

// StrPtrOr dereferences a string pointer or returns a default value if it is nil.
func StrPtrOr(str *string, or string) string {
	if str == nil {
		return or
	}
	return *str
}
