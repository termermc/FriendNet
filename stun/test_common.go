package stun

import (
	"net"
	"time"
)

func mkSockPortNet(port uint16, network string) *net.UDPConn {
	sock, err := net.ListenUDP(network, &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: int(port),
	})
	if err != nil {
		panic(err)
	}
	return sock
}

func mkSockPort(port uint16) *net.UDPConn {
	return mkSockPortNet(port, "udp")
}

func mkSock() *net.UDPConn {
	return mkSockPort(0)
}

func mkSockWithDeadline() *net.UDPConn {
	sock := mkSock()
	err := sock.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		panic(err)
	}
	return sock
}
