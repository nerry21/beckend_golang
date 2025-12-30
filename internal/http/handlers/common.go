package handlers

import (
	"net/http"

	"backend/internal/http/middleware"

	"github.com/gin-gonic/gin"
)

// RespondError sends standard error payload with request_id included.
// Keeps backward compatibility by always providing "message".
func RespondError(c *gin.Context, status int, message string, err error) {
	reqID := middleware.GetRequestID(c)
	payload := gin.H{
		"message":    message,
		"request_id": reqID,
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	c.JSON(status, payload)
}

// BindJSONOrError ensures body is present and parsable.
func BindJSONOrError[T any](c *gin.Context, dst *T) bool {
	if c.Request.Body == nil {
		RespondError(c, http.StatusBadRequest, "body kosong", nil)
		return false
	}
	if err := c.ShouldBindJSON(dst); err != nil {
		RespondError(c, http.StatusBadRequest, "payload tidak valid", err)
		return false
	}
	return true
}
