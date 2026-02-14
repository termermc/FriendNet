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
create table server_cert
(
    hostname text not null
		constraint server_cert_pk
			primary key,
	cert_der blob not null,
	created_ts integer default (strftime('%s', 'now')) not null
);

create index server_cert_created_ts_index
    on server_cert (created_ts);

create table server
(
    uuid text not null primary key,
    name text not null default '',
    address text not null,
    room text not null,
    username text not null,
    password text not null,
    created_ts integer default (strftime('%s', 'now')) not null
);

create index server_created_ts_index
    on server (created_ts);

create table share
(
    uuid text not null primary key,
    server text not null
		constraint share_server_server_uuid_fk
        references server
		on delete cascade,
	name text not null,
	path text not null,
	created_ts integer default (strftime('%s', 'now')) not null
);

create index share_created_ts_index
	on share (created_ts);
	`

	_, err := tx.Exec(q)
	return err
}

func (m *M20260208InitialSchema) Revert(tx *sql.Tx) error {
	const q = `
drop table server_cert;
drop table server;
drop table share;
	`

	_, err := tx.Exec(q)
	return err
}
