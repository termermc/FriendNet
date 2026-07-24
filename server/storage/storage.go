package storage

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"friendnet.org/common"
	"friendnet.org/server/storage/migration"
	_ "modernc.org/sqlite"
)

// ErrRecordExists is returned when trying to create a duplicate record.
var ErrRecordExists = fmt.Errorf("record already exists")

// Storage manages application state storage.
type Storage struct {
	// The underlying SQLite database connection.
	Db *sql.DB
}

func (s *Storage) Close() error {
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
		&migration.M20260723ServersideSearchBlocklist{},
	})
	if err != nil {
		return nil, fmt.Errorf(`failed to apply server database migrations: %w`, err)
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
		Db: db,
	}, nil
}

// CreateRoom creates a new room record.
// If the room already exists, returns ErrRecordExists.
func (s *Storage) CreateRoom(ctx context.Context, room common.NormalizedRoomName) error {
	_, err := s.Db.ExecContext(ctx, `insert into room (name) values (?)`, room.String())
	if err != nil {
		if strings.Contains(err.Error(), "constraint") {
			return ErrRecordExists
		}

		return fmt.Errorf(`failed to create room %q: %w`, room.String(), err)
	}
	return nil
}

// GetRoomByName returns the room record with the specified name, if any.
// If the room does not exist, `has` will be false.
func (s *Storage) GetRoomByName(ctx context.Context, room common.NormalizedRoomName) (record RoomRecord, has bool, err error) {
	row := s.Db.QueryRowContext(ctx, `select * from room where name = ?`, room.String())
	return ScanRoomRecord(row)
}

// GetRooms returns all room records.
func (s *Storage) GetRooms(ctx context.Context) ([]RoomRecord, error) {
	rows, err := s.Db.QueryContext(ctx, `select * from room`)
	if err != nil {
		return nil, fmt.Errorf(`failed to query rooms: %w`, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	records := make([]RoomRecord, 0)

	for rows.Next() {
		var record RoomRecord
		record, _, err = ScanRoomRecord(rows)
		if err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	return records, nil
}

// DeleteRoomByName will delete the room record with the specified name.
// Any accounts associated with it will also be deleted.
// If the room does not exist, this is a no-op.
func (s *Storage) DeleteRoomByName(
	ctx context.Context,
	room common.NormalizedRoomName,
) error {
	_, err := s.Db.ExecContext(ctx, `delete from room where name = ?`, room.String())
	if err != nil {
		return fmt.Errorf(`failed to delete room with name %q: %w`, room.String(), err)
	}
	return nil
}

// CreateAccount creates a new account record.
func (s *Storage) CreateAccount(
	ctx context.Context,
	room common.NormalizedRoomName,
	username common.NormalizedUsername,
	passwordHash string,
) error {
	_, err := s.Db.ExecContext(ctx, `insert into account (room, username, password_hash) values (?, ?, ?)`,
		room.String(),
		username.String(),
		passwordHash,
	)
	if err != nil {
		if strings.Contains(err.Error(), "constraint") {
			return ErrRecordExists
		}

		return err
	}

	return nil
}

// GetAccountByRoomAndUsername returns the account record with the specified room and username, if any.
func (s *Storage) GetAccountByRoomAndUsername(
	ctx context.Context,
	room common.NormalizedRoomName,
	username common.NormalizedUsername,
) (record AccountRecord, has bool, err error) {
	row := s.Db.QueryRowContext(ctx, `select * from account where room = ? and username = ?`,
		room.String(),
		username.String(),
	)
	return ScanAccountRecord(row)
}

// GetAccountsByRoom returns all account records for the specified room.
func (s *Storage) GetAccountsByRoom(ctx context.Context, room common.NormalizedRoomName) ([]AccountRecord, error) {
	rows, err := s.Db.QueryContext(ctx, `select * from account where room = ?`, room.String())
	if err != nil {
		return nil, fmt.Errorf(`failed to query accounts for room %q: %w`, room.String(), err)
	}
	defer func() {
		_ = rows.Close()
	}()

	records := make([]AccountRecord, 0)
	for rows.Next() {
		var record AccountRecord
		record, _, err = ScanAccountRecord(rows)
		if err != nil {
			return nil, err
		}

		records = append(records, record)
	}

	return records, nil
}

// UpdateAccountPasswordHash updates the password hash of the account with the specified room and username.
// If the account does not exist, this is a no-op.
func (s *Storage) UpdateAccountPasswordHash(
	ctx context.Context,
	room common.NormalizedRoomName,
	username common.NormalizedUsername,
	passwordHash string,
) error {
	_, err := s.Db.ExecContext(ctx, `update account set password_hash = ? where room = ? and username = ?`,
		passwordHash,
		room.String(),
		username.String(),
	)
	if err != nil {
		return fmt.Errorf(`failed to update password hash for account with room %q and username %q: %w`,
			room.String(),
			username.String(),
			err,
		)
	}
	return nil
}

// DeleteAccountByRoomAndUsername deletes the account with the specified room and username.
// If the account does not exist, this is a no-op.
func (s *Storage) DeleteAccountByRoomAndUsername(
	ctx context.Context,
	room common.NormalizedRoomName,
	username common.NormalizedUsername,
) error {
	_, err := s.Db.ExecContext(ctx, `delete from account where room = ? and username = ?`,
		room.String(),
		username.String(),
	)
	if err != nil {
		return fmt.Errorf(`failed to delete account with room %q and username %q: %w`,
			room.String(),
			username.String(),
			err,
		)
	}
	return nil
}

// AddKeywordToBlacklist adds a blacklist policy for a keyword to the persistent blacklist.
// If the room is not specified or does not exist, the policy applies serverwide.
// If the room does not exist or the keyword is empty, this is a no-op.
func (s *Storage) AddKeywordToBlacklist(ctx context.Context, room common.NormalizedRoomName, keyword string) error {
	if len(keyword) == 0 {
		return nil
	}

	if room.IsZero() {
		_, err := s.Db.ExecContext(ctx, `insert into word_blocklist (word) values (?)`, keyword)
		if err != nil {
			return fmt.Errorf("failed to add keyword \"%s\" to global blacklist: %w", keyword, err)
		}
	} else {
		if _, has, _ := s.GetRoomByName(ctx, room); !has {
			return nil
		}

		_, err := s.Db.ExecContext(ctx, `insert into word_blocklist (room, word) values (?, ?)`, room, keyword)
		if err != nil {
			return fmt.Errorf("failed to add keyword \"%s\" to blacklist for room %s: %w", keyword, room, err)
		}
	}

	return nil
}

// RemoveKeywordFromBlacklist removes a blacklist policy for a keyword to the persistent blacklist.
// If the room is not specified or does not exist and a serverwide policy for the term exists
// If the keyword is empty, this is a no-op.
func (s *Storage) RemoveKeywordFromBlacklist(ctx context.Context, room common.NormalizedRoomName, keyword string) error {
	if len(keyword) == 0 {
		return nil
	}

	if room.IsZero() {
		_, err := s.Db.ExecContext(ctx, `delete from word_blocklist where word = ?`, keyword)
		if err != nil {
			return fmt.Errorf("failed to remove keyword \"%s\" from global blacklist: %w", keyword, err)
		}
	} else {
		if _, has, _ := s.GetRoomByName(ctx, room); !has {
			return nil
		}

		_, err := s.Db.ExecContext(ctx, `delete from word_blocklist where room = ? and word = ?`, room, keyword)
		if err != nil {
			return fmt.Errorf("failed to remove keyword \"%s\" from blacklist for room %s: %w", keyword, room, err)
		}
	}

	return nil
}

// GetBlacklistedKeywordsForRoom will return a list of currently enforced blacklist policies for a given room.
// If room is zero, it will return the global blacklist.
// The string searching library necessitates returning a list of rune arrays.
func (s *Storage) GetBlacklistedKeywordsForRoom(ctx context.Context, room common.NormalizedRoomName) ([][]rune, error) {
	var keywords [][]rune
	var rows *sql.Rows
	var err error

	if room.IsZero() {
		rows, err = s.Db.QueryContext(ctx, `select word from word_blocklist where room is null`)
	} else {
		rows, err = s.Db.QueryContext(ctx, `select word from word_blocklist where room is ?`, room.String())
	}
	if err != nil {
		return nil, fmt.Errorf(`failed to query rooms: %w`, err)
	}
	defer rows.Close()

	for rows.Next() {
		var word []byte
		if err := rows.Scan(&word); err != nil {
			return keywords, err
		}

		keywords = append(keywords, bytes.Runes(word))
	}

	if err := rows.Err(); err != nil {
		return keywords, err
	}

	return keywords, nil
}
