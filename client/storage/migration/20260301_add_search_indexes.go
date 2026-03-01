package migration

import (
	"database/sql"

	"friendnet.org/common"
	"github.com/google/uuid"
)

type M20260301AddSearchIndexes struct {
}

var _ common.Migration = (*M20260301AddSearchIndexes)(nil)

func (m *M20260301AddSearchIndexes) Name() string {
	return "20260301_add_search_indexes"
}

func (m *M20260301AddSearchIndexes) Apply(tx *sql.Tx) error {
	const q1 = `
-- To make indexed file rows smaller, the share table needs to have a UUID instead of a composite primary key.
create table share_tmp
(
    server text not null
        constraint share_server_server_uuid_fk
            references server
            on delete cascade,
    name text not null,
    path text not null,
    created_ts integer default (strftime('%s', 'now')) not null,
    uuid text null primary key,
    enable_indexing boolean default true not null,
	enable_directories boolean default true not null,
	is_internal boolean default false not null
);

insert into share_tmp(server, name, path, created_ts)
select server, name, path, created_ts
from share;

drop table share;

alter table share_tmp
    rename to share;

create index share_created_ts_index
    on share (created_ts);

create unique index share_server_name_uindex
    on share (server, name);
	`

	_, err := tx.Exec(q1)
	if err != nil {
		return err
	}

	// Fill every share with a UUID.
	rows, err := tx.Query(`select server, name from share`)
	if err != nil {
		return err
	}
	defer func() {
		_ = rows.Close()
	}()
	for rows.Next() {
		var serverUuid string
		var name string
		err = rows.Scan(&serverUuid, &name)
		if err != nil {
			return err
		}

		var uuidRaw uuid.UUID
		uuidRaw, err = uuid.NewV7()
		if err != nil {
			return err
		}
		_, err = tx.Exec(`update share set uuid = ? where server = ? and name = ?`, uuidRaw.String(), serverUuid, name)
		if err != nil {
			return err
		}
	}

	const q2 = `
create table share_tmp
(
    server text not null
        constraint share_server_server_uuid_fk
            references server
            on delete cascade,
    name text not null,
    path text not null,
    created_ts integer default (strftime('%s', 'now')) not null,
    uuid text not null primary key,
    enable_indexing boolean default true not null,
	enable_directories boolean default true not null,
	is_internal boolean default false not null
);

insert into share_tmp(server, name, path, created_ts, uuid, enable_indexing, enable_directories, is_internal)
select server, name, path, created_ts, uuid, enable_indexing, enable_directories, is_internal
from share;

drop table share;

alter table share_tmp
    rename to share;

create index share_created_ts_index
    on share (created_ts);

create unique index share_server_name_uindex
    on share (server, name);

create virtual table share_index_fts using fts5(
    share unindexed,
    path,
    is_directory,
    size unindexed
);

create trigger share_delete_index
after delete on share
for each row
begin
    delete from share_index_fts where share = old.uuid;
end;
	`
	_, err = tx.Exec(q2)
	if err != nil {
		return err
	}

	return nil
}

func (m *M20260301AddSearchIndexes) Revert(tx *sql.Tx) error {
	const q = `
drop table share_index_fts;

create table share_tmp
(
    server     text                                    not null
        constraint share_server_server_uuid_fk
            references server
            on delete cascade,
    name       text                                    not null,
    path       text                                    not null,
    created_ts integer default (strftime('%s', 'now')) not null,
    constraint share_pk
        primary key (name, server)
);

insert into share_tmp(server, name, path, created_ts)
select server, name, path, created_ts
from share;

drop table share;

alter table share_tmp
    rename to share;

create index share_created_ts_index
    on share (created_ts);

create unique index share_server_name_uindex
    on share (server, name);

	`

	_, err := tx.Exec(q)
	return err
}
