// backend/config/db.go
package config

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

var DB *sql.DB

func ConnectDB() {
	var err error

	// sesuaikan dengan setting XAMPP/phpMyAdmin kamu
	// user: root, pass: "", db: lktravel_db (contoh)
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true",
		"root",        // username
		"",            // password
		"127.0.0.1:3306", // host:port
		"travel_app", // nama database
	)

	DB, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Gagal open DB: %v", err)
	}

	if err = DB.Ping(); err != nil {
		log.Fatalf("Gagal ping DB: %v", err)
	}

	log.Println("Berhasil konek ke database MySQL")
}

func CloseDB() {
	if DB != nil {
		_ = DB.Close()
	}
}
