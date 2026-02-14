package storage

import (
	"database/sql"
	"errors"
	"time"

	"friendnet.org/common"
)

type ServerCertRecord struct {
	Hostname  string
	CertDer   []byte
	CreatedTs time.Time
}

func ScanServerCertRecord(row common.Scannable) (record ServerCertRecord, has bool, err error) {
	var hostname string
	var certDer []byte
	var createdTs int64

	err = row.Scan(&hostname, &certDer, &createdTs)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return record, false, nil
		}
		return record, false, err
	}

	record.Hostname = hostname
	record.CertDer = certDer
	record.CreatedTs = time.Unix(createdTs, 0)

	return record, true, nil
}

type ServerRecord struct {
	Uuid      string
	Name      string
	Address   string
	Room      common.NormalizedRoomName
	Username  common.NormalizedUsername
	Password  string
	CreatedTs time.Time
}

func ScanAccountRecord(row common.Scannable) (record ServerRecord, has bool, err error) {
	var uuid string
	var name string
	var address string
	var room string
	var username string
	var password string
	var createdTs int64

	err = row.Scan(&uuid, &name, &address, &room, &username, &password, &createdTs)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return record, false, nil
		}
		return record, false, err
	}

	record.Uuid = uuid
	record.Name = name
	record.Address = address
	record.Room = common.UncheckedCreateNormalizedRoomName(room)
	record.Username = common.UncheckedCreateNormalizedUsername(username)
	record.Password = password
	record.CreatedTs = time.Unix(createdTs, 0)

	return record, true, nil
}

type ShareRecord struct {
	Uuid      string
	Server    string
	Name      string
	Path      string
	CreatedTs time.Time
}

func ScanShareRecord(row common.Scannable) (record ShareRecord, has bool, err error) {
	var uuid string
	var server string
	var name string
	var path string
	var createdTs int64

	err = row.Scan(&uuid, &server, &name, &path, &createdTs)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return record, false, nil
		}
		return record, false, err
	}

	record.Uuid = uuid
	record.Server = server
	record.Name = name
	record.Path = path
	record.CreatedTs = time.Unix(createdTs, 0)

	return record, true, nil
}
