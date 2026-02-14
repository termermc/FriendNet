package cert

import (
	"context"
	"database/sql"

	"friendnet.org/client/storage"
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
	db *sql.DB
}

// NewSqliteStore creates a new SqliteStore instance with the provided database connection.
func NewSqliteStore(db *sql.DB) *SqliteStore {
	return &SqliteStore{db: db}
}

func (s *SqliteStore) GetDer(ctx context.Context, hostname string) ([]byte, error) {
	row := s.db.QueryRowContext(ctx, "select * from server_cert where hostname = ?", hostname)

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
	_, err := s.db.ExecContext(ctx, "insert or replace into server_cert (hostname, cert_der) values (?, ?)", hostname, der)
	return err
}
