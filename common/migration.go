package common

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
)

// Migration represents a database migration.
type Migration interface {
	// Name returns the name of the migration.
	Name() string

	// Apply applies the migration to the database.
	Apply(tx *sql.Tx) error

	// Revert reverts the migration from the database.
	Revert(tx *sql.Tx) error
}

// DoMigrations applies all migrations to a database.
//
//goland:noinspection SqlNoDataSourceInspection
func DoMigrations(db *sql.DB, migrations []Migration) error {
	// Create table if it doesn't exist.
	_, err := db.Exec(`
		create table if not exists migration (
			name text not null primary key,
			created_ts integer not null default (strftime('%s', 'now'))
		)
	`)
	if err != nil {
		return fmt.Errorf(`failed to create migration table in DoMigrations: %w`, err)
	}

	// Get the names of already-applied migrations.
	var appliedNames []string
	rows, err := db.Query(`select name from migration`)
	if err != nil {
		return fmt.Errorf(`failed to query applied migrations in DoMigrations: %w`, err)
	}
	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		if err != nil {
			_ = rows.Close()
			return err
		}

		appliedNames = append(appliedNames, name)
	}
	_ = rows.Close()

	for _, m := range migrations {
		if slices.Contains(appliedNames, m.Name()) {
			continue
		}

		var tx *sql.Tx
		tx, err = db.Begin()
		if err != nil {
			return err
		}

		err = m.Apply(tx)
		if err != nil {
			errs := make([]error, 0, 2)
			errs = append(errs, fmt.Errorf(`failed to apply migration %q: %w`, m.Name(), err))

			if err = tx.Rollback(); err != nil {
				errs = append(errs, fmt.Errorf(`failed to roll back transaction after failed migration %q: %w`, m.Name(), err))
			}

			return errors.Join(errs...)
		}

		_, err = tx.Exec(`insert into migration (name) values (?)`, m.Name())
		if err != nil {
			errs := make([]error, 0, 2)
			errs = append(errs, fmt.Errorf(`failed to record applied migration %q: %w`, m.Name(), err))

			if err = tx.Rollback(); err != nil {
				errs = append(errs, fmt.Errorf(`failed to roll back transaction after failing to record applied migration %q: %w`, m.Name(), err))
			}
			return err
		}

		err = tx.Commit()
		if err != nil {
			return fmt.Errorf(`failed to commit transaction after successful migration %q: %w`, m.Name(), err)
		}
	}

	return nil
}
