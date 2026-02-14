//go:build old

package protocol

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
)

// ErrClientNotConnected is returned from functions when the client it depends on is not connected.
var ErrClientNotConnected = errors.New("client not connected")

type ProtoClient struct {
	conn *quic.Conn

	// S2cOnPing handles incoming MSG_TYPE_PING messages.
	OnPing PingHandler
	// OnGetDirFiles handles incoming MSG_TYPE_GET_DIR_FILES messages.
	OnGetDirFiles GetDirFilesHandler
	// OnGetFileMeta handles incoming MSG_TYPE_GET_FILE_META messages.
	OnGetFileMeta GetFileMetaHandler
	// OnGetFile handles incoming MSG_TYPE_GET_FILE messages.
	OnGetFile GetFileHandler
}

// PingHandler handles an incoming ping request.
// Implementations should write a response (typically MSG_TYPE_PONG) before returning.
type PingHandler func(ctx context.Context, client *ProtoClient, bidi ProtoBidi, msg *pb.MsgPing) error

// GetDirFilesHandler handles an incoming directory listing request.
// Implementations should write one or more MSG_TYPE_DIR_FILES messages before returning.
type GetDirFilesHandler func(ctx context.Context, client *ProtoClient, bidi ProtoBidi, msg *pb.MsgGetDirFiles) error

// GetFileMetaHandler handles an incoming file metadata request.
// Implementations should write a MSG_TYPE_FILE_META or MSG_TYPE_ERROR message before returning.
type GetFileMetaHandler func(ctx context.Context, client *ProtoClient, bidi ProtoBidi, msg *pb.MsgGetFileMeta) error

// GetFileHandler handles an incoming file request.
// Implementations should write MSG_TYPE_FILE_META then file bytes (or MSG_TYPE_ERROR) before returning.
type GetFileHandler func(ctx context.Context, client *ProtoClient, bidi ProtoBidi, msg *pb.MsgGetFile) error

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
// If there was already a certificate for the host and it did not match the new one, returns a CertMismatchError.
func ConnectTofu(addr string, certStore ClientCertStore) (*quic.Conn, error) {
	if certStore == nil {
		return nil, ErrCertStoreRequired
	}

	// Make sure the address is always lowercase.
	addr = strings.ToLower(addr)

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
			return ErrNoServerCerts
		}

		leafDer := rawCerts[0]
		leaf, err := x509.ParseCertificate(leafDer)
		if err != nil {
			return fmt.Errorf("failed to parse server certificate: %w", err)
		}

		now := time.Now()
		if now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
			return ErrServerCertNotValidNow
		}

		storedDer, err := certStore.Get(host)
		if err != nil {
			return fmt.Errorf("failed to look up stored certificate for %q: %w", host, err)
		}

		if len(storedDer) == 0 {
			if err := certStore.Put(host, leafDer); err != nil {
				return fmt.Errorf("failed to store certificate for %q: %w", host, err)
			}
			return nil
		}

		if !bytes.Equal(storedDer, leafDer) {
			return CertMismatchError{Host: host}
		}

		return nil
	}

	return quic.DialAddr(context.Background(), addr, tlsCfg, &quic.Config{
		KeepAlivePeriod: DefaultKeepAlivePeriod,
	})
}

// negotiateVersion negotiates the protocol version with the server.
// Returns the server's protocol version if successful.
// If the client's version was rejected, returns a VersionRejectedError.
func negotiateVersion(conn *quic.Conn, version *pb.ProtoVersion) (*pb.ProtoVersion, error) {
	if version == nil {
		return nil, ErrProtocolVersionRequired
	}

	bidi, err := OpenBidiWithMsg(conn, pb.MsgType_MSG_TYPE_VERSION, &pb.MsgVersion{
		Version: version,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = bidi.Close()
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
			msgText = *rejected.Payload.Message
		}
		return nil, VersionRejectedError{
			Reason:  rejected.Payload.Reason,
			Message: msgText,
		}
	default:
		return nil, NewUnexpectedMsgTypeError(pb.MsgType_MSG_TYPE_VERSION_ACCEPTED, msg.Type)
	}
}

// authenticate performs the authentication step on a client connection.
// If the server rejected the credentials, returns a AuthRejectedError.
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
		_ = bidi.Close()
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
			msgText = *rejected.Payload.Message
		}
		return AuthRejectedError{
			Reason:  rejected.Payload.Reason,
			Message: msgText,
		}
	default:
		return NewUnexpectedMsgTypeError(pb.MsgType_MSG_TYPE_AUTH_ACCEPTED, msg.Type)
	}
}

// NewClient establishes a QUIC connection, negotiates the protocol version, and authenticates.
// If authentication is successful, it returns a new, fully authenticated client.
//
// If the client's certificate did not match an existing entry in the cert store, returns a CertMismatchError.
// If the server rejected the client's protocol version, returns a VersionRejectedError.
// If the server rejected the client's credentials, returns a AuthRejectedError.
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

// Close closes the underlying connection to the server.
func (c *ProtoClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}

	return c.conn.CloseWithError(0, "client closed")
}

// Ping sends a ping to the server and waits for a pong response.
func (c *ProtoClient) Ping() (*pb.MsgPong, error) {
	bidi, err := OpenBidiWithMsg(c.conn, pb.MsgType_MSG_TYPE_PING, &pb.MsgPing{
		SentTs: time.Now().UnixMilli(),
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = bidi.Close()
	}()

	pong, err := ReadExpect[*pb.MsgPong](bidi.ProtoStreamReader, pb.MsgType_MSG_TYPE_PONG)
	if err != nil {
		return nil, err
	}

	return pong.Payload, nil
}

// GetDirFiles requests all filenames inside a directory.
func (c *ProtoClient) GetDirFiles(user string, path string) ([]*pb.MsgFileMeta, error) {
	bidi, err := OpenBidiWithMsg(c.conn, pb.MsgType_MSG_TYPE_GET_DIR_FILES, &pb.MsgGetDirFiles{
		User: user,
		Path: path,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = bidi.Close()
	}()

	var files []*pb.MsgFileMeta
	for {
		dirFiles, err := ReadExpect[*pb.MsgDirFiles](bidi.ProtoStreamReader, pb.MsgType_MSG_TYPE_DIR_FILES)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}

		files = append(files, dirFiles.Payload.Files...)
	}

	return files, nil
}

// GetFileMeta requests metadata about a file without reading it.
func (c *ProtoClient) GetFileMeta(user string, path string) (*pb.MsgFileMeta, error) {
	bidi, err := OpenBidiWithMsg(c.conn, pb.MsgType_MSG_TYPE_GET_FILE_META, &pb.MsgGetFileMeta{
		User: user,
		Path: path,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = bidi.Close()
	}()

	meta, err := ReadExpect[*pb.MsgFileMeta](bidi.ProtoStreamReader, pb.MsgType_MSG_TYPE_FILE_META)
	if err != nil {
		return nil, err
	}

	return meta.Payload, nil
}

// GetFile requests file metadata and returns a stream for reading the file contents.
// If limit is 0, the entire file is read.
func (c *ProtoClient) GetFile(user string, path string, offset uint64, limit uint64) (*pb.MsgFileMeta, io.ReadCloser, error) {
	bidi, err := OpenBidiWithMsg(c.conn, pb.MsgType_MSG_TYPE_GET_FILE, &pb.MsgGetFile{
		User:   user,
		Path:   path,
		Offset: offset,
		Limit:  limit,
	})
	if err != nil {
		return nil, nil, err
	}

	meta, err := ReadExpect[*pb.MsgFileMeta](bidi.ProtoStreamReader, pb.MsgType_MSG_TYPE_FILE_META)
	if err != nil {
		_ = bidi.Stream.Close()
		return nil, nil, err
	}

	return meta.Payload, bidi.Stream, nil
}

// GetOnlineUsers requests the list of users currently online in the room.
func (c *ProtoClient) GetOnlineUsers() ([]string, error) {
	bidi, err := OpenBidiWithMsg(c.conn, pb.MsgType_MSG_TYPE_GET_ONLINE_USERS, &pb.MsgGetOnlineUsers{})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = bidi.Close()
	}()

	var users []string
	for {
		resp, err := ReadExpect[*pb.MsgOnlineUsers](bidi.ProtoStreamReader, pb.MsgType_MSG_TYPE_ONLINE_USERS)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		users = append(users, resp.Payload.Users...)
	}

	return users, nil
}

// Listen waits for incoming requests and dispatches them to the configured handlers.
func (c *ProtoClient) Listen(ctx context.Context, errorHandler func(error)) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("client connection is not initialized")
	}
	if errorHandler == nil {
		errorHandler = func(error) {}
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := HandleBidiRequest(ctx, c.conn, c.listenerHandlers(ctx), nil, errorHandler)
		if err != nil {
			return err
		}
	}
}

func (c *ProtoClient) listenerHandlers(ctx context.Context) map[pb.MsgType]BidiHandler {
	return map[pb.MsgType]BidiHandler{
		pb.MsgType_MSG_TYPE_PING: func(_ *quic.Conn, bidi ProtoBidi, msg *UntypedProtoMsg) error {
			defer func() {
				_ = bidi.Close()
			}()

			if c.OnPing == nil {
				return writeUnimplementedError(bidi, msg.Type)
			}

			return c.OnPing(ctx, c, bidi, ToTyped[*pb.MsgPing](msg).Payload)
		},
		pb.MsgType_MSG_TYPE_GET_DIR_FILES: func(_ *quic.Conn, bidi ProtoBidi, msg *UntypedProtoMsg) error {
			defer func() {
				_ = bidi.Close()
			}()

			if c.OnGetDirFiles == nil {
				return writeUnimplementedError(bidi, msg.Type)
			}

			return c.OnGetDirFiles(ctx, c, bidi, ToTyped[*pb.MsgGetDirFiles](msg).Payload)
		},
		pb.MsgType_MSG_TYPE_GET_FILE_META: func(_ *quic.Conn, bidi ProtoBidi, msg *UntypedProtoMsg) error {
			defer func() {
				_ = bidi.Close()
			}()

			if c.OnGetFileMeta == nil {
				return writeUnimplementedError(bidi, msg.Type)
			}

			return c.OnGetFileMeta(ctx, c, bidi, ToTyped[*pb.MsgGetFileMeta](msg).Payload)
		},
		pb.MsgType_MSG_TYPE_GET_FILE: func(_ *quic.Conn, bidi ProtoBidi, msg *UntypedProtoMsg) error {
			defer func() {
				_ = bidi.Close()
			}()

			if c.OnGetFile == nil {
				return writeUnimplementedError(bidi, msg.Type)
			}

			return c.OnGetFile(ctx, c, bidi, ToTyped[*pb.MsgGetFile](msg).Payload)
		},
	}
}

func writeUnimplementedError(bidi ProtoBidi, msgType pb.MsgType) error {
	message := fmt.Sprintf("handler for %s is unimplemented", msgType.String())
	return bidi.Write(pb.MsgType_MSG_TYPE_ERROR, &pb.MsgError{
		Type:    pb.ErrType_ERR_TYPE_INTERNAL,
		Message: &message,
	})
}
