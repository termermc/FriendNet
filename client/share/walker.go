package share

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"friendnet.org/common"
	pb "friendnet.org/protocol/pb/v1"
)

// WalkShareDir walks files in a share's directory.
// The path can be root.
// fn will NOT be called for the initial path.
// If fn returns an error, it will be returned immediately and walking will stop.
// If fn returns false, walking will stop without an error.
// If fn returns true, walking will continue for another file.
func WalkShareDir(share Share, path common.ProtoPath, fn func(path common.ProtoPath, meta *pb.MsgFileMeta) (bool, error)) error {
	dirs := []string{path.String()}

	for len(dirs) > 0 {
		dir := dirs[0]
		dirs = dirs[1:]

		files, err := share.DirFiles(common.UncheckedCreateProtoPath(dir))
		if err != nil {
			// Skip files that were removed or we do not have permission to access.
			if os.IsNotExist(err) || os.IsPermission(err) || errors.Is(err, syscall.ESRCH) {
				continue
			}

			return fmt.Errorf("failed to read share %q directory %q before walking: %w", share.Name(), dir, err)
		}
		for _, file := range files {
			var path string
			if dir == "/" {
				path = "/" + file.Name
			} else {
				path = dir + "/" + file.Name
			}

			if file.IsDir {
				dirs = append(dirs, path)
			}

			ok, err := fn(common.UncheckedCreateProtoPath(path), file)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
		}
	}

	return nil
}
