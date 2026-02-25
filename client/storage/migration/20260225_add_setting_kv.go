package migration

import (
	"database/sql"

	"friendnet.org/common"
)

type M20260225AddSettingKv struct {
}

var _ common.Migration = (*M20260225AddSettingKv)(nil)

func (m *M20260225AddSettingKv) Name() string {
	return "20260225_add_setting_kv"
}

func (m *M20260225AddSettingKv) Apply(tx *sql.Tx) error {
	const q = `
create table setting
(
    key text not null
		constraint setting_pk
			primary key,
    value text not null,
	updated_ts integer default (strftime('%s', 'now')) not null
);

create trigger setting_update_timestamp
after update on setting
for each row
begin
  update setting set updated_ts = strftime('%s', 'now') where key = new.key;
end;
	`

	_, err := tx.Exec(q)
	return err
}

func (m *M20260225AddSettingKv) Revert(tx *sql.Tx) error {
	const q = `
drop table setting;
	`

	_, err := tx.Exec(q)
	return err
}
