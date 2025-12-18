package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RequireRoles adalah middleware role-based access control.
// Hanya mengizinkan request dengan role yang terdapat di allowedRoles.
// Contoh:
//   r.GET("/admin", RequireRoles("owner", "admin"), handler)
//
// Catatan:
//   - Diasumsikan middleware auth sebelumnya sudah set context:
//       c.Set("userRole", "<role-user>")
func RequireRoles(allowedRoles ...string) gin.HandlerFunc {
	// Build map untuk lookup role yang diizinkan (O(1))
	allowed := make(map[string]struct{}, len(allowedRoles))
	for _, r := range allowedRoles {
		allowed[strings.ToLower(strings.TrimSpace(r))] = struct{}{}
	}

	return func(c *gin.Context) {
		// Ambil role user dari context
		role := c.GetString("userRole")
		if role == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorized: role tidak ditemukan pada context",
			})
			return
		}

		normalizedRole := strings.ToLower(strings.TrimSpace(role))

		// Cek apakah role user termasuk dalam allowedRoles
		if _, ok := allowed[normalizedRole]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "forbidden: role tidak diizinkan",
			})
			return
		}

		// Role valid → lanjut ke handler berikutnya
		c.Next()
	}
}
