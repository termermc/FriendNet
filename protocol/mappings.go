package protocol

import (
	pb "friendnet.org/protocol/pb/v1"
	"google.golang.org/protobuf/proto"
)

// MsgTypeToEmptyMsg returns the appropriate empty message for the specified message type.
// The result can be unmarshalled with proto.UnmarshalMerge.
// The type is unknown, returns nil.
func MsgTypeToEmptyMsg(typ pb.MsgType) proto.Message {
	switch typ {
	case pb.MsgType_MSG_TYPE_PING:
		return &pb.MsgPing{}
	case pb.MsgType_MSG_TYPE_PONG:
		return &pb.MsgPong{}
	case pb.MsgType_MSG_TYPE_ACKNOWLEDGED:
		return &pb.MsgAcknowledged{}
	case pb.MsgType_MSG_TYPE_ERROR:
		return &pb.MsgError{}
	case pb.MsgType_MSG_TYPE_VERSION:
		return &pb.MsgVersion{}
	case pb.MsgType_MSG_TYPE_VERSION_ACCEPTED:
		return &pb.MsgVersionAccepted{}
	case pb.MsgType_MSG_TYPE_VERSION_REJECTED:
		return &pb.MsgVersionRejected{}
	case pb.MsgType_MSG_TYPE_AUTHENTICATE:
		return &pb.MsgAuthenticate{}
	case pb.MsgType_MSG_TYPE_AUTH_ACCEPTED:
		return &pb.MsgAuthAccepted{}
	case pb.MsgType_MSG_TYPE_AUTH_REJECTED:
		return &pb.MsgAuthRejected{}
	case pb.MsgType_MSG_TYPE_OPEN_OUTBOUND_PROXY:
		return &pb.MsgOpenOutboundProxy{}
	case pb.MsgType_MSG_TYPE_GET_DIR_FILES:
		return &pb.MsgGetDirFiles{}
	case pb.MsgType_MSG_TYPE_DIR_FILES:
		return &pb.MsgDirFiles{}
	case pb.MsgType_MSG_TYPE_GET_FILE_META:
		return &pb.MsgGetFileMeta{}
	case pb.MsgType_MSG_TYPE_FILE_META:
		return &pb.MsgFileMeta{}
	case pb.MsgType_MSG_TYPE_GET_FILE:
		return &pb.MsgGetFile{}
	case pb.MsgType_MSG_TYPE_GET_ONLINE_USERS:
		return &pb.MsgGetOnlineUsers{}
	case pb.MsgType_MSG_TYPE_ONLINE_USERS:
		return &pb.MsgOnlineUsers{}
	default:
		return nil
	}
}
