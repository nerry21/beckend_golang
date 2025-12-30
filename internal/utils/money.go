package utils

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatMoney keeps consistent decimal formatting for currency fields.
func FormatMoney(amount float64) string {
	return fmt.Sprintf("%.2f", amount)
}

// FormatRupiah renders integer amount with thousand separators.
func FormatRupiah(amount int64) string {
	sign := ""
	if amount < 0 {
		sign = "-"
		amount = -amount
	}
	return fmt.Sprintf("%sRp%s", sign, formatThousand(amount))
}

// ParseRupiahToInt parses "Rp 1.000" or "1,000" into an integer amount of Rupiah.
func ParseRupiahToInt(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.ToLower(s), "rp")
	s = strings.TrimSpace(s)
	replacer := strings.NewReplacer(".", "", ",", "", " ", "")
	s = replacer.Replace(s)
	if s == "" {
		return 0, fmt.Errorf("invalid rupiah amount")
	}
	return strconv.ParseInt(s, 10, 64)
}

func formatThousand(n int64) string {
	if n == 0 {
		return "0"
	}
	str := strconv.FormatInt(n, 10)
	var out strings.Builder
	for i, c := range str {
		if i != 0 && (len(str)-i)%3 == 0 {
			out.WriteByte('.')
		}
		out.WriteRune(c)
	}
	return out.String()
}
