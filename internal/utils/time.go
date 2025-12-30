package utils

import (
	"strings"
	"time"
)

const (
	layoutDate     = "2006-01-02"
	layoutDateTime = "2006-01-02 15:04:05"
)

// NowUTC returns current time in UTC.
func NowUTC() time.Time {
	return time.Now().UTC()
}

// ParseDate parses YYYY-MM-DD in local timezone.
func ParseDate(s string) (time.Time, error) {
	return time.ParseInLocation(layoutDate, strings.TrimSpace(s), time.Local)
}

// ParseDateTime parses "YYYY-MM-DD HH:MM:SS" in local timezone.
func ParseDateTime(s string) (time.Time, error) {
	return time.ParseInLocation(layoutDateTime, strings.TrimSpace(s), time.Local)
}

// FormatDate formats time to YYYY-MM-DD in local timezone.
func FormatDate(t time.Time) string {
	return t.In(time.Local).Format(layoutDate)
}

// FormatDateTime formats time to "YYYY-MM-DD HH:MM:SS" in local timezone.
func FormatDateTime(t time.Time) string {
	return t.In(time.Local).Format(layoutDateTime)
}
