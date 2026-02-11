package room

import (
	"context"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

// PingHandler handles an incoming ping request.
// Implementations must write a MSG_TYPE_PONG before returning.
type PingHandler func(ctx context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgPing]) error

// GetDirFilesHandler handles an incoming directory listing request.
// Implementations must write one or more MSG_TYPE_DIR_FILES messages before returning.
type GetDirFilesHandler func(ctx context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgGetDirFiles]) error

// GetFileMetaHandler handles an incoming file metadata request.
// Implementations must write a MSG_TYPE_FILE_META or MSG_TYPE_ERROR message before returning.
type GetFileMetaHandler func(ctx context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgGetFileMeta]) error

// GetFileHandler handles an incoming file request.
// Implementations must write MSG_TYPE_FILE_META then file bytes (or MSG_TYPE_ERROR) before returning.
type GetFileHandler func(ctx context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgGetFile]) error

// GetOnlineUsersHandler handles an incoming get online users request.
// Implementations must write one or more MSG_TYPE_ONLINE_USERS messages before returning.
type GetOnlineUsersHandler func(ctx context.Context, client *Client, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgGetOnlineUsers]) error

// ClientMessageHandlers handlers for incoming client messages.
// Handlers are called after receiving a new incoming bidi from a client with the corresponding message type.
//
// Important: Handlers must assume that the underlying bidi will be closed after the handler returns.
// References to the bidi or client must not be held after the handler returns.
type ClientMessageHandlers struct {
	OnPing           PingHandler
	OnGetDirFiles    GetDirFilesHandler
	OnGetFileMeta    GetFileMetaHandler
	OnGetFile        GetFileHandler
	OnGetOnlineUsers GetOnlineUsersHandler
}
