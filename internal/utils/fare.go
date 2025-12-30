package utils

import "strings"

// ComputeFare returns harga per seat berdasarkan rute (case-insensitive).
// Jika tidak ada yang cocok, akan mengembalikan fallbackPrice (mis: price_per_seat dari booking) atau 0.
func ComputeFare(from, to string, fallbackPrice int64) int64 {
	f := strings.TrimSpace(strings.ToLower(from))
	t := strings.TrimSpace(strings.ToLower(to))
	if f == "" || t == "" {
		return fallbackPrice
	}

	match := func(a, b string) bool {
		return (f == a && t == b) || (f == b && t == a)
	}

	// Kelompok rute utama (Pasirpengaraian cluster <-> Pekanbaru)
	cluster := []string{"skpd", "simpang d", "skpc", "simpang kumu", "muara rumbai", "surau tinggi", "pasirpengaraian", "pasir pengaraian"}
	for _, c := range cluster {
		if match(c, "pekanbaru") {
			return 150_000
		}
	}

	for _, c := range cluster {
		if match(c, "kabun") {
			return 120_000
		}
	}
	for _, c := range cluster {
		if match(c, "tandun") {
			return 100_000
		}
	}
	for _, c := range cluster {
		if match(c, "petapahan") {
			return 130_000
		}
	}
	for _, c := range cluster {
		if match(c, "suram") {
			return 120_000
		}
	}
	for _, c := range cluster {
		if match(c, "aliantan") {
			return 120_000
		}
	}
	for _, c := range cluster {
		if match(c, "bangkinang") {
			return 130_000
		}
	}

	if match("bangkinang", "pekanbaru") {
		return 100_000
	}
	if match("ujung batu", "pekanbaru") {
		return 130_000
	}
	if match("suram", "pekanbaru") {
		return 120_000
	}
	if match("petapahan", "pekanbaru") {
		return 100_000
	}

	return fallbackPrice
}
