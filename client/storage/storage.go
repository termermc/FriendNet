package storage

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"

	"friendnet.org/common"
	"friendnet.org/server/storage/migration"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Storage manages application state storage.
type Storage struct {
	db *sql.DB
}

func (s *Storage) Close() error {
	return s.db.Close()
}

// NewStorage creates a new storage instance using the specified DB path.
//
//goland:noinspection SqlNoDataSourceInspection
func NewStorage(path string) (*Storage, error) {
	if path == "" {
		panic("path is required for storage")
	}

	// Resolve full path.
	var err error
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve storage path: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = db.Close()
		}
	}()

	err = common.DoMigrations(db, []common.Migration{
		&migration.M20260208InitialSchema{},
	})
	if err != nil {
		return nil, fmt.Errorf(`failed to apply client database migrations: %w`, err)
	}

	// Set important pragmas.
	startupStmts := []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA synchronous = NORMAL`,
	}
	for _, stmt := range startupStmts {
		_, err = db.Exec(stmt)
		if err != nil {
			return nil, fmt.Errorf("failed to run startup statement: %q: %w", stmt, err)
		}
	}

	// Check database integrity.
	icRes := db.QueryRow(`PRAGMA integrity_check`)
	var icVal string
	err = icRes.Scan(&icVal)
	if err != nil {
		return nil, fmt.Errorf("failed to check database integrity: %w", err)
	}

	if icVal != "ok" {
		return nil, fmt.Errorf("database integrity check failed: %s", icVal)
	}

	return &Storage{
		db: db,
	}, nil
}

// CreateServer creates a new server record.
func (s *Storage) CreateServer(
	ctx context.Context,
	name string,
	address string,
	room common.NormalizedRoomName,
	username common.NormalizedUsername,
	password string,
) (string, error) {
	uuidRaw, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf(`failed to generate UUIDv7: %w`, err)
	}

	id := uuidRaw.String()

	_, err = s.db.ExecContext(ctx, `
insert into server
(
	uuid,
	name,
	address,
	room,
	username,
	password
) values (?, ?, ?, ?, ?, ?)
	`,
		id,
		name,
		address,
		room.String(),
		username.String(),
		password,
	)
	if err != nil {
		return "", fmt.Errorf(`failed to create server: %w`, err)
	}

	return id, nil
}

// GetServers returns all server records.
func (s *Storage) GetServers(ctx context.Context) ([]ServerRecord, error) {
	rows, err := s.db.QueryContext(ctx, `select * from server`)
	if err != nil {
		return nil, fmt.Errorf(`failed to query servers: %w`, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	records := make([]ServerRecord, 0)

	for rows.Next() {
		var record ServerRecord
		record, _, err = ScanServerRecord(rows)
		if err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	return records, nil
}

// DeleteServerByUuid will delete the server record with the specified UUID.
// Any other records associated with it will also be deleted.
// If the server does not exist, this is a no-op.
func (s *Storage) DeleteServerByUuid(
	ctx context.Context,
	uuid string,
) error {
	_, err := s.db.ExecContext(ctx, `delete from server where uuid = ?`, uuid)
	if err != nil {
		return fmt.Errorf(`failed to delete server with UUID %q: %w`, uuid, err)
	}
	return nil
}

// CreateShare creates a new share for a server.
// If an existing share with the same name exists, it will be replaced.
func (s *Storage) CreateShare(
	ctx context.Context,
	serverUuid string,
	name string,
	path string,
) error {
	_, err := s.db.ExecContext(ctx, `insert into share (server, name, path) values (?, ?, ?)`,
		serverUuid,
		name,
		path,
	)
	return err
}

// GetSharesByServer returns all share records for the server with the specified UUID.
func (s *Storage) GetSharesByServer(ctx context.Context, serverUuid string) ([]ShareRecord, error) {
	rows, err := s.db.QueryContext(ctx, `select * from share where server = ?`, serverUuid)
	if err != nil {
		return nil, fmt.Errorf(`failed to query shares for server %q: %w`, serverUuid, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	records := make([]ShareRecord, 0)
	for rows.Next() {
		var record ShareRecord
		record, _, err = ScanShareRecord(rows)
		if err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	return records, nil
}

// DeleteShareByServerAndName deletes the server with the specified server UUID and name.
// If the share does not exist, this is a no-op.
func (s *Storage) DeleteShareByServerAndName(
	ctx context.Context,
	room common.NormalizedRoomName,
	username common.NormalizedUsername,
) error {
	_, err := s.db.ExecContext(ctx, `delete from share where server = ? and name = ?`,
		room.String(),
		username.String(),
	)
	return err
}
