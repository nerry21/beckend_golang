package utils

import (
	"log"
	"strings"
)

// LogEvent prints standardized log line with module/action/request_id.
// Avoid logging sensitive payload; message should be summarized.
func LogEvent(requestID, module, action, message string) {
	req := strings.TrimSpace(requestID)
	log.Printf("[%s] action=%s request_id=%s msg=%s", strings.ToUpper(module), action, req, message)
}
