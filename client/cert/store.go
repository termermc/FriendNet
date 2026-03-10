package cert

import (
	"context"

	"friendnet.org/client/storage"
	"friendnet.org/common"
)

// Store is a certificate store that associates hostnames with DER-encoded leaf certificates.
type Store interface {
	// GetDer returns the stored DER-encoded leaf certificate for the specified hostname, or nil if none exists.
	// Hostname is case-insensitive.
	GetDer(ctx context.Context, hostname string) ([]byte, error)

	// PutDer stores the DER-encoded leaf certificate for the specified hostname.
	// Overrides any existing entry.
	// Hostname is case-insensitive.
	PutDer(ctx context.Context, hostname string, der []byte) error
}

// SqliteStore implements Store using the client's SQLite instance.
// It relies on the migrations in the migrations module, so it is not standalone.
type SqliteStore struct {
	store *storage.Storage
}

// NewSqliteStore creates a new SqliteStore instance with the provided storage.
func NewSqliteStore(store *storage.Storage) *SqliteStore {
	return &SqliteStore{store: store}
}

func (s *SqliteStore) GetDer(ctx context.Context, hostname string) ([]byte, error) {
	hostname = common.NormalizeHostname(hostname)

	row := s.store.QueryRow(ctx, "select * from server_cert where hostname = ?", hostname)

	record, has, err := storage.ScanServerCertRecord(row)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil
	}

	return record.CertDer, nil
}

func (s *SqliteStore) PutDer(ctx context.Context, hostname string, der []byte) error {
	hostname = common.NormalizeHostname(hostname)

	_, err := s.store.Exec(ctx, "insert or replace into server_cert (hostname, cert_der) values (?, ?)", hostname, der)
	return err
}

// DeleteDer deletes the certificate for the specified hostname.
// It returns true if the hostname had a certificate and it was deleted.
func (s *SqliteStore) DeleteDer(ctx context.Context, hostname string) (bool, error) {
	hostname = common.NormalizeHostname(hostname)

	res, err := s.store.Exec(ctx, "delete from server_cert where hostname = ?", hostname)
	if err != nil {
		return false, err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}

	return rowsAffected > 0, nil
}
