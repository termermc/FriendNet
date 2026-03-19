//go:build linux

package fsys

import (
	"errors"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func withStatfsMock(t *testing.T, fn func(path string, st *unix.Statfs_t) error, test func()) {
	t.Helper()
	prev := statfsFn
	statfsFn = fn
	t.Cleanup(func() { statfsFn = prev })
	test()
}

func TestGetFilenameReplacerForPath_LenientOnKnownLocalFS(t *testing.T) {
	withStatfsMock(t, func(path string, st *unix.Statfs_t) error {
		st.Type = extSuperMagic
		return nil
	}, func() {
		r, err := getFilenameReplacerForPath("/any/path")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}

		// Lenient should NOT replace ':' or '*' (strict would).
		in := `a:b*c`
		got := r.ReplaceFilename(in)
		if got != in {
			t.Fatalf("expected lenient behavior; got %q want %q", got, in)
		}

		// Lenient MUST still replace '/' since it's a path separator.
		in2 := `a/b`
		got2 := r.ReplaceFilename(in2)
		if got2 != `a／b` {
			t.Fatalf("got %q want %q", got2, `a／b`)
		}
	})
}

func TestGetFilenameReplacerForPath_StrictOnCIFS(t *testing.T) {
	withStatfsMock(t, func(path string, st *unix.Statfs_t) error {
		st.Type = cifsMagic
		return nil
	}, func() {
		r, err := getFilenameReplacerForPath("/mnt/share")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}

		in := `a:b*c?d`
		got := r.ReplaceFilename(in)
		want := `a꞉b∗c？d`
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})
}

func TestGetFilenameReplacerForPath_UnknownFS_Strict(t *testing.T) {
	withStatfsMock(t, func(path string, st *unix.Statfs_t) error {
		st.Type = 0xDEADBEEF
		return nil
	}, func() {
		r, err := getFilenameReplacerForPath("/somewhere")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}

		in := `a:b`
		got := r.ReplaceFilename(in)
		want := `a꞉b`
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})
}

func TestGetFilenameReplacerForPath_WalksUpToExistingParent(t *testing.T) {
	// Simulate: /a/b/c doesn't exist; /a/b exists; statfs works there.
	existing := filepath.Clean("/a/b")

	withStatfsMock(t, func(path string, st *unix.Statfs_t) error {
		if filepath.Clean(path) == existing {
			st.Type = tmpfsMagic
			return nil
		}
		return unix.ENOENT
	}, func() {
		r, err := getFilenameReplacerForPath("/a/b/c/d/e")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}

		// tmpfs => lenient
		in := `a:b`
		got := r.ReplaceFilename(in)
		if got != in {
			t.Fatalf("expected lenient behavior; got %q want %q", got, in)
		}
	})
}

func TestGetFilenameReplacerForPath_StatfsErrorReturned(t *testing.T) {
	someErr := errors.New("boom")
	withStatfsMock(t, func(path string, st *unix.Statfs_t) error {
		return someErr
	}, func() {
		_, err := getFilenameReplacerForPath("/x")
		if !errors.Is(err, someErr) {
			t.Fatalf("expected %v, got %v", someErr, err)
		}
	})
}
