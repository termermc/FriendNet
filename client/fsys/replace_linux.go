//go:build linux

package fsys

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

var statfsFn = unix.Statfs

func getFilenameReplacerForPath(path string) (FilenameReplacer, error) {
	st, err := statfsNearestExisting(path)
	if err != nil {
		return StrictReplacer, err
	}

	switch uint64(st.Type) {
	case extSuperMagic, xfsSuperMagic, btrfsSuperMagic, f2fsSuperMagic,
		tmpfsMagic, overlayfsMagic:
		return lenientReplacerPOSIX, nil
	case nfsSuperMagic, cifsMagic:
		return StrictReplacer, nil
	default:
		return StrictReplacer, nil
	}
}

func statfsNearestExisting(path string) (*unix.Statfs_t, error) {
	p := filepath.Clean(path)

	for {
		var st unix.Statfs_t
		err := statfsFn(p, &st)
		if err == nil {
			return &st, nil
		}
		if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ENOTDIR) {
			parent := filepath.Dir(p)
			if parent == p {
				return nil, err
			}
			p = parent
			continue
		}
		return nil, err
	}
}

var lenientReplacerPOSIX FilenameReplacer = func(str string) string {
	var b strings.Builder
	b.Grow(len(str))
	for _, r := range str {
		switch r {
		case 0:
			b.WriteRune('␀')
		case '/':
			b.WriteRune('／')
		default:
			if r == utf8.RuneError {
				b.WriteRune('�')
				continue
			}
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		out = "_"
	}
	return out
}

// Filesystem magic numbers from Linux uapi headers (linux/magic.h).
const (
	extSuperMagic   = 0xEF53
	xfsSuperMagic   = 0x58465342
	btrfsSuperMagic = 0x9123683E
	f2fsSuperMagic  = 0xF2F52010
	tmpfsMagic      = 0x01021994
	overlayfsMagic  = 0x794C7630
	nfsSuperMagic   = 0x6969
	cifsMagic       = 0xFF534D42
)
