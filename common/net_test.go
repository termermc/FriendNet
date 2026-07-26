package common

import "testing"

func TestResolveUdpAddrFromStr(t *testing.T) {
	addr, err := ResolveUdpAddrFromStr("love-ipv6:0")
	if err != nil {
		t.Fatal(err)
	}

	println(addr.String())
}
