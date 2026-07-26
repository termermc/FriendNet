package stun

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
)

func buildStunBindingRequest(tid [12]byte) []byte {
	b := make([]byte, 20)
	binary.BigEndian.PutUint16(b[0:2], stunBindingRequest)
	binary.BigEndian.PutUint16(b[2:4], 0)
	binary.BigEndian.PutUint32(b[4:8], stunMagicCookie)
	copy(b[8:20], tid[:])
	return b
}

func randomTid() [12]byte {
	var tid [12]byte
	_, _ = rand.Read(tid[:])
	return tid
}

func decodeAddrXORMapped(response []byte) (ap netip.AddrPort, ok bool) {
	if len(response) < 20 {
		return netip.AddrPort{}, false
	}

	cookie := binary.BigEndian.Uint32(response[4:8])

	off := 20
	for off+4 <= len(response) {
		attrType := binary.BigEndian.Uint16(response[off : off+2])
		attrLen := binary.BigEndian.Uint16(response[off+2 : off+4])
		off += 4

		// attributes are padded to 32-bit boundaries
		aligned := (int(attrLen) + 3) &^ 3
		if off+int(attrLen) > len(response) {
			return netip.AddrPort{}, false
		}
		if off+aligned > len(response) {
			return netip.AddrPort{}, false
		}

		// XOR-MAPPED-ADDRESS: STUN attribute type 0x0020
		if attrType == attrXORMappedAddress {
			switch attrLen {
			case 8: // IPv4
				// value: 1 byte reserved, 1 byte family(0x01), 2 bytes xport, 4 bytes xaddr
				v := response[off : off+8]
				if v[1] != 0x01 {
					return netip.AddrPort{}, false
				}

				xport := binary.BigEndian.Uint16(v[2:4])
				port := xport ^ uint16(cookie>>16)

				xaddr := binary.BigEndian.Uint32(v[4:8])
				ip4 := xaddr ^ cookie

				addr := netip.AddrFrom4([4]byte{
					byte(ip4 >> 24), byte(ip4 >> 16), byte(ip4 >> 8), byte(ip4),
				})
				return netip.AddrPortFrom(addr, port), true

			case 20: // IPv6
				// value: 1 byte reserved, 1 byte family(0x02), 2 bytes xport, 16 bytes xaddr
				v := response[off : off+20]
				if v[1] != 0x02 {
					return netip.AddrPort{}, false
				}

				xport := binary.BigEndian.Uint16(v[2:4])
				port := xport ^ uint16(cookie>>16)

				// x-addr[0..15] XOR rules:
				// - first 4 bytes XOR magic cookie
				// - last 12 bytes XOR transaction-id
				var xaddr [16]byte
				copy(xaddr[:], v[4:20])

				var addr [16]byte
				// first 32 bits
				addr[0] = xaddr[0] ^ byte(cookie>>24)
				addr[1] = xaddr[1] ^ byte(cookie>>16)
				addr[2] = xaddr[2] ^ byte(cookie>>8)
				addr[3] = xaddr[3] ^ byte(cookie)

				// last 96 bits XOR with transaction-id (bytes 8..19 of STUN header)
				tid := response[8:20] // must exist since len>=20
				for i := 0; i < 12; i++ {
					addr[4+i] = xaddr[4+i] ^ tid[i]
				}

				parsed := netip.AddrFrom16(addr)
				return netip.AddrPortFrom(parsed, port), true
			}
		}

		off += aligned
	}

	return netip.AddrPort{}, false
}

func queryStun(conn *net.UDPConn, serverHostPort string) ([]byte, error) {
	server, err := net.ResolveUDPAddr("udp", serverHostPort)
	if err != nil {
		return nil, err
	}
	msg := buildStunBindingRequest(randomTid())

	_, err = conn.WriteToUDP(msg, server)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 1500)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// ErrBadServerResponse is returned when the STUN server returned an invalid response.
var ErrBadServerResponse = errors.New("STUN server returned invalid response")

// GetPublicAddrPort gets the public address and port reported by the STUN server.
// This function does not set the socket read deadline.
// The caller should set a read deadline to avoid blocking forever.
func GetPublicAddrPort(sock *net.UDPConn, stunServerAddr string) (addrPort netip.AddrPort, err error) {
	raw, err := queryStun(sock, stunServerAddr)
	if err != nil {
		return addrPort, err
	}

	addrPort, ok := decodeAddrXORMapped(raw)
	if !ok {
		return addrPort, ErrBadServerResponse
	}

	return addrPort, nil
}

var ErrNoServers = errors.New("no STUN servers")

// RaceStunServers tries to queries STUN servers in parallel and returns the first successful response.
// This function does not set the socket read deadline.
// The caller should set a read deadline to avoid blocking forever or leaking goroutines that are blocked forever.
func RaceStunServers(sock *net.UDPConn, stunServerAddrs []string) (addrPort netip.AddrPort, err error) {
	// TODO Does this actually work with multiple STUN servers properly?
	// Verify that it works properly.
	// Also, make a version that waits until a timeout or the first IPv4.
	// It will fall back to IPv6 if there are no IPv4 addresses.

	if len(stunServerAddrs) == 0 {
		return addrPort, ErrNoServers
	}

	type addrPortRes struct {
		AddrPort netip.AddrPort
		Err      error
	}
	resAddrs := make(chan addrPortRes, len(stunServerAddrs))

	for _, addr := range stunServerAddrs {
		go func(addr string) {
			ap, apErr := GetPublicAddrPort(sock, addr)
			resAddrs <- addrPortRes{AddrPort: ap, Err: apErr}
		}(addr)
	}

	var errs []error
	for i := 0; i < len(stunServerAddrs); i++ {
		res := <-resAddrs
		if res.Err != nil {
			errs = append(errs, res.Err)
			continue
		}

		return res.AddrPort, nil
	}

	return addrPort, fmt.Errorf(`failed to resolve any addresses from STUN servers: %w`, errors.Join(errs...))
}
