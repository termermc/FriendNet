package upnp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// ForwardedIP reports a successful forwarding action.
type ForwardedIP struct {
	IP       net.IP
	Port     uint16
	Version  IPVersion
	DeviceID string
}

// ForwardFailure reports a failure that did not stop the overall process.
type ForwardFailure struct {
	IP       net.IP
	Port     uint16
	Version  IPVersion
	DeviceID string
	Err      error
}

// ForwardResult aggregates successes and failures.
type ForwardResult struct {
	Forwarded []ForwardedIP
	Failures  []ForwardFailure
}

// ForwardUDPForPublicIPs discovers IGDs, finds public IPv4/IPv6 addresses, and
// forwards UDP on the given port for each address found. Failures are recorded
// in the result but do not halt the process.
func ForwardUDPForPublicIPs(ctx context.Context, port uint16, description string, duration time.Duration, discoverTimeout time.Duration) ForwardResult {
	result := ForwardResult{}

	if port <= 0 || port > 65535 {
		result.Failures = append(result.Failures, ForwardFailure{
			Port:    port,
			Version: IPvAny,
			Err:     fmt.Errorf("invalid port %d", port),
		})
		return result
	}

	if discoverTimeout <= 0 {
		discoverTimeout = 2 * time.Second
	}

	devices := Discover(ctx, 0, discoverTimeout)
	if len(devices) == 0 {
		result.Failures = append(result.Failures, ForwardFailure{
			Port:    port,
			Version: IPvAny,
			Err:     errors.New("no UPnP devices discovered"),
		})
		return result
	}

	seen := make(map[string]struct{})

	for _, dev := range devices {
		deviceID := dev.ID()

		if dev.SupportsIPVersion(IPv4Only) {
			ip, err := dev.GetExternalIPv4Address(ctx)
			switch {
			case err != nil:
				result.Failures = append(result.Failures, ForwardFailure{
					Port:     port,
					Version:  IPv4Only,
					DeviceID: deviceID,
					Err:      err,
				})
			case !isPublicIPv4(ip):
				result.Failures = append(result.Failures, ForwardFailure{
					IP:       ip,
					Port:     port,
					Version:  IPv4Only,
					DeviceID: deviceID,
					Err:      fmt.Errorf("external IPv4 not public: %v", ip),
				})
			default:
				key := fmt.Sprintf("v4|%s|%d|%s", ip.String(), port, deviceID)
				if _, ok := seen[key]; !ok {
					if _, err := dev.AddPortMapping(ctx, UDP, int(port), int(port), description, duration); err != nil {
						result.Failures = append(result.Failures, ForwardFailure{
							IP:       ip,
							Port:     port,
							Version:  IPv4Only,
							DeviceID: deviceID,
							Err:      err,
						})
					} else {
						result.Forwarded = append(result.Forwarded, ForwardedIP{
							IP:       ip,
							Port:     port,
							Version:  IPv4Only,
							DeviceID: deviceID,
						})
						seen[key] = struct{}{}
					}
				}
			}
		}

		if !dev.SupportsIPVersion(IPv6Only) {
			continue
		}

		igd, ok := dev.(*IGDService)
		if !ok {
			result.Failures = append(result.Failures, ForwardFailure{
				Port:     port,
				Version:  IPv6Only,
				DeviceID: deviceID,
				Err:      errors.New("unexpected device type (IPv6)"),
			})
			continue
		}

		if igd.Interface == nil {
			result.Failures = append(result.Failures, ForwardFailure{
				Port:     port,
				Version:  IPv6Only,
				DeviceID: deviceID,
				Err:      errors.New("no interface for IPv6 pinholing"),
			})
			continue
		}

		ipv6Addrs, err := publicIPv6Addrs(igd.Interface)
		if err != nil {
			result.Failures = append(result.Failures, ForwardFailure{
				Port:     port,
				Version:  IPv6Only,
				DeviceID: deviceID,
				Err:      err,
			})
			continue
		}

		if len(ipv6Addrs) == 0 {
			result.Failures = append(result.Failures, ForwardFailure{
				Port:     port,
				Version:  IPv6Only,
				DeviceID: deviceID,
				Err:      errors.New("no public IPv6 addresses on interface"),
			})
			continue
		}

		for _, ip := range ipv6Addrs {
			key := fmt.Sprintf("v6|%s|%d|%s", ip.String(), port, deviceID)
			if _, ok := seen[key]; ok {
				continue
			}
			_, err := dev.AddPinhole(ctx, UDP, Address{IP: ip, Port: int(port)}, duration)
			if err != nil {
				result.Failures = append(result.Failures, ForwardFailure{
					IP:       ip,
					Port:     port,
					Version:  IPv6Only,
					DeviceID: deviceID,
					Err:      err,
				})
				continue
			}
			result.Forwarded = append(result.Forwarded, ForwardedIP{
				IP:       ip,
				Port:     port,
				Version:  IPv6Only,
				DeviceID: deviceID,
			})
			seen[key] = struct{}{}
		}
	}

	return result
}

func publicIPv6Addrs(intf *net.Interface) ([]net.IP, error) {
	addrs, err := interfaceAddrsByInterface(intf)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var result []net.IP
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			continue
		}
		if !isPublicIPv6(ip) {
			continue
		}
		key := ip.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, ip)
	}
	return result, nil
}

func isPublicIPv4(ip net.IP) bool {
	if ip == nil {
		return false
	}
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	return v4.IsGlobalUnicast() && !v4.IsPrivate() && !v4.IsLoopback()
}

func isPublicIPv6(ip net.IP) bool {
	if ip == nil || ip.To4() != nil {
		return false
	}
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback()
}
