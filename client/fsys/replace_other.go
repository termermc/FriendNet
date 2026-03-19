//go:build !linux

package fsys

func getFilenameReplacerForPath(path string) (FilenameReplacer, error) {
	return StrictReplacer, nil
}
