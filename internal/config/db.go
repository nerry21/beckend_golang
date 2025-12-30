package config

import (
	"database/sql"

	legacyconfig "backend/config"
)

// DB exposes the shared connection for internal packages while keeping legacy config intact.
var DB *sql.DB

func ConnectDB() *sql.DB {
	legacyconfig.ConnectDB()
	DB = legacyconfig.DB
	return DB
}

func CloseDB() {
	legacyconfig.CloseDB()
	DB = nil
}
