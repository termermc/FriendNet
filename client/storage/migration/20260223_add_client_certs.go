package migration

import (
	"database/sql"

	"friendnet.org/common"
)

type M20260223AddClientCerts struct {
}

var _ common.Migration = (*M20260223AddClientCerts)(nil)

func (m *M20260223AddClientCerts) Name() string {
	return "20260223_add_client_certs"
}

func (m *M20260223AddClientCerts) Apply(tx *sql.Tx) error {
	const q = `
create table client_cert
(
    uuid text not null
		constraint client_cert_pk
			primary key,
    cert_pem text not null,
    key_pem text not null,
    server text null
		constraint client_cert_server_uuid_fk
        references server
		on delete cascade,
	created_ts integer default (strftime('%s', 'now')) not null
);

create unique index client_cert_server_uindex
    on client_cert (server);

create index client_cert_created_ts_index
    on client_cert (created_ts);
	`

	_, err := tx.Exec(q)
	return err
}

func (m *M20260223AddClientCerts) Revert(tx *sql.Tx) error {
	const q = `
drop table client_cert;
	`

	_, err := tx.Exec(q)
	return err
}
