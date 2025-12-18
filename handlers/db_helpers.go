package handlers

import (
	"database/sql"
	"log"
)

// queryRower dipakai oleh *sql.DB dan *sql.Tx (keduanya punya QueryRow)
type queryRower interface {
	QueryRow(query string, args ...any) *sql.Row
}

// nullIfEmpty: helper untuk insert/update nullable string
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// hasTable: cek tabel ada di schema/database yang aktif (DATABASE()).
// Return bool saja agar kompatibel dengan call-site lama.
func hasTable(q queryRower, table string) bool {
	if q == nil || table == "" {
		return false
	}

	var name sql.NullString
	err := q.QueryRow(
		`SELECT table_name
		   FROM information_schema.tables
		  WHERE table_schema = DATABASE()
		    AND table_name = ?
		  LIMIT 1`,
		table,
	).Scan(&name)

	switch {
	case err == nil:
		return name.Valid
	case err == sql.ErrNoRows:
		return false
	default:
		// jangan bikin build gagal hanya karena pengecekan schema
		log.Println("hasTable error:", table, err)
		return false
	}
}

// hasColumn: cek kolom ada di schema/database yang aktif (DATABASE()).
// Return bool saja agar kompatibel dengan call-site lama.
func hasColumn(q queryRower, table, column string) bool {
	if q == nil || table == "" || column == "" {
		return false
	}

	var name sql.NullString
	err := q.QueryRow(
		`SELECT column_name
		   FROM information_schema.columns
		  WHERE table_schema = DATABASE()
		    AND table_name = ?
		    AND column_name = ?
		  LIMIT 1`,
		table, column,
	).Scan(&name)

	switch {
	case err == nil:
		return name.Valid
	case err == sql.ErrNoRows:
		return false
	default:
		log.Println("hasColumn error:", table, column, err)
		return false
	}
}
