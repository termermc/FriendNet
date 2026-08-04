package migration

import (
	"database/sql"

	"friendnet.org/common"
)

type M20260723SearchBlacklist struct {
}

var _ common.Migration = (*M20260723SearchBlacklist)(nil)

func (m *M20260723SearchBlacklist) Name() string {
	return "20260723_search_blacklist"
}

func (m *M20260723SearchBlacklist) Apply(tx *sql.Tx) error {
	const q = `
create table search_blacklist
(
	word text not null,
	room text null
		constraint search_blacklist_room_room_name_fk
		references room
		on delete cascade,
	match_mode integer not null,
	created_ts integer default (strftime('%s', 'now')) not null,

	primary key (word, room)
);

create index search_blacklist_room_index on search_blacklist (room);
create index search_blacklist_room_not_null_index on search_blacklist (room) where room is null;
create index index_search_blacklist_created_ts_index on search_blacklist (created_ts);
	`

	_, err := tx.Exec(q)
	return err
}

func (m *M20260723SearchBlacklist) Revert(tx *sql.Tx) error {
	const q = `
drop table search_blacklist;
	`

	_, err := tx.Exec(q)
	return err
}
