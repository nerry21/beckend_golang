// backend/handlers/booking_sync.go
package handlers

import (
	"backend/config"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
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
	CustomerName   string
	CustomerPhone  string
	SelectedSeats  []string
	PassengerCount int

	PricePerSeat  int64
	ETicketPhoto  string
	TotalAmount   int64
	PaymentMethod string
	PaymentStatus string
	CreatedAt     time.Time
}

// ✅ kompatibel dengan call site lama: SyncConfirmedRegulerBooking(tx, bookingID)
func SyncConfirmedRegulerBooking(tx *sql.Tx, bookingID int64) error {
	if bookingID <= 0 {
		return fmt.Errorf("SyncConfirmedRegulerBooking: bookingID invalid")
	}

	// Saat status belum "Lunas" tapi total sudah ada (Menunggu Validasi), kita izinkan sync.
	// Untuk panggilan setelah approve (status Lunas), tetap wajib sync.
	// Di luar itu (tidak ada total dan status belum Lunas) boleh dilewati.

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

	// kunci: jika total ada (TotalAmount > 0), kita SELALU sync agar nominal tidak hilang.
	// jika total 0 dan status belum paid, skip seperti biasa.
	if p.TotalAmount == 0 && !isPaidSuccess(p.PaymentStatus, p.PaymentMethod) {
		log.Println("[SYNC] skip booking", p.BookingID, "status:", p.PaymentStatus, "method:", p.PaymentMethod, "total kosong")
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

// SyncConfirmedRegulerBookingTx dipertahankan untuk kompatibilitas call site lama.
func SyncConfirmedRegulerBookingTx(tx *sql.Tx, bookingID int64) error {
	return SyncConfirmedRegulerBooking(tx, bookingID)
}

func isPaidSuccess(paymentStatus, paymentMethod string) bool {
	s := strings.ToLower(strings.TrimSpace(paymentStatus))
	m := strings.ToLower(strings.TrimSpace(paymentMethod))

	switch m {
	case "cash":
		return s == "" || s == "sukses" || s == "lunas" || s == "paid"
	default:
		switch s {
		case "sukses", "lunas", "paid":
			return true
		default:
			return false
		}
	}
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
	if hasColumn(tx, table, "customer_name") {
		cols = append(cols, "customer_name")
	}
	if hasColumn(tx, table, "customer_phone") {
		cols = append(cols, "customer_phone")
	}

	// dokumen e-ticket dari booking jika ada
	if hasColumn(tx, table, "eticket_photo") {
		cols = append(cols, "eticket_photo")
	} else if hasColumn(tx, table, "e_ticket_photo") {
		cols = append(cols, "e_ticket_photo")
	} else if hasColumn(tx, table, "eticket_url") {
		cols = append(cols, "eticket_url")
	} else if hasColumn(tx, table, "eticket") {
		cols = append(cols, "eticket")
	}

	// total
	switch {
	case hasColumn(tx, table, "total_amount"):
		cols = append(cols, "total_amount")
	case hasColumn(tx, table, "total"):
		cols = append(cols, "total")
	case hasColumn(tx, table, "grand_total"):
		cols = append(cols, "grand_total")
	case hasColumn(tx, table, "total_invoice"):
		cols = append(cols, "total_invoice")
	case hasColumn(tx, table, "invoice_total"):
		cols = append(cols, "invoice_total")
	case hasColumn(tx, table, "amount"):
		cols = append(cols, "amount")
	case hasColumn(tx, table, "nominal"):
		cols = append(cols, "nominal")
	}

	// passenger count (untuk hitung total jika total kosong)
	if hasColumn(tx, table, "passenger_count") {
		cols = append(cols, "passenger_count")
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
	if hasColumn(tx, table, "price_per_seat") {
		cols = append(cols, "price_per_seat")
	}

	q := fmt.Sprintf("SELECT %s FROM %s WHERE id=? LIMIT 1", strings.Join(cols, ","), table)

	// scan targets
	var (
		id int64

		category        sql.NullString
		fromCity        sql.NullString
		toCity          sql.NullString
		tripDate        sql.NullString
		tripTime        sql.NullString
		pickupLocation  sql.NullString
		dropoffLocation sql.NullString
		bookingFor      sql.NullString
		passengerName   sql.NullString
		passengerPhone  sql.NullString
		customerName    sql.NullString
		customerPhone   sql.NullString

		totalAmount  sql.NullInt64
		totalAmount2 sql.NullInt64
		payMethod    sql.NullString
		payStatus    sql.NullString
		createdAt    sql.NullTime

		selectedSeatsRaw sql.NullString
		seatsJSONRaw     sql.NullString
		eticketPhoto     sql.NullString

		pricePerSeat sql.NullInt64
		passengerCnt sql.NullInt64
	)

	dests := []any{&id}
	for _, c := range cols[1:] {
		switch c {
		case "category":
			dests = append(dests, &category)
		case "from_city", "route_from":
			dests = append(dests, &fromCity)
		case "to_city", "route_to":
			dests = append(dests, &toCity)
		case "trip_date":
			dests = append(dests, &tripDate)
		case "trip_time":
			dests = append(dests, &tripTime)
		case "pickup_location":
			dests = append(dests, &pickupLocation)
		case "dropoff_location":
			dests = append(dests, &dropoffLocation)
		case "booking_for":
			dests = append(dests, &bookingFor)
		case "passenger_name":
			dests = append(dests, &passengerName)
		case "passenger_phone":
			dests = append(dests, &passengerPhone)
		case "customer_name":
			dests = append(dests, &customerName)
		case "customer_phone":
			dests = append(dests, &customerPhone)
		case "total_amount", "total":
			dests = append(dests, &totalAmount)
		case "grand_total", "total_invoice", "invoice_total", "amount", "nominal":
			dests = append(dests, &totalAmount2)
		case "payment_method":
			dests = append(dests, &payMethod)
		case "payment_status":
			dests = append(dests, &payStatus)
		case "created_at":
			dests = append(dests, &createdAt)
		case "passenger_count":
			dests = append(dests, &passengerCnt)
		case "selected_seats":
			dests = append(dests, &selectedSeatsRaw)
		case "seats_json":
			dests = append(dests, &seatsJSONRaw)
		case "eticket_photo", "e_ticket_photo", "eticket_url", "eticket":
			dests = append(dests, &eticketPhoto)
		case "price_per_seat":
			dests = append(dests, &pricePerSeat)
		default:
			// fallback
			var tmp any
			dests = append(dests, &tmp)
		}
	}

	if err := tx.QueryRow(q, bookingID).Scan(dests...); err != nil {
		return BookingSyncPayload{}, err
	}

	p := BookingSyncPayload{
		BookingID: id,
	}

	if category.Valid {
		p.Category = strings.TrimSpace(category.String)
	}
	if fromCity.Valid {
		p.From = strings.TrimSpace(fromCity.String)
	}
	if toCity.Valid {
		p.To = strings.TrimSpace(toCity.String)
	}
	if tripDate.Valid {
		p.Date = strings.TrimSpace(tripDate.String)
	}
	if tripTime.Valid {
		p.Time = strings.TrimSpace(tripTime.String)
	}
	if pickupLocation.Valid {
		p.PickupLocation = strings.TrimSpace(pickupLocation.String)
	}
	if dropoffLocation.Valid {
		p.DropoffLocation = strings.TrimSpace(dropoffLocation.String)
	}

	// nama pemesan fallback booking_for -> passenger_name
	if passengerName.Valid && strings.TrimSpace(passengerName.String) != "" {
		p.PassengerName = strings.TrimSpace(passengerName.String)
	} else if bookingFor.Valid {
		p.PassengerName = strings.TrimSpace(bookingFor.String)
	} else if customerName.Valid {
		p.PassengerName = strings.TrimSpace(customerName.String)
	}
	if customerName.Valid {
		p.CustomerName = strings.TrimSpace(customerName.String)
	}

	if passengerPhone.Valid && strings.TrimSpace(passengerPhone.String) != "" {
		p.PassengerPhone = strings.TrimSpace(passengerPhone.String)
	} else if customerPhone.Valid {
		p.PassengerPhone = strings.TrimSpace(customerPhone.String)
	}
	if customerPhone.Valid {
		p.CustomerPhone = strings.TrimSpace(customerPhone.String)
	}
	if passengerCnt.Valid && passengerCnt.Int64 > 0 {
		p.PassengerCount = int(passengerCnt.Int64)
	}

	if totalAmount.Valid {
		p.TotalAmount = totalAmount.Int64
	}
	if p.TotalAmount == 0 && totalAmount2.Valid {
		p.TotalAmount = totalAmount2.Int64
	}
	if payMethod.Valid {
		p.PaymentMethod = strings.TrimSpace(payMethod.String)
	}
	if payStatus.Valid {
		p.PaymentStatus = strings.TrimSpace(payStatus.String)
	}
	if createdAt.Valid {
		p.CreatedAt = createdAt.Time
	}
	if pricePerSeat.Valid {
		p.PricePerSeat = pricePerSeat.Int64
	}

	// seats parsing
	var seats []string
	if selectedSeatsRaw.Valid && strings.TrimSpace(selectedSeatsRaw.String) != "" {
		seats = parseSeatsFlexible(selectedSeatsRaw.String)
	} else if seatsJSONRaw.Valid && strings.TrimSpace(seatsJSONRaw.String) != "" {
		seats = parseSeatsFlexible(seatsJSONRaw.String)
	}
	p.SelectedSeats = normalizeSeatsUnique(seats)

	// fallback price per seat dari rute jika kolom DB kosong
	if p.PricePerSeat == 0 {
		if fare := regulerFareFromBooking(p.From, p.To); fare > 0 {
			p.PricePerSeat = fare
		}
	}

	if eticketPhoto.Valid {
		p.ETicketPhoto = strings.TrimSpace(eticketPhoto.String)
	}

	// fallback nama/no hp dari payment_validations jika masih kosong
	if (strings.TrimSpace(p.PassengerName) == "" || strings.TrimSpace(p.PassengerPhone) == "") && hasTable(tx, "payment_validations") {
		n, ph := loadPaymentValidationContact(tx, p.BookingID)
		if strings.TrimSpace(p.PassengerName) == "" && n != "" {
			p.PassengerName = n
		}
		if strings.TrimSpace(p.PassengerPhone) == "" && ph != "" {
			p.PassengerPhone = ph
		}
	}

	// fallback TotalAmount dari jumlah kursi x harga kursi jika total masih kosong
	seatCount := len(p.SelectedSeats)
	if seatCount == 0 && p.PassengerCount > 0 {
		seatCount = p.PassengerCount
	}
	if seatCount == 0 {
		seatCount = 1
	}
	if p.TotalAmount == 0 && p.PricePerSeat > 0 {
		p.TotalAmount = p.PricePerSeat * int64(seatCount)
	}
	if p.TotalAmount == 0 {
		if inv := lookupInvoiceTotal(tx, p.BookingID); inv > 0 {
			p.TotalAmount = inv
		}
	}

	return p, nil
}

func syncAll(tx *sql.Tx, p BookingSyncPayload) error {
	if hasTable(tx, "trip_information") {
		if err := upsertTripInformation(tx, p); err != nil {
			return err
		}
	}
	if hasTable(tx, "passengers") {
		if err := upsertPassengers(tx, p); err != nil {
			return err
		}
	}

	// ✅ tambahan agar booking yg sudah lunas otomatis masuk ke pengaturan keberangkatan
	if hasTable(tx, "departure_settings") {
		if err := upsertDepartureSettings(tx, p); err != nil {
			return err
		}
	}

	// ✅ tambahan: sync trips (laporan keuangan)
	if hasTable(tx, "trips") {
		if err := upsertTripsFinance(tx, p); err != nil {
			return err
		}
	}

	log.Println("[SYNC] OK booking", p.BookingID, "=> trip_information + passengers + departure_settings + trips")
	return nil
}

func normalizeSeatsUnique(seats []string) []string {
	m := map[string]bool{}
	out := []string{}
	for _, s := range seats {
		s = strings.ToUpper(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if !m[s] {
			m[s] = true
			out = append(out, s)
		}
	}
	return out
}

// parseSeatsFlexible handle input:
// - json array ["1A","1B"]
// - json string "\"1A,1B\""
// - string "1A, 1B"
// - string "1A 1B"
func parseSeatsFlexible(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	// coba parse json array
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return arr
	}

	// coba parse json string
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err == nil {
		raw = s
	}

	// split by comma/space
	raw = strings.ReplaceAll(raw, "\n", ",")
	raw = strings.ReplaceAll(raw, "\t", ",")
	raw = strings.ReplaceAll(raw, ";", ",")
	parts := strings.Split(raw, ",")
	out := []string{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// kalau masih ada spasi, pecah
		sub := strings.Fields(p)
		if len(sub) > 1 {
			for _, x := range sub {
				x = strings.TrimSpace(x)
				if x != "" {
					out = append(out, x)
				}
			}
		} else {
			out = append(out, p)
		}
	}
	return out
}

func normalizeDateOnly(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// kalau datetime "YYYY-MM-DD HH:MM:SS"
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func normalizeTripDate(s string) string {
	return normalizeDateOnly(s)
}

func normalizeTripTime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// normalisasi "08:00:00" -> "08:00"
	if len(s) >= 5 {
		return s[:5]
	}
	return s
}

func autoTripNumber(dateStr, timeStr, from, to string) string {
	dateOnly := normalizeDateOnly(dateStr)
	timeOnly := normalizeTripTime(timeStr)
	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	if dateOnly == "" {
		dateOnly = time.Now().Format("2006-01-02")
	}
	if timeOnly == "" {
		timeOnly = "00:00"
	}
	return fmt.Sprintf("%s-%s-%s-%s", dateOnly, timeOnly, from, to)
}

func buildSuratJalanAPI(bookingID int64) string {
	return fmt.Sprintf("http://localhost:8080/api/reguler/bookings/%d/surat-jalan", bookingID)
}

func buildTripInformationSuratJalanAPI(tripID int64) string {
	return fmt.Sprintf("http://localhost:8080/api/trip-information/%d/surat-jalan", tripID)
}

func buildBookingHint(bookingID int64) string {
	return fmt.Sprintf("BOOKING-%d", bookingID)
}

func buildTicketInvoiceHint(bookingID int64) string {
	return fmt.Sprintf("INVOICE-%d", bookingID)
}

func buildEticketInvoiceMarker(bookingID int64) string {
	if bookingID <= 0 {
		return ""
	}
	return fmt.Sprintf("ETICKET_INVOICE_FROM_BOOKING:%d", bookingID)
}

// loadDepartureDriverVehicle mengambil driver_name + vehicle_code + vehicle_type dari departure_settings (relasi booking).
func loadDepartureDriverVehicle(tx *sql.Tx, bookingID int64) (string, string, string) {
	if bookingID <= 0 || !hasTable(tx, "departure_settings") {
		return "", "", ""
	}

	var dsID int64
	if hasColumn(tx, "departure_settings", "booking_id") {
		_ = tx.QueryRow(`SELECT id FROM departure_settings WHERE booking_id=? LIMIT 1`, bookingID).Scan(&dsID)
	} else if hasColumn(tx, "departure_settings", "reguler_booking_id") {
		_ = tx.QueryRow(`SELECT id FROM departure_settings WHERE reguler_booking_id=? LIMIT 1`, bookingID).Scan(&dsID)
	}

	if dsID == 0 {
		return "", "", ""
	}

	var driverName, vehicleCode, vehicleType sql.NullString
	if hasColumn(tx, "departure_settings", "driver_name") {
		_ = tx.QueryRow(`SELECT COALESCE(driver_name,'') FROM departure_settings WHERE id=?`, dsID).Scan(&driverName)
	} else if hasColumn(tx, "departure_settings", "driver") {
		_ = tx.QueryRow(`SELECT COALESCE(driver,'') FROM departure_settings WHERE id=?`, dsID).Scan(&driverName)
	}
	if hasColumn(tx, "departure_settings", "vehicle_code") {
		_ = tx.QueryRow(`SELECT COALESCE(vehicle_code,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicleCode)
	} else if hasColumn(tx, "departure_settings", "car_code") {
		_ = tx.QueryRow(`SELECT COALESCE(car_code,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicleCode)
	}
	if hasColumn(tx, "departure_settings", "vehicle_type") {
		_ = tx.QueryRow(`SELECT COALESCE(vehicle_type,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicleType)
	} else if hasColumn(tx, "departure_settings", "vehicle_name") {
		_ = tx.QueryRow(`SELECT COALESCE(vehicle_name,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicleType)
	} else if hasColumn(tx, "departure_settings", "vehicle") {
		_ = tx.QueryRow(`SELECT COALESCE(vehicle,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicleType)
	}

	// fallback jenis kendaraan dari driver jika belum ada
	vType := strings.TrimSpace(vehicleType.String)
	if vType == "" && strings.TrimSpace(driverName.String) != "" {
		vType = loadDriverVehicleTypeByDriverName(driverName.String)
	}

	return strings.TrimSpace(driverName.String), strings.TrimSpace(vehicleCode.String), vType
}

// loadDepartureLicensePlate mengambil license_plate dari departure_settings jika ada.
func loadDepartureLicensePlate(tx *sql.Tx, bookingID int64) string {
	if bookingID <= 0 || !hasTable(tx, "departure_settings") {
		return ""
	}

	var dsID int64
	if hasColumn(tx, "departure_settings", "booking_id") {
		_ = tx.QueryRow(`SELECT id FROM departure_settings WHERE booking_id=? LIMIT 1`, bookingID).Scan(&dsID)
	} else if hasColumn(tx, "departure_settings", "reguler_booking_id") {
		_ = tx.QueryRow(`SELECT id FROM departure_settings WHERE reguler_booking_id=? LIMIT 1`, bookingID).Scan(&dsID)
	}
	if dsID == 0 {
		return ""
	}

	if hasColumn(tx, "departure_settings", "license_plate") {
		var plate sql.NullString
		_ = tx.QueryRow(`SELECT COALESCE(license_plate,'') FROM departure_settings WHERE id=?`, dsID).Scan(&plate)
		return strings.TrimSpace(plate.String)
	}
	return ""
}

// loadPaymentValidationContact: ambil customer_name/phone dari payment_validations (relasi booking_id) jika tersedia.
func loadPaymentValidationContact(tx *sql.Tx, bookingID int64) (string, string) {
	if bookingID <= 0 {
		return "", ""
	}
	if !hasColumn(tx, "payment_validations", "booking_id") {
		return "", ""
	}

	cols := []string{}
	if hasColumn(tx, "payment_validations", "customer_name") {
		cols = append(cols, "COALESCE(customer_name,'')")
	}
	if hasColumn(tx, "payment_validations", "customer_phone") {
		cols = append(cols, "COALESCE(customer_phone,'')")
	}
	if len(cols) == 0 {
		return "", ""
	}

	q := fmt.Sprintf(`SELECT %s FROM payment_validations WHERE booking_id=? ORDER BY id DESC LIMIT 1`, strings.Join(cols, ","))

	var n sql.NullString
	var ph sql.NullString
	dests := []any{}
	switch len(cols) {
	case 2:
		dests = []any{&n, &ph}
	case 1:
		if strings.Contains(cols[0], "customer_name") {
			dests = []any{&n}
		} else {
			dests = []any{&ph}
		}
	default:
		dests = []any{&n, &ph}
	}

	if err := tx.QueryRow(q, bookingID).Scan(dests...); err != nil {
		return "", ""
	}

	outName := ""
	outPhone := ""
	if n.Valid {
		outName = strings.TrimSpace(n.String)
	}
	if ph.Valid {
		outPhone = strings.TrimSpace(ph.String)
	}
	return outName, outPhone
}

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

// mergeNotes menambahkan booking hint ke catatan tanpa menduplikasi entri.
func mergeNotes(existing string, bookingID int64) string {
	existing = strings.TrimSpace(existing)
	hint := buildBookingHint(bookingID)

	if existing == "" {
		return hint
	}
	if strings.Contains(existing, hint) {
		return existing
	}
	return existing + "\n" + hint
}

func upsertTripInformation(tx *sql.Tx, p BookingSyncPayload) error {
	tripNo := autoTripNumber(p.Date, p.Time, p.From, p.To)

	// tetap simpan e_surat_jalan minimal (fallback) dari booking endpoint
	// (nanti frontend TripInformation bisa memanggil /api/trip-information/:id/surat-jalan untuk preview file asli)
	esuratURL := buildSuratJalanAPI(p.BookingID)
	driverName, vehicleCode, vehicleType := loadDepartureDriverVehicle(tx, p.BookingID)
	licensePlate := loadDepartureLicensePlate(tx, p.BookingID)

	var existingID int64
	_ = tx.QueryRow(`SELECT id FROM trip_information WHERE trip_number=? LIMIT 1`, tripNo).Scan(&existingID)

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
		if hasColumn(tx, "trip_information", "driver_name") && strings.TrimSpace(driverName) != "" {
			sets = append(sets, "driver_name=?")
			args = append(args, driverName)
		}
		if hasColumn(tx, "trip_information", "vehicle_code") {
			val := vehicleCode
			if val == "" {
				val = vehicleType
			}
			if val != "" {
				sets = append(sets, "vehicle_code=?")
				args = append(args, val)
			}
		}
		if hasColumn(tx, "trip_information", "license_plate") && strings.TrimSpace(licensePlate) != "" {
			sets = append(sets, "license_plate=?")
			args = append(args, licensePlate)
		}

		// tambahan
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
			args = append(args, int64(len(p.SelectedSeats)))
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
		vals = append(vals, driverName)
	}
	if hasColumn(tx, "trip_information", "vehicle_code") {
		val := vehicleCode
		if val == "" {
			val = vehicleType
		}
		cols = append(cols, "vehicle_code")
		vals = append(vals, val)
	}
	if hasColumn(tx, "trip_information", "license_plate") {
		cols = append(cols, "license_plate")
		vals = append(vals, licensePlate)
	}
	if hasColumn(tx, "trip_information", "e_surat_jalan") {
		cols = append(cols, "e_surat_jalan")
		vals = append(vals, esuratURL)
	}

	// tambahan
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
		vals = append(vals, int64(len(p.SelectedSeats)))
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
	seatName := map[string]string{}
	seatPhone := map[string]string{}
	seatPaid := map[string]int64{}
	seenSeat := map[string]bool{}
	var seatsFromPassengers []string

	if hasTable(tx, "booking_passengers") && hasColumn(tx, "booking_passengers", "booking_id") && hasColumn(tx, "booking_passengers", "seat_code") {
		if hasColumn(tx, "booking_passengers", "passenger_name") {
			rows, err := tx.Query(`SELECT seat_code, COALESCE(passenger_name,'') FROM booking_passengers WHERE booking_id=?`, p.BookingID)
			if err == nil {
				for rows.Next() {
					var seat, name string
					_ = rows.Scan(&seat, &name)
					seat = strings.ToUpper(strings.TrimSpace(seat))
					name = strings.TrimSpace(name)
					if seat == "" {
						continue
					}
					if name != "" {
						seatName[seat] = name
					}
					if !seenSeat[seat] {
						seatsFromPassengers = append(seatsFromPassengers, seat)
						seenSeat[seat] = true
					}
				}
				_ = rows.Close()
			}
		}
		if hasColumn(tx, "booking_passengers", "passenger_phone") {
			rows, err := tx.Query(`SELECT seat_code, COALESCE(passenger_phone,'') FROM booking_passengers WHERE booking_id=?`, p.BookingID)
			if err == nil {
				for rows.Next() {
					var seat, ph string
					_ = rows.Scan(&seat, &ph)
					seat = strings.ToUpper(strings.TrimSpace(seat))
					ph = strings.TrimSpace(ph)
					if seat != "" && ph != "" {
						seatPhone[seat] = ph
					}
					if seat != "" && !seenSeat[seat] {
						seatsFromPassengers = append(seatsFromPassengers, seat)
						seenSeat[seat] = true
					}
				}
				_ = rows.Close()
			}
		}
		if hasColumn(tx, "booking_passengers", "paid_price") {
			rows, err := tx.Query(`SELECT seat_code, COALESCE(paid_price,0) FROM booking_passengers WHERE booking_id=?`, p.BookingID)
			if err == nil {
				for rows.Next() {
					var seat string
					var price int64
					_ = rows.Scan(&seat, &price)
					seat = strings.ToUpper(strings.TrimSpace(seat))
					if seat != "" && price > 0 {
						seatPaid[seat] = price
					}
					if seat != "" && !seenSeat[seat] {
						seatsFromPassengers = append(seatsFromPassengers, seat)
						seenSeat[seat] = true
					}
				}
				_ = rows.Close()
			}
		}
	}

	seats := seatsFromPassengers
	if len(seats) == 0 {
		seats = p.SelectedSeats
	}
	if len(seats) == 0 {
		// kalau booking tidak simpan kursi, pakai jumlah penumpang (atau minimal 1)
		if p.PassengerCount > 0 {
			seats = make([]string, p.PassengerCount)
		} else {
			seats = []string{""}
		}
	}
	seatCount := len(normalizeSeatsUnique(seats))
	if seatCount == 0 && p.PassengerCount > 0 {
		seatCount = p.PassengerCount
	}
	perSeatAmount := int64(0)
	if p.TotalAmount > 0 {
		perSeatAmount = p.TotalAmount
		if seatCount > 0 {
			perSeatAmount = p.TotalAmount / int64(seatCount)
			if perSeatAmount == 0 {
				perSeatAmount = p.TotalAmount
			}
		}
	}
	if perSeatAmount == 0 && p.PricePerSeat > 0 {
		perSeatAmount = p.PricePerSeat
	}
	// gunakan paid_price per kursi jika sudah ada di booking_passengers
	if perSeatAmount == 0 && len(seatPaid) > 0 {
		for _, v := range seatPaid {
			if v > 0 {
				perSeatAmount = v
				break
			}
		}
	}
	// fallback total dari paid_price jika total masih kosong
	if p.TotalAmount == 0 && len(seatPaid) > 0 {
		var sum int64
		for _, v := range seatPaid {
			sum += v
		}
		if sum > 0 {
			p.TotalAmount = sum
		}
	}
	if perSeatAmount == 0 {
		agg := aggregatePaidBookingsTx(tx, p.Date, p.Time, p.From, p.To)
		if agg.DeptTotal > 0 {
			div := int64(seatCount)
			if div == 0 && agg.DeptCount > 0 {
				div = int64(agg.DeptCount)
			}
			if div == 0 {
				div = 1
			}
			perSeatAmount = agg.DeptTotal / div
		}
	}
	if perSeatAmount == 0 {
		if fare := regulerFareFromBooking(p.From, p.To); fare > 0 {
			perSeatAmount = fare
		}
	}

	driverName, vehicleCode, vehicleType := loadDepartureDriverVehicle(tx, p.BookingID)
	serviceType := p.Category
	if strings.TrimSpace(serviceType) == "" {
		serviceType = "Reguler"
	}

	for _, seat := range seats {
		seat = strings.ToUpper(strings.TrimSpace(seat))

		name := strings.TrimSpace(seatName[seat])
		if name == "" {
			name = p.PassengerName
		}
		if name == "" {
			name = p.CustomerName
		}
		phone := strings.TrimSpace(p.PassengerPhone)
		if phone == "" {
			phone = strings.TrimSpace(seatPhone[seat])
		}
		if phone == "" {
			phone = strings.TrimSpace(p.CustomerPhone)
		}

		dest := strings.TrimSpace(p.DropoffLocation)
		if dest == "" {
			dest = strings.TrimSpace(p.To)
		}

		eticketURL := strings.TrimSpace(p.ETicketPhoto)
		if eticketURL == "" {
			eticketURL = buildEticketInvoiceMarker(p.BookingID)
		}
		if eticketURL == "" {
			eticketURL = buildSuratJalanAPI(p.BookingID)
		}
		invoiceHint := buildTicketInvoiceHint(p.BookingID)

		var existingID int64
		if hasColumn(tx, "passengers", "booking_id") {
			_ = tx.QueryRow(`SELECT id FROM passengers WHERE booking_id=? AND COALESCE(selected_seats,'')=? LIMIT 1`, p.BookingID, seat).Scan(&existingID)
		} else {
			dateCol := "date"
			if !hasColumn(tx, "passengers", "date") && hasColumn(tx, "passengers", "departure_date") {
				dateCol = "departure_date"
			}
			timeCol := "departure_time"
			if !hasColumn(tx, "passengers", "departure_time") && hasColumn(tx, "passengers", "trip_time") {
				timeCol = "trip_time"
			}
			phoneCol := "phone"
			if !hasColumn(tx, "passengers", "phone") && hasColumn(tx, "passengers", "passenger_phone") {
				phoneCol = "passenger_phone"
			}
			q := fmt.Sprintf(`SELECT id FROM passengers WHERE COALESCE(%s,'')=? AND COALESCE(%s,'')=? AND COALESCE(%s,'')=? AND COALESCE(selected_seats,'')=? LIMIT 1`, dateCol, timeCol, phoneCol)
			_ = tx.QueryRow(q, p.Date, p.Time, phone, seat).Scan(&existingID)
		}

		seatAmount := perSeatAmount
		if v := seatPaid[seat]; v > 0 {
			seatAmount = v
		}
		if seatAmount == 0 && perSeatAmount > 0 {
			seatAmount = perSeatAmount
		}
		totalStr := strconv.FormatInt(seatAmount, 10)
		syncJSON := fmt.Sprintf("{\"booking_id\":%d,\"trip\":\"%s %s\",\"route\":\"%s-%s\"}", p.BookingID, p.Date, p.Time, p.From, p.To)
		notes := "Synced from booking"

		// update paid_price di booking_passengers jika kolom tersedia
		if hasColumn(tx, "booking_passengers", "paid_price") && seatAmount > 0 {
			_, _ = tx.Exec(`UPDATE booking_passengers SET paid_price=? WHERE booking_id=? AND COALESCE(seat_code,'')=?`, seatAmount, p.BookingID, seat)
		}

		if existingID > 0 {
			sets := []string{}
			args := []any{}

			if hasColumn(tx, "passengers", "passenger_name") {
				sets = append(sets, "passenger_name=?")
				args = append(args, name)
			}
			if hasColumn(tx, "passengers", "name") {
				sets = append(sets, "name=?")
				args = append(args, name)
			}
			if hasColumn(tx, "passengers", "passenger_phone") {
				sets = append(sets, "passenger_phone=?")
				args = append(args, phone)
			}
			if hasColumn(tx, "passengers", "phone") {
				sets = append(sets, "phone=?")
				args = append(args, phone)
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
			} else if hasColumn(tx, "passengers", "trip_time") {
				sets = append(sets, "trip_time=?")
				args = append(args, p.Time)
			}
			if hasColumn(tx, "passengers", "pickup_address") {
				sets = append(sets, "pickup_address=?")
				args = append(args, p.PickupLocation)
			}
			if hasColumn(tx, "passengers", "destination") {
				sets = append(sets, "destination=?")
				args = append(args, dest)
			}
			if hasColumn(tx, "passengers", "dropoff_address") {
				sets = append(sets, "dropoff_address=?")
				args = append(args, dest)
			}
			if hasColumn(tx, "passengers", "total_amount") {
				sets = append(sets, "total_amount=?")
				args = append(args, totalStr)
			} else if hasColumn(tx, "passengers", "total") {
				sets = append(sets, "total=?")
				args = append(args, seatAmount)
			}
			if hasColumn(tx, "passengers", "selected_seats") {
				sets = append(sets, "selected_seats=?")
				args = append(args, seat)
			}
			if hasColumn(tx, "passengers", "service_type") {
				sets = append(sets, "service_type=?")
				args = append(args, serviceType)
			}
			if hasColumn(tx, "passengers", "eticket_photo") {
				sets = append(sets, "eticket_photo=?")
				args = append(args, eticketURL)
			}
			if hasColumn(tx, "passengers", "eticket_invoice_hint") {
				sets = append(sets, "eticket_invoice_hint=?")
				args = append(args, invoiceHint)
			}
			if hasColumn(tx, "passengers", "driver_name") {
				sets = append(sets, "driver_name=?")
				args = append(args, driverName)
			}
			if hasColumn(tx, "passengers", "driver") {
				sets = append(sets, "driver=?")
				args = append(args, driverName)
			}
			if hasColumn(tx, "passengers", "vehicle_code") {
				val := vehicleCode
				if val == "" {
					val = vehicleType
				}
				sets = append(sets, "vehicle_code=?")
				args = append(args, val)
			}
			if hasColumn(tx, "passengers", "vehicle_type") {
				val := vehicleType
				if val == "" {
					val = vehicleCode
				}
				sets = append(sets, "vehicle_type=?")
				args = append(args, val)
			}
			if hasColumn(tx, "passengers", "vehicle_name") {
				val := vehicleType
				if val == "" {
					val = vehicleCode
				}
				sets = append(sets, "vehicle_name=?")
				args = append(args, val)
			}
			if hasColumn(tx, "passengers", "notes") {
				sets = append(sets, "notes=?")
				args = append(args, notes+"\n"+syncJSON)
			}
			if hasColumn(tx, "passengers", "booking_id") {
				sets = append(sets, "booking_id=?")
				args = append(args, p.BookingID)
			}
			if hasColumn(tx, "passengers", "booking_hint") {
				sets = append(sets, "booking_hint=?")
				args = append(args, buildBookingHint(p.BookingID))
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
			if _, err := tx.Exec(`UPDATE passengers SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
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
		if hasColumn(tx, "passengers", "name") {
			cols = append(cols, "name")
			vals = append(vals, name)
		}
		if hasColumn(tx, "passengers", "passenger_phone") {
			cols = append(cols, "passenger_phone")
			vals = append(vals, phone)
		}
		if hasColumn(tx, "passengers", "phone") {
			cols = append(cols, "phone")
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
		} else if hasColumn(tx, "passengers", "trip_time") {
			cols = append(cols, "trip_time")
			vals = append(vals, p.Time)
		}
		if hasColumn(tx, "passengers", "pickup_address") {
			cols = append(cols, "pickup_address")
			vals = append(vals, p.PickupLocation)
		}
		if hasColumn(tx, "passengers", "destination") {
			cols = append(cols, "destination")
			vals = append(vals, dest)
		}
		if hasColumn(tx, "passengers", "dropoff_address") {
			cols = append(cols, "dropoff_address")
			vals = append(vals, dest)
		}
		if hasColumn(tx, "passengers", "category") {
			cols = append(cols, "category")
			vals = append(vals, p.Category)
		}
		if hasColumn(tx, "passengers", "service_type") {
			cols = append(cols, "service_type")
			vals = append(vals, serviceType)
		}
		if hasColumn(tx, "passengers", "route_from") {
			cols = append(cols, "route_from")
			vals = append(vals, p.From)
		}
		if hasColumn(tx, "passengers", "route_to") {
			cols = append(cols, "route_to")
			vals = append(vals, p.To)
		}
		if hasColumn(tx, "passengers", "selected_seats") {
			cols = append(cols, "selected_seats")
			vals = append(vals, seat)
		}
		if hasColumn(tx, "passengers", "driver_name") {
			cols = append(cols, "driver_name")
			vals = append(vals, driverName)
		}
		if hasColumn(tx, "passengers", "driver") {
			cols = append(cols, "driver")
			vals = append(vals, driverName)
		}
		if hasColumn(tx, "passengers", "vehicle_code") {
			val := vehicleCode
			if val == "" {
				val = vehicleType
			}
			cols = append(cols, "vehicle_code")
			vals = append(vals, val)
		}
		if hasColumn(tx, "passengers", "vehicle_type") {
			val := vehicleType
			if val == "" {
				val = vehicleCode
			}
			cols = append(cols, "vehicle_type")
			vals = append(vals, val)
		}
		if hasColumn(tx, "passengers", "vehicle_name") {
			val := vehicleType
			if val == "" {
				val = vehicleCode
			}
			cols = append(cols, "vehicle_name")
			vals = append(vals, val)
		}
		if hasColumn(tx, "passengers", "total_amount") {
			cols = append(cols, "total_amount")
			vals = append(vals, seatAmount)
		} else if hasColumn(tx, "passengers", "total") {
			cols = append(cols, "total")
			vals = append(vals, seatAmount)
		}
		if hasColumn(tx, "passengers", "booking_id") {
			cols = append(cols, "booking_id")
			vals = append(vals, p.BookingID)
		}
		if hasColumn(tx, "passengers", "eticket_photo") {
			cols = append(cols, "eticket_photo")
			vals = append(vals, eticketURL)
		}
		if hasColumn(tx, "passengers", "eticket_invoice_hint") {
			cols = append(cols, "eticket_invoice_hint")
			vals = append(vals, invoiceHint)
		}
		if hasColumn(tx, "passengers", "notes") {
			cols = append(cols, "notes")
			vals = append(vals, mergeNotes("", p.BookingID))
		}
		if hasColumn(tx, "passengers", "booking_hint") {
			cols = append(cols, "booking_hint")
			vals = append(vals, buildBookingHint(p.BookingID))
		}
		if hasColumn(tx, "passengers", "surat_jalan_api") {
			cols = append(cols, "surat_jalan_api")
			vals = append(vals, buildSuratJalanAPI(p.BookingID))
		}
		if hasColumn(tx, "passengers", "created_at") {
			cols = append(cols, "created_at")
			vals = append(vals, time.Now())
		}
		if hasColumn(tx, "passengers", "updated_at") {
			cols = append(cols, "updated_at")
			vals = append(vals, time.Now())
		}

		ph := make([]string, 0, len(cols))
		for range cols {
			ph = append(ph, "?")
		}

		if _, err := tx.Exec(`INSERT INTO passengers (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(ph, ",")+`)`, vals...); err != nil {
			return err
		}
	}

	return nil
}

/*
	========================================================
	✅ TAMBAHAN (TIDAK MENGHAPUS KODE LAMA):
	Sync ke tabel departure_settings supaya:
	- Booking yang sudah lunas otomatis tampil di Pengaturan Keberangkatan
	- E-Surat Jalan tampil seperti Informasi 10 (via /api/trip-information/:id/surat-jalan)
	- Data pemesan/penumpang ikut tersimpan (jika kolom tersedia)
	========================================================
*/

func joinSeatsForDB(seats []string) string {
	if len(seats) == 0 {
		return ""
	}
	return strings.Join(normalizeSeatsUnique(seats), ", ")
}

// build string daftar penumpang dari booking_passengers (kalau ada)
// format:
//
//	1A - Nerry
//	1B - Budi
func buildPassengerListText(tx *sql.Tx, bookingID int64) string {
	if bookingID <= 0 {
		return ""
	}
	if !hasTable(tx, "booking_passengers") {
		return ""
	}
	if !hasColumn(tx, "booking_passengers", "booking_id") || !hasColumn(tx, "booking_passengers", "seat_code") {
		return ""
	}
	if !hasColumn(tx, "booking_passengers", "passenger_name") {
		return ""
	}

	rows, err := tx.Query(`SELECT COALESCE(seat_code,''), COALESCE(passenger_name,'') FROM booking_passengers WHERE booking_id=? ORDER BY seat_code ASC`, bookingID)
	if err != nil {
		return ""
	}

	var lines []string
	for rows.Next() {
		var seat, name string
		_ = rows.Scan(&seat, &name)
		seat = strings.ToUpper(strings.TrimSpace(seat))
		name = strings.TrimSpace(name)
		if seat == "" && name == "" {
			continue
		}
		if seat == "" {
			lines = append(lines, name)
		} else if name == "" {
			lines = append(lines, seat)
		} else {
			lines = append(lines, fmt.Sprintf("%s - %s", seat, name))
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return ""
	}
	_ = rows.Close()
	return strings.Join(lines, "\n")
}

// ✅ ambil ID trip_information berdasarkan trip_number
func findTripInformationID(tx *sql.Tx, tripNo string) int64 {
	if strings.TrimSpace(tripNo) == "" {
		return 0
	}
	if !hasTable(tx, "trip_information") {
		return 0
	}
	var id int64
	_ = tx.QueryRow(`SELECT id FROM trip_information WHERE trip_number=? LIMIT 1`, tripNo).Scan(&id)
	return id
}

// ✅ Tentukan URL surat jalan untuk departure_settings:
// - prioritas: /api/trip-information/:id/surat-jalan  (hasilnya seperti Informasi 10)
// - fallback: /api/reguler/bookings/:id/surat-jalan?scope=trip (JSON)
func buildDepartureSuratJalanURL(tx *sql.Tx, p BookingSyncPayload) (string, string) {
	tripNo := autoTripNumber(p.Date, p.Time, p.From, p.To)
	tripID := findTripInformationID(tx, tripNo)
	if tripID > 0 {
		return buildTripInformationSuratJalanAPI(tripID), fmt.Sprintf("SuratJalan-TRIP-%d", tripID)
	}
	// fallback lama
	return buildSuratJalanAPI(p.BookingID) + "?scope=trip", fmt.Sprintf("SuratJalan-BOOKING-%d", p.BookingID)
}

func upsertDepartureSettings(tx *sql.Tx, p BookingSyncPayload) error {
	table := "departure_settings"
	if !hasTable(tx, table) {
		return nil
	}

	// idempotent: pakai booking_id kalau ada
	var existingID int64
	if hasColumn(tx, table, "booking_id") {
		_ = tx.QueryRow(`SELECT id FROM `+table+` WHERE booking_id=? LIMIT 1`, p.BookingID).Scan(&existingID)
	} else if hasColumn(tx, table, "reguler_booking_id") {
		_ = tx.QueryRow(`SELECT id FROM `+table+` WHERE reguler_booking_id=? LIMIT 1`, p.BookingID).Scan(&existingID)
	}

	seatNumbers := joinSeatsForDB(p.SelectedSeats)
	passengerCount := strconv.Itoa(len(p.SelectedSeats))
	if passengerCount == "0" {
		// kalau seat kosong tapi booking_passengers ada, hitung dari sana
		if hasTable(tx, "booking_passengers") && hasColumn(tx, "booking_passengers", "booking_id") {
			var cnt int
			_ = tx.QueryRow(`SELECT COUNT(*) FROM booking_passengers WHERE booking_id=?`, p.BookingID).Scan(&cnt)
			if cnt > 0 {
				passengerCount = strconv.Itoa(cnt)
			}
		}
	}

	passengerListText := buildPassengerListText(tx, p.BookingID)

	// ✅ ini inti perbaikan agar tampil seperti Informasi 10
	suratURL, suratName := buildDepartureSuratJalanURL(tx, p)

	if existingID > 0 {
		sets := []string{}
		args := []any{}

		if hasColumn(tx, table, "booking_name") {
			sets = append(sets, "booking_name=?")
			args = append(args, p.PassengerName)
		}
		if hasColumn(tx, table, "phone") {
			sets = append(sets, "phone=?")
			args = append(args, p.PassengerPhone)
		}
		if hasColumn(tx, table, "pickup_address") {
			sets = append(sets, "pickup_address=?")
			args = append(args, p.PickupLocation)
		}
		if hasColumn(tx, table, "departure_date") {
			sets = append(sets, "departure_date=?")
			args = append(args, p.Date)
		}
		if hasColumn(tx, table, "seat_numbers") {
			sets = append(sets, "seat_numbers=?")
			args = append(args, seatNumbers)
		}
		if hasColumn(tx, table, "passenger_count") {
			sets = append(sets, "passenger_count=?")
			args = append(args, passengerCount)
		}
		if hasColumn(tx, table, "service_type") {
			sets = append(sets, "service_type=?")
			args = append(args, p.Category)
		}

		// ✅ surat jalan
		if hasColumn(tx, table, "surat_jalan_file") {
			sets = append(sets, "surat_jalan_file=?")
			args = append(args, suratURL)
		}
		if hasColumn(tx, table, "surat_jalan_file_name") {
			sets = append(sets, "surat_jalan_file_name=?")
			args = append(args, suratName)
		}

		// ✅ simpan daftar penumpang jika ada kolomnya
		if passengerListText != "" && hasColumn(tx, table, "passenger_list") {
			sets = append(sets, "passenger_list=?")
			args = append(args, passengerListText)
		}

		// route tambahan
		if hasColumn(tx, table, "route_from") {
			sets = append(sets, "route_from=?")
			args = append(args, p.From)
		}
		if hasColumn(tx, table, "route_to") {
			sets = append(sets, "route_to=?")
			args = append(args, p.To)
		}

		// trip number tambahan
		if hasColumn(tx, table, "trip_number") {
			sets = append(sets, "trip_number=?")
			args = append(args, autoTripNumber(p.Date, p.Time, p.From, p.To))
		}

		// booking_id jika ada
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

	cols := []string{}
	vals := []any{}

	if hasColumn(tx, table, "booking_name") {
		cols = append(cols, "booking_name")
		vals = append(vals, p.PassengerName)
	}
	if hasColumn(tx, table, "phone") {
		cols = append(cols, "phone")
		vals = append(vals, p.PassengerPhone)
	}
	if hasColumn(tx, table, "pickup_address") {
		cols = append(cols, "pickup_address")
		vals = append(vals, p.PickupLocation)
	}
	if hasColumn(tx, table, "departure_date") {
		cols = append(cols, "departure_date")
		vals = append(vals, p.Date)
	}
	if hasColumn(tx, table, "seat_numbers") {
		cols = append(cols, "seat_numbers")
		vals = append(vals, seatNumbers)
	}
	if hasColumn(tx, table, "passenger_count") {
		cols = append(cols, "passenger_count")
		vals = append(vals, passengerCount)
	}
	if hasColumn(tx, table, "service_type") {
		cols = append(cols, "service_type")
		vals = append(vals, p.Category)
	}

	// ✅ surat jalan
	if hasColumn(tx, table, "surat_jalan_file") {
		cols = append(cols, "surat_jalan_file")
		vals = append(vals, suratURL)
	}
	if hasColumn(tx, table, "surat_jalan_file_name") {
		cols = append(cols, "surat_jalan_file_name")
		vals = append(vals, suratName)
	}

	// ✅ simpan daftar penumpang jika ada kolomnya
	if passengerListText != "" && hasColumn(tx, table, "passenger_list") {
		cols = append(cols, "passenger_list")
		vals = append(vals, passengerListText)
	}

	// route tambahan
	if hasColumn(tx, table, "route_from") {
		cols = append(cols, "route_from")
		vals = append(vals, p.From)
	}
	if hasColumn(tx, table, "route_to") {
		cols = append(cols, "route_to")
		vals = append(vals, p.To)
	}

	// trip number tambahan
	if hasColumn(tx, table, "trip_number") {
		cols = append(cols, "trip_number")
		vals = append(vals, autoTripNumber(p.Date, p.Time, p.From, p.To))
	}

	// booking_id jika ada
	if hasColumn(tx, table, "booking_id") {
		cols = append(cols, "booking_id")
		vals = append(vals, p.BookingID)
	} else if hasColumn(tx, table, "reguler_booking_id") {
		cols = append(cols, "reguler_booking_id")
		vals = append(vals, p.BookingID)
	}

	if hasColumn(tx, table, "created_at") {
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

	_, err := tx.Exec(`INSERT INTO `+table+` (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(ph, ",")+`)`, vals...)
	return err
}

/*
	========================================================
	✅ TAMBAHAN: sinkronisasi ke laporan keuangan (trips)
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
func loadPaidSeatAggregate(q queryRower, bookingID int64) (int, int64) {
	if bookingID <= 0 || !hasTable(q, "booking_passengers") {
		return 0, 0
	}
	if !hasColumn(q, "booking_passengers", "booking_id") || !hasColumn(q, "booking_passengers", "paid_price") {
		return 0, 0
	}

	var cnt int
	var sum int64
	_ = q.QueryRow(`SELECT COUNT(*), SUM(COALESCE(paid_price,0)) FROM booking_passengers WHERE booking_id=?`, bookingID).Scan(&cnt, &sum)
	return cnt, sum
}

// loadBookingTotal fetch total/total_amount langsung dari tabel bookings jika ada.
func loadBookingTotal(q queryRower, bookingID int64) int64 {
	if bookingID <= 0 {
		return 0
	}
	table := ""
	switch {
	case hasTable(q, "bookings"):
		table = "bookings"
	case hasTable(q, "reguler_bookings"):
		table = "reguler_bookings"
	default:
		return 0
	}
	col := ""
	for _, c := range []string{"total_amount", "total"} {
		if hasColumn(q, table, c) {
			col = c
			break
		}
	}
	if col == "" {
		return 0
	}
	var total sql.NullInt64
	_ = q.QueryRow(`SELECT COALESCE(`+col+`,0) FROM `+table+` WHERE id=? LIMIT 1`, bookingID).Scan(&total)
	if total.Valid && total.Int64 > 0 {
		return total.Int64
	}
	return 0
}

// loadBookingTotalAndCount mengambil total + passenger_count langsung dari tabel bookings/reguler_bookings.
func loadBookingTotalAndCount(q queryRower, bookingID int64) (int64, int) {
	if bookingID <= 0 {
		return 0, 0
	}
	table := ""
	switch {
	case hasTable(q, "bookings"):
		table = "bookings"
	case hasTable(q, "reguler_bookings"):
		table = "reguler_bookings"
	default:
		return 0, 0
	}

	totalCol := ""
	for _, c := range []string{"total_amount", "total"} {
		if hasColumn(q, table, c) {
			totalCol = c
			break
		}
	}
	countCol := ""
	if hasColumn(q, table, "passenger_count") {
		countCol = "passenger_count"
	}

	selects := []string{}
	if totalCol != "" {
		selects = append(selects, "COALESCE("+totalCol+",0)")
	}
	if countCol != "" {
		selects = append(selects, "COALESCE("+countCol+",0)")
	}
	if len(selects) == 0 {
		return 0, 0
	}

	qstr := `SELECT ` + strings.Join(selects, ",") + ` FROM ` + table + ` WHERE id=? LIMIT 1`
	var total sql.NullInt64
	var cnt sql.NullInt64
	dest := []any{}
	if totalCol != "" && countCol != "" {
		dest = []any{&total, &cnt}
	} else if totalCol != "" {
		dest = []any{&total}
	} else {
		dest = []any{&cnt}
	}
	_ = q.QueryRow(qstr, bookingID).Scan(dest...)

	outTotal := int64(0)
	outCount := 0
	if total.Valid && total.Int64 > 0 {
		outTotal = total.Int64
	}
	if cnt.Valid && cnt.Int64 > 0 {
		outCount = int(cnt.Int64)
	}
	return outTotal, outCount
}

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

func upsertTripsFinance(tx *sql.Tx, p BookingSyncPayload) error {
	table := "trips"
	if !hasTable(tx, table) {
		return nil
	}

	tripKey := autoTripNumber(p.Date, p.Time, p.From, p.To)
	day, month, year := parseDayMonthYear(p.Date)

	// ambil driver + unit + vehicleType dari departure_settings berdasarkan booking_id
	var (
		carCode     string
		driverName  string
		vehicleName string
		serviceType string
		depOrigin   string
		depDest     string
	)
	serviceType = p.Category
	depOrigin = p.From
	depDest = p.To

	if hasTable(tx, "departure_settings") {
		// cari row departure_settings untuk booking ini
		var dsID int64
		if hasColumn(tx, "departure_settings", "booking_id") {
			_ = tx.QueryRow(`SELECT id FROM departure_settings WHERE booking_id=? LIMIT 1`, p.BookingID).Scan(&dsID)
		} else if hasColumn(tx, "departure_settings", "reguler_booking_id") {
			_ = tx.QueryRow(`SELECT id FROM departure_settings WHERE reguler_booking_id=? LIMIT 1`, p.BookingID).Scan(&dsID)
		}

		if dsID > 0 {
			if hasColumn(tx, "departure_settings", "vehicle_code") {
				_ = tx.QueryRow(`SELECT COALESCE(vehicle_code,'') FROM departure_settings WHERE id=?`, dsID).Scan(&carCode)
			}
			if hasColumn(tx, "departure_settings", "driver_name") {
				_ = tx.QueryRow(`SELECT COALESCE(driver_name,'') FROM departure_settings WHERE id=?`, dsID).Scan(&driverName)
			}
			if hasColumn(tx, "departure_settings", "vehicle_type") {
				_ = tx.QueryRow(`SELECT COALESCE(vehicle_type,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicleName)
			}
		}
	}

	// fallback vehicle type dari akun driver jika kosong
	if strings.TrimSpace(vehicleName) == "" && strings.TrimSpace(driverName) != "" {
		vehicleName = loadDriverVehicleTypeByDriverName(driverName)
	}

	// order no
	orderNo := ""
	if strings.TrimSpace(carCode) != "" {
		orderNo = buildOrderNumberForTrip(carCode, day, month, year)
	}

	// agregasi penumpang & tarif untuk slot tanggal+jam
	agg := aggregatePaidBookingsTx(tx, p.Date, p.Time, p.From, p.To)

	// paid_price per kursi dari booking_passengers
	paidCount, paidSum := loadPaidSeatAggregate(tx, p.BookingID)
	bookingTotal := p.TotalAmount
	if bookingTotal == 0 {
		bookingTotal = loadBookingTotal(tx, p.BookingID)
	}
	bookingTotal2, bookingCount := loadBookingTotalAndCount(tx, p.BookingID)
	if bookingTotal == 0 && bookingTotal2 > 0 {
		bookingTotal = bookingTotal2
	}

	// tentukan jumlah penumpang
	deptCount := 0
	if p.PassengerCount > 0 {
		deptCount = p.PassengerCount
	} else if bookingCount > 0 {
		deptCount = bookingCount
	} else if paidCount > 0 {
		deptCount = paidCount
	} else if agg.DeptCount > 0 {
		deptCount = agg.DeptCount
	} else {
		deptCount = len(normalizeSeatsUnique(p.SelectedSeats))
		if deptCount == 0 {
			deptCount = 1
		}
	}

	// tentukan tarif penumpang (prioritas total booking)
	deptFare := bookingTotal
	if deptFare == 0 && paidSum > 0 {
		deptFare = paidSum
	} else if deptFare == 0 && agg.DeptTotal > 0 {
		deptFare = agg.DeptTotal
	} else if deptFare == 0 && p.PricePerSeat > 0 {
		deptFare = p.PricePerSeat * int64(deptCount)
	} else if deptFare == 0 {
		if fare := regulerFareFromBooking(depOrigin, depDest); fare > 0 {
			deptFare = fare * int64(deptCount)
		}
	}
	if deptFare < 0 {
		deptFare = 0
	}

	// idempotent by trip_key jika ada
	var existingID int64
	if hasColumn(tx, table, "trip_key") {
		_ = tx.QueryRow(`SELECT id FROM trips WHERE trip_key=? LIMIT 1`, tripKey).Scan(&existingID)
	} else {
		// fallback: by date+time+car_code
		hasDMY := hasColumn(tx, table, "day") && hasColumn(tx, table, "month") && hasColumn(tx, table, "year")
		hasCar := hasColumn(tx, table, "car_code")
		if hasDMY && hasCar && strings.TrimSpace(carCode) != "" {
			_ = tx.QueryRow(`SELECT id FROM trips WHERE day=? AND month=? AND year=? AND car_code=? LIMIT 1`, day, month, year, carCode).Scan(&existingID)
		}
		// jika belum ketemu, longgar: cocokkan hanya day/month/year tanpa car_code agar tidak dobel
		if existingID == 0 && hasDMY {
			_ = tx.QueryRow(`SELECT id FROM trips WHERE day=? AND month=? AND year=? LIMIT 1`, day, month, year).Scan(&existingID)
		}
	}

	if existingID > 0 {
		// baca nilai lama agar tidak menimpa nominal jika perhitungan baru nol
		var oldDeptFare, oldRetFare int64
		var oldDeptCount, oldRetCount int
		if hasColumn(tx, table, "dept_passenger_fare") || hasColumn(tx, table, "ret_passenger_fare") ||
			hasColumn(tx, table, "dept_passenger_count") || hasColumn(tx, table, "ret_passenger_count") {
			selectCols := []string{}
			if hasColumn(tx, table, "dept_passenger_fare") {
				selectCols = append(selectCols, "COALESCE(dept_passenger_fare,0)")
			}
			if hasColumn(tx, table, "ret_passenger_fare") {
				selectCols = append(selectCols, "COALESCE(ret_passenger_fare,0)")
			}
			if hasColumn(tx, table, "dept_passenger_count") {
				selectCols = append(selectCols, "COALESCE(dept_passenger_count,0)")
			}
			if hasColumn(tx, table, "ret_passenger_count") {
				selectCols = append(selectCols, "COALESCE(ret_passenger_count,0)")
			}
			if len(selectCols) > 0 {
				var dests []any
				for _, col := range selectCols {
					switch {
					case strings.Contains(col, "dept_passenger_fare"):
						dests = append(dests, &oldDeptFare)
					case strings.Contains(col, "ret_passenger_fare"):
						dests = append(dests, &oldRetFare)
					case strings.Contains(col, "dept_passenger_count"):
						dests = append(dests, &oldDeptCount)
					case strings.Contains(col, "ret_passenger_count"):
						dests = append(dests, &oldRetCount)
					}
				}
				_ = tx.QueryRow(`SELECT `+strings.Join(selectCols, ",")+` FROM `+table+` WHERE id=? LIMIT 1`, existingID).Scan(dests...)
			}
		}

		// jangan timpa dengan nol jika sebelumnya sudah ada nilai > 0
		if deptFare == 0 && oldDeptFare > 0 {
			deptFare = oldDeptFare
		}
		if agg.RetTotal == 0 && oldRetFare > 0 {
			agg.RetTotal = oldRetFare
		}
		if deptCount == 0 && oldDeptCount > 0 {
			deptCount = oldDeptCount
		}
		if agg.RetCount == 0 && oldRetCount > 0 {
			agg.RetCount = oldRetCount
		}

		sets := []string{}
		args := []any{}
		payStatus := strings.TrimSpace(p.PaymentStatus)
		if payStatus == "" {
			payStatus = "Lunas"
		}

		if hasColumn(tx, table, "trip_key") {
			sets = append(sets, "trip_key=?")
			args = append(args, tripKey)
		}
		if hasColumn(tx, table, "day") {
			sets = append(sets, "day=?")
			args = append(args, day)
		}
		if hasColumn(tx, table, "month") {
			sets = append(sets, "month=?")
			args = append(args, month)
		}
		if hasColumn(tx, table, "year") {
			sets = append(sets, "year=?")
			args = append(args, year)
		}
		if hasColumn(tx, table, "car_code") && strings.TrimSpace(carCode) != "" {
			sets = append(sets, "car_code=?")
			args = append(args, strings.ToUpper(strings.TrimSpace(carCode)))
		}
		if hasColumn(tx, table, "vehicle_name") && strings.TrimSpace(vehicleName) != "" {
			sets = append(sets, "vehicle_name=?")
			args = append(args, vehicleName)
		}
		if hasColumn(tx, table, "driver_name") && strings.TrimSpace(driverName) != "" {
			sets = append(sets, "driver_name=?")
			args = append(args, driverName)
		}
		if hasColumn(tx, table, "order_no") && strings.TrimSpace(orderNo) != "" {
			sets = append(sets, "order_no=?")
			args = append(args, orderNo)
		}

		if hasColumn(tx, table, "dept_origin") {
			sets = append(sets, "dept_origin=?")
			args = append(args, depOrigin)
		}
		if hasColumn(tx, table, "dept_dest") {
			sets = append(sets, "dept_dest=?")
			args = append(args, depDest)
		}
		if hasColumn(tx, table, "dept_category") {
			sets = append(sets, "dept_category=?")
			args = append(args, serviceType)
		}
		if hasColumn(tx, table, "dept_passenger_count") {
			sets = append(sets, "dept_passenger_count=?")
			args = append(args, deptCount)
		}
		if hasColumn(tx, table, "dept_passenger_fare") {
			sets = append(sets, "dept_passenger_fare=?")
			args = append(args, deptFare)
		}

		// return (kepulangan) masih mapping rule lanjut tahap berikutnya
		if hasColumn(tx, table, "ret_origin") {
			sets = append(sets, "ret_origin=?")
			args = append(args, "")
		}
		if hasColumn(tx, table, "ret_dest") {
			sets = append(sets, "ret_dest=?")
			args = append(args, "")
		}
		if hasColumn(tx, table, "ret_category") {
			sets = append(sets, "ret_category=?")
			args = append(args, "")
		}
		if hasColumn(tx, table, "ret_passenger_count") {
			sets = append(sets, "ret_passenger_count=?")
			args = append(args, agg.RetCount)
		}
		if hasColumn(tx, table, "ret_passenger_fare") {
			sets = append(sets, "ret_passenger_fare=?")
			args = append(args, agg.RetTotal)
		}

		// default biaya lainnya
		for _, c := range []string{"other_income", "bbm_fee", "meal_fee", "courier_fee"} {
			if hasColumn(tx, table, c) {
				sets = append(sets, c+"=?")
				args = append(args, 0)
			}
		}

		if hasColumn(tx, table, "payment_status") {
			sets = append(sets, "payment_status=?")
			args = append(args, payStatus)
		}

		if hasColumn(tx, table, "updated_at") {
			sets = append(sets, "updated_at=?")
			args = append(args, time.Now())
		}

		if len(sets) == 0 {
			return nil
		}

		args = append(args, existingID)
		_, err := tx.Exec(`UPDATE trips SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...)
		return err
	}

	cols := []string{}
	vals := []any{}
	payStatus := strings.TrimSpace(p.PaymentStatus)
	if payStatus == "" {
		payStatus = "Lunas"
	}

	if hasColumn(tx, table, "trip_key") {
		cols = append(cols, "trip_key")
		vals = append(vals, tripKey)
	}
	if hasColumn(tx, table, "day") {
		cols = append(cols, "day")
		vals = append(vals, day)
	}
	if hasColumn(tx, table, "month") {
		cols = append(cols, "month")
		vals = append(vals, month)
	}
	if hasColumn(tx, table, "year") {
		cols = append(cols, "year")
		vals = append(vals, year)
	}
	if hasColumn(tx, table, "car_code") {
		cols = append(cols, "car_code")
		vals = append(vals, strings.ToUpper(strings.TrimSpace(carCode)))
	}
	if hasColumn(tx, table, "vehicle_name") {
		cols = append(cols, "vehicle_name")
		vals = append(vals, vehicleName)
	}
	if hasColumn(tx, table, "driver_name") {
		cols = append(cols, "driver_name")
		vals = append(vals, driverName)
	}
	if hasColumn(tx, table, "order_no") {
		cols = append(cols, "order_no")
		vals = append(vals, orderNo)
	}

	if hasColumn(tx, table, "dept_origin") {
		cols = append(cols, "dept_origin")
		vals = append(vals, depOrigin)
	}
	if hasColumn(tx, table, "dept_dest") {
		cols = append(cols, "dept_dest")
		vals = append(vals, depDest)
	}
	if hasColumn(tx, table, "dept_category") {
		cols = append(cols, "dept_category")
		vals = append(vals, serviceType)
	}
	if hasColumn(tx, table, "dept_passenger_count") {
		cols = append(cols, "dept_passenger_count")
		vals = append(vals, deptCount)
	}
	if hasColumn(tx, table, "dept_passenger_fare") {
		cols = append(cols, "dept_passenger_fare")
		vals = append(vals, deptFare)
	}

	// return (kepulangan) masih mapping rule lanjut tahap berikutnya
	if hasColumn(tx, table, "ret_origin") {
		cols = append(cols, "ret_origin")
		vals = append(vals, "")
	}
	if hasColumn(tx, table, "ret_dest") {
		cols = append(cols, "ret_dest")
		vals = append(vals, "")
	}
	if hasColumn(tx, table, "ret_category") {
		cols = append(cols, "ret_category")
		vals = append(vals, "")
	}
	if hasColumn(tx, table, "ret_passenger_count") {
		cols = append(cols, "ret_passenger_count")
		vals = append(vals, agg.RetCount)
	}
	if hasColumn(tx, table, "ret_passenger_fare") {
		cols = append(cols, "ret_passenger_fare")
		vals = append(vals, agg.RetTotal)
	}

	for _, c := range []string{"other_income", "bbm_fee", "meal_fee", "courier_fee"} {
		if hasColumn(tx, table, c) {
			cols = append(cols, c)
			vals = append(vals, 0)
		}
	}

	if hasColumn(tx, table, "payment_status") {
		cols = append(cols, "payment_status")
		vals = append(vals, payStatus)
	}
	if hasColumn(tx, table, "created_at") {
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

	_, err := tx.Exec(`INSERT INTO trips (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(ph, ",")+`)`, vals...)
	return err
}
