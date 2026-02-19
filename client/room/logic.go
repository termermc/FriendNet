package room

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"friendnet.org/client/share"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
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

func (l *LogicImpl) validatePath(bidi protocol.ProtoBidi, path string) (protocol.ProtoPath, bool) {
	protoPath, err := protocol.ValidatePath(path)
	if err != nil {
		_ = bidi.WriteError(pb.ErrType_ERR_TYPE_INVALID_FIELDS, err.Error())
		return protocol.ZeroProtoPath, false
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

func (l *LogicImpl) OnGetDirFiles(_ context.Context, _ *Conn, bidi C2cBidi, msg *protocol.TypedProtoMsg[*pb.MsgGetDirFiles]) error {
	req := msg.Payload
	outerPath, ok := l.validatePath(bidi.ProtoBidi, req.Path)
	if !ok {
		return nil
	}

	if outerPath.IsRoot() {
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

	// Get path within share.
	segments := outerPath.ToSegments()
	shareName := segments[0]
	sharePath, err := protocol.SegmentsToPath(segments[1:])
	if err != nil {
		return err
	}

	sh, has := l.shares.GetByName(shareName)
	if !has {
		return bidi.WriteError(pb.ErrType_ERR_TYPE_FILE_NOT_EXIST, fmt.Sprintf("no such path %q", shareName))
	}

	files, err := sh.DirFiles(sharePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return bidi.WriteError(pb.ErrType_ERR_TYPE_FILE_NOT_EXIST, fmt.Sprintf("no such path %q", shareName))
		}

		return err
	}

	if err = l.sendDirFiles(bidi, files); err != nil {
		return err
	}

	return nil
}
