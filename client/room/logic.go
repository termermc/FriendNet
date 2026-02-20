package room

import (
	"context"
	"errors"
	"io"
	"io/fs"

	"friendnet.org/client/share"
	"friendnet.org/protocol"
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

// resolveShareAndPath returns share and path within share based on the specified path.
// If the path is root, share will be nil.
// If shareNotFound is true, the share was not found.
func (l *LogicImpl) resolveShareAndPath(path protocol.ProtoPath) (shareOrNil share.Share, sharePath protocol.ProtoPath, shareNotFound bool, err error) {
	if path.IsRoot() {
		return
	}

	// Get path within share.
	segments := path.ToSegments()
	shareName := segments[0]
	sharePath, err = protocol.SegmentsToPath(segments[1:])
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
		var streamErr *quic.StreamError
		if errors.As(err, &streamErr) {
			// If the other side closed, we can just quit.
			return nil
		}

		return err
	}

	return nil
}
