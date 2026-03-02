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

func ScanServerRecord(row common.Scannable) (record ServerRecord, has bool, err error) {
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
	Server            string
	Name              string
	Path              common.ProtoPath
	CreatedTs         time.Time
	Uuid              string
	EnableIndexing    bool
	EnableDirectories bool
	IsInternal        bool
	FollowLinks       bool
}

func ScanShareRecord(row common.Scannable) (record ShareRecord, has bool, err error) {
	var server string
	var name string
	var path string
	var createdTs int64
	var uuid string
	var enableIndexing bool
	var enableDirectories bool
	var isInternal bool
	var followLinks bool

	err = row.Scan(
		&server,
		&name,
		&path,
		&createdTs,
		&uuid,
		&enableIndexing,
		&enableDirectories,
		&isInternal,
		&followLinks,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return record, false, nil
		}
		return record, false, err
	}

	record.Server = server
	record.Name = name
	record.Path = common.UncheckedCreateProtoPath(path)
	record.CreatedTs = time.Unix(createdTs, 0)
	record.Uuid = uuid
	record.EnableIndexing = enableIndexing
	record.EnableDirectories = enableDirectories
	record.IsInternal = isInternal
	record.FollowLinks = followLinks

	return record, true, nil
}

type ShareIndexRecord struct {
	Share       string
	IndexId     int64
	Path        common.ProtoPath
	IsDirectory bool
	Size        int64
}

func ScanShareIndexRecord(row common.Scannable) (record ShareIndexRecord, has bool, err error) {
	var share string
	var indexId int64
	var path string
	var isDirectory bool
	var size int64

	err = row.Scan(&share, &indexId, &path, &isDirectory, &size)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return record, false, nil
		}
		return record, false, err
	}

	record.Share = share
	record.IndexId = indexId
	record.Path = common.UncheckedCreateProtoPath(path)
	record.IsDirectory = isDirectory
	record.Size = size

	return record, true, nil
}

type ClientCertRecord struct {
	Uuid      string
	CertPem   []byte
	KeyPem    []byte
	Server    string
	CreatedTs time.Time
}

func ScanClientCertRecord(row common.Scannable) (record ClientCertRecord, has bool, err error) {
	var uuid string
	var certPem []byte
	var keyPem []byte
	var server string
	var createdTs int64

	err = row.Scan(&uuid, &certPem, &keyPem, &server, &createdTs)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return record, false, nil
		}
		return record, false, err
	}

	record.Uuid = uuid
	record.CertPem = certPem
	record.KeyPem = keyPem
	record.Server = server
	record.CreatedTs = time.Unix(createdTs, 0)

	return record, true, nil
}
