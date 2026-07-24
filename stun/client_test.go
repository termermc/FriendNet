package stun

import (
	"net"
	"testing"
	"time"
)

func mkSock() *net.UDPConn {
	sock, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		panic(err)
	}
	err = sock.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		panic(err)
	}
	return sock
}

func TestGetAddrPortForSocket(t *testing.T) {
	sock := mkSock()
	defer func() {
		_ = sock.Close()
	}()

	addrPort, err := GetPublicAddrPort(sock, "stun.l.google.com:19302")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Public address: %s", addrPort.String())
}

func TestRaceStunServers(t *testing.T) {
	servers := []string{
		"stun.l.google.com:19302",
		"stun1.l.google.com:19302",
		"stun2.l.google.com:19302",
		"stun.cloudflare.com:3478",
	}

	sock := mkSock()
	defer func() {
		_ = sock.Close()
	}()

	addrPort, err := RaceStunServers(sock, servers)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Public address: %s", addrPort.String())
}
