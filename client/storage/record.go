package storage

import (
	"database/sql"
	"errors"
	"time"

	"friendnet.org/common"
	pb "friendnet.org/protocol/pb/v1"
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
	Snippet     string
}

func ScanShareIndexRecord(row common.Scannable) (record ShareIndexRecord, has bool, err error) {
	var share string
	var indexId int64
	var path string
	var isDirectory bool
	var size int64
	var snippet string

	err = row.Scan(&share, &indexId, &path, &isDirectory, &size, &snippet)
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
	record.Snippet = snippet

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

type DownloadStateRecord struct {
	Uuid                string
	CreatedTs           time.Time
	UpdatedTs           time.Time
	Server              string
	PeerUsername        common.NormalizedUsername
	Status              pb.DownloadStatus
	FilePath            common.ProtoPath
	FileTotalSize       int64
	FileDownloadedBytes int64
	Error               *string
}

func ScanDownloadStateRecord(row common.Scannable) (record DownloadStateRecord, has bool, err error) {
	var uuid string
	var createdTs int64
	var updatedTs int64
	var server string
	var peerUsername string
	var status int64
	var filePath string
	var fileTotalSize int64
	var fileDownloadedBytes int64
	var errorStr *string

	err = row.Scan(&uuid, &createdTs, &updatedTs, &server, &peerUsername, &status, &filePath, &fileTotalSize, &fileDownloadedBytes, &errorStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return record, false, nil
		}
		return record, false, err
	}

	record.Uuid = uuid
	record.CreatedTs = time.Unix(createdTs, 0)
	record.UpdatedTs = time.Unix(updatedTs, 0)
	record.Server = server
	record.PeerUsername = common.UncheckedCreateNormalizedUsername(peerUsername)
	record.Status = pb.DownloadStatus(status)
	record.FilePath = common.UncheckedCreateProtoPath(filePath)
	record.FileTotalSize = fileTotalSize
	record.FileDownloadedBytes = fileDownloadedBytes
	record.Error = errorStr
	return record, true, nil
}
