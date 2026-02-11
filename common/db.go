package common

type Scannable interface {
	Scan(dest ...any) error
}
