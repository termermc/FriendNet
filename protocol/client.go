package protocol

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"time"

	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
)

type ProtoClient struct {
	conn *quic.Conn
}

// ClientCredentials are the credentials used for clients to authenticate with a server.
type ClientCredentials struct {
	// The room to connect to.
	Room string

	// The user's username.
	Username string

	// The user's password.
	Password string
}

// ClientCertStore defines an interface for storing and checking certificates for hostnames.
// It is used to facilitate SSH-style Trust On First Use for server TLS certificates.
type ClientCertStore interface {
	// Get returns the stored leaf certificate (DER) for the hostname, or nil if none exists.
	Get(host string) ([]byte, error)

	// Put stores the leaf certificate (DER) for the hostname, replacing any existing entry.
	Put(host string, certDER []byte) error
}

// ConnectTofu connects to the QUIC server at the specified address, using Trust On First Use
// to verify the server's TLS certificate.
func ConnectTofu(addr string, certStore ClientCertStore) (*quic.Conn, error) {
	if certStore == nil {
		return nil, fmt.Errorf("cert store is required")
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse address %q: %w", addr, err)
	}

	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{AlpnProtoName},
		ServerName:         host,
		InsecureSkipVerify: true,
	}

	tlsCfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return fmt.Errorf("no server certificates presented")
		}

		leafDER := rawCerts[0]
		leaf, err := x509.ParseCertificate(leafDER)
		if err != nil {
			return fmt.Errorf("failed to parse server certificate: %w", err)
		}

		now := time.Now()
		if now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
			return fmt.Errorf("server certificate is not valid at the current time")
		}

		storedDer, err := certStore.Get(host)
		if err != nil {
			return fmt.Errorf("failed to look up stored certificate for %q: %w", host, err)
		}

		if len(storedDer) == 0 {
			if err := certStore.Put(host, leafDER); err != nil {
				return fmt.Errorf("failed to store certificate for %q: %w", host, err)
			}
			return nil
		}

		if !bytes.Equal(storedDer, leafDER) {
			return fmt.Errorf("server certificate mismatch for %q", host)
		}

		return nil
	}

	return quic.DialAddr(context.Background(), addr, tlsCfg, &quic.Config{})
}

// negotiateVersion negotiates the protocol version with the server.
// Returns the server's protocol version if successful.
func negotiateVersion(conn *quic.Conn, version *pb.ProtoVersion) (*pb.ProtoVersion, error) {
	if version == nil {
		return nil, fmt.Errorf("protocol version is required")
	}

	bidi, err := OpenBidiWithMsg(conn, pb.MsgType_MSG_TYPE_VERSION, &pb.MsgVersion{
		Version: version,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = bidi.Stream.Close()
	}()

	msg, err := bidi.Read()
	if err != nil {
		return nil, err
	}

	switch msg.Type {
	case pb.MsgType_MSG_TYPE_VERSION_ACCEPTED:
		accepted := ToTyped[*pb.MsgVersionAccepted](msg)
		return accepted.Payload.Version, nil
	case pb.MsgType_MSG_TYPE_VERSION_REJECTED:
		rejected := ToTyped[*pb.MsgVersionRejected](msg)
		msgText := ""
		if rejected.Payload.Message != nil {
			msgText = ": " + *rejected.Payload.Message
		}
		return nil, fmt.Errorf("protocol version rejected: %s%s", rejected.Payload.Reason.String(), msgText)
	default:
		return nil, NewUnexpectedMsgTypeError(pb.MsgType_MSG_TYPE_VERSION_ACCEPTED, msg.Type)
	}
}

func authenticate(conn *quic.Conn, creds ClientCredentials) error {
	bidi, err := OpenBidiWithMsg(conn, pb.MsgType_MSG_TYPE_AUTHENTICATE, &pb.MsgAuthenticate{
		Room:     creds.Room,
		Username: creds.Username,
		Password: creds.Password,
	})
	if err != nil {
		return err
	}
	defer func() {
		_ = bidi.Stream.Close()
	}()

	msg, err := bidi.Read()
	if err != nil {
		return err
	}

	switch msg.Type {
	case pb.MsgType_MSG_TYPE_AUTH_ACCEPTED:
		// Accepted message is empty for now, don't need to access the payload.
		return nil
	case pb.MsgType_MSG_TYPE_AUTH_REJECTED:
		rejected := ToTyped[*pb.MsgAuthRejected](msg)
		msgText := ""
		if rejected.Payload.Message != nil {
			msgText = ": " + *rejected.Payload.Message
		}
		return fmt.Errorf("authentication rejected: %s%s", rejected.Payload.Reason.String(), msgText)
	default:
		return NewUnexpectedMsgTypeError(pb.MsgType_MSG_TYPE_AUTH_ACCEPTED, msg.Type)
	}
}

// NewClient establishes a QUIC connection, negotiates the protocol version, and authenticates.
// If authentication is successful, it returns a new, fully authenticated client.
func NewClient(addr string, creds ClientCredentials, certStore ClientCertStore) (*ProtoClient, error) {
	conn, err := ConnectTofu(addr, certStore)
	if err != nil {
		return nil, err
	}

	_, err = negotiateVersion(conn, CurrentProtocolVersion)
	if err != nil {
		_ = conn.CloseWithError(0, "version negotiation failed")
		return nil, err
	}

	if err := authenticate(conn, creds); err != nil {
		_ = conn.CloseWithError(0, "authentication failed")
		return nil, err
	}

	return &ProtoClient{conn: conn}, nil
}
