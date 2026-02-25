package direct

import (
	"crypto/tls"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"friendnet.org/client/storage"
)

const SettingDisable = "direct_server_disable"
const SettingKeyPem = "direct_server_key_pem"
const SettingCertPem = "direct_server_cert_pem"
const SettingAddrs = "direct_server_addresses"
const SettingDefaultPort = "direct_server_default_port"
const SettingDisableProbeIpsToAdvertise = "direct_server_disable_probe_ips_to_advertise"
const SettingAdvertisePrivateIps = "direct_server_advertise_private_ips"
const SettingDisablePublicIpDiscovery = "direct_server_disable_public_ip_discovery"
const SettingDisableUPnP = "direct_server_disable_upnp"
const SettingUpnpTimeoutMs = "direct_server_upnp_timeout_ms"

const DefaultDirectPort = 20048

func ConfigFromSettings(store *storage.Storage) (*Config, error) {
	// TODO
	return nil, nil
}

var anyIpv4 = netip.MustParseAddr("0.0.0.0")
var anyIpv6 = netip.MustParseAddr("::")
var anyIpv4Port0 = netip.AddrPortFrom(anyIpv4, 0)
var anyIpv6Port0 = netip.AddrPortFrom(anyIpv6, 0)

// Config is the configuration for the direct connection manager.
type Config struct {
	// Whether to disable direct connections entirely.
	// If true, all other fields will be ignored.
	Disable bool

	// The certificate to use for the direct connect server.
	// Required if Disable is not true.
	Cert tls.Certificate

	// The initial addresses to listen on.
	// Each address must be in the format `IPv4:PORT`, `[IPv6]:PORT`, `IP` (IPv6 without port does not need brackets).
	// Must specify at least one.
	// Can use addresses like `0.0.0.0` and `[::]` (with or without port) to listen on all interfaces.
	// Any addresses without a port will have a port assigned to them.
	Addresses []string

	// The default port to use for addresses that do not have a specified port.
	// It will also be the port opened by UPnP.
	//
	// If 0, a random port will be used.
	// Using a random port is not recommended because it will cause port churn across reconnects.
	// Keeping the port consistent across reconnects is useful because external clients will be able to more reliably reach the client.
	//
	// A port >= 1024 is recommended to avoid permission denied errors from the OS.
	DefaultPort uint16

	// Whether to disable probing the machine for IPs to advertise.
	// It does not advertise private IPs unless AdvertisePrivateIps is true.
	DisableProbeIpsToAdvertise bool

	// Whether to advertise private IPs (like 192.168.0.0/16, 172.16.0.0/12, 10.0.0.0/8).
	// Has no effect if ProbeIpsToAdvertise is false.
	// This only makes sense when multiple clients are on the same LAN or VPN.
	AdvertisePrivateIps bool

	// Whether to disable public IP discovery via the server.
	// By default, the client will try to discover its public IP by asking the server for it.
	DisablePublicIpDiscovery bool

	// Whether to disable UPnP.
	DisableUPnP bool

	// The timeout for using UPnP.
	// Defaults to 10 seconds.
	// Has no effect if DisableUPnP is true.
	UpnpTimeout time.Duration
}

// Validate validates a Config and returns its parsed IP-port values.
// All returned values are deduplicated (taking into account addresses like "0.0.0.0").
// The addresses without ports will have 0 as port.
func (cfg Config) Validate() (addrs map[netip.AddrPort]struct{}, err error) {
	if cfg.Disable {
		return nil, nil
	}

	if cfg.Cert.Certificate == nil {
		return nil, fmt.Errorf(`missing Cert in DirectConfig`)
	}

	anyIpv4Ports := make(map[uint16]struct{})
	anyIpv6Ports := make(map[uint16]struct{})

	// Validate address formats.
	addrs = make(map[netip.AddrPort]struct{}, len(cfg.Addresses))
	for _, addrStr := range cfg.Addresses {
		if strings.ContainsRune(addrStr, ':') {
			val, parseErr := netip.ParseAddrPort(addrStr)
			if parseErr != nil {
				return nil, fmt.Errorf(`invalid address %q in DirectConfig: %w`, addrStr, parseErr)
			}
			addrs[val] = struct{}{}

			if val.Addr() == anyIpv4 {
				anyIpv4Ports[val.Port()] = struct{}{}
			} else if val.Addr() == anyIpv6 {
				anyIpv6Ports[val.Port()] = struct{}{}
			}
		} else {
			addr, parseErr := netip.ParseAddr(addrStr)
			if parseErr != nil {
				return nil, fmt.Errorf(`invalid address %q in DirectConfig: %w`, addrStr, parseErr)
			}
			addrs[netip.AddrPortFrom(addr, 0)] = struct{}{}
		}
	}

	// If there are any `0.0.0.0` or `::` addresses, remove duplicate entries that listen on specific addresses.
	_, hasAnyIpv4 := addrs[anyIpv4Port0]
	_, hasAnyIpv6 := addrs[anyIpv6Port0]
	if hasAnyIpv4 || hasAnyIpv6 || len(anyIpv4Ports) > 0 || len(anyIpv6Ports) > 0 {
		for addrPort := range addrs {
			addr := addrPort.Addr()
			port := addrPort.Port()

			// If this is "0.0.0.0" or "::" (with or without port), it does not need to be removed.
			if addr == anyIpv4 || addr == anyIpv6 {
				continue
			}

			if addr.Is6() {
				if port == 0 && hasAnyIpv6 {
					// This is a specific address without a port, like "::1".
					// There is a "::" present, so this can be removed.
					delete(addrs, addrPort)
					continue
				}

				_, hasAnyPort := anyIpv6Ports[port]
				if !hasAnyPort {
					// This is a specific address with a specific port, like "[::1]:20048".
					// There is an equivalent address like "[::]:20048" present, so this can be removed.
					delete(addrs, addrPort)
					continue
				}
			} else {
				if port == 0 && hasAnyIpv4 {
					// This is a specific address without a port, like "127.0.0.1".
					// There is a "0.0.0.0" present, so this can be removed.
					delete(addrs, addrPort)
					continue
				}

				_, hasAnyPort := anyIpv4Ports[port]
				if !hasAnyPort {
					// This is a specific address with a specific port, like "127.0.0.1:20048".
					// There is an equivalent address like "0.0.0.0:20048" present, so this can be removed.
					delete(addrs, addrPort)
					continue
				}
			}
		}
	}

	return addrs, nil
}
