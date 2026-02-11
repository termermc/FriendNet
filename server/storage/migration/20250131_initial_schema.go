package migration

import (
	"database/sql"

	"friendnet.org/common"
)

type M20260208InitialSchema struct {
}

var _ common.Migration = (*M20260208InitialSchema)(nil)

func (m *M20260208InitialSchema) Name() string {
	return "20260208_initial_schema"
}

func (m *M20260208InitialSchema) Apply(tx *sql.Tx) error {
	const q = `
create table room
(
    name text not null
		constraint room_pk
			primary key,
	created_ts integer default (strftime('%s', 'now')) not null
)

create index room_created_ts_index
    on room (created_ts);

create table account
(
    room text not null
		constraint account_room_room_name_fk
        references room
		on delete cascade,
    username text not null,
    password_hash text not null,
    created_ts integer default (strftime('%s', 'now')) not null,
    primary key (room, username)
);

create index account_created_ts_index
    on account (created_ts);
	`

	_, err := tx.Exec(q)
	return err
}

func (m *M20260208InitialSchema) Revert(tx *sql.Tx) error {
	const q = `
drop table room;
drop table account;
	`

	_, err := tx.Exec(q)
	return err
}
