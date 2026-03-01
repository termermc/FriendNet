package room

import (
	"context"
	"errors"
	"io"
	"io/fs"

	"friendnet.org/client/share"
	"friendnet.org/common"
	"friendnet.org/protocol"
	v1 "friendnet.org/protocol/pb/clientrpc/v1"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
)

// Logic exposes handlers for incoming client messages, both S2C and C2C.
//
// Each handler is provided with the information it needs to return a response.
// Handlers must not hold references to the bidi or connection outside the handler.
// Handlers do not need to close bidis; they are closed by the caller after the handler returns.
type Logic interface {
	io.Closer

	// OnPing handles an incoming ping request.
	//
	// S2C, C2C
	OnPing(ctx context.Context, room *Conn, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgPing]) error

	// OnGetDirFiles handles an incoming get dir files request.
	//
	// C2C
	OnGetDirFiles(ctx context.Context, room *Conn, bidi C2cBidi, msg *protocol.TypedProtoMsg[*pb.MsgGetDirFiles]) error

	// OnGetFileMeta handles an incoming get file meta request.
	//
	// C2C
	OnGetFileMeta(ctx context.Context, room *Conn, bidi C2cBidi, msg *protocol.TypedProtoMsg[*pb.MsgGetFileMeta]) error

	// OnGetFile handles an incoming get file request.
	//
	// C2C
	OnGetFile(ctx context.Context, room *Conn, bidi C2cBidi, msg *protocol.TypedProtoMsg[*pb.MsgGetFile]) error

	// OnConnectToMe handles an incoming connect to me request.
	//
	// C2C
	OnConnectToMe(ctx context.Context, room *Conn, bidi C2cBidi, msg *protocol.TypedProtoMsg[*pb.MsgConnectToMe]) error

	// OnClientOnline handles an incoming client online notification.
	//
	// S2C
	OnClientOnline(ctx context.Context, room *Conn, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgClientOnline]) error

	// OnClientOffline handles an incoming client offline notification.
	//
	// S2C
	OnClientOffline(ctx context.Context, room *Conn, bidi protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgClientOffline]) error
}

// LogicImpl implements Logic.
type LogicImpl struct {
	shares *share.ServerShareManager
}

var _ Logic = (*LogicImpl)(nil)

func NewLogicImpl(shares *share.ServerShareManager) *LogicImpl {
	return &LogicImpl{
		shares: shares,
	}
}

func (l *LogicImpl) validatePath(bidi protocol.ProtoBidi, path string) (common.ProtoPath, bool) {
	protoPath, err := common.ValidatePath(path)
	if err != nil {
		_ = bidi.WriteError(pb.ErrType_ERR_TYPE_INVALID_FIELDS, err.Error())
		return common.ZeroProtoPath, false
	}
	return protoPath, true
}

func (l *LogicImpl) Close() error {
	return l.shares.Close()
}

func (l *LogicImpl) OnPing(_ context.Context, _ *Conn, bidi protocol.ProtoBidi, _ *protocol.TypedProtoMsg[*pb.MsgPing]) error {
	return bidi.Write(pb.MsgType_MSG_TYPE_PONG, &pb.MsgPong{})
}

func (l *LogicImpl) sendDirFiles(bidi C2cBidi, files []*pb.MsgFileMeta) error {
	const pageSize = 50

	// Send paginated.
	sent := 0
	for sent < len(files) {
		end := sent + pageSize
		if end > len(files) {
			end = len(files)
		}

		err := bidi.Write(pb.MsgType_MSG_TYPE_DIR_FILES, &pb.MsgDirFiles{
			Files: files[sent:end],
		})
		if err != nil {
			return err
		}

		sent += pageSize
	}

	return nil
}

// resolveShareAndPath returns share and path within share based on the specified path.
// If the path is root, share will be nil.
// If shareNotFound is true, the share was not found.
func (l *LogicImpl) resolveShareAndPath(path common.ProtoPath) (shareOrNil share.Share, sharePath common.ProtoPath, shareNotFound bool, err error) {
	if path.IsRoot() {
		return
	}

	// Get path within share.
	segments := path.ToSegments()
	shareName := segments[0]
	sharePath, err = common.SegmentsToPath(segments[1:])
	if err != nil {
		return
	}

	sh, has := l.shares.GetByName(shareName)
	if !has {
		shareNotFound = true
		return
	}

	shareOrNil = sh
	return
}

func (l *LogicImpl) OnGetDirFiles(_ context.Context, _ *Conn, bidi C2cBidi, msg *protocol.TypedProtoMsg[*pb.MsgGetDirFiles]) error {
	req := msg.Payload
	reqPath, ok := l.validatePath(bidi.ProtoBidi, req.Path)
	if !ok {
		return nil
	}

	shareOrNil, sharePath, shareNotFound, err := l.resolveShareAndPath(reqPath)
	if err != nil {
		return err
	}
	if shareNotFound {
		return bidi.WriteFileNotExistError(reqPath.String())
	}

	if shareOrNil == nil {
		// List all shares.
		shares := l.shares.GetAll()
		metas := make([]*pb.MsgFileMeta, len(shares))
		for i, sh := range shares {
			metas[i] = &pb.MsgFileMeta{
				Name:  sh.Name(),
				IsDir: true,
				Size:  0,
			}
		}
		return l.sendDirFiles(bidi, metas)
	}

	files, err := shareOrNil.DirFiles(sharePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return bidi.WriteFileNotExistError(reqPath.String())
		}

		return err
	}

	if err = l.sendDirFiles(bidi, files); err != nil {
		return err
	}

	return nil
}

func (l *LogicImpl) OnGetFileMeta(_ context.Context, _ *Conn, bidi C2cBidi, msg *protocol.TypedProtoMsg[*pb.MsgGetFileMeta]) error {
	req := msg.Payload
	reqPath, ok := l.validatePath(bidi.ProtoBidi, req.Path)
	if !ok {
		return nil
	}

	shareOrNil, sharePath, shareNotFound, err := l.resolveShareAndPath(reqPath)
	if err != nil {
		return err
	}
	if shareNotFound {
		return bidi.WriteFileNotExistError(reqPath.String())
	}

	var meta *pb.MsgFileMeta

	if shareOrNil == nil {
		meta = &pb.MsgFileMeta{
			Name:  "/",
			IsDir: true,
			Size:  0,
		}
	} else {
		meta, err = shareOrNil.GetFileMeta(sharePath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return bidi.WriteFileNotExistError(reqPath.String())
			}
			return err
		}
	}

	return bidi.Write(pb.MsgType_MSG_TYPE_FILE_META, meta)
}

func (l *LogicImpl) OnGetFile(_ context.Context, _ *Conn, bidi C2cBidi, msg *protocol.TypedProtoMsg[*pb.MsgGetFile]) error {
	req := msg.Payload
	reqPath, ok := l.validatePath(bidi.ProtoBidi, req.Path)
	if !ok {
		return nil
	}

	shareOrNil, sharePath, shareNotFound, err := l.resolveShareAndPath(reqPath)
	if err != nil {
		return err
	}
	if shareNotFound {
		return bidi.WriteFileNotExistError(reqPath.String())
	}

	var meta *pb.MsgFileMeta
	var reader io.ReadCloser

	if shareOrNil == nil {
		meta = &pb.MsgFileMeta{
			Name:  "/",
			IsDir: true,
			Size:  0,
		}
	} else {
		meta, reader, err = shareOrNil.GetFile(
			sharePath,
			msg.Payload.Offset,
			msg.Payload.Limit,
		)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return bidi.WriteFileNotExistError(reqPath.String())
			}
			return err
		}
	}

	err = bidi.Write(pb.MsgType_MSG_TYPE_FILE_META, meta)
	if err != nil {
		return err
	}

	// No data to send if this is a directory.
	if meta.IsDir {
		return nil
	}

	_, err = io.Copy(bidi.ProtoBidi.Stream, reader)
	if err != nil {
		if _, is := errors.AsType[*quic.StreamError](err); is {
			// If the other side closed, we can just quit.
			return nil
		}

		return err
	}

	return nil
}

func (l *LogicImpl) OnConnectToMe(ctx context.Context, room *Conn, bidi C2cBidi, _ *protocol.TypedProtoMsg[*pb.MsgConnectToMe]) error {
	if room.directMgr.IsDisabled() {
		return bidi.Write(pb.MsgType_MSG_TYPE_DIRECT_CONN_RESULT, &pb.MsgDirectConnResult{
			Result: pb.ConnResult_CONN_RESULT_DID_NOT_TRY,
		})
	}

	_, result, err := room.tryConnectToPeer(ctx, bidi.Username)
	if err != nil && result == pb.ConnResult_CONN_RESULT_INTERNAL_ERROR {
		room.logger.Error("internal error while connecting to peer",
			"service", "room.LogicImpl",
			"room", room.RoomName.String(),
			"peer", bidi.Username.String(),
			"err", err,
		)
	}

	return bidi.Write(pb.MsgType_MSG_TYPE_DIRECT_CONN_RESULT, &pb.MsgDirectConnResult{
		Result: result,
	})
}

func (l *LogicImpl) OnClientOnline(_ context.Context, room *Conn, _ protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgClientOnline]) error {
	info := msg.Payload.Info

	room.eventPublisher.Publish(&v1.Event{
		Type: v1.Event_TYPE_CLIENT_ONLINE,
		ClientOnline: &v1.Event_ClientOnline{
			Info: &v1.OnlineUserInfo{
				Username: info.Username,
			},
		},
	})
	return nil
}

func (l *LogicImpl) OnClientOffline(_ context.Context, room *Conn, _ protocol.ProtoBidi, msg *protocol.TypedProtoMsg[*pb.MsgClientOffline]) error {
	username, usernameOk := common.NormalizeUsername(msg.Payload.Username)
	if !usernameOk {
		return errors.New("OnClientOffline: server sent invalid username")
	}

	room.eventPublisher.Publish(&v1.Event{
		Type: v1.Event_TYPE_CLIENT_OFFLINE,
		ClientOffline: &v1.Event_ClientOffline{
			Username: username.String(),
		},
	})
	return nil
}
