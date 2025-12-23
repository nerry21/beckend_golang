package handlers

import "backend/utils"

// queryRower dipakai oleh *sql.DB dan *sql.Tx (keduanya punya QueryRow)
type queryRower = utils.QueryRower

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
	return utils.HasTable(q, table)
}

// hasColumn: cek kolom ada di schema/database yang aktif (DATABASE()).
// Return bool saja agar kompatibel dengan call-site lama.
func hasColumn(q queryRower, table, column string) bool {
	return utils.HasColumn(q, table, column)
}
