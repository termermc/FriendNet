package common

import (
	"net"
	"net/netip"
	"strings"
)

// GetUnicastIpsFromInterfaces returns all unicast IP addresses of the system's network interfaces.
// Omits IPv6 link-local addresses.
// Errors are ignored, only valid IPs are returned.
func GetUnicastIpsFromInterfaces(allowLoopback bool, allowPrivate bool) []netip.Addr {
	ifaces, ifaceErr := net.Interfaces()
	if ifaceErr != nil {
		return nil
	}

	addrs := make([]netip.Addr, 0)

	for _, iface := range ifaces {
		addrsRaw, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}

		for _, addrRaw := range addrsRaw {
			oldStr := addrRaw.String()
			if slashIdx := strings.IndexRune(oldStr, '/'); slashIdx != -1 {
				oldStr = oldStr[:slashIdx]
			}

			var err error
			var addr netip.Addr
			if colonIdx := strings.IndexRune(oldStr, ':'); colonIdx != -1 {
				// First try to parse as IPv6.
				addr, err = netip.ParseAddr(oldStr)
				if err != nil {
					// Not IPv6, try to parse without port.
					addr, err = netip.ParseAddr(oldStr[:colonIdx])
					if err != nil {
						continue
					}

					goto check
				}

				goto check
			}

			addr, err = netip.ParseAddr(oldStr)
			if err != nil {
				continue
			}

		check:
			if addr.IsMulticast() ||
				addr.IsLinkLocalUnicast() {
				continue
			}
			if addr.IsLoopback() && !allowLoopback {
				continue
			}
			if addr.IsPrivate() && !allowPrivate {
				continue
			}

			addrs = append(addrs, addr)
		}
	}

	return addrs
}
