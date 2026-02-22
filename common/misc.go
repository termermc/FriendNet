package common

import (
	"crypto/rand"
	"encoding/base64"
)

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

// StrOrNil returns a pointer to the specified string if it is not empty, otherwise returns nil.
func StrOrNil(str string) *string {
	if str == "" {
		return nil
	}
	return &str
}

// RandomB64UrlStr returns a random base64 URL string based on random bytes of the specified length.
// It uses raw encoding, so it does not include padding.
func RandomB64UrlStr(byteLen int) string {
	buf := make([]byte, byteLen)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}
