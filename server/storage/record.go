package storage

import (
	"database/sql"
	"errors"
	"time"

	"friendnet.org/common"
)

type RoomRecord struct {
	Name      common.NormalizedRoomName
	CreatedTs time.Time
}

func ScanRoomRecord(row common.Scannable) (record RoomRecord, has bool, err error) {
	var name string
	var createdTs int64

	err = row.Scan(&name, &createdTs)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return record, false, nil
		}
		return record, false, err
	}

	record.Name = common.UncheckedCreateNormalizedRoomName(name)
	record.CreatedTs = time.Unix(createdTs, 0)

	return record, true, nil
}

type AccountRecord struct {
	Room         common.NormalizedRoomName
	Username     common.NormalizedUsername
	PasswordHash string
	CreatedTs    time.Time
}

func ScanAccountRecord(row common.Scannable) (record AccountRecord, has bool, err error) {
	var room string
	var username string
	var passwordHash string
	var createdTs int64

	err = row.Scan(&room, &username, &passwordHash, &createdTs)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return record, false, nil
		}
		return record, false, err
	}

	record.Room = common.UncheckedCreateNormalizedRoomName(room)
	record.Username = common.UncheckedCreateNormalizedUsername(username)
	record.PasswordHash = passwordHash
	record.CreatedTs = time.Unix(createdTs, 0)

	return record, true, nil
}
