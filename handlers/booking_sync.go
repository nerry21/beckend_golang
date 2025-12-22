package handlers

import (
	"backend/config"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"
)

type BookingSyncPayload struct {
	BookingID int64

	Category string
	From     string
	To       string
	Date     string
	Time     string

	PickupLocation  string
	DropoffLocation string

	PassengerName  string
	PassengerPhone string
	SelectedSeats  []string

	TotalAmount   int64
	PaymentMethod string
	PaymentStatus string
	CreatedAt     time.Time
}

// ==============================
// ✅ MySQL named lock (anti race condition)
// Tujuan: kalau 2 booking dibayar bersamaan untuk trip yang sama,
// jangan sampai trip_information ter-insert dobel karena race.
// Catatan: GET_LOCK hanya ada di MySQL/MariaDB (phpMyAdmin umumnya ini).
// Kalau DB bukan MySQL, fungsi ini akan error dan kita fallback ke logic biasa.
// ==============================

func acquireNamedLock(tx *sql.Tx, key string, timeoutSec int) error {
	if tx == nil || key == "" {
		return errors.New("acquireNamedLock: invalid args")
	}
	var got sql.NullInt64
	if err := tx.QueryRow(`SELECT GET_LOCK(?, ?)`, key, timeoutSec).Scan(&got); err != nil {
		return err
	}
	if !got.Valid || got.Int64 != 1 {
		return fmt.Errorf("acquireNamedLock: cannot get lock %s", key)
	}
	return nil
}

func releaseNamedLock(tx *sql.Tx, key string) {
	if tx == nil || key == "" {
		return
	}
	_, _ = tx.Exec(`SELECT RELEASE_LOCK(?)`, key)
}

// ✅ kompatibel dengan call site lama: SyncConfirmedRegulerBooking(tx, bookingID)
func SyncConfirmedRegulerBooking(tx *sql.Tx, bookingID int64) error {
	if bookingID <= 0 {
		return fmt.Errorf("SyncConfirmedRegulerBooking: bookingID invalid")
	}

	ownTx := false
	committed := false

	if tx == nil {
		t, err := config.DB.Begin()
		if err != nil {
			return err
		}
		tx = t
		ownTx = true
	}

	if ownTx {
		defer func() {
			if !committed {
				_ = tx.Rollback()
			}
		}()
	}

	p, err := readBookingPayload(tx, bookingID)
	if err != nil {
		return err
	}

	// hanya sync kalau booking sudah sukses/lunas
	if !isPaidSuccess(p.PaymentStatus, p.PaymentMethod) {
		log.Println("[SYNC] skip booking", p.BookingID, "status:", p.PaymentStatus, "method:", p.PaymentMethod)
		if ownTx {
			if err := tx.Commit(); err != nil {
				return err
			}
			committed = true
		}
		return nil
	}

	if err := syncAll(tx, p); err != nil {
		return err
	}

	if ownTx {
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
	}

	return nil
}

func isPaidSuccess(paymentStatus, paymentMethod string) bool {
	s := strings.ToLower(strings.TrimSpace(paymentStatus))
	m := strings.ToLower(strings.TrimSpace(paymentMethod))

	// cash biasanya langsung dianggap sukses
	if m == "cash" && (s == "" || s == "sukses" || s == "lunas" || s == "paid") {
		return true
	}

	// transfer/qris harus sukses/lunas
	if s == "sukses" || s == "lunas" || s == "paid" {
		return true
	}
	return false
}

// ===== internal =====

func readBookingPayload(tx *sql.Tx, bookingID int64) (BookingSyncPayload, error) {
	table := "bookings"
	if !hasTable(tx, table) {
		if hasTable(tx, "reguler_bookings") {
			table = "reguler_bookings"
		}
	}

	cols := []string{"id"}

	// kategori
	if hasColumn(tx, table, "category") {
		cols = append(cols, "category")
	}

	// route/from/to (support beberapa schema)
	if hasColumn(tx, table, "from_city") {
		cols = append(cols, "from_city")
	} else if hasColumn(tx, table, "route_from") {
		cols = append(cols, "route_from")
	}
	if hasColumn(tx, table, "to_city") {
		cols = append(cols, "to_city")
	} else if hasColumn(tx, table, "route_to") {
		cols = append(cols, "route_to")
	}

	// tanggal/jam
	if hasColumn(tx, table, "trip_date") {
		cols = append(cols, "trip_date")
	}
	if hasColumn(tx, table, "trip_time") {
		cols = append(cols, "trip_time")
	}

	// pickup/dropoff
	if hasColumn(tx, table, "pickup_location") {
		cols = append(cols, "pickup_location")
	}
	if hasColumn(tx, table, "dropoff_location") {
		cols = append(cols, "dropoff_location")
	}

	// nama pemesan
	if hasColumn(tx, table, "booking_for") {
		cols = append(cols, "booking_for")
	}
	if hasColumn(tx, table, "passenger_name") {
		cols = append(cols, "passenger_name")
	}
	if hasColumn(tx, table, "passenger_phone") {
		cols = append(cols, "passenger_phone")
	}

	// total
	if hasColumn(tx, table, "total_amount") {
		cols = append(cols, "total_amount")
	} else if hasColumn(tx, table, "total") {
		cols = append(cols, "total")
	}

	// payment
	if hasColumn(tx, table, "payment_method") {
		cols = append(cols, "payment_method")
	}
	if hasColumn(tx, table, "payment_status") {
		cols = append(cols, "payment_status")
	}

	// created_at
	if hasColumn(tx, table, "created_at") {
		cols = append(cols, "created_at")
	}

	// seats (json/string)
	if hasColumn(tx, table, "selected_seats") {
		cols = append(cols, "selected_seats")
	}
	if hasColumn(tx, table, "seats_json") {
		cols = append(cols, "seats_json")
	}

	q := fmt.Sprintf("SELECT %s FROM %s WHERE id=? LIMIT 1", strings.Join(cols, ","), table)

	var (
		id int64

		category sql.NullString

		fromC sql.NullString
		toC   sql.NullString

		tripDate sql.NullString
		tripTime sql.NullString

		pickup  sql.NullString
		dropoff sql.NullString

		bookingFor     sql.NullString
		passengerName  sql.NullString
		passengerPhone sql.NullString

		paymentMethod sql.NullString
		paymentStatus sql.NullString

		selectedSeatsJSON sql.NullString
		seatsJSON         sql.NullString

		totalAmount sql.NullInt64
		createdAt   sql.NullTime
	)

	dest := []any{&id}
	for _, c := range cols[1:] {
		switch c {
		case "category":
			dest = append(dest, &category)

		case "from_city", "route_from":
			dest = append(dest, &fromC)
		case "to_city", "route_to":
			dest = append(dest, &toC)

		case "trip_date":
			dest = append(dest, &tripDate)
		case "trip_time":
			dest = append(dest, &tripTime)

		case "pickup_location":
			dest = append(dest, &pickup)
		case "dropoff_location":
			dest = append(dest, &dropoff)

		case "booking_for":
			dest = append(dest, &bookingFor)
		case "passenger_name":
			dest = append(dest, &passengerName)
		case "passenger_phone":
			dest = append(dest, &passengerPhone)

		case "total_amount", "total":
			dest = append(dest, &totalAmount)

		case "payment_method":
			dest = append(dest, &paymentMethod)
		case "payment_status":
			dest = append(dest, &paymentStatus)

		case "created_at":
			dest = append(dest, &createdAt)

		case "selected_seats":
			dest = append(dest, &selectedSeatsJSON)
		case "seats_json":
			dest = append(dest, &seatsJSON)
		}
	}

	if err := tx.QueryRow(q, bookingID).Scan(dest...); err != nil {
		return BookingSyncPayload{}, err
	}

	// resolve name
	name := ""
	if passengerName.Valid {
		name = passengerName.String
	} else if bookingFor.Valid {
		name = bookingFor.String
	}

	// resolve seats
	rawSeats := ""
	if selectedSeatsJSON.Valid {
		rawSeats = selectedSeatsJSON.String
	} else if seatsJSON.Valid {
		rawSeats = seatsJSON.String
	}

	seats := parseSeatsFlexible(rawSeats)

	p := BookingSyncPayload{
		BookingID: bookingID,
		Category:  strings.TrimSpace(category.String),
		From:      strings.TrimSpace(fromC.String),
		To:        strings.TrimSpace(toC.String),
		// ✅ NORMALIZE: supaya trip_information tidak dobel
		Date:      normalizeTripDate(tripDate.String),
		Time:      normalizeTripTime(tripTime.String),

		PickupLocation:  strings.TrimSpace(pickup.String),
		DropoffLocation: strings.TrimSpace(dropoff.String),

		PassengerName:  strings.TrimSpace(name),
		PassengerPhone: strings.TrimSpace(passengerPhone.String),
		SelectedSeats:  normalizeSeatsUnique(seats),

		TotalAmount:   totalAmount.Int64,
		PaymentMethod: strings.TrimSpace(paymentMethod.String),
		PaymentStatus: strings.TrimSpace(paymentStatus.String),
		CreatedAt:     time.Now(),
	}

	if createdAt.Valid {
		p.CreatedAt = createdAt.Time
	}

	// default category
	if strings.TrimSpace(p.Category) == "" {
		p.Category = "Reguler"
	}

	return p, nil
}

func parseSeatsFlexible(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	// coba JSON array
	var seats []string
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &seats); err == nil {
			return seats
		}
	}

	// fallback: "1A, 2B"
	parts := strings.Split(raw, ",")
	for _, p := range parts {
		s := strings.ToUpper(strings.TrimSpace(p))
		if s != "" {
			seats = append(seats, s)
		}
	}
	return seats
}

// ==============================
// ✅ NORMALIZER (anti duplikat trip)
// - trip_date kadang tersimpan sebagai DATETIME / RFC3339 (contoh: 2025-12-22T00:00:00Z)
// - trip_time kadang tersimpan sebagai "08:00 WIB" / "08:00:00" / dll
// Tujuan: kunci trip (trip_number) selalu konsisten agar upsert tidak bikin baris ganda.
// ==============================

func normalizeFromTo(s string) string {
	// untuk membuat trip_number stabil
	x := strings.ToUpper(strings.TrimSpace(s))
	x = strings.ReplaceAll(x, "  ", " ")
	x = strings.ReplaceAll(x, " ", "")
	x = strings.ReplaceAll(x, "/", "-")
	x = strings.ReplaceAll(x, "\\", "-")
	return x
}

func normalizeSeatsUnique(seats []string) []string {
	out := make([]string, 0, len(seats))
	seen := map[string]bool{}
	for _, s := range seats {
		s = strings.ToUpper(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// normalizeTripDate memastikan nilai tanggal konsisten "YYYY-MM-DD".
// Ini penting supaya trip_number stabil dan trip_information tidak double untuk booking yang sama.
func normalizeTripDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// kalau sudah ada prefix YYYY-MM-DD, ambil 10 char pertama
	if len(s) >= 10 {
		prefix := s[:10]
		if _, err := time.Parse("2006-01-02", prefix); err == nil {
			return prefix
		}
	}

	// coba parse RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Format("2006-01-02")
	}

	// coba layout umum MySQL DATETIME
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"02-01-2006",
		"02/01/2006",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}

	return s
}

// normalizeTripTime mengubah berbagai format time ("08:00 WIB", "08:00:00") menjadi "HH:mm".
func normalizeTripTime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	re := regexp.MustCompile(`\b(\d{2}):(\d{2})\b`)
	m := re.FindStringSubmatch(s)
	if len(m) >= 3 {
		hhmm := m[0]
		if _, err := time.Parse("15:04", hhmm); err == nil {
			return hhmm
		}
	}
	return s
}

func autoTripNumber(date, timeStr, from, to string) string {
	// ✅ pastikan kunci trip stabil
	date = normalizeTripDate(date)
	timeStr = normalizeTripTime(timeStr)

	norm := func(s string) string {
		return normalizeFromTo(s)
	}

	dd := strings.ReplaceAll(strings.TrimSpace(date), "-", "")
	if dd == "" {
		dd = time.Now().Format("20060102")
	}
	tt := strings.TrimSpace(timeStr)
	if tt == "" {
		tt = "00:00"
	}
	return fmt.Sprintf("TRIP-%s-%s-%s-%s", dd, tt, norm(from), norm(to))
}

/*
	========================================================
	✅ TAMBAHAN (TIDAK MENGHAPUS KODE LAMA):
	- Build "hint/link" agar frontend PassengerInfo/TripInformation bisa membuka
	  E-ticket+invoice dan E-surat-jalan dari data yang disimpan.
	- Karena E-ticket+invoice dirender frontend, backend menyimpan referensinya.
	========================================================
*/

// Link E-Surat Jalan memang sudah ada endpoint API
func buildSuratJalanAPI(bookingID int64) string {
	return fmt.Sprintf("/api/reguler/bookings/%d/surat-jalan", bookingID)
}

// "Hint" untuk frontend agar bisa membuka invoice/e-ticket dari bookingId.
// Kalau di frontend route kamu berbeda, tinggal pakai bookingId dari notes.
func buildBookingHint(bookingID int64) string {
	// opsi aman: simpan bookingId saja; frontend yang bentuk URL
	return fmt.Sprintf("BOOKING_ID:%d", bookingID)
}

func buildTicketInvoiceHint(bookingID int64) string {
	// Disarankan dipakai PassengerInfo.jsx untuk tombol:
	// navigate(`/reguler?bookingId=${bookingID}`) atau route yang kamu pakai.
	return fmt.Sprintf("ETICKET_INVOICE_FROM_BOOKING:%d", bookingID)
}

type syncNotes struct {
	BookingID          int64  `json:"bookingId"`
	SuratJalanAPI      string `json:"suratJalanApi"`
	ETicketInvoiceHint string `json:"eTicketInvoiceHint"`
	BookingHint        string `json:"bookingHint"`
}

func buildSyncNotesJSON(bookingID int64) string {
	n := syncNotes{
		BookingID:          bookingID,
		SuratJalanAPI:      buildSuratJalanAPI(bookingID),
		ETicketInvoiceHint: buildTicketInvoiceHint(bookingID),
		BookingHint:        buildBookingHint(bookingID),
	}
	b, _ := json.Marshal(n)
	return string(b)
}

func mergeNotes(existing string, bookingID int64) string {
	existing = strings.TrimSpace(existing)
	newJSON := buildSyncNotesJSON(bookingID)

	// kalau kosong, langsung isi json
	if existing == "" {
		return newJSON
	}

	// kalau sudah berisi json dan ada bookingId sama, biarkan
	var cur syncNotes
	if err := json.Unmarshal([]byte(existing), &cur); err == nil {
		if cur.BookingID == bookingID && cur.SuratJalanAPI != "" {
			return existing
		}
	}

	// kalau notes sudah ada teks lain, jangan hilangkan:
	// append dengan separator yang aman
	return existing + "\n" + newJSON
}

func syncAll(tx *sql.Tx, p BookingSyncPayload) error {
	// 1) INFORMASI PERJALANAN (trip_information) + e-surat jalan
	if hasTable(tx, "trip_information") {
		if err := upsertTripInformation(tx, p); err != nil {
			return err
		}
	}

	// 2) DATA PENUMPANG (passengers) + e-ticket/invoice hint
	if hasTable(tx, "passengers") {
		if err := upsertPassengers(tx, p); err != nil {
			return err
		}
	}

	// 3) PENGATURAN KEBERANGKATAN (departure_settings) + e-surat jalan
	// ✅ 1 trip (tanggal+jam+rute) = 1 baris, jadi tidak dobel per booking
	if hasTable(tx, "departure_settings") {
		if err := upsertDepartureSettingsTrip(tx, p); err != nil {
			return err
		}
	}

	log.Println("[SYNC] OK booking", p.BookingID, "=> trip_information + passengers + departure_settings")
	return nil
}

func upsertTripInformation(tx *sql.Tx, p BookingSyncPayload) error {
	tripNo := autoTripNumber(p.Date, p.Time, p.From, p.To)
	esuratURL := buildSuratJalanAPI(p.BookingID)

	// ✅ agregasi kursi & jumlah penumpang untuk 1 trip (tanggal+jam+rute)
	allSeats, paxCount := aggregateTripSeatsAndCount(tx, p)
	if paxCount <= 0 {
		paxCount = len(p.SelectedSeats)
		allSeats = p.SelectedSeats
	}
	seatsCSV := formatSeatsCSV(allSeats)

	// ✅ anti dobel: lock berdasarkan tripNo
	lockKey := "trip_information:" + tripNo
	if err := acquireNamedLock(tx, lockKey, 5); err == nil {
		defer releaseNamedLock(tx, lockKey)
	} else {
		// bukan error fatal; tetap jalan dengan best-effort
		log.Println("[SYNC] warning: cannot acquire named lock:", err)
	}

	var existingID int64
	// ambil record terbaru jika (sudah terlanjur) ada duplikat
	_ = tx.QueryRow(`SELECT id FROM trip_information WHERE trip_number=? ORDER BY id DESC LIMIT 1`, tripNo).Scan(&existingID)

	// kolom bisa beda2, update/insert dinamis
	if existingID > 0 {
		sets := []string{}
		args := []any{}

		if hasColumn(tx, "trip_information", "departure_date") {
			sets = append(sets, "departure_date=?")
			args = append(args, p.Date)
		}
		if hasColumn(tx, "trip_information", "departure_time") {
			sets = append(sets, "departure_time=?")
			args = append(args, p.Time)
		}
		if hasColumn(tx, "trip_information", "e_surat_jalan") {
			sets = append(sets, "e_surat_jalan=?")
			args = append(args, esuratURL)
		}

		// ✅ TAMBAHAN (tanpa menghapus yang lama)
		if hasColumn(tx, "trip_information", "route_from") {
			sets = append(sets, "route_from=?")
			args = append(args, p.From)
		}
		if hasColumn(tx, "trip_information", "route_to") {
			sets = append(sets, "route_to=?")
			args = append(args, p.To)
		}
		if hasColumn(tx, "trip_information", "category") {
			sets = append(sets, "category=?")
			args = append(args, p.Category)
		}
		if hasColumn(tx, "trip_information", "passenger_count") {
			sets = append(sets, "passenger_count=?")
			args = append(args, int64(paxCount))
		}
		if hasColumn(tx, "trip_information", "seat_numbers") {
			sets = append(sets, "seat_numbers=?")
			args = append(args, seatsCSV)
		}
		if hasColumn(tx, "trip_information", "updated_at") {
			sets = append(sets, "updated_at=?")
			args = append(args, time.Now())
		}

		if len(sets) == 0 {
			return nil
		}

		args = append(args, existingID)
		_, err := tx.Exec(`UPDATE trip_information SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...)
		return err
	}

	cols := []string{"trip_number"}
	vals := []any{tripNo}

	// isi minimal (biarkan kosong untuk diatur admin)
	if hasColumn(tx, "trip_information", "departure_date") {
		cols = append(cols, "departure_date")
		vals = append(vals, p.Date)
	}
	if hasColumn(tx, "trip_information", "departure_time") {
		cols = append(cols, "departure_time")
		vals = append(vals, p.Time)
	}
	if hasColumn(tx, "trip_information", "driver_name") {
		cols = append(cols, "driver_name")
		vals = append(vals, "")
	}
	if hasColumn(tx, "trip_information", "vehicle_code") {
		cols = append(cols, "vehicle_code")
		vals = append(vals, "")
	}
	if hasColumn(tx, "trip_information", "license_plate") {
		cols = append(cols, "license_plate")
		vals = append(vals, "")
	}
	if hasColumn(tx, "trip_information", "e_surat_jalan") {
		cols = append(cols, "e_surat_jalan")
		vals = append(vals, esuratURL)
	}

	// ✅ TAMBAHAN (tanpa menghapus yang lama)
	if hasColumn(tx, "trip_information", "route_from") {
		cols = append(cols, "route_from")
		vals = append(vals, p.From)
	}
	if hasColumn(tx, "trip_information", "route_to") {
		cols = append(cols, "route_to")
		vals = append(vals, p.To)
	}
	if hasColumn(tx, "trip_information", "category") {
		cols = append(cols, "category")
		vals = append(vals, p.Category)
	}
	if hasColumn(tx, "trip_information", "passenger_count") {
		cols = append(cols, "passenger_count")
		vals = append(vals, int64(paxCount))
	}
	if hasColumn(tx, "trip_information", "seat_numbers") {
		cols = append(cols, "seat_numbers")
		vals = append(vals, seatsCSV)
	}
	if hasColumn(tx, "trip_information", "created_at") {
		cols = append(cols, "created_at")
		vals = append(vals, time.Now())
	}
	if hasColumn(tx, "trip_information", "updated_at") {
		cols = append(cols, "updated_at")
		vals = append(vals, time.Now())
	}

	ph := make([]string, 0, len(cols))
	for range cols {
		ph = append(ph, "?")
	}

	_, err := tx.Exec(`INSERT INTO trip_information (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(ph, ",")+`)`, vals...)
	return err
}

func upsertPassengers(tx *sql.Tx, p BookingSyncPayload) error {
	// map seat -> (name, phone) kalau tabel booking_passengers ada
	seatName := map[string]string{}
	seatPhone := map[string]string{}
	if hasTable(tx, "booking_passengers") && hasColumn(tx, "booking_passengers", "booking_id") && hasColumn(tx, "booking_passengers", "seat_code") {
		// name
		if hasColumn(tx, "booking_passengers", "passenger_name") {
			rows, err := tx.Query(`SELECT seat_code, COALESCE(passenger_name,'') FROM booking_passengers WHERE booking_id=?`, p.BookingID)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var seat, name string
					_ = rows.Scan(&seat, &name)
					seat = strings.ToUpper(strings.TrimSpace(seat))
					name = strings.TrimSpace(name)
					if seat != "" && name != "" {
						seatName[seat] = name
					}
				}
			}
		}
		// phone (kalau ada)
		if hasColumn(tx, "booking_passengers", "passenger_phone") {
			rows, err := tx.Query(`SELECT seat_code, COALESCE(passenger_phone,'') FROM booking_passengers WHERE booking_id=?`, p.BookingID)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var seat, ph string
					_ = rows.Scan(&seat, &ph)
					seat = strings.ToUpper(strings.TrimSpace(seat))
					ph = strings.TrimSpace(ph)
					if seat != "" && ph != "" {
						seatPhone[seat] = ph
					}
				}
			}
		}
	}

	seats := p.SelectedSeats
	if len(seats) == 0 {
		// minimal 1 record (tanpa seat) agar “Data Penumpang” tetap terisi
		seats = []string{""}
	}

	for _, seat := range seats {
		seat = strings.ToUpper(strings.TrimSpace(seat))

		name := strings.TrimSpace(seatName[seat])
		if name == "" {
			name = p.PassengerName
		}
		phone := strings.TrimSpace(seatPhone[seat])
		if phone == "" {
			phone = p.PassengerPhone
		}

		// idempotent: kalau ada booking_id, pakai itu
		var existingID int64
		if hasColumn(tx, "passengers", "booking_id") {
			_ = tx.QueryRow(`SELECT id FROM passengers WHERE booking_id=? AND COALESCE(selected_seats,'')=? LIMIT 1`, p.BookingID, seat).Scan(&existingID)
		} else {
			// fallback: pakai kombinasi date+time+phone+seat
			dateCol := "date"
			if !hasColumn(tx, "passengers", "date") && hasColumn(tx, "passengers", "departure_date") {
				dateCol = "departure_date"
			}
			timeCol := "departure_time"
			if !hasColumn(tx, "passengers", "departure_time") && hasColumn(tx, "passengers", "trip_time") {
				timeCol = "trip_time"
			}
			_ = tx.QueryRow(
				fmt.Sprintf(`SELECT id FROM passengers WHERE COALESCE(passenger_phone,'')=? AND COALESCE(%s,'')=? AND COALESCE(%s,'')=? AND COALESCE(selected_seats,'')=? LIMIT 1`, dateCol, timeCol),
				phone, p.Date, p.Time, seat,
			).Scan(&existingID)
		}

		totalStr := fmt.Sprintf("%d", p.TotalAmount)
		notes := fmt.Sprintf("Auto sync dari booking %d. Lihat E-Ticket/Invoice di halaman booking.", p.BookingID)

		// ✅ TAMBAHAN: simpan JSON hint agar PassengerInfo.jsx bisa tampilkan tombol buka E-ticket/Invoice dan surat jalan
		syncJSON := buildSyncNotesJSON(p.BookingID)

		if existingID > 0 {
			sets := []string{}
			args := []any{}

			if hasColumn(tx, "passengers", "passenger_name") {
				sets = append(sets, "passenger_name=?")
				args = append(args, name)
			}
			if hasColumn(tx, "passengers", "passenger_phone") {
				sets = append(sets, "passenger_phone=?")
				args = append(args, phone)
			}

			if hasColumn(tx, "passengers", "pickup_address") {
				sets = append(sets, "pickup_address=?")
				args = append(args, p.PickupLocation)
			} else if hasColumn(tx, "passengers", "pickup_location") {
				sets = append(sets, "pickup_location=?")
				args = append(args, p.PickupLocation)
			}

			if hasColumn(tx, "passengers", "dropoff_address") {
				sets = append(sets, "dropoff_address=?")
				args = append(args, p.DropoffLocation)
			} else if hasColumn(tx, "passengers", "dropoff_location") {
				sets = append(sets, "dropoff_location=?")
				args = append(args, p.DropoffLocation)
			}

			if hasColumn(tx, "passengers", "date") {
				sets = append(sets, "date=?")
				args = append(args, p.Date)
			} else if hasColumn(tx, "passengers", "departure_date") {
				sets = append(sets, "departure_date=?")
				args = append(args, p.Date)
			}

			if hasColumn(tx, "passengers", "departure_time") {
				sets = append(sets, "departure_time=?")
				args = append(args, p.Time)
			}

			if hasColumn(tx, "passengers", "selected_seats") {
				sets = append(sets, "selected_seats=?")
				args = append(args, seat)
			}

			if hasColumn(tx, "passengers", "service_type") {
				sets = append(sets, "service_type=?")
				args = append(args, p.Category)
			}

			if hasColumn(tx, "passengers", "total_amount") {
				sets = append(sets, "total_amount=?")
				args = append(args, totalStr)
			}

			if hasColumn(tx, "passengers", "notes") {
				// ✅ TAMBAHAN: jangan hilangkan notes lama
				var existingNotes sql.NullString
				_ = tx.QueryRow(`SELECT COALESCE(notes,'') FROM passengers WHERE id=? LIMIT 1`, existingID).Scan(&existingNotes)
				merged := notes
				if existingNotes.Valid {
					merged = mergeNotes(existingNotes.String, p.BookingID)
				} else {
					merged = notes + "\n" + syncJSON
				}

				sets = append(sets, "notes=?")
				args = append(args, merged)
			}

			// ✅ TAMBAHAN: kalau ada kolom khusus untuk simpan hint/url
			if hasColumn(tx, "passengers", "booking_hint") {
				sets = append(sets, "booking_hint=?")
				args = append(args, buildBookingHint(p.BookingID))
			}
			if hasColumn(tx, "passengers", "eticket_invoice_hint") {
				sets = append(sets, "eticket_invoice_hint=?")
				args = append(args, buildTicketInvoiceHint(p.BookingID))
			}
			if hasColumn(tx, "passengers", "surat_jalan_api") {
				sets = append(sets, "surat_jalan_api=?")
				args = append(args, buildSuratJalanAPI(p.BookingID))
			}

			if hasColumn(tx, "passengers", "updated_at") {
				sets = append(sets, "updated_at=?")
				args = append(args, time.Now())
			}

			if len(sets) == 0 {
				continue
			}

			args = append(args, existingID)
			_, err := tx.Exec(`UPDATE passengers SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...)
			if err != nil {
				return err
			}
			continue
		}

		cols := []string{}
		vals := []any{}

		if hasColumn(tx, "passengers", "passenger_name") {
			cols = append(cols, "passenger_name")
			vals = append(vals, name)
		}
		if hasColumn(tx, "passengers", "passenger_phone") {
			cols = append(cols, "passenger_phone")
			vals = append(vals, phone)
		}

		if hasColumn(tx, "passengers", "date") {
			cols = append(cols, "date")
			vals = append(vals, p.Date)
		} else if hasColumn(tx, "passengers", "departure_date") {
			cols = append(cols, "departure_date")
			vals = append(vals, p.Date)
		}

		if hasColumn(tx, "passengers", "departure_time") {
			cols = append(cols, "departure_time")
			vals = append(vals, p.Time)
		}

		if hasColumn(tx, "passengers", "pickup_address") {
			cols = append(cols, "pickup_address")
			vals = append(vals, p.PickupLocation)
		} else if hasColumn(tx, "passengers", "pickup_location") {
			cols = append(cols, "pickup_location")
			vals = append(vals, p.PickupLocation)
		}

		if hasColumn(tx, "passengers", "dropoff_address") {
			cols = append(cols, "dropoff_address")
			vals = append(vals, p.DropoffLocation)
		} else if hasColumn(tx, "passengers", "dropoff_location") {
			cols = append(cols, "dropoff_location")
			vals = append(vals, p.DropoffLocation)
		}

		if hasColumn(tx, "passengers", "total_amount") {
			cols = append(cols, "total_amount")
			vals = append(vals, totalStr)
		}

		if hasColumn(tx, "passengers", "selected_seats") {
			cols = append(cols, "selected_seats")
			vals = append(vals, seat)
		}

		if hasColumn(tx, "passengers", "service_type") {
			cols = append(cols, "service_type")
			vals = append(vals, p.Category)
		}

		// e-ticket: kalau kolomnya ada, isi minimal marker (biar UI bisa tampilkan link/btn)
		if hasColumn(tx, "passengers", "eticket_photo") {
			cols = append(cols, "eticket_photo")
			vals = append(vals, "") // (opsional) kalau nanti ada endpoint e-ticket, bisa isi URL/base64 di sini
		}

		if hasColumn(tx, "passengers", "notes") {
			// ✅ TAMBAHAN: gabungkan notes dengan sync json hint
			cols = append(cols, "notes")
			vals = append(vals, notes+"\n"+syncJSON)
		}

		if hasColumn(tx, "passengers", "booking_id") {
			cols = append(cols, "booking_id")
			vals = append(vals, p.BookingID)
		}

		// ✅ TAMBAHAN: kolom hint/url kalau ada
		if hasColumn(tx, "passengers", "booking_hint") {
			cols = append(cols, "booking_hint")
			vals = append(vals, buildBookingHint(p.BookingID))
		}
		if hasColumn(tx, "passengers", "eticket_invoice_hint") {
			cols = append(cols, "eticket_invoice_hint")
			vals = append(vals, buildTicketInvoiceHint(p.BookingID))
		}
		if hasColumn(tx, "passengers", "surat_jalan_api") {
			cols = append(cols, "surat_jalan_api")
			vals = append(vals, buildSuratJalanAPI(p.BookingID))
		}

		if hasColumn(tx, "passengers", "created_at") {
			cols = append(cols, "created_at")
			vals = append(vals, time.Now())
		}

		if len(cols) == 0 {
			return nil
		}

		ph := make([]string, 0, len(cols))
		for range cols {
			ph = append(ph, "?")
		}

		_, err := tx.Exec(`INSERT INTO passengers (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(ph, ",")+`)`, vals...)
		if err != nil {
			return err
		}
	}

	return nil
}


// ========================================================
// ✅ TAMBAHAN (TIDAK MENGHAPUS KODE LAMA)
// Sinkronisasi ke tabel departure_settings (Pengaturan Keberangkatan)
// - 1 trip (tanggal+jam+rute) = 1 baris (tidak dobel per booking)
// - seat_numbers & passenger_count di-aggregate dari semua booking yang "paid/sukses"
// - surat_jalan_file otomatis mengarah ke e-surat-jalan dari booking/trip
// ========================================================

// formatSeatsCSV: rapikan seat menjadi "1A, 2B, 5A"
func formatSeatsCSV(seats []string) string {
	seats = normalizeSeatsUnique(seats)
	sort.Strings(seats)
	return strings.Join(seats, ", ")
}

// aggregateTripSeatsAndCount: ambil semua seat dari booking yang sesuai trip (date+time+from+to+category)
// lalu gabungkan menjadi unique seats, return (seats, count).
func aggregateTripSeatsAndCount(tx *sql.Tx, p BookingSyncPayload) ([]string, int) {
	if tx == nil {
		return p.SelectedSeats, len(p.SelectedSeats)
	}

	// deteksi tabel booking
	table := "bookings"
	if !hasTable(tx, table) {
		if hasTable(tx, "reguler_bookings") {
			table = "reguler_bookings"
		}
	}

	// cari kolom-kolom yang tersedia
	dateCol := "trip_date"
	if !hasColumn(tx, table, dateCol) {
		if hasColumn(tx, table, "departure_date") {
			dateCol = "departure_date"
		}
	}
	timeCol := "trip_time"
	if !hasColumn(tx, table, timeCol) {
		if hasColumn(tx, table, "departure_time") {
			timeCol = "departure_time"
		}
	}

	fromCol := "from_city"
	if !hasColumn(tx, table, fromCol) {
		if hasColumn(tx, table, "route_from") {
			fromCol = "route_from"
		}
	}
	toCol := "to_city"
	if !hasColumn(tx, table, toCol) {
		if hasColumn(tx, table, "route_to") {
			toCol = "route_to"
		}
	}

	categoryCol := "category"
	if !hasColumn(tx, table, categoryCol) {
		categoryCol = ""
	}

	statusCol := "payment_status"
	if !hasColumn(tx, table, statusCol) {
		statusCol = ""
	}
	methodCol := "payment_method"
	if !hasColumn(tx, table, methodCol) {
		methodCol = ""
	}

	seatCol := ""
	for _, c := range []string{"selected_seats_json", "seats_json", "selected_seats", "seat_numbers"} {
		if hasColumn(tx, table, c) {
			seatCol = c
			break
		}
	}
	if seatCol == "" || dateCol == "" || timeCol == "" || fromCol == "" || toCol == "" {
		return p.SelectedSeats, len(p.SelectedSeats)
	}

	// query semua booking yang match trip ini (filternya fleksibel untuk date/time yang kadang berupa datetime/RFC3339)
	q := fmt.Sprintf(`
		SELECT
			COALESCE(%s,'') AS seats_raw,
			COALESCE(%s,'') AS pay_status,
			COALESCE(%s,'') AS pay_method
		FROM %s
		WHERE
			(%s = ? OR %s LIKE CONCAT(?, '%%'))
			AND (%s = ? OR %s LIKE CONCAT(?, '%%'))
			AND COALESCE(%s,'') = ?
			AND COALESCE(%s,'') = ?`,
		seatCol,
		// status/method bisa kosong; kalau kolomnya tidak ada kita pakai '' (sudah di atas)
		func() string {
			if statusCol == "" {
				return "''"
			}
			return statusCol
		}(),
		func() string {
			if methodCol == "" {
				return "''"
			}
			return methodCol
		}(),
		table,
		dateCol, dateCol,
		timeCol, timeCol,
		fromCol,
		toCol,
	)

	args := []any{p.Date, p.Date, p.Time, p.Time, p.From, p.To}

	// optional kategori
	if categoryCol != "" {
		q += fmt.Sprintf(` AND COALESCE(%s,'') = ?`, categoryCol)
		args = append(args, p.Category)
	}

	rows, err := tx.Query(q, args...)
	if err != nil {
		// best effort
		return p.SelectedSeats, len(p.SelectedSeats)
	}
	defer rows.Close()

	seen := map[string]bool{}
	out := make([]string, 0, 8)

	for rows.Next() {
		var seatsRaw, ps, pm sql.NullString
		if err := rows.Scan(&seatsRaw, &ps, &pm); err != nil {
			continue
		}

		if !isPaidSuccess(ps.String, pm.String) {
			continue
		}

		seats := parseSeatsFlexible(seatsRaw.String)
		for _, s := range normalizeSeatsUnique(seats) {
			if s == "" {
				continue
			}
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}

	out = normalizeSeatsUnique(out)
	sort.Strings(out)
	return out, len(out)
}

// getTripESuratJalan: ambil e_surat_jalan dari trip_information berdasarkan trip_number (jika ada)
func getTripESuratJalan(tx *sql.Tx, tripNo string) string {
	if tx == nil || tripNo == "" {
		return ""
	}
	if !hasTable(tx, "trip_information") || !hasColumn(tx, "trip_information", "e_surat_jalan") {
		return ""
	}
	var s sql.NullString
	_ = tx.QueryRow(`SELECT COALESCE(e_surat_jalan,'') FROM trip_information WHERE trip_number=? ORDER BY id DESC LIMIT 1`, tripNo).Scan(&s)
	return strings.TrimSpace(s.String)
}

// upsertDepartureSettingsTrip: 1 trip = 1 row (tidak dobel per booking)
func upsertDepartureSettingsTrip(tx *sql.Tx, p BookingSyncPayload) error {
	if tx == nil {
		return nil
	}

	table := "departure_settings"
	if !hasTable(tx, table) {
		return nil
	}

	tripNo := autoTripNumber(p.Date, p.Time, p.From, p.To)

	// ✅ named lock untuk mencegah race (2 booking dibayar bersamaan)
	lockKey := "departure_settings:" + tripNo
	if err := acquireNamedLock(tx, lockKey, 5); err == nil {
		defer releaseNamedLock(tx, lockKey)
	} else {
		log.Println("[SYNC] warning: cannot acquire named lock for departure_settings:", err)
	}

	// agregasi seat & pax untuk trip ini
	allSeats, paxCount := aggregateTripSeatsAndCount(tx, p)
	if paxCount <= 0 {
		paxCount = len(p.SelectedSeats)
		allSeats = p.SelectedSeats
	}
	seatsCSV := formatSeatsCSV(allSeats)

	// surat jalan: prioritaskan yang sudah ada di trip_information, kalau kosong fallback ke endpoint booking
	suratURL := strings.TrimSpace(getTripESuratJalan(tx, tripNo))
	if suratURL == "" {
		suratURL = buildSuratJalanAPI(p.BookingID)
	}
	suratName := "E-Surat-Jalan-" + tripNo + ".png"

	// cari existing row (prioritas trip_number, kalau tidak ada gunakan kombinasi field yang tersedia)
	var existingID int64
	if hasColumn(tx, table, "trip_number") {
		_ = tx.QueryRow(`SELECT id FROM departure_settings WHERE trip_number=? ORDER BY id DESC LIMIT 1`, tripNo).Scan(&existingID)
	} else if hasColumn(tx, table, "departure_time") && hasColumn(tx, table, "route_from") && hasColumn(tx, table, "route_to") {
		_ = tx.QueryRow(`SELECT id FROM departure_settings
			WHERE departure_date=? AND departure_time=? AND route_from=? AND route_to=? AND service_type=?
			ORDER BY id DESC LIMIT 1`,
			p.Date, p.Time, p.From, p.To, p.Category,
		).Scan(&existingID)
	} else {
		// fallback minimal: date + category
		_ = tx.QueryRow(`SELECT id FROM departure_settings
			WHERE departure_date=? AND service_type=?
			ORDER BY id DESC LIMIT 1`,
			p.Date, p.Category,
		).Scan(&existingID)
	}

	// fields dasar dari booking (yang diinginkan user masuk ke Pengaturan Keberangkatan)
	bookingName := strings.TrimSpace(p.PassengerName)
	if bookingName == "" {
		bookingName = "Booking"
	}
	phone := strings.TrimSpace(p.PassengerPhone)
	pickup := strings.TrimSpace(p.PickupLocation)

	if existingID > 0 {
		sets := []string{}
		args := []any{}

		if hasColumn(tx, table, "booking_name") {
			sets = append(sets, "booking_name=?")
			args = append(args, bookingName)
		}
		if hasColumn(tx, table, "phone") {
			sets = append(sets, "phone=?")
			args = append(args, phone)
		}
		if hasColumn(tx, table, "pickup_address") {
			sets = append(sets, "pickup_address=?")
			args = append(args, pickup)
		}
		if hasColumn(tx, table, "departure_date") {
			sets = append(sets, "departure_date=?")
			args = append(args, nullIfEmpty(p.Date))
		}
		if hasColumn(tx, table, "departure_time") {
			sets = append(sets, "departure_time=?")
			args = append(args, nullIfEmpty(p.Time))
		}
		if hasColumn(tx, table, "route_from") {
			sets = append(sets, "route_from=?")
			args = append(args, p.From)
		}
		if hasColumn(tx, table, "route_to") {
			sets = append(sets, "route_to=?")
			args = append(args, p.To)
		}
		if hasColumn(tx, table, "service_type") {
			sets = append(sets, "service_type=?")
			args = append(args, p.Category)
		}

		if hasColumn(tx, table, "seat_numbers") {
			sets = append(sets, "seat_numbers=?")
			args = append(args, seatsCSV)
		}
		if hasColumn(tx, table, "passenger_count") {
			sets = append(sets, "passenger_count=?")
			args = append(args, paxCount)
		}

		if hasColumn(tx, table, "surat_jalan_file") {
			sets = append(sets, "surat_jalan_file=?")
			args = append(args, suratURL)
		}
		if hasColumn(tx, table, "surat_jalan_file_name") {
			sets = append(sets, "surat_jalan_file_name=?")
			args = append(args, suratName)
		}

		// optional: simpan trip_number & booking_id untuk referensi
		if hasColumn(tx, table, "trip_number") {
			sets = append(sets, "trip_number=?")
			args = append(args, tripNo)
		}
		if hasColumn(tx, table, "booking_id") {
			sets = append(sets, "booking_id=?")
			args = append(args, p.BookingID)
		}
		if hasColumn(tx, table, "updated_at") {
			sets = append(sets, "updated_at=?")
			args = append(args, time.Now())
		}

		if len(sets) == 0 {
			return nil
		}

		args = append(args, existingID)
		_, err := tx.Exec(`UPDATE `+table+` SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...)
		return err
	}

	// INSERT baru
	cols := []string{}
	vals := []any{}

	if hasColumn(tx, table, "booking_name") {
		cols = append(cols, "booking_name")
		vals = append(vals, bookingName)
	}
	if hasColumn(tx, table, "phone") {
		cols = append(cols, "phone")
		vals = append(vals, phone)
	}
	if hasColumn(tx, table, "pickup_address") {
		cols = append(cols, "pickup_address")
		vals = append(vals, pickup)
	}
	if hasColumn(tx, table, "departure_date") {
		cols = append(cols, "departure_date")
		vals = append(vals, nullIfEmpty(p.Date))
	}
	if hasColumn(tx, table, "departure_time") {
		cols = append(cols, "departure_time")
		vals = append(vals, nullIfEmpty(p.Time))
	}
	if hasColumn(tx, table, "route_from") {
		cols = append(cols, "route_from")
		vals = append(vals, p.From)
	}
	if hasColumn(tx, table, "route_to") {
		cols = append(cols, "route_to")
		vals = append(vals, p.To)
	}
	if hasColumn(tx, table, "seat_numbers") {
		cols = append(cols, "seat_numbers")
		vals = append(vals, seatsCSV)
	}
	if hasColumn(tx, table, "passenger_count") {
		cols = append(cols, "passenger_count")
		vals = append(vals, paxCount)
	}
	if hasColumn(tx, table, "service_type") {
		cols = append(cols, "service_type")
		vals = append(vals, p.Category)
	}
	if hasColumn(tx, table, "surat_jalan_file") {
		cols = append(cols, "surat_jalan_file")
		vals = append(vals, suratURL)
	}
	if hasColumn(tx, table, "surat_jalan_file_name") {
		cols = append(cols, "surat_jalan_file_name")
		vals = append(vals, suratName)
	}
	if hasColumn(tx, table, "departure_status") {
		cols = append(cols, "departure_status")
		vals = append(vals, "Berangkat")
	}
	if hasColumn(tx, table, "trip_number") {
		cols = append(cols, "trip_number")
		vals = append(vals, tripNo)
	}
	if hasColumn(tx, table, "booking_id") {
		cols = append(cols, "booking_id")
		vals = append(vals, p.BookingID)
	}
	if hasColumn(tx, table, "created_at") {
		cols = append(cols, "created_at")
		vals = append(vals, time.Now())
	}

	if len(cols) == 0 {
		return nil
	}

	ph := make([]string, len(cols))
	for i := range ph {
		ph[i] = "?"
	}
	_, err := tx.Exec(`INSERT INTO `+table+` (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(ph, ",")+`)`, vals...)
	return err
}
