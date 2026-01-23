package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	intconfig "backend/internal/config"
	router "backend/internal/http"

	"github.com/gin-gonic/gin"
)

func main() {
	env := intconfig.LoadEnv()
	if env.GinMode != "" {
		gin.SetMode(env.GinMode)
	}

	intconfig.ConnectDB()
	defer intconfig.CloseDB()

	// Router (Gin engine)
	r := router.NewRouter(env)

	srv := &http.Server{
		Addr:              env.AppAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("Server berjalan di http://localhost%s", env.AppAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Gagal menjalankan server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("Mematikan server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Shutdown server gagal: %v", err)
	}

	log.Println("Server berhenti dengan aman.")
}
