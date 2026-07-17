package common

import (
	"crypto/rand"
	"encoding/base64"
	"net"
	"time"
)

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

// TryTcpHost tries to connect to a TCP host.
// It returns true if it succeeded, false if it was rejected or timed out.
func TryTcpHost(host string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", host, timeout)
	defer func() {
		_ = conn.Close()
	}()
	return err == nil
}
