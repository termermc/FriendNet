package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"time"

	"friendnet.org/client/cert"
	"friendnet.org/protocol"
	"github.com/quic-go/quic-go"
)

// ConnectWithCertStore attempts to connect to the specified address, verifying its certificate using the specified cert.Store for TOFU.
//
// Errors:
//   - protocol.ErrNoServerCerts: Server returned no certs.
//   - protocol.ErrServerCertNotValidNow: Server certificate is not valid at the current time.
//   - protocol.CertMismatchError: Server returned a certificate that is different from the one associated with the hostname in the cert.Store.
func ConnectWithCertStore(ctx context.Context, certStore cert.Store, address string) (protocol.ProtoConn, error) {
	hostname, _, parseErr := net.SplitHostPort(address)
	if parseErr != nil {
		return nil, fmt.Errorf(`failed to parse address %q in ConnectWithCertStore: %w`, address, parseErr)
	}
	hostname = cert.NormalizeHostname(hostname)

	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{protocol.AlpnProtoName},
		ServerName:         hostname,
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return protocol.ErrNoServerCerts
			}

			leafDer := rawCerts[0]
			leaf, err := x509.ParseCertificate(leafDer)
			if err != nil {
				return fmt.Errorf("failed to parse server certificate: %w", err)
			}

			now := time.Now()
			if now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
				return protocol.ErrServerCertNotValidNow
			}

			storedDer, err := certStore.GetDer(ctx, hostname)
			if err != nil {
				return fmt.Errorf("failed to look up stored certificate for %q: %w", hostname, err)
			}

			if len(storedDer) == 0 {
				if err := certStore.PutDer(ctx, hostname, leafDer); err != nil {
					return fmt.Errorf("failed to store certificate for %q: %w", hostname, err)
				}
				return nil
			}

			if !bytes.Equal(storedDer, leafDer) {
				return protocol.CertMismatchError{Host: hostname}
			}

			return nil
		},
	}

	qConn, err := quic.DialAddr(ctx, address, tlsCfg, &quic.Config{
		KeepAlivePeriod: protocol.DefaultKeepAlivePeriod,
	})
	if err != nil {
		return nil, fmt.Errorf(`failed to dial QUIC %q: %w`, address, err)
	}

	return protocol.ToProtoConn(qConn), nil
}
