package migration

import (
	"database/sql"

	"friendnet.org/common"
)

type M20260222AddLog struct {
}

var _ common.Migration = (*M20260222AddLog)(nil)

func (m *M20260222AddLog) Name() string {
	return "20260222_add_log"
}

func (m *M20260222AddLog) Apply(tx *sql.Tx) error {
	const q = `
create table log
(
    uuid text not null primary key,
    created_ts integer not null,
    run_id integer not null,
    level integer not null,
    message text not null,
    metadata_serial_ver integer not null,
    metadata text not null
);

create index log_created_ts_index
    on log (created_ts);

create index log_run_id_index
    on log (run_id);

create index log_level_index
    on log (level);
	`

	_, err := tx.Exec(q)
	return err
}

func (m *M20260222AddLog) Revert(tx *sql.Tx) error {
	const q = `
drop table log;
	`

	_, err := tx.Exec(q)
	return err
}
