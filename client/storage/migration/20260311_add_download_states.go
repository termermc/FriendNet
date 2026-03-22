package migration

import (
	"database/sql"

	"friendnet.org/common"
)

type M20260311AddDownloadStates struct {
}

var _ common.Migration = (*M20260311AddDownloadStates)(nil)

func (m *M20260311AddDownloadStates) Name() string {
	return "20260311_add_download_states"
}

func (m *M20260311AddDownloadStates) Apply(tx *sql.Tx) error {
	const q = `
create table download_state
(
    uuid text not null
		constraint download_state_pk
			primary key,
	created_ts integer default (strftime('%s', 'now')) not null,
	updated_ts integer default (strftime('%s', 'now')) not null,
    server text not null
		constraint share_server_server_uuid_fk
        references server
		on delete cascade,
	peer_username text not null,
	status integer not null,
	file_path text not null,
	file_total_size integer not null default -1,
	file_downloaded_bytes integer not null default 0,
	error text null
);

create index download_state_created_ts_index
    on download_state (created_ts);

create index download_state_updated_ts_index
    on download_state (updated_ts);

create index download_state_status_index
	on download_state (status);

create unique index download_state_server_peer_file_uindex
    on download_state (server, peer_username, file_path);

create trigger download_state_update_timestamp
after update on download_state
for each row
begin
  update download_state set updated_ts = strftime('%s', 'now') where uuid = new.uuid;
end;
	`

	_, err := tx.Exec(q)
	return err
}

func (m *M20260311AddDownloadStates) Revert(tx *sql.Tx) error {
	const q = `
drop table download_state;
	`

	_, err := tx.Exec(q)
	return err
}
