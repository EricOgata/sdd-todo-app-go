package db

import (
	"errors"
	"strings"

	"modernc.org/sqlite"
)

// SQLite primary result codes used by the retry and corruption policies.
const (
	sqliteBusy    = 5
	sqliteLocked  = 6
	sqliteCorrupt = 11
	sqliteNotADB  = 26
)

// IsBusy reports whether err is a SQLITE_BUSY or SQLITE_LOCKED failure,
// including extended result codes.
func IsBusy(err error) bool {
	if err == nil {
		return false
	}
	var se *sqlite.Error
	if errors.As(err, &se) {
		switch se.Code() & 0xff {
		case sqliteBusy, sqliteLocked:
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "sqlite_busy") ||
		strings.Contains(msg, "sqlite_locked")
}

// IsCorruption reports whether err is a corruption-class failure.
func IsCorruption(err error) bool {
	if err == nil {
		return false
	}
	var se *sqlite.Error
	if errors.As(err, &se) {
		switch se.Code() & 0xff {
		case sqliteCorrupt, sqliteNotADB:
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database disk image is malformed") ||
		strings.Contains(msg, "file is not a database") ||
		strings.Contains(msg, "malformed database schema") ||
		strings.Contains(msg, "database file is corrupt")
}
