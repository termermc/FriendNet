package main

import (
	"context"
	"fmt"
	"time"

	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"friendnet.org/server/storage"
	"github.com/quic-go/quic-go"
	mcfpassword "github.com/termermc/go-mcf-password"
)

// LobbyTimeout is the timeout for connections in the lobby (unauthenticated).
const LobbyTimeout = 10 * time.Second

// NegotiateClientVersion performs the version negotiation phase with the provided connection.
// If the negotiation succeeds, the client's version will be returned.
// Negotiation will fail with an error if the client and server versions are incompatible.
func NegotiateClientVersion(
	ctx context.Context,
	conn *quic.Conn,
	serverVersion *pb.ProtoVersion,
) (*pb.ProtoVersion, error) {
	bidi, err := protocol.WaitForBidi(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for version negotiation stream: %w", err)
	}
	defer func() {
		protocol.CloseBidi(&bidi)
	}()

	msg, err := bidi.Read()
	if err != nil {
		return nil, err
	}

	if msg.Type != pb.MsgType_MSG_TYPE_VERSION {
		if writeErr := protocol.WriteUnexpectedReplyError(bidi, pb.MsgType_MSG_TYPE_VERSION, msg.Type); writeErr != nil {
			return nil, writeErr
		}
		return nil, protocol.NewUnexpectedMsgTypeError(pb.MsgType_MSG_TYPE_VERSION, msg.Type)
	}

	version := protocol.ToTyped[*pb.MsgVersion](msg).Payload.Version

	accepted, rejected, err := func() (*pb.ProtoVersion, *pb.MsgVersionRejected, error) {
		if version == nil {
			message := "missing protocol version"
			return nil, &pb.MsgVersionRejected{
				Version: serverVersion,
				Reason:  pb.VersionRejectionReason_VERSION_REJECTION_REASON_UNSPECIFIED,
				Message: &message,
			}, nil
		}

		cmp := protocol.CompareProtoVersions(version, serverVersion)
		if cmp == 0 || (version.Major == serverVersion.Major && version.Minor == serverVersion.Minor) {
			return serverVersion, nil, nil
		}

		var reason pb.VersionRejectionReason
		if cmp > 0 {
			reason = pb.VersionRejectionReason_VERSION_REJECTION_REASON_TOO_NEW
		} else {
			reason = pb.VersionRejectionReason_VERSION_REJECTION_REASON_TOO_OLD
		}
		message := "unsupported protocol version"

		return nil, &pb.MsgVersionRejected{
			Version: serverVersion,
			Reason:  reason,
			Message: &message,
		}, nil
	}()
	if err != nil {
		if writeErr := protocol.WriteInternalError(bidi, err); writeErr != nil {
			return nil, writeErr
		}
		return nil, err
	}

	if rejected != nil {
		if rejected.Version == nil {
			rejected.Version = serverVersion
		}
		if err := bidi.Write(pb.MsgType_MSG_TYPE_VERSION_REJECTED, rejected); err != nil {
			return nil, err
		}
		return nil, protocol.VersionRejectedError{
			Reason:  rejected.Reason,
			Message: rejected.GetMessage(),
		}
	}

	if accepted == nil {
		accepted = serverVersion
	}

	err = bidi.Write(pb.MsgType_MSG_TYPE_VERSION_ACCEPTED, &pb.MsgVersionAccepted{
		Version: accepted,
	})
	if err != nil {
		return nil, err
	}

	return version, nil
}

// AuthenticateClient performs the authentication phase with the provided connection.
// If the authentication succeeds, the client's room and username will be returned.
// Authentication will fail with an error if the client provides invalid credentials.
func AuthenticateClient(
	ctx context.Context,
	conn *quic.Conn,
	storage *storage.Storage,
) (room common.NormalizedRoomName, username common.NormalizedUsername, err error) {
	bidi, err := protocol.WaitForBidi(ctx, conn)
	if err != nil {
		return room, username, fmt.Errorf("failed to wait for authentication stream: %w", err)
	}
	defer func() {
		protocol.CloseBidi(&bidi)
	}()

	msg, err := bidi.Read()
	if err != nil {
		return room, username, err
	}

	if msg.Type != pb.MsgType_MSG_TYPE_AUTHENTICATE {
		if writeErr := protocol.WriteUnexpectedReplyError(bidi, pb.MsgType_MSG_TYPE_AUTHENTICATE, msg.Type); writeErr != nil {
			return room, username, writeErr
		}
		return room, username, protocol.NewUnexpectedMsgTypeError(pb.MsgType_MSG_TYPE_AUTHENTICATE, msg.Type)
	}

	authMsg := protocol.ToTyped[*pb.MsgAuthenticate](msg).Payload

	accepted, rejected, err := func() (*pb.MsgAuthAccepted, *pb.MsgAuthRejected, error) {
		invalidCreds := func() (*pb.MsgAuthAccepted, *pb.MsgAuthRejected, error) {
			reason := "invalid credentials"
			return nil, &pb.MsgAuthRejected{
				Reason:  pb.AuthRejectionReason_AUTH_REJECTION_REASON_INVALID_CREDENTIALS,
				Message: &reason,
			}, nil
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
		accountRec, hasAcc, err := storage.GetAccountByRoomAndUsername(ctx, room, username)
		if err != nil {
			return nil, nil, err
		}
		if !hasAcc {
			return invalidCreds()
		}

		// Check password.
		matches, needsRehash, err := mcfpassword.VerifyPassword(authMsg.Password, accountRec.PasswordHash)
		if err != nil {
			return nil, nil, fmt.Errorf(`failed to verify password for account with room %q and username %q: %w`,
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
			newHash, err := mcfpassword.HashPassword(authMsg.Password)
			if err != nil {
				return nil, nil, fmt.Errorf(`failed to rehash password for account with room %q and username %q: %w`,
					room.String(),
					username.String(),
					err,
				)
			}

			err = storage.UpdateAccountPasswordHash(ctx, room, username, newHash)
			if err != nil {
				return nil, nil, fmt.Errorf(`failed to update account with room %q and username %q with rehashed password: %w`,
					room.String(),
					username.String(),
					err,
				)
			}
		}

		// Authenticate successful.
		return &pb.MsgAuthAccepted{}, nil, nil
	}()
	if err != nil {
		if writeErr := protocol.WriteInternalError(bidi, err); writeErr != nil {
			return room, username, writeErr
		}
		return room, username, err
	}

	if rejected != nil {
		if err := bidi.Write(pb.MsgType_MSG_TYPE_AUTH_REJECTED, rejected); err != nil {
			return room, username, err
		}
		return room, username, protocol.AuthRejectedError{
			Reason:  rejected.Reason,
			Message: rejected.GetMessage(),
		}
	}

	if accepted == nil {
		accepted = &pb.MsgAuthAccepted{}
	}

	return room, username, bidi.Write(pb.MsgType_MSG_TYPE_AUTH_ACCEPTED, accepted)
}
