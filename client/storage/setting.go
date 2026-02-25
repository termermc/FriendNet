package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

// GetSettingOr returns the value of the setting with the specified key.
// If the setting does not exist, returns the default value.
// If you want to put a default value while returning one, use GetSettingOrPut.
func (s *Storage) GetSettingOr(ctx context.Context, key string, def string) (string, error) {
	row := s.Db.QueryRowContext(ctx, `select value from setting where key = ?`, key)
	var val string
	err := row.Scan(&val)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return def, nil
		}
		return "", err
	}
	return val, nil
}

// GetSettingOrFunc returns the value of the setting with the specified key.
// If the setting does not exist, runs a default function and returns its result.
// If you want to put a default value while returning one, use GetSettingOrPutFunc.
func (s *Storage) GetSettingOrFunc(ctx context.Context, key string, fn func() (string, error)) (string, error) {
	row := s.Db.QueryRowContext(ctx, `select value from setting where key = ?`, key)
	var val string
	err := row.Scan(&val)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fn()
		}
		return "", err
	}
	return val, nil
}

// GetSettingOrPutFunc returns the value of the setting with the specified key.
// If the setting does not exist, it will be created with the result of fn, and that value will be returned.
func (s *Storage) GetSettingOrPutFunc(ctx context.Context, key string, fn func() (string, error)) (string, error) {
	row := s.Db.QueryRowContext(ctx, `select value from setting where key = ?`, key)
	var val string
	err := row.Scan(&val)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			def, fnErr := fn()
			if fnErr != nil {
				return "", fnErr
			}

			_, err = s.Db.ExecContext(ctx, `insert into setting (key, value) values (?, ?)`, key, def)
			if err != nil {
				return "", err
			}
		}

		return "", err
	}
	return val, nil
}

// GetSettingOrPut returns the value of the setting with the specified key.
// If the setting does not exist, it will be created with the default value, and that value will be returned.
func (s *Storage) GetSettingOrPut(ctx context.Context, key string, def string) (string, error) {
	row := s.Db.QueryRowContext(ctx, `select value from setting where key = ?`, key)
	var val string
	err := row.Scan(&val)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_, err = s.Db.ExecContext(ctx, `insert into setting (key, value) values (?, ?)`, key, def)
			if err != nil {
				return "", err
			}
		}

		return "", err
	}
	return val, nil
}

// PutSetting sets the value of the setting with the specified key.
func (s *Storage) PutSetting(ctx context.Context, key string, value string) error {
	_, err := s.Db.ExecContext(ctx, `insert or replace into setting (key, value) values (?, ?)`, key, value)
	return err
}

// GetSettingIntOr returns the integer value of the setting with the specified key.
// If the setting does not exist, returns the default value.
// If you want to put a default value while returning one, use GetSettingIntOrPut.
func (s *Storage) GetSettingIntOr(ctx context.Context, key string, def int64) (int64, error) {
	str, err := s.GetSettingOr(ctx, key, "")
	if err != nil {
		return 0, err
	}
	if str == "" {
		return def, nil
	}

	return strconv.ParseInt(str, 10, 64)
}

// GetSettingIntOrPut returns the integer value of the setting with the specified key.
// If the setting does not exist, it will be created with the default value, and that value will be returned.
func (s *Storage) GetSettingIntOrPut(ctx context.Context, key string, def int64) (int64, error) {
	str, err := s.GetSettingOrPut(ctx, key, strconv.FormatInt(def, 10))
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(str, 10, 64)
}

// PutSettingInt sets the integer value of the setting with the specified key.
func (s *Storage) PutSettingInt(ctx context.Context, key string, value int64) error {
	return s.PutSetting(ctx, key, strconv.FormatInt(value, 10))
}

// GetSettingBoolOr returns the boolean value of the setting with the specified key.
// If the setting does not exist, returns the default value.
// If you want to put a default value while returning one, use GetSettingBoolOrPut.
func (s *Storage) GetSettingBoolOr(ctx context.Context, key string, def bool) (bool, error) {
	str, err := s.GetSettingOr(ctx, key, "")
	if err != nil {
		return false, err
	}
	if str == "" {
		return def, nil
	}

	if str == "true" {
		return true, nil
	} else if str == "false" {
		return false, nil
	}

	return false, fmt.Errorf("invalid boolean value for key %q: %q", key, str)
}

// GetSettingBoolOrPut returns the boolean value of the setting with the specified key.
// If the setting does not exist, it will be created with the default value, and that value will be returned.
func (s *Storage) GetSettingBoolOrPut(ctx context.Context, key string, def bool) (bool, error) {
	var defStr string
	if def {
		defStr = "true"
	} else {
		defStr = "false"
	}

	str, err := s.GetSettingOrPut(ctx, key, defStr)
	if err != nil {
		return false, err
	}

	if str == "true" {
		return true, nil
	} else if str == "false" {
		return false, nil
	}

	return false, fmt.Errorf("invalid boolean value for key %q: %q", key, str)
}

// PutSettingBool sets the boolean value of the setting with the specified key.
func (s *Storage) PutSettingBool(ctx context.Context, key string, value bool) error {
	var defStr string
	if value {
		defStr = "true"
	} else {
		defStr = "false"
	}

	return s.PutSetting(ctx, key, defStr)
}
