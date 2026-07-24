package stun

import (
	"testing"
)

func TestGetAddrPortForSocket(t *testing.T) {
	sock := mkSockWithDeadline()
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

	sock := mkSockWithDeadline()
	defer func() {
		_ = sock.Close()
	}()

	addrPort, err := RaceStunServers(sock, servers)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Public address: %s", addrPort.String())
}
