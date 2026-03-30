//package main
//
//import (
//	"log"
//	"time"
//
//	"github.com/go-i2p/onramp"
//	"github.com/go-i2p/onramp/hybrid2"
//)
//
//func main() {
//	// Create a hybrid session using the garlic integration helper
//	// This connects to the local SAM bridge and creates I2P tunnels
//	// onramp.SAM_ADDR defaults to "127.0.0.1:7656"
//	integration, err := hybrid2.NewGarlicIntegration(onramp.SAM_ADDR, "my-hybrid")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer func() {
//		_ = integration.Close()
//	}()
//
//	// Get a standard net.PacketConn for UDP-like operations
//	conn := integration.PacketConn()
//
//	// Set read deadline for timeout support
//	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
//
//	// Use like any other PacketConn
//	buf := make([]byte, 4096)
//	n, addr, err := conn.ReadFrom(buf)
//	if err != nil {
//		log.Printf("Read error: %v", err)
//		return
//	}
//	log.Printf("Received %d bytes from %s", n, addr)
//}

package main

import (
	"log"

	"github.com/go-i2p/onramp"
)

func main() {
	garlic := &onramp.Garlic{}
	defer garlic.Close()
	listener, err := garlic.Listen()
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	// TODO Use release instead of this commit version.
	// We can use this function to get a net.PacketConn
	// TODO Figure out how to connect and get a net.PacketConn.
	garlic.ListenPacket()
}
