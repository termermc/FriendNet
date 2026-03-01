package protocol

import (
	pb "friendnet.org/protocol/pb/v1"
	"google.golang.org/protobuf/proto"
)

// MsgTypeToEmptyMsg returns the appropriate empty message for the specified message type.
// The result can be unmarshalled with proto.UnmarshalMerge.
// If the type is unknown, returns nil.
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
	case pb.MsgType_MSG_TYPE_INBOUND_PROXY:
		return &pb.MsgInboundProxy{}
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
	case pb.MsgType_MSG_TYPE_BYE:
		return &pb.MsgBye{}
	case pb.MsgType_MSG_TYPE_ADVERTISE_CONN_METHOD:
		return &pb.MsgAdvertiseConnMethod{}
	case pb.MsgType_MSG_TYPE_ADVERTISE_CONN_METHOD_RESULT:
		return &pb.MsgAdvertiseConnMethodResult{}
	case pb.MsgType_MSG_TYPE_REMOVE_CONN_METHOD:
		return &pb.MsgRemoveConnMethod{}
	case pb.MsgType_MSG_TYPE_CONNECT_TO_ME:
		return &pb.MsgConnectToMe{}
	case pb.MsgType_MSG_TYPE_DIRECT_CONN_RESULT:
		return &pb.MsgDirectConnResult{}
	case pb.MsgType_MSG_TYPE_GET_PUBLIC_IP:
		return &pb.MsgGetPublicIp{}
	case pb.MsgType_MSG_TYPE_PUBLIC_IP:
		return &pb.MsgPublicIp{}
	case pb.MsgType_MSG_TYPE_GET_CLIENT_CONN_METHODS:
		return &pb.MsgGetClientConnMethods{}
	case pb.MsgType_MSG_TYPE_CLIENT_CONN_METHODS:
		return &pb.MsgClientConnMethods{}
	case pb.MsgType_MSG_TYPE_GET_DIRECT_CONN_HANDSHAKE_TOKEN:
		return &pb.MsgGetDirectConnHandshakeToken{}
	case pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE_TOKEN:
		return &pb.MsgDirectConnHandshakeToken{}
	case pb.MsgType_MSG_TYPE_REDEEM_CONN_HANDSHAKE_TOKEN:
		return &pb.MsgRedeemConnHandshakeToken{}
	case pb.MsgType_MSG_TYPE_REDEEM_CONN_HANDSHAKE_TOKEN_RESULT:
		return &pb.MsgRedeemConnHandshakeTokenResult{}
	case pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE:
		return &pb.MsgDirectConnHandshake{}
	case pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE_RESULT:
		return &pb.MsgDirectConnHandshakeResult{}
	case pb.MsgType_MSG_TYPE_CHANGE_ACCOUNT_PASSWORD:
		return &pb.MsgChangeAccountPassword{}
	case pb.MsgType_MSG_TYPE_CLIENT_ONLINE:
		return &pb.MsgClientOnline{}
	case pb.MsgType_MSG_TYPE_CLIENT_OFFLINE:
		return &pb.MsgClientOffline{}
	case pb.MsgType_MSG_TYPE_SEARCH:
		return &pb.MsgSearch{}
	case pb.MsgType_MSG_TYPE_SEARCH_RESULT:
		return &pb.MsgSearchResult{}
	default:
		return nil
	}
}
