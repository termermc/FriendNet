package stun

import "net/netip"

// This function will respond with
func parse(request []byte, address netip.AddrPort) {

}

// TODO Implement a subset of STUN features, the minimum required for client.go to talk to it.
// The constructor for the server will take a read() function instead of a UDP socket to allow for multiplexing with
// the server's main QUIC listener.
// We also need a field in the server to override the public address of the server for the purpose of STUN server
// listing.
// Right now, our best bet is to try and resolve the public IPv4 of the machine that's being listened to. If no public
// address is specified in the server config, try to guess the server's public IP based on the bind address. If the bind
// address is 0.0.0.0, try to find the server's public IP from its interfaces. If it had to guess its public address,
// print a warning in the console that the server operator should specify the public address in the config.
