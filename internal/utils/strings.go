package utils

import (
	"strings"
)

// TrimOrEmpty normalizes user input without turning nil into "nil".
func TrimOrEmpty(s string) string {
	return strings.TrimSpace(s)
}

// NormalizeSpace collapses repeated whitespace into a single space.
func NormalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// SplitSeatList splits comma/semicolon separated seat strings into cleaned slices.
func SplitSeatList(raw string) []string {
	out := []string{}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, strings.ToUpper(p))
	}
	return out
}
