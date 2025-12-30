package handlers

import (
	"net/http"

	"backend/internal/domain"
	"backend/internal/http/middleware"

	"github.com/gin-gonic/gin"
)

// ErrorResponse standardizes error payloads for new handlers.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details any    `json:"details,omitempty"`
}

func respondError(c *gin.Context, status int, code, message string, details any) {
	if code == "" {
		code = http.StatusText(status)
	}
	resp := ErrorResponse{
		Error:   message,
		Code:    code,
		Details: details,
	}
	reqID := middleware.GetRequestID(c)
	if reqID != "" {
		c.JSON(status, gin.H{
			"error":      resp.Error,
			"code":       resp.Code,
			"details":    resp.Details,
			"request_id": reqID,
			"message":    message,
		})
		return
	}
	c.JSON(status, resp)
}

// RespondDomainError maps domain errors to HTTP responses.
func RespondDomainError(c *gin.Context, err error) {
	switch {
	case domain.IsValidation(err):
		respondError(c, http.StatusBadRequest, "validation_error", err.Error(), nil)
	case domain.IsNotFound(err):
		respondError(c, http.StatusNotFound, "not_found", err.Error(), nil)
	case domain.IsConflict(err):
		respondError(c, http.StatusConflict, "conflict", err.Error(), nil)
	default:
		respondError(c, http.StatusInternalServerError, "internal_error", "terjadi kesalahan", nil)
	}
}
