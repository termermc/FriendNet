package protocol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
)

// ProtoServer receives new connections and performs the handshake flow.
type ProtoServer struct {
	listener *quic.Listener

	// VersionHandler decides whether to accept or reject a client's protocol version.
	VersionHandler ServerVersionHandler
	// AuthHandler decides whether to accept or reject a client's authentication request.
	AuthHandler ServerAuthHandler
}

// ServerVersionHandler handles a client's version negotiation request.
// Return a non-nil accepted version to accept or a non-nil rejection message to reject.
type ServerVersionHandler func(ctx context.Context, client *ProtoServerClient, version *pb.ProtoVersion) (*pb.ProtoVersion, *pb.MsgVersionRejected, error)

// ServerAuthHandler handles a client's authentication request.
// Return a non-nil accepted message to accept or a non-nil rejection message to reject.
type ServerAuthHandler func(ctx context.Context, client *ProtoServerClient, msg *pb.MsgAuthenticate) (*pb.MsgAuthAccepted, *pb.MsgAuthRejected, error)

// NewProtoServer creates a server around an existing QUIC listener.
func NewProtoServer(listener *quic.Listener) *ProtoServer {
	return &ProtoServer{
		listener:       listener,
		VersionHandler: DefaultServerVersionHandler,
		AuthHandler:    DefaultServerAuthHandler,
	}
}

// Accept waits for a new connection, performs the handshake, and returns an authenticated client.
func (s *ProtoServer) Accept(ctx context.Context) (*ProtoServerClient, error) {
	if s == nil || s.listener == nil {
		return nil, fmt.Errorf("server listener is not initialized")
	}

	conn, err := s.listener.Accept(ctx)
	if err != nil {
		return nil, err
	}

	client := &ProtoServerClient{
		conn:   conn,
		server: s,
	}

	if err := s.negotiateVersion(ctx, client); err != nil {
		_ = conn.CloseWithError(0, "version negotiation failed")
		return nil, err
	}

	if err := s.authenticate(ctx, client); err != nil {
		_ = conn.CloseWithError(0, "authentication failed")
		return nil, err
	}

	return client, nil
}

// ProtoServerClient represents an authenticated client connection.
type ProtoServerClient struct {
	conn   *quic.Conn
	server *ProtoServer

	// OnPing handles incoming MSG_TYPE_PING messages.
	OnPing ServerPingHandler
	// OnGetDirFiles handles incoming MSG_TYPE_GET_DIR_FILES messages.
	OnGetDirFiles ServerGetDirFilesHandler
	// OnGetFileMeta handles incoming MSG_TYPE_GET_FILE_META messages.
	OnGetFileMeta ServerGetFileMetaHandler
	// OnGetFile handles incoming MSG_TYPE_GET_FILE messages.
	OnGetFile ServerGetFileHandler
	// OnGetOnlineUsers handles incoming MSG_TYPE_GET_ONLINE_USERS messages.
	OnGetOnlineUsers ServerGetOnlineUsersHandler
}

// ServerPingHandler handles an incoming ping request.
// Implementations should write a response (typically MSG_TYPE_PONG) before returning.
type ServerPingHandler func(ctx context.Context, client *ProtoServerClient, bidi ProtoBidi, msg *pb.MsgPing) error

// ServerGetDirFilesHandler handles an incoming directory listing request.
// Implementations should write one or more MSG_TYPE_DIR_FILES messages before returning.
type ServerGetDirFilesHandler func(ctx context.Context, client *ProtoServerClient, bidi ProtoBidi, msg *pb.MsgGetDirFiles) error

// ServerGetFileMetaHandler handles an incoming file metadata request.
// Implementations should write a MSG_TYPE_FILE_META or MSG_TYPE_ERROR message before returning.
type ServerGetFileMetaHandler func(ctx context.Context, client *ProtoServerClient, bidi ProtoBidi, msg *pb.MsgGetFileMeta) error

// ServerGetFileHandler handles an incoming file request.
// Implementations should write MSG_TYPE_FILE_META then file bytes (or MSG_TYPE_ERROR) before returning.
type ServerGetFileHandler func(ctx context.Context, client *ProtoServerClient, bidi ProtoBidi, msg *pb.MsgGetFile) error

// ServerGetOnlineUsersHandler handles an incoming online users request.
// Implementations should write a MSG_TYPE_ONLINE_USERS before returning.
type ServerGetOnlineUsersHandler func(ctx context.Context, client *ProtoServerClient, bidi ProtoBidi, msg *pb.MsgGetOnlineUsers) error

// Close closes the underlying connection to the client.
func (c *ProtoServerClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}

	return c.conn.CloseWithError(0, "server closed")
}

// Ping sends a ping to the client and waits for a pong response.
func (c *ProtoServerClient) Ping() (*pb.MsgPong, error) {
	bidi, err := OpenBidiWithMsg(c.conn, pb.MsgType_MSG_TYPE_PING, &pb.MsgPing{
		SentTs: time.Now().UnixMilli(),
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		CloseBidi(&bidi)
	}()

	pong, err := ReadExpect[*pb.MsgPong](bidi.ProtoStreamReader, pb.MsgType_MSG_TYPE_PONG)
	if err != nil {
		return nil, err
	}

	return pong, nil
}

// GetDirFiles requests all filenames inside a directory on the client.
func (c *ProtoServerClient) GetDirFiles(user string, path string) ([]*pb.MsgFileMeta, error) {
	bidi, err := OpenBidiWithMsg(c.conn, pb.MsgType_MSG_TYPE_GET_DIR_FILES, &pb.MsgGetDirFiles{
		User: user,
		Path: path,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		CloseBidi(&bidi)
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

		files = append(files, dirFiles.Files...)
	}

	return files, nil
}

// GetFileMeta requests metadata about a file without reading it.
func (c *ProtoServerClient) GetFileMeta(user string, path string) (*pb.MsgFileMeta, error) {
	bidi, err := OpenBidiWithMsg(c.conn, pb.MsgType_MSG_TYPE_GET_FILE_META, &pb.MsgGetFileMeta{
		User: user,
		Path: path,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		CloseBidi(&bidi)
	}()

	meta, err := ReadExpect[*pb.MsgFileMeta](bidi.ProtoStreamReader, pb.MsgType_MSG_TYPE_FILE_META)
	if err != nil {
		return nil, err
	}

	return meta, nil
}

// GetFile requests file metadata and returns a stream for reading the file contents.
// If limit is 0, the entire file is read.
func (c *ProtoServerClient) GetFile(user string, path string, offset uint64, limit uint64) (*pb.MsgFileMeta, io.ReadCloser, error) {
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

	return meta, bidi.Stream, nil
}

// Listen waits for incoming requests and dispatches them to the configured handlers.
func (c *ProtoServerClient) Listen(ctx context.Context, errorHandler func(error)) error {
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

func (c *ProtoServerClient) listenerHandlers(ctx context.Context) map[pb.MsgType]BidiHandler {
	return map[pb.MsgType]BidiHandler{
		pb.MsgType_MSG_TYPE_PING: func(_ *quic.Conn, bidi ProtoBidi, msg *UntypedProtoMsg) error {
			defer func() {
				CloseBidi(&bidi)
			}()

			if c.OnPing == nil {
				return WriteUnimplementedError(bidi, msg.Type)
			}

			return c.OnPing(ctx, c, bidi, ToTyped[*pb.MsgPing](msg).Payload)
		},
		pb.MsgType_MSG_TYPE_GET_DIR_FILES: func(_ *quic.Conn, bidi ProtoBidi, msg *UntypedProtoMsg) error {
			defer func() {
				CloseBidi(&bidi)
			}()

			if c.OnGetDirFiles == nil {
				return WriteUnimplementedError(bidi, msg.Type)
			}

			return c.OnGetDirFiles(ctx, c, bidi, ToTyped[*pb.MsgGetDirFiles](msg).Payload)
		},
		pb.MsgType_MSG_TYPE_GET_FILE_META: func(_ *quic.Conn, bidi ProtoBidi, msg *UntypedProtoMsg) error {
			defer func() {
				CloseBidi(&bidi)
			}()

			if c.OnGetFileMeta == nil {
				return WriteUnimplementedError(bidi, msg.Type)
			}

			return c.OnGetFileMeta(ctx, c, bidi, ToTyped[*pb.MsgGetFileMeta](msg).Payload)
		},
		pb.MsgType_MSG_TYPE_GET_FILE: func(_ *quic.Conn, bidi ProtoBidi, msg *UntypedProtoMsg) error {
			defer func() {
				CloseBidi(&bidi)
			}()

			if c.OnGetFile == nil {
				return WriteUnimplementedError(bidi, msg.Type)
			}

			return c.OnGetFile(ctx, c, bidi, ToTyped[*pb.MsgGetFile](msg).Payload)
		},
		pb.MsgType_MSG_TYPE_GET_ONLINE_USERS: func(_ *quic.Conn, bidi ProtoBidi, msg *UntypedProtoMsg) error {
			defer func() {
				CloseBidi(&bidi)
			}()

			if c.OnGetOnlineUsers == nil {
				return WriteUnimplementedError(bidi, msg.Type)
			}

			return c.OnGetOnlineUsers(ctx, c, bidi, ToTyped[*pb.MsgGetOnlineUsers](msg).Payload)
		},
	}
}

func (s *ProtoServer) negotiateVersion(ctx context.Context, client *ProtoServerClient) error {
	bidi, err := WaitForBidi(ctx, client.conn)
	if err != nil {
		return fmt.Errorf("failed to wait for version negotiation stream: %w", err)
	}
	defer func() {
		CloseBidi(&bidi)
	}()

	msg, err := bidi.Read()
	if err != nil {
		return err
	}

	if msg.Type != pb.MsgType_MSG_TYPE_VERSION {
		if writeErr := WriteUnexpectedReplyError(bidi, pb.MsgType_MSG_TYPE_VERSION, msg.Type); writeErr != nil {
			return writeErr
		}
		return NewUnexpectedMsgTypeError(pb.MsgType_MSG_TYPE_VERSION, msg.Type)
	}

	version := ToTyped[*pb.MsgVersion](msg).Payload.Version

	handler := s.VersionHandler
	if handler == nil {
		handler = DefaultServerVersionHandler
	}

	accepted, rejected, err := handler(ctx, client, version)
	if err != nil {
		if writeErr := WriteInternalError(bidi, err); writeErr != nil {
			return writeErr
		}
		return err
	}

	if rejected != nil {
		if rejected.Version == nil {
			rejected.Version = CurrentProtocolVersion
		}
		if err := bidi.Write(pb.MsgType_MSG_TYPE_VERSION_REJECTED, rejected); err != nil {
			return err
		}
		return VersionRejectedError{
			Reason:  rejected.Reason,
			Message: rejected.GetMessage(),
		}
	}

	if accepted == nil {
		accepted = CurrentProtocolVersion
	}

	return bidi.Write(pb.MsgType_MSG_TYPE_VERSION_ACCEPTED, &pb.MsgVersionAccepted{
		Version: accepted,
	})
}

func (s *ProtoServer) authenticate(ctx context.Context, client *ProtoServerClient) error {
	bidi, err := WaitForBidi(ctx, client.conn)
	if err != nil {
		return fmt.Errorf("failed to wait for authentication stream: %w", err)
	}
	defer func() {
		CloseBidi(&bidi)
	}()

	msg, err := bidi.Read()
	if err != nil {
		return err
	}

	if msg.Type != pb.MsgType_MSG_TYPE_AUTHENTICATE {
		if writeErr := WriteUnexpectedReplyError(bidi, pb.MsgType_MSG_TYPE_AUTHENTICATE, msg.Type); writeErr != nil {
			return writeErr
		}
		return NewUnexpectedMsgTypeError(pb.MsgType_MSG_TYPE_AUTHENTICATE, msg.Type)
	}

	authMsg := ToTyped[*pb.MsgAuthenticate](msg).Payload

	handler := s.AuthHandler
	if handler == nil {
		handler = DefaultServerAuthHandler
	}

	accepted, rejected, err := handler(ctx, client, authMsg)
	if err != nil {
		if writeErr := WriteInternalError(bidi, err); writeErr != nil {
			return writeErr
		}
		return err
	}

	if rejected != nil {
		if err := bidi.Write(pb.MsgType_MSG_TYPE_AUTH_REJECTED, rejected); err != nil {
			return err
		}
		return AuthRejectedError{
			Reason:  rejected.Reason,
			Message: rejected.GetMessage(),
		}
	}

	if accepted == nil {
		accepted = &pb.MsgAuthAccepted{}
	}

	return bidi.Write(pb.MsgType_MSG_TYPE_AUTH_ACCEPTED, accepted)
}

// DefaultServerVersionHandler accepts only the current protocol version.
func DefaultServerVersionHandler(_ context.Context, _ *ProtoServerClient, version *pb.ProtoVersion) (*pb.ProtoVersion, *pb.MsgVersionRejected, error) {
	if version == nil {
		message := "missing protocol version"
		return nil, &pb.MsgVersionRejected{
			Version: CurrentProtocolVersion,
			Reason:  pb.VersionRejectionReason_VERSION_REJECTION_REASON_UNSPECIFIED,
			Message: &message,
		}, nil
	}

	cmp := CompareProtoVersions(version, CurrentProtocolVersion)
	if cmp == 0 {
		return CurrentProtocolVersion, nil, nil
	}

	var reason pb.VersionRejectionReason
	if cmp > 0 {
		reason = pb.VersionRejectionReason_VERSION_REJECTION_REASON_TOO_NEW
	} else {
		reason = pb.VersionRejectionReason_VERSION_REJECTION_REASON_TOO_OLD
	}
	message := "unsupported protocol version"

	return nil, &pb.MsgVersionRejected{
		Version: CurrentProtocolVersion,
		Reason:  reason,
		Message: &message,
	}, nil
}

// DefaultServerAuthHandler rejects authentication by default.
func DefaultServerAuthHandler(_ context.Context, _ *ProtoServerClient, _ *pb.MsgAuthenticate) (*pb.MsgAuthAccepted, *pb.MsgAuthRejected, error) {
	message := "authentication unimplemented"
	return nil, &pb.MsgAuthRejected{
		Reason:  pb.AuthRejectionReason_AUTH_REJECTION_REASON_INVALID_CREDENTIALS,
		Message: &message,
	}, nil
}
