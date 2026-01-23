package db

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"
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

var (
	tableCache  sync.Map // map[string]bool
	columnCache sync.Map // map[string]bool, key: "table|column"
)

func HasTable(q QueryRower, table string) bool {
	if table == "" {
		return false
	}
	if v, ok := tableCache.Load(table); ok {
		return v.(bool)
	}

	var name sql.NullString
	err := q.QueryRow(`
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_name = ?
		LIMIT 1
	`, table).Scan(&name)

	ok := (err == nil && name.Valid && name.String != "")
	if err != nil && errors.Is(err, driver.ErrBadConn) {
		ok = false
	}

	tableCache.Store(table, ok)
	return ok
}

func HasColumn(q QueryRower, table, column string) bool {
	if table == "" || column == "" {
		return false
	}
	key := table + "|" + column
	if v, ok := columnCache.Load(key); ok {
		return v.(bool)
	}

	var name sql.NullString
	err := q.QueryRow(`
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name = ?
		  AND column_name = ?
		LIMIT 1
	`, table, column).Scan(&name)

	ok := (err == nil && name.Valid && name.String != "")
	if err != nil && errors.Is(err, driver.ErrBadConn) {
		ok = false
	}

	columnCache.Store(key, ok)
	return ok
}

// Keep lowercase helpers for call-site compatibility during refactor.
//nolint:unused // kept for backward compatibility during refactor.
func hasTable(q QueryRower, table string) bool { return HasTable(q, table) }
//nolint:unused // kept for backward compatibility during refactor.
func hasColumn(q QueryRower, table, column string) bool {
	return HasColumn(q, table, column)
}
