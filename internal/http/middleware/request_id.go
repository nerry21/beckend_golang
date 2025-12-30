package middleware

import (
	"math/rand"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

const requestIDKey = "request_id"

func init() {
	rand.Seed(time.Now().UnixNano())
}

// RequestID ensures every request has an ID for tracing and logs.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.Request.Header.Get("X-Request-ID")
		if rid == "" {
			// lightweight unique id (time + rand) to avoid heavy deps
			rid = strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.Itoa(rand.Intn(1000000))
		}
		c.Set(requestIDKey, rid)
		c.Writer.Header().Set("X-Request-ID", rid)
		c.Next()
	}
}

// GetRequestID extracts request_id from gin context when available.
func GetRequestID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get(requestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
