package db

import "backend/utils"

type QueryRower = utils.QueryRower

// NullIfEmpty helps store optional strings without wiping existing data.
func NullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func HasTable(q QueryRower, table string) bool {
	return utils.HasTable(q, table)
}

func HasColumn(q QueryRower, table, column string) bool {
	return utils.HasColumn(q, table, column)
}

// Keep lowercase helpers for call-site compatibility during refactor.
func hasTable(q QueryRower, table string) bool {
	return HasTable(q, table)
}

func hasColumn(q QueryRower, table, column string) bool {
	return HasColumn(q, table, column)
}
