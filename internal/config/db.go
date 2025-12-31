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

// ConnectDB initializes the shared DB connection (idempotent).
func ConnectDB() *sql.DB {
	dbMu.Lock()
	defer dbMu.Unlock()

	if DB != nil {
		return DB
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true&loc=Local&charset=utf8mb4&timeout=5s&readTimeout=30s&writeTimeout=30s",
		"root",
		"",
		"127.0.0.1:3306",
		"travel_app",
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Gagal open DB: %v", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(10*time.Minute)
	db.SetConnMaxIdleTime(5*time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Gagal ping DB: %v", err)
	}

	DB = db
	log.Println("Berhasil konek ke database MySQL")
	return DB
}

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
