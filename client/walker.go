package client

import (
	"errors"
	"fmt"
	"io"

	"friendnet.org/client/room"
	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

// WalkPeerPath walks the specified path on a peer.
// The function fn is called for every file found.
// If fn returns false, the walk is aborted.
//
// If the path does not exist, no items will be crawled, and a nil error will be returned.
func WalkPeerPath(conn room.VirtualC2cConn, path common.ProtoPath, fn func(path common.ProtoPath, meta *pb.MsgFileMeta) bool) error {
	toCrawl := []common.ProtoPath{path}
	for len(toCrawl) > 0 {
		dirPath := toCrawl[0]
		toCrawl = toCrawl[1:]

		err := func() error {
			stream, nextErr := conn.GetDirFiles(dirPath)
			if nextErr != nil {
				if protoErr, ok := errors.AsType[protocol.ProtoMsgError](nextErr); ok {
					// File might change while we are crawling it.
					// Just skip errors that might happen if the directory is changing.
					if protoErr.Msg.Type == pb.ErrType_ERR_TYPE_FILE_NOT_EXIST ||
						protoErr.Msg.Type == pb.ErrType_ERR_TYPE_PATH_NOT_DIRECTORY {
						return nil
					}
				}

				return fmt.Errorf(`failed to crawl path %q: %w`, dirPath, nextErr)
			}
			defer func() {
				_ = stream.Close()
			}()

			for {
				next, err := stream.ReadNext()
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					return err
				}

				for _, file := range next.Files {
					filePath := common.JoinPaths(dirPath, common.UncheckedCreateProtoPath("/"+file.Name))
					if file.IsDir {
						toCrawl = append(toCrawl, filePath)
					}
					doNext := fn(dirPath, file)
					if !doNext {
						return io.EOF
					}
				}
			}

			return nil
		}()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return err
		}
	}

	return nil
}
