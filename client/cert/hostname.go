package cert

import (
	"net"
	"strings"

	"golang.org/x/net/idna"
)

// NormalizeHostname normalizes a hostname or IP address string.
// Rules:
//   - Lowercase everything
//   - Handle IPv4 and IPv6
//   - Accept bare IPv6 without brackets (and normalize/compress it)
//   - Non-IP hostnames are converted to ASCII (punycode) via IDNA
//   - On invalid input, returns the original string
//
// Notes:
//   - This function expects a host only (no port, no scheme, no path).
//   - Zone-scoped IPv6 (e.g. "fe80::1%en0") is preserved but lowercased.
//   - If the input has surrounding brackets (e.g. "[::1]"), they are removed.
func NormalizeHostname(hostname string) string {
	orig := hostname
	s := strings.TrimSpace(hostname)
	if s == "" {
		return orig
	}

	// Lowercase early to satisfy "lowercase everything".
	s = strings.ToLower(s)

	// If the user passed bracketed IPv6, strip brackets.
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") && len(s) >= 2 {
		s = strings.TrimPrefix(strings.TrimSuffix(s, "]"), "[")
		if s == "" {
			return orig
		}
	}

	// Try IP parsing first (handles IPv4, IPv6, and IPv6 with zone).
	if ip := net.ParseIP(s); ip != nil {
		// Normalize representation:
		// - IPv4 as dotted quad
		// - IPv6 in compressed canonical form
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
		return ip.String()
	}

	// If it looks like an IPv6 zone address, normalize the IP part and keep the zone.
	// net.ParseIP doesn't accept zones; net.ParseIP("fe80::1%en0") returns nil.
	if i := strings.LastIndexByte(s, '%'); i > 0 && i < len(s)-1 {
		ipPart := s[:i]
		zone := s[i+1:] // already lowercased
		if ip := net.ParseIP(ipPart); ip != nil && ip.To4() == nil {
			return ip.String() + "%" + zone
		}
	}

	// Otherwise treat as a DNS hostname: IDNA (punycode) to ASCII.
	// We keep it conservative and "graceful": on any failure, return original.
	ascii, err := idna.Lookup.ToASCII(s)
	if err != nil || ascii == "" {
		return orig
	}

	ascii = strings.ToLower(ascii)

	// Optional light cleanup:
	// - remove a trailing dot (FQDN) if present, since many callers want a "host" not "fqdn".
	// If you'd rather preserve it, delete this block.
	ascii = strings.TrimSuffix(ascii, ".")

	if ascii == "" {
		return orig
	}
	return ascii
}
