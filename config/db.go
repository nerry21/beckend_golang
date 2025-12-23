// backend/config/db.go
package config

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var (
	DB   *sql.DB
	dbMu sync.Mutex
)

func ConnectDB() {
	dbMu.Lock()
	defer dbMu.Unlock()

	// kalau sudah ada DB aktif, jangan paksa reconnect di sini
	if DB != nil {
		return
	}

	// ✅ Tambah parameter timeout + loc + charset supaya stabil
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true&loc=Local&charset=utf8mb4&timeout=5s&readTimeout=30s&writeTimeout=30s",
		"root",           // username
		"",               // password
		"127.0.0.1:3306", // host:port
		"travel_app",     // nama database
	)

	var err error
	DB, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Gagal open DB: %v", err)
	}

	// ✅ Pool setting (penting untuk hindari invalid connection / stale conn)
	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(25)
	DB.SetConnMaxLifetime(10 * time.Minute)
	DB.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err = DB.PingContext(ctx); err != nil {
		log.Fatalf("Gagal ping DB: %v", err)
	}

	log.Println("Berhasil konek ke database MySQL")
}

// EnsureDB: dipanggil sebelum transaksi dimulai.
// Kalau DB putus, balikan error agar caller fail fast (tanpa menutup koneksi lain).
func EnsureDB() error {
	dbMu.Lock()
	defer dbMu.Unlock()

	if DB == nil {
		ConnectDB()
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := DB.PingContext(ctx); err != nil {
		return err
	}
	return nil
}

func CloseDB() {
	dbMu.Lock()
	defer dbMu.Unlock()

	if DB != nil {
		_ = DB.Close()
		DB = nil
	}
}
