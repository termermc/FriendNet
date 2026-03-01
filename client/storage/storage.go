package storage

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"friendnet.org/client/storage/migration"
	"friendnet.org/common"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Storage manages application state storage.
type Storage struct {
	// The underlying SQLite database connection.
	Db *sql.DB

	insertShareIndexStmt *sql.Stmt
}

func (s *Storage) Close() error {
	_ = s.insertShareIndexStmt.Close()
	return s.Db.Close()
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

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	err = common.DoMigrations(db, []common.Migration{
		&migration.M20260208InitialSchema{},
		&migration.M20260222AddLog{},
		&migration.M20260223AddClientCerts{},
		&migration.M20260225AddSettingKv{},
		&migration.M20260301AddSearchIndexes{},
	})
	if err != nil {
		return nil, fmt.Errorf(`failed to apply client database migrations: %w`, err)
	}

	// Set important pragmas.
	startupStmts := []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA synchronous = NORMAL`,
		`PRAGMA busy_timeout = 5000`,
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

	insertShareIndexStmt, err := db.Prepare(`insert into share_index_fts (share, path, is_directory, size) values (?, ?, ?, ?)`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare insert into share_index_fts: %w", err)
	}

	return &Storage{
		Db:                   db,
		insertShareIndexStmt: insertShareIndexStmt,
	}, nil
}

func (s *Storage) Exec(ctx context.Context, sqlCode string, args ...any) (sql.Result, error) {
	return s.Db.ExecContext(ctx, sqlCode, args...)
}

func (s *Storage) Query(ctx context.Context, sqlCode string, args ...any) (*sql.Rows, error) {
	return s.Db.QueryContext(ctx, sqlCode, args...)
}

func (s *Storage) QueryRow(ctx context.Context, sqlCode string, args ...any) *sql.Row {
	return s.Db.QueryRowContext(ctx, sqlCode, args...)
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

	_, err = s.Exec(ctx, `
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
	rows, err := s.Query(ctx, `select * from server`)
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

// GetServerByUuid returns the server record with the specified UUID.
func (s *Storage) GetServerByUuid(ctx context.Context, uuid string) (record ServerRecord, has bool, err error) {
	row := s.QueryRow(ctx, `select * from server where uuid = ?`, uuid)
	return ScanServerRecord(row)
}

// DeleteServerByUuid will delete the server record with the specified UUID.
// Any other records associated with it will also be deleted.
// If the server does not exist, this is a no-op.
func (s *Storage) DeleteServerByUuid(
	ctx context.Context,
	uuid string,
) error {
	_, err := s.Exec(ctx, `delete from server where uuid = ?`, uuid)
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
	uuidRaw, err := uuid.NewV7()
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `insert into share (server, name, path, uuid) values (?, ?, ?, ?)`,
		serverUuid,
		name,
		path,
		uuidRaw.String(),
	)
	return err
}

// GetSharesByServer returns all share records for the server with the specified UUID.
func (s *Storage) GetSharesByServer(ctx context.Context, serverUuid string) ([]ShareRecord, error) {
	rows, err := s.Query(ctx, `select * from share where server = ?`, serverUuid)
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

func (s *Storage) GetShareByServerUuidAndName(ctx context.Context, serverUuid string, name string) (record ShareRecord, has bool, err error) {
	row := s.QueryRow(ctx, `select * from share where server = ? and name = ?`, serverUuid, name)
	return ScanShareRecord(row)
}

func (s *Storage) GetShareByUuid(ctx context.Context, uuid string) (record ShareRecord, has bool, err error) {
	row := s.QueryRow(ctx, `select * from share where uuid = ?`, uuid)
	return ScanShareRecord(row)
}

// DeleteShareByServerUuidAndName deletes the share with the specified server UUID and name.
// If the share does not exist, this is a no-op.
func (s *Storage) DeleteShareByServerUuidAndName(
	ctx context.Context,
	serverUuid string,
	name string,
) error {
	_, err := s.Exec(ctx, `delete from share where server = ? and name = ?`,
		serverUuid,
		name,
	)
	return err
}

// DeleteShareByUuid deletes the share with the specified UUID.
// If the share does not exist, this is a no-op.
func (s *Storage) DeleteShareByUuid(
	ctx context.Context,
	uuid string,
) error {
	_, err := s.Exec(ctx, `delete from share where uuid = ?`,
		uuid,
	)
	return err
}

// ClearShareIndex clears the search index for the share with the specified UUID.
func (s *Storage) ClearShareIndex(ctx context.Context, uuid string) error {
	_, err := s.Exec(ctx, `delete from share_index_fts where share = ?`, uuid)
	if err != nil {
		return fmt.Errorf("failed to clear index for share %q: %w", uuid, err)
	}
	return nil
}

// InsertShareIndex inserts a new entry into the search index for the share with the specified UUID.
func (s *Storage) InsertShareIndex(ctx context.Context, uuid string, path string, isDir bool, size int64) error {
	_, err := s.insertShareIndexStmt.ExecContext(ctx, uuid, path, isDir, size)
	return err
}

// QueryShareIndexByShareUuids searches indexes for the shares with the specified UUIDs.
// The returned records are sorted by path.
//
// The query is a full-text search query.
//
// The limit is the maximum number of records to return.
func (s *Storage) QueryShareIndexByShareUuids(ctx context.Context, uuids []string, query string, limit int64) ([]ShareIndexRecord, error) {
	if len(uuids) == 0 {
		return nil, nil
	}

	// Process the query string.
	// There are a few things we can do to improve the quality of results.
	esc := common.EscapeQueryString(query)
	var q string
	for part := range strings.SplitSeq(esc, " ") {
		if part == "" {
			continue
		}

		q += part + "* "
	}

	ql := `select * from share_index_fts where share in (?` + strings.Repeat(", ?", len(uuids)-1) + `) and (video_fts match ?) order by rank limit ?`
	rows, err := s.Query(ctx, ql, append([]any{uuids}, q, limit)...)
	if err != nil {
		return nil, fmt.Errorf("failed to query share index: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	records := make([]ShareIndexRecord, 0, limit)
	for rows.Next() {
		var rec ShareIndexRecord
		var has bool
		rec, has, err = ScanShareIndexRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan share index record: %w", err)
		}
		if !has {
			break
		}

		records = append(records, rec)
	}

	return records, nil
}

type UpdateServerFields struct {
	Name     *string
	Address  *string
	Room     *common.NormalizedRoomName
	Username *common.NormalizedUsername
	Password *string
}

// UpdateServer updates the specified server record.
// Any nil fields will be left unchanged.
func (s *Storage) UpdateServer(
	ctx context.Context,
	uuid string,
	fields UpdateServerFields,
) error {
	fieldStrs := make([]string, 0, 5)
	vals := make([]any, 0, 5)
	if fields.Name != nil {
		fieldStrs = append(fieldStrs, `name = ?`)
		vals = append(vals, *fields.Name)
	}
	if fields.Address != nil {
		fieldStrs = append(fieldStrs, `address = ?`)
		vals = append(vals, *fields.Address)
	}
	if fields.Room != nil {
		fieldStrs = append(fieldStrs, `room = ?`)
		vals = append(vals, fields.Room.String())
	}
	if fields.Username != nil {
		fieldStrs = append(fieldStrs, `username = ?`)
		vals = append(vals, fields.Username.String())
	}
	if fields.Password != nil {
		fieldStrs = append(fieldStrs, `password = ?`)
		vals = append(vals, *fields.Password)
	}

	// Nothing to update.
	if len(fieldStrs) == 0 {
		return nil
	}

	syntax := fmt.Sprintf(`update server set %s where uuid = ?`, strings.Join(fieldStrs, ", "))
	_, err := s.Exec(ctx, syntax, append(vals, uuid)...)
	return err
}

// SetClientHttpsCert sets the certificate to use for HTTPS for the client.
func (s *Storage) SetClientHttpsCert(ctx context.Context, certPem []byte, keyPem []byte) error {
	_, err := s.Exec(ctx, `insert or replace into client_cert (uuid, cert_pem, key_pem) values ('', ?, ?)`, certPem, keyPem)
	return err
}

// GetClientHttpsCert returns the certificate to use for HTTPS for the client.
// If it is not found, returns sql.ErrNoRows.
func (s *Storage) GetClientHttpsCert(ctx context.Context) (certPem []byte, keyPem []byte, err error) {
	row := s.QueryRow(ctx, `select cert_pem, key_pem from client_cert where uuid = ''`)
	err = row.Scan(&certPem, &keyPem)
	return certPem, keyPem, err
}

// GetCertForServer returns the certificate and private key for the server with the specified UUID.
// If it is not found, returns sql.ErrNoRows.
func (s *Storage) GetCertForServer(ctx context.Context, serverUuid string) (certPem []byte, keyPem []byte, err error) {
	row := s.QueryRow(ctx, `select cert_pem, key_pem from client_cert where server = ?`, serverUuid)
	err = row.Scan(&certPem, &keyPem)
	return certPem, keyPem, err
}
