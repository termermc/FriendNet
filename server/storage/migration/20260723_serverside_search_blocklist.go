package migration

import (
	"database/sql"

	"friendnet.org/common"
)

type M20260723ServersideSearchBlocklist struct {
}

var _ common.Migration = (*M20260723ServersideSearchBlocklist)(nil)

func (m *M20260723ServersideSearchBlocklist) Name() string {
	return "20260723_serverside_search_blocklist"
}

func (m *M20260723ServersideSearchBlocklist) Apply(tx *sql.Tx) error {
	const q = `
create table word_blocklist
(
	word text not null primary key,
	room text
);
	`

	_, err := tx.Exec(q)
	return err
}

func (m *M20260723ServersideSearchBlocklist) Revert(tx *sql.Tx) error {
	const q = `
drop table word_blocklist;
	`

	_, err := tx.Exec(q)
	return err
}
