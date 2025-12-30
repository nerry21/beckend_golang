package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger prints minimal request log including request_id when available.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)

		reqID := GetRequestID(c)
		status := c.Writer.Status()

		log.Printf("[HTTP] request_id=%s method=%s path=%s status=%d latency_ms=%.3f ip=%s",
			reqID,
			c.Request.Method,
			c.Request.URL.Path,
			status,
			float64(latency.Microseconds())/1000.0,
			c.ClientIP(),
		)
	}
}
