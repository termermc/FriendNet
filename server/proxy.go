//go:build old

package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

func proxyDirFiles(registry *ClientRegistry) protocol.ServerGetDirFilesHandler {
	return func(_ context.Context, client *protocol.ProtoServerClient, bidi protocol.ProtoBidi, msg *pb.MsgGetDirFiles) error {
		info, ok := registry.Info(client)
		if !ok {
			return writeProxyError(bidi, pb.ErrType_ERR_TYPE_INTERNAL, fmt.Errorf("missing client info"))
		}
		if msg.User == "" {
			return writeProxyError(bidi, pb.ErrType_ERR_TYPE_INVALID_FIELDS, fmt.Errorf("user is required"))
		}

		target := registry.Lookup(info.Room, msg.User)
		if target == nil {
			return writeProxyError(bidi, pb.ErrType_ERR_TYPE_INVALID_FIELDS, fmt.Errorf("unknown user %q", msg.User))
		}

		files, err := target.GetDirFiles(msg.User, msg.Path)
		if err != nil {
			return writeProxyErrorFromErr(bidi, err)
		}

		return bidi.Write(pb.MsgType_MSG_TYPE_DIR_FILES, &pb.MsgDirFiles{Files: files})
	}
}

func proxyFileMeta(registry *ClientRegistry) protocol.ServerGetFileMetaHandler {
	return func(_ context.Context, client *protocol.ProtoServerClient, bidi protocol.ProtoBidi, msg *pb.MsgGetFileMeta) error {
		info, ok := registry.Info(client)
		if !ok {
			return writeProxyError(bidi, pb.ErrType_ERR_TYPE_INTERNAL, fmt.Errorf("missing client info"))
		}
		if msg.User == "" {
			return writeProxyError(bidi, pb.ErrType_ERR_TYPE_INVALID_FIELDS, fmt.Errorf("user is required"))
		}

		target := registry.Lookup(info.Room, msg.User)
		if target == nil {
			return writeProxyError(bidi, pb.ErrType_ERR_TYPE_INVALID_FIELDS, fmt.Errorf("unknown user %q", msg.User))
		}

		meta, err := target.GetFileMeta(msg.User, msg.Path)
		if err != nil {
			return writeProxyErrorFromErr(bidi, err)
		}

		return bidi.Write(pb.MsgType_MSG_TYPE_FILE_META, meta)
	}
}

func proxyFile(registry *ClientRegistry) protocol.ServerGetFileHandler {
	return func(_ context.Context, client *protocol.ProtoServerClient, bidi protocol.ProtoBidi, msg *pb.MsgGetFile) error {
		info, ok := registry.Info(client)
		if !ok {
			return writeProxyError(bidi, pb.ErrType_ERR_TYPE_INTERNAL, fmt.Errorf("missing client info"))
		}
		if msg.User == "" {
			return writeProxyError(bidi, pb.ErrType_ERR_TYPE_INVALID_FIELDS, fmt.Errorf("user is required"))
		}

		target := registry.Lookup(info.Room, msg.User)
		if target == nil {
			return writeProxyError(bidi, pb.ErrType_ERR_TYPE_INVALID_FIELDS, fmt.Errorf("unknown user %q", msg.User))
		}

		meta, reader, err := target.GetFile(msg.User, msg.Path, msg.Offset, msg.Limit)
		if err != nil {
			return writeProxyErrorFromErr(bidi, err)
		}
		defer func() {
			_ = reader.Close()
		}()

		if err := bidi.Write(pb.MsgType_MSG_TYPE_FILE_META, meta); err != nil {
			return err
		}

		if _, err := io.Copy(bidi.Stream, reader); err != nil && !errors.Is(err, io.EOF) {
			return err
		}

		return nil
	}
}

func writeProxyErrorFromErr(bidi protocol.ProtoBidi, err error) error {
	var protoErr protocol.ProtoMsgError
	if errors.As(err, &protoErr) && protoErr.Msg != nil {
		return bidi.Write(pb.MsgType_MSG_TYPE_ERROR, protoErr.Msg)
	}

	return writeProxyError(bidi, pb.ErrType_ERR_TYPE_INTERNAL, err)
}

func writeProxyError(bidi protocol.ProtoBidi, errType pb.ErrType, err error) error {
	message := err.Error()
	return bidi.Write(pb.MsgType_MSG_TYPE_ERROR, &pb.MsgError{
		Type:    errType,
		Message: &message,
	})
}
