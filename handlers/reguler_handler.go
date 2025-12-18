// backend/handlers/reguler_handler.go
package handlers

import (
	"backend/config"
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
)

const (
	RegulerMaxTotal = int64(900000) // guard maksimum (6 x 150k, dll)
)

// ====== quote endpoint ======

type RegulerQuoteRequest struct {
	Category       string   `json:"category"`
	From           string   `json:"from"`
	To             string   `json:"to"`
	Date           string   `json:"date"`
	Time           string   `json:"time"`
	SelectedSeats  []string `json:"selectedSeats"`
	PassengerCount int      `json:"passengerCount"`
}

type RegulerQuoteResponse struct {
	PricePerSeat int   `json:"pricePerSeat"`
	Total        int64 `json:"total"`
	Route        string `json:"route"`
}

// ====== stops endpoint ======

type StopItem struct {
	Key     string `json:"key"`
	Display string `json:"display"`
}

// ====== util normalisasi ======

func normalizeKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, ".", "")
	return s
}

// canonical display name (konsisten untuk DB & unique index booking_seats)
func canonicalStopDisplay(s string) (display string, key string, ok bool) {
	k := normalizeKey(s)

	switch k {
	case "skpd":
		return "SKPD", "skpd", true
	case "simpangd":
		return "Simpang D", "simpangd", true
	case "skpc":
		return "SKPC", "skpc", true
	case "simpangkumu":
		return "Simpang Kumu", "simpangkumu", true
	case "muararumbai":
		return "Muara Rumbai", "muararumbai", true
	case "surautinggi":
		return "Surau Tinggi", "surautinggi", true
	case "pasirpengaraian", "pasipengaraian":
		return "Pasir Pengaraian", "pasirpengaraian", true
	case "ub", "ujungbatu":
		return "Ujung Batu", "ujungbatu", true
	case "tandun":
		return "Tandun", "tandun", true
	case "silam":
		return "Silam", "silam", true
	case "petapahan":
		return "Petapahan", "petapahan", true
	case "suram":
		return "Suram", "suram", true
	case "aliantan":
		return "Aliantan", "aliantan", true
	case "kuok":
		return "Kuok", "kuok", true
	case "bangkinang":
		return "Bangkinang", "bangkinang", true
	case "kabun":
		return "Kabun", "kabun", true
	case "pku", "pekanbaru":
		return "Pekanbaru", "pekanbaru", true
	default:
		return "", "", false
	}
}

// ====== pricing rules (bidirectional) sesuai ongkos ======

var groupA = map[string]bool{
	"skpd":            true,
	"simpangd":        true,
	"skpc":            true,
	"simpangkumu":     true,
	"muararumbai":     true,
	"surautinggi":     true,
	"pasirpengaraian": true,
}

func isPair(a, b, x, y string) bool {
	return (a == x && b == y) || (a == y && b == x)
}

func regulerFarePerSeat(fromKey, toKey string) int64 {
	if fromKey == "" || toKey == "" || fromKey == toKey {
		return 0
	}

	// khusus
	if isPair(fromKey, toKey, "bangkinang", "pekanbaru") {
		return 100000
	}
	if isPair(fromKey, toKey, "ujungbatu", "pekanbaru") {
		return 130000
	}
	if isPair(fromKey, toKey, "suram", "pekanbaru") {
		return 120000
	}
	if isPair(fromKey, toKey, "petapahan", "pekanbaru") {
		return 100000
	}

	// Group A <-> tujuan tertentu
	if groupA[fromKey] || groupA[toKey] {
		other := toKey
		if groupA[toKey] {
			other = fromKey
		}
		switch other {
		case "pekanbaru":
			return 150000
		case "kabun":
			return 120000
		case "tandun":
			return 100000
		case "petapahan":
			return 130000
		case "suram":
			return 120000
		case "aliantan":
			return 120000
		case "bangkinang":
			return 130000
		}
	}

	return 0
}

// ====== util validation ======

func normalizeTimeStr(t string) (string, error) {
	re := regexp.MustCompile(`\b(\d{2}):(\d{2})\b`)
	m := re.FindStringSubmatch(t)
	if len(m) < 3 {
		return "", errors.New("format time tidak valid (contoh: 08:00 atau 08:00 WIB)")
	}
	hhmm := m[0]
	if _, err := time.Parse("15:04", hhmm); err != nil {
		return "", errors.New("format time tidak valid")
	}
	return hhmm, nil
}

func hasDuplicates(arr []string) bool {
	seen := map[string]bool{}
	for _, v := range arr {
		k := strings.ToUpper(strings.TrimSpace(v))
		if k == "" {
			continue
		}
		if seen[k] {
			return true
		}
		seen[k] = true
	}
	return false
}

func normalizeBookingFor(s string) string {
	v := strings.ToLower(strings.TrimSpace(s))
	if v == "" {
		return ""
	}
	if v == "self" || v == "other" {
		return v
	}
	return ""
}

func normalizeSeats(arr []string) []string {
	out := make([]string, 0, len(arr))
	for _, s := range arr {
		x := strings.ToUpper(strings.TrimSpace(s))
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

func bestOrderBy(db queryRower, table string) string {
	if hasColumn(db, table, "created_at") {
		return "created_at ASC"
	}
	if hasColumn(db, table, "seat_code") {
		return "seat_code ASC"
	}
	return ""
}

// ======================================================
// ✅ 1) GET /api/reguler/stops
// ======================================================
func GetRegulerStops(c *gin.Context) {
	stops := []StopItem{
		{Key: "skpd", Display: "SKPD"},
		{Key: "simpangd", Display: "Simpang D"},
		{Key: "skpc", Display: "SKPC"},
		{Key: "simpangkumu", Display: "Simpang Kumu"},
		{Key: "muararumbai", Display: "Muara Rumbai"},
		{Key: "surautinggi", Display: "Surau Tinggi"},
		{Key: "pasirpengaraian", Display: "Pasir Pengaraian"},
		{Key: "ujungbatu", Display: "Ujung Batu"},
		{Key: "tandun", Display: "Tandun"},
		{Key: "silam", Display: "Silam"},
		{Key: "petapahan", Display: "Petapahan"},
		{Key: "suram", Display: "Suram"},
		{Key: "aliantan", Display: "Aliantan"},
		{Key: "kuok", Display: "Kuok"},
		{Key: "bangkinang", Display: "Bangkinang"},
		{Key: "kabun", Display: "Kabun"},
		{Key: "pekanbaru", Display: "Pekanbaru"},
	}
	c.JSON(http.StatusOK, gin.H{"stops": stops})
}

// ======================================================
// ✅ 2) GET /api/reguler/seats?from=&to=&date=&time=
// ======================================================
func GetRegulerSeats(c *gin.Context) {
	from := strings.TrimSpace(c.Query("from"))
	to := strings.TrimSpace(c.Query("to"))
	date := strings.TrimSpace(c.Query("date"))
	tm := strings.TrimSpace(c.Query("time"))

	if from == "" || to == "" || date == "" || tm == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "from, to, date, time wajib"})
		return
	}

	fromDisplay, _, ok := canonicalStopDisplay(from)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Origin tidak didukung"})
		return
	}
	toDisplay, _, ok := canonicalStopDisplay(to)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Destination tidak didukung"})
		return
	}

	if _, err := time.Parse("2006-01-02", date); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Format date tidak valid (YYYY-MM-DD)"})
		return
	}

	hhmm, err := normalizeTimeStr(tm)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	rows, err := config.DB.Query(`
		SELECT seat_code
		FROM booking_seats
		WHERE route_from = ?
		  AND route_to = ?
		  AND trip_date = ?
		  AND trip_time = ?
	`, fromDisplay, toDisplay, date, hhmm)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal mengambil seat"})
		return
	}
	defer rows.Close()

	booked := []string{}
	for rows.Next() {
		var seat string
		if err := rows.Scan(&seat); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal membaca seat"})
			return
		}
		booked = append(booked, strings.ToUpper(strings.TrimSpace(seat)))
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal membaca hasil seat"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"bookedSeats": booked})
}

// ======================================================
// ✅ 3) POST /api/reguler/quote
// ======================================================
func GetRegulerQuote(c *gin.Context) {
	var req RegulerQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "JSON tidak valid"})
		return
	}

	req.From = strings.TrimSpace(req.From)
	req.To = strings.TrimSpace(req.To)
	req.Date = strings.TrimSpace(req.Date)
	req.Time = strings.TrimSpace(req.Time)

	fromDisplay, fromKey, ok := canonicalStopDisplay(req.From)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Origin tidak didukung"})
		return
	}
	toDisplay, toKey, ok := canonicalStopDisplay(req.To)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Destination tidak didukung"})
		return
	}
	if fromKey == toKey {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Origin dan Destination tidak boleh sama"})
		return
	}

	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Format date tidak valid (YYYY-MM-DD)"})
		return
	}
	hhmm, err := normalizeTimeStr(req.Time)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	seats := normalizeSeats(req.SelectedSeats)
	if len(seats) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "selectedSeats wajib"})
		return
	}
	if hasDuplicates(seats) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Seat tidak boleh duplikat"})
		return
	}

	pax := req.PassengerCount
	if pax <= 0 {
		pax = len(seats)
	}
	if pax != len(seats) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "PassengerCount harus sama dengan jumlah seat"})
		return
	}
	if pax > RegulerMaxPax {
		c.JSON(http.StatusBadRequest, gin.H{"message": "PassengerCount maksimal 6"})
		return
	}

	pricePerSeat := regulerFarePerSeat(fromKey, toKey)
	if pricePerSeat <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Tarif rute ini belum tersedia. Pilih rute lain."})
		return
	}

	total := int64(pax) * pricePerSeat
	if total > RegulerMaxTotal {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Total melebihi batas maksimal (900.000)"})
		return
	}

	// cek conflict seat sebelum user klik Next
	placeholders := make([]string, 0, len(seats))
	args := make([]any, 0, 4+len(seats))
	for range seats {
		placeholders = append(placeholders, "?")
	}
	args = append(args, fromDisplay, toDisplay, req.Date, hhmm)
	for _, s := range seats {
		args = append(args, s)
	}

	q := `
		SELECT seat_code
		FROM booking_seats
		WHERE route_from = ?
		  AND route_to = ?
		  AND trip_date = ?
		  AND trip_time = ?
		  AND seat_code IN (` + strings.Join(placeholders, ",") + `)
		LIMIT 1
	`
	var taken string
	err = config.DB.QueryRow(q, args...).Scan(&taken)
	if err == nil && strings.TrimSpace(taken) != "" {
		c.JSON(http.StatusConflict, gin.H{"message": "Ada seat yang sudah dibooking. Silakan pilih seat lain."})
		return
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal cek seat availability"})
		return
	}

	c.JSON(http.StatusOK, RegulerQuoteResponse{
		PricePerSeat: int(pricePerSeat),
		Total:        total,
		Route:        fromDisplay + " -> " + toDisplay,
	})
}

// ======================================================
// ✅ 4) POST /api/reguler/bookings
// ======================================================
func CreateRegulerBooking(c *gin.Context) {
	var req RegulerBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "JSON tidak valid"})
		return
	}

	req.Category = strings.TrimSpace(req.Category)
	req.From = strings.TrimSpace(req.From)
	req.To = strings.TrimSpace(req.To)
	req.Date = strings.TrimSpace(req.Date)
	req.Time = strings.TrimSpace(req.Time)
	req.PassengerName = strings.TrimSpace(req.PassengerName)
	req.PassengerPhone = strings.TrimSpace(req.PassengerPhone)
	req.PickupLocation = strings.TrimSpace(req.PickupLocation)
	req.DropoffLocation = strings.TrimSpace(req.DropoffLocation)
	req.PaymentMethod = strings.ToLower(strings.TrimSpace(req.PaymentMethod))

	if req.Category == "" {
		req.Category = "Reguler"
	}

	fromDisplay, fromKey, ok := canonicalStopDisplay(req.From)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Origin tidak didukung"})
		return
	}
	toDisplay, toKey, ok := canonicalStopDisplay(req.To)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Destination tidak didukung"})
		return
	}
	if fromKey == toKey {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Origin dan Destination tidak boleh sama"})
		return
	}

	pricePerSeat := regulerFarePerSeat(fromKey, toKey)
	if pricePerSeat <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Tarif rute ini belum tersedia. Pilih rute lain."})
		return
	}

	if _, err := time.Parse("2006-01-02", req.Date); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Format date tidak valid (YYYY-MM-DD)"})
		return
	}

	hhmm, err := normalizeTimeStr(req.Time)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if req.PassengerName == "" || req.PickupLocation == "" || req.DropoffLocation == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Nama pemesan, pickup, dan dropoff wajib diisi"})
		return
	}
	if req.PassengerPhone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "No HP pemesan wajib diisi"})
		return
	}

	req.SelectedSeats = normalizeSeats(req.SelectedSeats)
	if len(req.SelectedSeats) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Wajib pilih seat"})
		return
	}

	if req.PassengerCount <= 0 {
		req.PassengerCount = len(req.SelectedSeats)
	}
	if req.PassengerCount > RegulerMaxPax {
		c.JSON(http.StatusBadRequest, gin.H{"message": "PassengerCount maksimal 6"})
		return
	}
	if len(req.SelectedSeats) != req.PassengerCount {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Jumlah seat yang dipilih harus sama dengan jumlah penumpang"})
		return
	}
	if hasDuplicates(req.SelectedSeats) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Seat tidak boleh duplikat"})
		return
	}

	req.BookingFor = normalizeBookingFor(req.BookingFor)

	total := int64(req.PassengerCount) * pricePerSeat
	if total > RegulerMaxTotal {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Total melebihi batas maksimal (900.000)"})
		return
	}

	// payment status default
	paymentMethod := req.PaymentMethod
	if paymentMethod != "cash" && paymentMethod != "transfer" && paymentMethod != "qris" {
		paymentMethod = "" // biar tidak nyimpan value aneh
	}
	paymentStatus := ""
	if paymentMethod == "cash" {
		paymentStatus = "Lunas"
	} else if paymentMethod == "transfer" || paymentMethod == "qris" {
		paymentStatus = "Menunggu Validasi"
	}

	tx, err := config.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal membuka transaction"})
		return
	}
	defer tx.Rollback()

	cols := []string{
		"category",
		"route_from",
		"route_to",
		"trip_date",
		"trip_time",
		"passenger_name",
		"passenger_count",
		"pickup_location",
		"dropoff_location",
		"price_per_seat",
		"total",
	}
	args := []any{
		req.Category,
		fromDisplay,
		toDisplay,
		req.Date,
		hhmm,
		req.PassengerName,
		req.PassengerCount,
		req.PickupLocation,
		req.DropoffLocation,
		pricePerSeat,
		total,
	}

	if hasColumn(tx, "bookings", "booking_for") && req.BookingFor != "" {
		cols = append(cols, "booking_for")
		args = append(args, req.BookingFor)
	}
	if hasColumn(tx, "bookings", "passenger_phone") {
		cols = append(cols, "passenger_phone")
		args = append(args, req.PassengerPhone)
	}

	// payment columns (opsional, tergantung schema)
	if hasColumn(tx, "bookings", "payment_method") && paymentMethod != "" {
		cols = append(cols, "payment_method")
		args = append(args, paymentMethod)
	}
	if hasColumn(tx, "bookings", "payment_status") && paymentStatus != "" {
		cols = append(cols, "payment_status")
		args = append(args, paymentStatus)
	}

	ph := make([]string, 0, len(cols))
	for range cols {
		ph = append(ph, "?")
	}

	insertBookingSQL := `
		INSERT INTO bookings (` + strings.Join(cols, ",") + `, created_at)
		VALUES (` + strings.Join(ph, ",") + `, NOW())
	`
	bookingRes, err := tx.Exec(insertBookingSQL, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal menyimpan booking"})
		return
	}
	bookingID, _ := bookingRes.LastInsertId()

	for _, seat := range req.SelectedSeats {
		_, err := tx.Exec(`
			INSERT INTO booking_seats
			(booking_id, route_from, route_to, trip_date, trip_time, seat_code, created_at)
			VALUES (?, ?, ?, ?, ?, ?, NOW())
		`,
			bookingID,
			fromDisplay,
			toDisplay,
			req.Date,
			hhmm,
			seat,
		)
		if err != nil {
			var me *mysql.MySQLError
			if errors.As(err, &me) && me.Number == 1062 {
				c.JSON(http.StatusConflict, gin.H{"message": "Seat sudah dibooking orang lain, silakan pilih seat lain"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal menyimpan seat"})
			return
		}
	}

	// simpan nama penumpang per seat (opsional)
	if hasTable(tx, "booking_passengers") && len(req.Passengers) > 0 {
		if len(req.Passengers) != req.PassengerCount {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Jika mengisi passengers, jumlahnya harus sama dengan passengerCount"})
			return
		}

		selectedSet := map[string]bool{}
		for _, s := range req.SelectedSeats {
			selectedSet[s] = true
		}
		seatSeen := map[string]bool{}
		for _, p := range req.Passengers {
			seat := strings.ToUpper(strings.TrimSpace(p.Seat))
			name := strings.TrimSpace(p.Name)
			if seat == "" || name == "" {
				c.JSON(http.StatusBadRequest, gin.H{"message": "Data penumpang tidak boleh kosong"})
				return
			}
			if !selectedSet[seat] {
				c.JSON(http.StatusBadRequest, gin.H{"message": "Seat penumpang harus sesuai seat yang dipilih"})
				return
			}
			if seatSeen[seat] {
				c.JSON(http.StatusBadRequest, gin.H{"message": "Seat penumpang tidak boleh duplikat"})
				return
			}
			seatSeen[seat] = true

			_, err := tx.Exec(`
				INSERT INTO booking_passengers
				(booking_id, seat_code, passenger_name, created_at)
				VALUES (?, ?, ?, NOW())
			`, bookingID, seat, name)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal menyimpan data penumpang"})
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "Gagal commit booking"})
		return
	}

	c.JSON(http.StatusCreated, RegulerBookingResponse{
		BookingID: bookingID,

		Category: req.Category,
		From:     fromDisplay,
		To:       toDisplay,
		Date:     req.Date,
		Time:     hhmm,

		SelectedSeats: req.SelectedSeats,

		PassengerName:  req.PassengerName,
		PassengerPhone: req.PassengerPhone,

		PickupLocation:  req.PickupLocation,
		DropoffLocation: req.DropoffLocation,

		PricePerSeat: pricePerSeat,
		TotalAmount:  total,

		PaymentMethod: paymentMethod,
		PaymentStatus: paymentStatus,
	})
}

// ======================================================
// ✅ 5) GET /api/reguler/bookings/:id/surat-jalan
// ======================================================

type SuratJalanPassenger struct {
	Seat string `json:"seat"`
	Name string `json:"name"`
}

type RegulerSuratJalanResponse struct {
	BookingID       int64                `json:"bookingId"`
	RouteFrom       string               `json:"routeFrom"`
	RouteTo         string               `json:"routeTo"`
	TripDate        string               `json:"tripDate"`
	TripTime        string               `json:"tripTime"`
	PickupLocation  string               `json:"pickupLocation"`
	DropoffLocation string               `json:"dropoffLocation"`
	PricePerSeat    int64                `json:"pricePerSeat"`
	Total           int64                `json:"total"`
	PassengerPhone  string               `json:"passengerPhone"`
	Passengers      []SuratJalanPassenger `json:"passengers"`
}

func GetRegulerSuratJalan(c *gin.Context) {
	idStr := strings.TrimSpace(c.Param("id"))
	id64, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id64 <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id tidak valid"})
		return
	}

	var (
		routeFrom       string
		routeTo         string
		tripDate        string
		tripTime        string
		pickupLocation  string
		dropoffLocation string
		pricePerSeat    int64
		total           int64
		phoneStr        string
	)

	if hasColumn(config.DB, "bookings", "passenger_phone") {
		var passengerPhone sql.NullString
		err = config.DB.QueryRow(`
			SELECT route_from, route_to, trip_date, trip_time, pickup_location, dropoff_location, price_per_seat, total, passenger_phone
			FROM bookings
			WHERE id = ?
			LIMIT 1
		`, id64).Scan(
			&routeFrom, &routeTo, &tripDate, &tripTime,
			&pickupLocation, &dropoffLocation,
			&pricePerSeat, &total, &passengerPhone,
		)
		if passengerPhone.Valid {
			phoneStr = strings.TrimSpace(passengerPhone.String)
		}
	} else {
		err = config.DB.QueryRow(`
			SELECT route_from, route_to, trip_date, trip_time, pickup_location, dropoff_location, price_per_seat, total
			FROM bookings
			WHERE id = ?
			LIMIT 1
		`, id64).Scan(
			&routeFrom, &routeTo, &tripDate, &tripTime,
			&pickupLocation, &dropoffLocation,
			&pricePerSeat, &total,
		)
	}

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"message": "booking tidak ditemukan"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal mengambil data booking: " + err.Error()})
		return
	}

	passengers := []SuratJalanPassenger{}

	// ---- ambil dari booking_passengers ----
	if hasTable(config.DB, "booking_passengers") {
		order := bestOrderBy(config.DB, "booking_passengers")
		q := `
			SELECT seat_code, passenger_name
			FROM booking_passengers
			WHERE booking_id = ?
		`
		if order != "" {
			q += " ORDER BY " + order
		}

		rows, qerr := config.DB.Query(q, id64)
		if qerr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal query booking_passengers: " + qerr.Error()})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var seat, name string
			if err := rows.Scan(&seat, &name); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal membaca data penumpang: " + err.Error()})
				return
			}
			passengers = append(passengers, SuratJalanPassenger{
				Seat: strings.ToUpper(strings.TrimSpace(seat)),
				Name: strings.TrimSpace(name),
			})
		}
		if err := rows.Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal membaca hasil penumpang: " + err.Error()})
			return
		}
	}

	// ---- fallback dari booking_seats (minimal ada seat) ----
	if len(passengers) == 0 && hasTable(config.DB, "booking_seats") {
		order := bestOrderBy(config.DB, "booking_seats")
		q := `
			SELECT seat_code
			FROM booking_seats
			WHERE booking_id = ?
		`
		if order != "" {
			q += " ORDER BY " + order
		}

		rows, qerr := config.DB.Query(q, id64)
		if qerr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal query booking_seats: " + qerr.Error()})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var seat string
			if err := rows.Scan(&seat); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal membaca seat: " + err.Error()})
				return
			}
			passengers = append(passengers, SuratJalanPassenger{
				Seat: strings.ToUpper(strings.TrimSpace(seat)),
				Name: "",
			})
		}
	}

	c.JSON(http.StatusOK, RegulerSuratJalanResponse{
		BookingID:       id64,
		RouteFrom:       routeFrom,
		RouteTo:         routeTo,
		TripDate:        tripDate,
		TripTime:        tripTime,
		PickupLocation:  pickupLocation,
		DropoffLocation: dropoffLocation,
		PricePerSeat:    pricePerSeat,
		Total:           total,
		PassengerPhone:  phoneStr,
		Passengers:      passengers,
	})
}
