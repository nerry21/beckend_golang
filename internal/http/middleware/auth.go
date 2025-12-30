package middleware

import "github.com/gin-gonic/gin"

// AuthOptional is a placeholder for future auth middleware. It currently passes through.
func AuthOptional() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}
