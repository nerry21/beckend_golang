package handlers

import (
	"net/http"
	"sync"

	intconfig "backend/internal/config"

	"github.com/gin-gonic/gin"
)

var (
	routerMu sync.RWMutex
	router   *gin.Engine
)

// SetRouter stores the active gin engine for later inspection (e.g., /api/routes).
func SetRouter(r *gin.Engine) {
	routerMu.Lock()
	defer routerMu.Unlock()
	router = r
}

func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "backend golang berjalan"})
}

func DBCheck(c *gin.Context) {
	if intconfig.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database belum terhubung"})
		return
	}
	var count int
	err := intconfig.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal query ke database: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "koneksi database OK", "users_in_db": count})
}

func Routes(c *gin.Context) {
	routerMu.RLock()
	r := router
	routerMu.RUnlock()
	if r == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "router belum siap"})
		return
	}

	routes := r.Routes()
	out := make([]gin.H, 0, len(routes))
	for _, rt := range routes {
		out = append(out, gin.H{
			"method":  rt.Method,
			"path":    rt.Path,
			"handler": rt.Handler,
		})
	}
	c.JSON(http.StatusOK, gin.H{"routes": out})
}
