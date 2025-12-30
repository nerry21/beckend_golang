package handlers

import intdb "backend/internal/db"

// queryRower dipakai oleh *sql.DB dan *sql.Tx (keduanya punya QueryRow)
type queryRower = intdb.QueryRower

// nullIfEmpty: helper untuk insert/update nullable string
func nullIfEmpty(s string) any {
	return intdb.NullIfEmpty(s)
}

// hasTable: cek tabel ada di schema/database yang aktif (DATABASE()).
// Return bool saja agar kompatibel dengan call-site lama.
func hasTable(q queryRower, table string) bool {
	return intdb.HasTable(q, table)
}

// hasColumn: cek kolom ada di schema/database yang aktif (DATABASE()).
// Return bool saja agar kompatibel dengan call-site lama.
func hasColumn(q queryRower, table, column string) bool {
	return intdb.HasColumn(q, table, column)
}
