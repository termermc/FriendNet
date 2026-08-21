package stun

import (
	"context"
	"encoding/binary"
	"errors"
	"net/netip"
)

// parseStunBindingRequest extracts transaction ID for a binding request.
func parseStunBindingRequest(b []byte) (tid [12]byte, ok bool) {
	// STUN message header is 20 bytes:
	// 0-1: type
	// 2-3: length
	// 4-7: magic cookie
	// 8-19: transaction id (12 bytes)
	if len(b) < 20 {
		return tid, false
	}

	msgType := binary.BigEndian.Uint16(b[0:2])
	if msgType != stunBindingRequest {
		return tid, false
	}

	cookie := binary.BigEndian.Uint32(b[4:8])
	if cookie != stunMagicCookie {
		return tid, false
	}

	copy(tid[:], b[8:20])
	return tid, true
}

func buildStunBindingResponseXORMapped(tid [12]byte, src netip.AddrPort) ([]byte, error) {
	xport := src.Port() ^ uint16(stunMagicCookie>>16)

	if src.Addr().Is4() {
		ip4 := src.Addr().As4() // [4]byte

		xaddr := binary.BigEndian.Uint32(ip4[:]) ^ stunMagicCookie

		attrLen := uint16(8)
		out := make([]byte, 20+4+8)

		binary.BigEndian.PutUint16(out[0:2], stunBindingResponse)
		binary.BigEndian.PutUint16(out[2:4], 4+attrLen) // message length (attr header + value)
		binary.BigEndian.PutUint32(out[4:8], stunMagicCookie)
		copy(out[8:20], tid[:])

		off := 20
		binary.BigEndian.PutUint16(out[off:off+2], attrXORMappedAddress)
		binary.BigEndian.PutUint16(out[off+2:off+4], attrLen)
		off += 4

		out[off] = 0x00
		out[off+1] = 0x01
		binary.BigEndian.PutUint16(out[off+2:off+4], xport)
		binary.BigEndian.PutUint32(out[off+4:off+8], xaddr)
		return out, nil
	}

	// IPv6
	if !src.Addr().Is6() {
		return nil, errors.New("unsupported address family")
	}

	ip16 := src.Addr().As16() // [16]byte

	var xaddr [16]byte
	copy(xaddr[:], ip16[:])

	// XOR first 4 bytes with magic cookie
	cookieBytes := [4]byte{}
	binary.BigEndian.PutUint32(cookieBytes[:], stunMagicCookie)
	for i := range 4 {
		xaddr[i] ^= cookieBytes[i]
	}
	// XOR last 12 bytes with transaction id
	for i := 4; i < 16; i++ {
		xaddr[i] ^= tid[i-4]
	}

	attrLen := uint16(20)
	out := make([]byte, 20+4+20)

	binary.BigEndian.PutUint16(out[0:2], stunBindingResponse)
	binary.BigEndian.PutUint16(out[2:4], 4+attrLen) // message length (attr header + value)
	binary.BigEndian.PutUint32(out[4:8], stunMagicCookie)
	copy(out[8:20], tid[:])

	off := 20
	binary.BigEndian.PutUint16(out[off:off+2], attrXORMappedAddress)
	binary.BigEndian.PutUint16(out[off+2:off+4], attrLen)
	off += 4

	out[off] = 0x00
	out[off+1] = 0x02
	binary.BigEndian.PutUint16(out[off+2:off+4], xport)
	copy(out[off+4:off+20], xaddr[:])

	return out, nil
}

// RunServer runs a minimal server that implements a subset of STUN.
// It implements:
//   - Binding Request
//   - XOR-MAPPED-ADDRESS (IPv4 and IPv6)
//
// This function will only return if readPacket returns an error, in which case it will just return the error.
func RunServer(
	ctx context.Context,
	readPacket func(context.Context, []byte) (int, netip.AddrPort, error),
	writePacket func([]byte, netip.AddrPort) (int, error),
) error {
	buf := make([]byte, 2048)

	for {
		n, src, err := readPacket(ctx, buf)
		if err != nil {
			return err
		}

		if src.Addr().Is4In6() {
			src = netip.AddrPortFrom(src.Addr().Unmap(), src.Port())
		}

		req := buf[:n]

		tid, ok := parseStunBindingRequest(req)
		if !ok {
			// Unsupported packet.
			continue
		}

		resp, err := buildStunBindingResponseXORMapped(tid, src)
		if err != nil {
			continue
		}

		if _, err := writePacket(resp, src); err != nil {
			// Ignore packet writing errors, just try and accept another packet.
			continue
		}
	}
}
