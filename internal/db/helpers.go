package db

import (
	"database/sql"
	"database/sql/driver"
	"errors"
)

type QueryRower interface {
	QueryRow(query string, args ...any) *sql.Row
}

// NullIfEmpty helps store optional strings without wiping existing data.
func NullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func HasTable(q QueryRower, table string) bool {
	var name sql.NullString
	err := q.QueryRow(`
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_name = ?
		LIMIT 1
	`, table).Scan(&name)
	if err != nil {
		if errors.Is(err, driver.ErrBadConn) {
			return false
		}
		return false
	}
	return name.Valid && name.String != ""
}

func HasColumn(q QueryRower, table, column string) bool {
	var name sql.NullString
	err := q.QueryRow(`
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name = ?
		  AND column_name = ?
		LIMIT 1
	`, table, column).Scan(&name)
	if err != nil {
		if errors.Is(err, driver.ErrBadConn) {
			return false
		}
		return false
	}
	return name.Valid && name.String != ""
}

// Keep lowercase helpers for call-site compatibility during refactor.
func hasTable(q QueryRower, table string) bool {
	return HasTable(q, table)
}

func hasColumn(q QueryRower, table, column string) bool {
	return HasColumn(q, table, column)
}
