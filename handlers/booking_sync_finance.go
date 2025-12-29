package handlers

import (
	"backend/config"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// lookupInvoiceTotal membaca nominal total invoice terkait booking (jika ada tabel/kolomnya).
// Dipakai sebagai fallback ketika kolom total di tabel booking kosong.
func lookupInvoiceTotal(tx *sql.Tx, bookingID int64) int64 {
	if bookingID <= 0 {
		return 0
	}

	type invoiceSource struct {
		table       string
		bookingCols []string
		totalCols   []string
	}

	sources := []invoiceSource{
		{table: "booking_invoices", bookingCols: []string{"booking_id", "reguler_booking_id"}, totalCols: []string{"total_amount", "grand_total", "total", "amount", "nominal"}},
		{table: "invoices", bookingCols: []string{"booking_id", "reguler_booking_id"}, totalCols: []string{"total_amount", "grand_total", "total", "amount", "nominal"}},
	}

	for _, src := range sources {
		if !hasTable(tx, src.table) {
			continue
		}

		totalCol := ""
		for _, col := range src.totalCols {
			if hasColumn(tx, src.table, col) {
				totalCol = col
				break
			}
		}
		if totalCol == "" {
			continue
		}

		for _, bcol := range src.bookingCols {
			if !hasColumn(tx, src.table, bcol) {
				continue
			}

			var raw sql.NullString
			q := fmt.Sprintf(`SELECT COALESCE(%s,'') FROM %s WHERE %s=? ORDER BY id DESC LIMIT 1`, totalCol, src.table, bcol)
			if err := tx.QueryRow(q, bookingID).Scan(&raw); err != nil {
				continue
			}

			if raw.Valid {
				val := strings.TrimSpace(raw.String)
				if val == "" {
					continue
				}
				if n, err := strconv.ParseFloat(val, 64); err == nil && n > 0 {
					return int64(n)
				}
			}
		}
	}

	return 0
}

/*
	========================================================
	バ. TAMBAHAN: sinkronisasi ke laporan keuangan (trips)
	- day/month/year dari tanggal booking (trip_date)
	- car_code/driver_name/vehicle_name dari departure_settings (Drivers & Unit + Jenis Kendaraan)
	- order_no format LKT/NN/KODE
	- dept_origin/dept_dest dari booking (origin/destination)
	- dept_category dari service type (reguler/droping/rental)
	- dept_passenger_count & fare dari agregasi invoice/booking paid
	========================================================
*/

func parseDayMonthYear(dateStr string) (int, int, int) {
	clean := normalizeDateOnly(dateStr)
	if clean == "" {
		now := time.Now()
		return now.Day(), int(now.Month()), now.Year()
	}
	d, err := time.Parse("2006-01-02", clean)
	if err != nil {
		now := time.Now()
		return now.Day(), int(now.Month()), now.Year()
	}
	return d.Day(), int(d.Month()), d.Year()
}

// routeFareIDR: daftar ongkos otomatis (bisa kamu perluas).
func routeFareIDR(from, to string) int64 {
	f := strings.ToLower(strings.TrimSpace(from))
	t := strings.ToLower(strings.TrimSpace(to))
	if f == "" || t == "" {
		return 0
	}

	// kelompok Pasirpengaraian & sekitarnya
	group := map[string]bool{
		"skpd":            true,
		"simpang d":       true,
		"skpc":            true,
		"simpang kumu":    true,
		"muara rumbai":    true,
		"surau tinggi":    true,
		"pasirpengaraian": true,
	}

	isGroup := func(x string) bool { return group[x] }

	// helper bidirectional fare
	match := func(a, b string) bool {
		return (f == a && t == b) || (f == b && t == a)
	}

	// 1) group <-> pekanbaru 150k
	if (isGroup(f) && t == "pekanbaru") || (isGroup(t) && f == "pekanbaru") {
		return 150_000
	}

	// 2) group <-> kabun 120k
	if (isGroup(f) && t == "kabun") || (isGroup(t) && f == "kabun") {
		return 120_000
	}

	// 4) group <-> tandun 100k
	if (isGroup(f) && t == "tandun") || (isGroup(t) && f == "tandun") {
		return 100_000
	}

	// 5) group <-> petapahan 130k
	if (isGroup(f) && t == "petapahan") || (isGroup(t) && f == "petapahan") {
		return 130_000
	}

	// 6) group <-> suram 120k
	if (isGroup(f) && t == "suram") || (isGroup(t) && f == "suram") {
		return 120_000
	}

	// 7) group <-> aliantan 120k
	if (isGroup(f) && t == "aliantan") || (isGroup(t) && f == "aliantan") {
		return 120_000
	}

	// 8) group <-> bangkinang 130k
	if (isGroup(f) && t == "bangkinang") || (isGroup(t) && f == "bangkinang") {
		return 130_000
	}

	// 9) bangkinang <-> pekanbaru 100k
	if match("bangkinang", "pekanbaru") {
		return 100_000
	}

	// 10) ujung batu <-> pekanbaru 130k
	if match("ujung batu", "pekanbaru") {
		return 130_000
	}

	// 11) suram <-> pekanbaru 120k
	if match("suram", "pekanbaru") {
		return 120_000
	}

	// 12) petapahan <-> pekanbaru 100k (catatan: kamu tulis nomor 7 lagi)
	if match("petapahan", "pekanbaru") {
		return 100_000
	}

	return 0
}

// regulerFareFromBooking: samakan tarif dengan aturan di reguler_handler.go (regulerFarePerSeat).
// Jika tidak cocok, fallback ke routeFareIDR (legacy mapping).
func regulerFareFromBooking(from, to string) int64 {
	fk := normalizeKey(from)
	tk := normalizeKey(to)
	if fare := regulerFarePerSeat(fk, tk); fare > 0 {
		return fare
	}
	return routeFareIDR(from, to)
}

// loadDriverVehicleType: ambil vehicle_type dari akun driver (driver_accounts/drivers) berdasarkan driver name
func loadDriverVehicleTypeByDriverName(driverName string) string {
	n := strings.ToLower(strings.TrimSpace(driverName))
	if n == "" {
		return ""
	}

	// coba dari driver_accounts dulu
	if hasTable(config.DB, "driver_accounts") && hasColumn(config.DB, "driver_accounts", "vehicle_type") {
		var vt sql.NullString
		_ = config.DB.QueryRow(
			`SELECT COALESCE(vehicle_type,'') FROM driver_accounts WHERE LOWER(TRIM(driver_name)) = ? LIMIT 1`,
			n,
		).Scan(&vt)
		if strings.TrimSpace(vt.String) != "" {
			return strings.TrimSpace(vt.String)
		}
	}

	// fallback: tabel drivers
	if hasTable(config.DB, "drivers") && hasColumn(config.DB, "drivers", "vehicle_type") {
		var vt sql.NullString
		_ = config.DB.QueryRow(
			`SELECT COALESCE(vehicle_type,'') FROM drivers WHERE LOWER(TRIM(name)) = ? LIMIT 1`,
			n,
		).Scan(&vt)
		return strings.TrimSpace(vt.String)
	}

	return ""
}

// loadBookingFinancePayload mengambil data booking untuk kebutuhan sinkronisasi laporan keuangan.
func loadBookingFinancePayload(bookingID int64) (BookingSyncPayload, bool) {
	if bookingID <= 0 {
		return BookingSyncPayload{}, false
	}

	tx, err := config.DB.Begin()
	if err != nil {
		log.Println("loadBookingFinancePayload begin tx error:", err)
		return BookingSyncPayload{}, false
	}
	defer func() { _ = tx.Rollback() }()

	p, err := readBookingPayload(tx, bookingID)
	if err != nil {
		log.Println("loadBookingFinancePayload read error:", err)
		return BookingSyncPayload{}, false
	}

	if err := tx.Commit(); err != nil {
		log.Println("loadBookingFinancePayload commit error:", err)
		return BookingSyncPayload{}, false
	}

	return p, true
}

type bookingAggregate struct {
	DeptCount int
	DeptTotal int64
	RetCount  int
	RetTotal  int64
}

// loadPaidSeatAggregate mengembalikan jumlah kursi dan total paid_price dari booking_passengers (kalau kolom tersedia).
func aggregatePaidBookings(date, timeStr, from, to string) bookingAggregate {
	tx, err := config.DB.Begin()
	if err != nil {
		return bookingAggregate{}
	}
	defer func() { _ = tx.Rollback() }()

	agg := aggregatePaidBookingsTx(tx, date, timeStr, from, to)

	if err := tx.Commit(); err != nil {
		return agg
	}

	return agg
}

// aggregatePaidBookingsTx menjumlahkan penumpang & tarif invoice untuk slot (tanggal + jam) yang sudah tervalidasi.
// Versi ini pakai tx agar konsisten di transaksi approve.
func aggregatePaidBookingsTx(tx *sql.Tx, date, timeStr, from, to string) bookingAggregate {
	table := ""
	switch {
	case hasTable(tx, "bookings"):
		table = "bookings"
	case hasTable(tx, "reguler_bookings"):
		table = "reguler_bookings"
	default:
		return bookingAggregate{}
	}

	if !hasColumn(tx, table, "trip_date") || !hasColumn(tx, table, "trip_time") {
		return bookingAggregate{}
	}

	dateOnly := normalizeDateOnly(date)
	if dateOnly == "" {
		return bookingAggregate{}
	}
	timeOnly := normalizeTripTime(timeStr)

	seatCol := ""
	for _, c := range []string{"selected_seats", "seats_json"} {
		if hasColumn(tx, table, c) {
			seatCol = c
			break
		}
	}

	fromCol := ""
	for _, c := range []string{"from_city", "route_from"} {
		if hasColumn(tx, table, c) {
			fromCol = c
			break
		}
	}

	toCol := ""
	for _, c := range []string{"to_city", "route_to"} {
		if hasColumn(tx, table, c) {
			toCol = c
			break
		}
	}

	totalCol := "total_amount"
	if !hasColumn(tx, table, totalCol) && hasColumn(tx, table, "total") {
		totalCol = "total"
	}

	seatSel := "''"
	if seatCol != "" {
		seatSel = "COALESCE(" + seatCol + ", '')"
	}

	fromSel := "''"
	if fromCol != "" {
		fromSel = "COALESCE(" + fromCol + ", '')"
	}

	toSel := "''"
	if toCol != "" {
		toSel = "COALESCE(" + toCol + ", '')"
	}

	payStatusSel := "''"
	if hasColumn(tx, table, "payment_status") {
		payStatusSel = "COALESCE(payment_status, '')"
	}

	payMethodSel := "''"
	if hasColumn(tx, table, "payment_method") {
		payMethodSel = "COALESCE(payment_method, '')"
	}

	q := fmt.Sprintf(
		`SELECT id, %s, COALESCE(%s,0), %s, %s, %s, %s
		   FROM %s
		  WHERE DATE(COALESCE(trip_date,'')) = ?
		    AND LEFT(COALESCE(trip_time,''),5) = ?
		  ORDER BY id ASC`,
		seatSel, totalCol, payStatusSel, payMethodSel, fromSel, toSel, table,
	)

	rows, err := tx.Query(q, dateOnly, timeOnly)
	if err != nil {
		return bookingAggregate{}
	}

	type bookingRow struct {
		id        int64
		seatRaw   string
		totalAmt  int64
		payStatus string
		payMethod string
		routeFrom string
		routeTo   string
	}

	rowsData := []bookingRow{}
	hasRoute := strings.TrimSpace(from) != "" && strings.TrimSpace(to) != ""
	fromLC := strings.ToLower(strings.TrimSpace(from))
	toLC := strings.ToLower(strings.TrimSpace(to))

	for rows.Next() {
		var br bookingRow
		if err := rows.Scan(&br.id, &br.seatRaw, &br.totalAmt, &br.payStatus, &br.payMethod, &br.routeFrom, &br.routeTo); err != nil {
			continue
		}
		rowsData = append(rowsData, br)
	}

	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return bookingAggregate{}
	}
	_ = rows.Close()

	var agg bookingAggregate

	for _, row := range rowsData {
		if !isPaidSuccess(row.payStatus, row.payMethod) {
			continue
		}

		cnt := len(normalizeSeatsUnique(parseSeatsFlexible(row.seatRaw)))
		if cnt == 0 {
			cnt = 1
		}

		totalAmt := row.totalAmt
		if totalAmt == 0 {
			totalAmt = lookupInvoiceTotal(tx, row.id)
		}
		if totalAmt == 0 {
			if fare := regulerFareFromBooking(row.routeFrom, row.routeTo); fare > 0 {
				totalAmt = fare * int64(cnt)
			}
		}

		classified := false
		if hasRoute {
			rf := strings.ToLower(strings.TrimSpace(row.routeFrom))
			rt := strings.ToLower(strings.TrimSpace(row.routeTo))
			if rf == fromLC && rt == toLC {
				agg.DeptCount += cnt
				agg.DeptTotal += totalAmt
				classified = true
			} else if rf == toLC && rt == fromLC {
				agg.RetCount += cnt
				agg.RetTotal += totalAmt
				classified = true
			}
		}

		if !classified {
			agg.DeptCount += cnt
			agg.DeptTotal += totalAmt
		}
	}

	return agg
}

// buildOrderNumberForTrip membuat nomor order berbasis kode mobil dengan format LKT/NN/KODE.
func buildOrderNumberForTrip(carCode string, day, month, year int) string {
	cc := strings.ToUpper(strings.TrimSpace(carCode))
	if cc == "" {
		return ""
	}

	seq := 1
	if hasTable(config.DB, "trips") && hasColumn(config.DB, "trips", "car_code") {
		if hasColumn(config.DB, "trips", "day") && hasColumn(config.DB, "trips", "month") && hasColumn(config.DB, "trips", "year") {
			_ = config.DB.QueryRow(
				`SELECT COUNT(*) + 1 FROM trips WHERE car_code=? AND day=? AND month=? AND year=?`,
				cc, day, month, year,
			).Scan(&seq)
		} else {
			_ = config.DB.QueryRow(`SELECT COUNT(*) + 1 FROM trips WHERE car_code=?`, cc).Scan(&seq)
		}
	}

	if seq < 1 {
		seq = 1
	}

	return fmt.Sprintf("LKT/%02d/%s", seq, cc)
}

