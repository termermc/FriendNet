package common

import "io"

// EofReadCloser is an io.ReadCloser that always returns EOF.
// Its Close always returns nil.
type EofReadCloser struct{}

func (EofReadCloser) Read([]byte) (int, error) {
	return 0, io.EOF
}
func (EofReadCloser) Close() error {
	return nil
}

var _ io.ReadCloser = EofReadCloser{}

// LimitReadCloser wraps an io.ReadCloser with io.LimitReader while still supporting the original io.ReadCloser's Close method.
type LimitReadCloser struct {
	reader io.Reader
	closer io.Closer
}

func NewLimitReadCloser(rc io.ReadCloser, limit int64) LimitReadCloser {
	return LimitReadCloser{
		reader: io.LimitReader(rc, limit),
		closer: rc,
	}
}

// Read wraps io.Reader's Read.
func (l LimitReadCloser) Read(p []byte) (int, error) {
	return l.reader.Read(p)
}

// Close wraps io.ReadCloser's Close.
func (l LimitReadCloser) Close() error {
	return l.closer.Close()
}

var _ io.ReadCloser = LimitReadCloser{}
