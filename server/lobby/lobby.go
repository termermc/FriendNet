package lobby

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"friendnet.org/server/room"
	"friendnet.org/server/storage"
	mcfpassword "github.com/termermc/go-mcf-password"
)

// DefaultTimeout is the default timeout for connections in the lobby (unauthenticated).
const DefaultTimeout = 10 * time.Second

// Lobby is where clients go when they first connect.
// It accepts new connections and handles authentication.
// After successful authentication, they are sent to the appropriate room.
type Lobby struct {
	logger *slog.Logger

	storage *storage.Storage
	roomMgr *room.Manager

	timeout   time.Duration
	serverVer *pb.ProtoVersion
}

// NewLobby creates a new lobby instance.
// The timeout is how long a connection can stay in the lobby until it is disconnected.
func NewLobby(
	logger *slog.Logger,

	storage *storage.Storage,
	roomMgr *room.Manager,

	timeout time.Duration,
	serverVer *pb.ProtoVersion,
) *Lobby {
	if timeout <= 0 {
		panic("lobby timeout must be positive")
	}
	if serverVer == nil {
		panic("server version cannot be nil")
	}

	return &Lobby{
		logger: logger,

		storage: storage,
		roomMgr: roomMgr,

		timeout:   timeout,
		serverVer: serverVer,
	}
}

// Onboard takes ownership of a connection and performs negotiation and authentication steps.
// It returns immediately.
func (l *Lobby) Onboard(conn protocol.ProtoConn) {
	// Onboard in its own goroutine so that the method can return immediately.
	go func() {
		lobbyCtx, lobbyCancel := context.WithTimeout(context.Background(), l.timeout)
		defer lobbyCancel()

		clientVer, err := l.negotiateClientVersion(lobbyCtx, conn)
		if err != nil {
			_ = conn.CloseWithReason(err.Error())
			return
		}

		authRoom, authUsername, err := l.authenticateClient(
			lobbyCtx,
			conn,
		)
		if err != nil {
			_ = conn.CloseWithReason(err.Error())
			return
		}

		// Get room instance from the manager.
		roomInst, has := l.roomMgr.GetRoomByName(authRoom)
		if !has {
			_ = conn.CloseWithReason("room not found")
			return
		}

		// Pass ownership of connection to the room instance.
		err = roomInst.Onboard(conn, clientVer, authUsername)
		if err != nil {
			l.logger.Error("failed to onboard client to room",
				"service", "main.Lobby",
				"room", authRoom.String(),
				"username", authUsername.String(),
				"error", err,
			)

			_ = conn.CloseWithReason("internal error")
			return
		}
	}()
}

// negotiateClientVersion performs the version negotiation phase with the provided connection.
// If the negotiation succeeds, the client's version will be returned.
// Negotiation will fail with an error if the client and server versions are incompatible.
// This method still takes care of sending the appropriate reply to the client's authentication request, even if there was an error.
func (l *Lobby) negotiateClientVersion(
	ctx context.Context,
	conn protocol.ProtoConn,
) (clientVer *pb.ProtoVersion, finalErr error) {
	bidi, bidiErr := conn.WaitForBidi(ctx)
	if bidiErr != nil {
		return nil, fmt.Errorf("failed to wait for version negotiation stream: %w", bidiErr)
	}
	defer func() {
		_ = bidi.Close()
	}()

	finalErr = func() error {
		msg, err := protocol.ReadExpect[*pb.MsgVersion](bidi.ProtoStreamReader, pb.MsgType_MSG_TYPE_VERSION)
		if err != nil {
			return err
		}

		clientVer = msg.Payload.Version

		if clientVer == nil {
			return &protocol.VersionRejectedError{
				Reason:  pb.VersionRejectionReason_VERSION_REJECTION_REASON_UNSPECIFIED,
				Message: "missing protocol version",
			}
		}

		// Check if versions are the same, or at least the major and minor parts are the same.
		cmp := protocol.CompareProtoVersions(clientVer, l.serverVer)
		if cmp == 0 || (clientVer.Major == l.serverVer.Major && clientVer.Minor == l.serverVer.Minor) {
			return nil
		}

		var reason pb.VersionRejectionReason
		if cmp > 0 {
			reason = pb.VersionRejectionReason_VERSION_REJECTION_REASON_TOO_NEW
		} else {
			reason = pb.VersionRejectionReason_VERSION_REJECTION_REASON_TOO_OLD
		}

		return &protocol.VersionRejectedError{
			Reason:  reason,
			Message: "unsupported protocol version",
		}
	}()
	if finalErr != nil {
		// Write appropriate error reply to bidi before closure.
		var rejErr protocol.VersionRejectedError
		var unexpectedErr protocol.UnexpectedMsgTypeError
		if errors.As(finalErr, &rejErr) {
			_ = bidi.Write(pb.MsgType_MSG_TYPE_VERSION_REJECTED, &pb.MsgVersionRejected{
				Reason:  rejErr.Reason,
				Message: &rejErr.Message,
			})
		} else if errors.As(finalErr, &unexpectedErr) {
			_ = bidi.WriteUnexpectedMsgTypeError(unexpectedErr.Expected, unexpectedErr.Actual)
		} else {
			_ = bidi.WriteInternalError(finalErr)
		}

		clientVer = nil
		return clientVer, finalErr
	}

	return clientVer, bidi.Write(pb.MsgType_MSG_TYPE_VERSION_ACCEPTED, &pb.MsgVersionAccepted{})
}

// authenticateClient performs the authentication phase with the provided connection.
// If the authentication succeeds, the client's room and username will be returned.
// Authentication will fail with an error if the client provides invalid credentials.
// This method still takes care of sending the appropriate reply to the client's authentication request, even if there was an error.
func (l *Lobby) authenticateClient(
	ctx context.Context,
	conn protocol.ProtoConn,
) (room common.NormalizedRoomName, username common.NormalizedUsername, finalErr error) {
	bidi, bidiErr := conn.WaitForBidi(ctx)
	if bidiErr != nil {
		return room, username, fmt.Errorf("failed to wait for authentication stream: %w", bidiErr)
	}
	defer func() {
		_ = bidi.Close()
	}()

	finalErr = func() error {
		msg, err := protocol.ReadExpect[*pb.MsgAuthenticate](bidi.ProtoStreamReader, pb.MsgType_MSG_TYPE_AUTHENTICATE)
		authMsg := msg.Payload

		invalidCreds := func() error {
			return protocol.AuthRejectedError{
				Reason:  pb.AuthRejectionReason_AUTH_REJECTION_REASON_INVALID_CREDENTIALS,
				Message: "invalid credentials",
			}
		}

		// Validate room name and username.
		var isValid bool
		room, isValid = common.NormalizeRoomName(authMsg.Room)
		if !isValid {
			return invalidCreds()
		}
		username, isValid = common.NormalizeUsername(authMsg.Username)
		if !isValid {
			return invalidCreds()
		}

		// Look up account and verify password.
		var accountRec storage.AccountRecord
		var hasAcc bool
		accountRec, hasAcc, err = l.storage.GetAccountByRoomAndUsername(ctx, room, username)
		if err != nil {
			return err
		}
		if !hasAcc {
			return invalidCreds()
		}

		// Check password.
		var matches bool
		var needsRehash bool
		matches, needsRehash, err = mcfpassword.VerifyPassword(authMsg.Password, accountRec.PasswordHash)
		if err != nil {
			return fmt.Errorf(`failed to verify password for account with room %q and username %q: %w`,
				room.String(),
				username.String(),
				err,
			)
		}
		if !matches {
			return invalidCreds()
		}

		// Rehash password if necessary.
		if needsRehash {
			var newHash string
			newHash, err = mcfpassword.HashPassword(authMsg.Password)
			if err != nil {
				return fmt.Errorf(`failed to rehash password for account with room %q and username %q: %w`,
					room.String(),
					username.String(),
					err,
				)
			}

			err = l.storage.UpdateAccountPasswordHash(ctx, room, username, newHash)
			if err != nil {
				return fmt.Errorf(`failed to update account with room %q and username %q with rehashed password: %w`,
					room.String(),
					username.String(),
					err,
				)
			}
		}

		// Authenticate successful.
		return nil
	}()
	if finalErr != nil {
		// Write appropriate error reply to bidi before closure.
		var rejErr protocol.AuthRejectedError
		var unexpectedErr protocol.UnexpectedMsgTypeError
		if errors.As(finalErr, &rejErr) {
			_ = bidi.Write(pb.MsgType_MSG_TYPE_AUTH_REJECTED, &pb.MsgAuthRejected{
				Reason:  rejErr.Reason,
				Message: &rejErr.Message,
			})
		} else if errors.As(finalErr, &unexpectedErr) {
			_ = bidi.WriteUnexpectedMsgTypeError(unexpectedErr.Expected, unexpectedErr.Actual)
		} else {
			_ = bidi.WriteInternalError(finalErr)
		}

		room = common.ZeroNormalizedRoomName
		username = common.ZeroNormalizedUsername
		return room, username, finalErr
	}

	return room, username, bidi.Write(pb.MsgType_MSG_TYPE_AUTH_ACCEPTED, &pb.MsgAuthAccepted{})
}
