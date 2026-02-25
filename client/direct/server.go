package direct

import (
	"crypto/tls"
	"net/netip"

	"friendnet.org/protocol"
)

// Server is a direct connect server that accepts new direct connections from clients.
// It does not perform any authentication, it simply sends the connections along with
// their handshake messages to the appropriate Partition.
type Server struct {
	m *Manager

	// The server's address and port.
	// Do not update.
	AddrPort netip.AddrPort

	listener protocol.ProtoListener
}

// NewServer creates a new direct connect server.
// It returns an error if a listener could not be created.
// Once created, it listens and handles incoming connections on its own.
func NewServer(
	m *Manager,
	addrPort netip.AddrPort,
	cert tls.Certificate,
) (*Server, error) {
	listener, err := protocol.NewQuicProtoListener(addrPort.String(), &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{protocol.AlpnProtoName},
	})
	if err != nil {
		return nil, err
	}

	s := &Server{
		m:        m,
		AddrPort: addrPort,
		listener: listener,
	}

	// TODO Launch goroutine to handle incoming connections.

	return s, nil
}

// Close closes the server.
func (s *Server) Close() error {
	return s.listener.Close()
}

func (s *Server) loop() error {
	for {

	}
}
