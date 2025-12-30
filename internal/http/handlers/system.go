package handlers

import (
	"net/http"

	legacyconfig "backend/config"
	legacy "backend/handlers"

	"github.com/gin-gonic/gin"
)

func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "backend golang berjalan"})
}

func DBCheck(c *gin.Context) {
	if legacyconfig.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database belum terhubung"})
		return
	}
	var count int
	err := legacyconfig.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal query ke database: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "koneksi database OK", "users_in_db": count})
}

func Routes(c *gin.Context) {
	legacy.GetRoutes(c)
}
