package utils

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"log"
)

type QueryRower interface {
	QueryRow(query string, args ...any) *sql.Row
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
		// kalau bad connection, jangan spam log, cukup false
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
		// bad conn -> false (biar caller handle)
		if errors.Is(err, driver.ErrBadConn) {
			return false
		}
		// opsional: log sekali kalau mau, tapi jangan per kolom (spam)
		// log.Println("HasColumn error:", table, column, err)
		return false
	}
	return name.Valid && name.String != ""
}

// Optional: debugging safe, dipakai kalau kamu mau log 1x saja.
func LogBadConn(tag string, err error) {
	if errors.Is(err, driver.ErrBadConn) {
		log.Println(tag, "driver.ErrBadConn")
	}
}
