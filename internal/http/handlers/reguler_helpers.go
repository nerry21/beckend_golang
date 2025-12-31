package handlers

import (
	intdb "backend/internal/db"
)

// queryRower aliases the shared QueryRower interface for DB helpers.
type queryRower = intdb.QueryRower

func hasTable(q queryRower, table string) bool {
	return intdb.HasTable(q, table)
}

func hasColumn(q queryRower, table, column string) bool {
	return intdb.HasColumn(q, table, column)
}
