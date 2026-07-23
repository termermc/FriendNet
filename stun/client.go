package stun

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
)

const stunBindingRequest uint16 = 0x0001
const stunMagicCookie = 0x2112A442

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

func decodeAddrXORMapped(response []byte) (ip net.IP, port int, ok bool) {
	if len(response) < 20 {
		return nil, 0, false
	}
	cookie := binary.BigEndian.Uint32(response[4:8])

	off := 20
	for off+4 <= len(response) {
		attrType := binary.BigEndian.Uint16(response[off : off+2])
		attrLen := binary.BigEndian.Uint16(response[off+2 : off+4])
		off += 4

		aligned := (int(attrLen) + 3) &^ 3
		if off+int(attrLen) > len(response) {
			return nil, 0, false
		}

		if attrType == 0x0020 && attrLen == 8 {
			v := response[off : off+8]
			family := v[1]
			if family != 0x01 {
				return nil, 0, false
			}

			xport := binary.BigEndian.Uint16(v[2:4])
			port = int(xport ^ uint16(cookie>>16))

			xaddr := binary.BigEndian.Uint32(v[4:8])
			addrX := xaddr ^ (cookie & 0xffffffff)

			ip = make(net.IP, 4)
			binary.BigEndian.PutUint32(ip, addrX)
			return ip, port, true
		}

		off += aligned
	}
	return nil, 0, false
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

// GetPublicAddrPort gets the public address and port reported by the STUN server.
// This function does not set the socket read deadline.
func GetPublicAddrPort(sock *net.UDPConn, stunServerAddr string) (*netip.AddrPort, error) {
	raw, err := queryStun(sock, stunServerAddr)
	if err != nil {
		return nil, err
	}

	ip, port, ok := decodeAddrXORMapped(raw)
	if !ok {
		return nil, err
	}

	fmtAddr := fmt.Sprintf("%s:%d", ip.String(), port)
	addrPort, err := netip.ParseAddrPort(fmtAddr)
	if err != nil {
		return nil, err
	}

	return &addrPort, nil
}
