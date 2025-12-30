// backend/handlers/departure_berangkat_sync.go
package handlers

import (
	"backend/config"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

// SyncAfterDepartureBerangkat akan dipanggil setelah departure_settings berhasil di-update menjadi "Berangkat".
// Tugas:
// - push data ke tabel passengers (ambil dari booking + driver/vehicle dari departure_settings)
// - push data ke tabel trip_information (trip record + surat jalan)
func SyncAfterDepartureBerangkat(ds DepartureSetting) error {
	// Hanya jalan jika status benar-benar "Berangkat"
	if strings.TrimSpace(strings.ToLower(ds.DepartureStatus)) != strings.ToLower("Berangkat") {
		log.Println("[BERANGKAT SYNC] skip: status bukan Berangkat, id:", ds.ID, "status:", ds.DepartureStatus)
		return nil
	}

	// BookingID wajib ada untuk ambil data booking (nama/no hp/e-ticket/total/pickup/dropoff)
	if ds.BookingID <= 0 {
		// coba ambil ulang dari DB (kalau UI tidak kirim booking_id)
		if bid := lookupBookingID(ds.ID); bid > 0 {
			ds.BookingID = bid
			log.Println("[BERANGKAT SYNC] fallback booking_id dari DB:", ds.BookingID, "for departure_settings id:", ds.ID)
		} else if bid := lookupBookingIDFromTripNumber(ds.TripNumber); bid > 0 {
			ds.BookingID = bid
			log.Println("[BERANGKAT SYNC] fallback booking_id dari trip_number:", ds.BookingID, "trip:", ds.TripNumber)
		} else if bid := lookupBookingIDFromPaymentValidations(ds.Phone, ds.PickupAddress, ds.DepartureDate); bid > 0 {
			ds.BookingID = bid
			log.Println("[BERANGKAT SYNC] fallback booking_id dari payment_validations:", ds.BookingID)
		} else if bid := lookupBookingIDFromBookings(ds.Phone, ds.PickupAddress, ds.DepartureDate); bid > 0 {
			ds.BookingID = bid
			log.Println("[BERANGKAT SYNC] fallback booking_id dari bookings:", ds.BookingID)
		} else {
			log.Println("[BERANGKAT SYNC] skip: BookingID kosong & tidak ditemukan. dep_id:", ds.ID, "trip:", ds.TripNumber)
			return nil
		}
	}

	// validasi booking ada
	if !bookingExists(ds.BookingID) {
		log.Println("[BERANGKAT SYNC] skip: booking tidak ditemukan, booking_id:", ds.BookingID)
		return nil
	}

	// buka transaksi
	tx, err := config.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// baca payload booking (kalau ada di project kamu)
	var p BookingSyncPayload
	// NOTE: readBookingPayload ada di file lain di project kamu.
	// Kalau error, kita tetap lanjut pakai ds + fallback loader (seats/nama/nohp).
	if rp, rerr := readBookingPayload(tx, ds.BookingID); rerr == nil {
		p = rp
		// safety: isi booking id kalau kosong
		if p.BookingID <= 0 {
			p.BookingID = ds.BookingID
		}
	} else {
		log.Println("[BERANGKAT SYNC] warning: readBookingPayload gagal, pakai fallback ds. booking_id:", ds.BookingID, "err:", rerr)
	}

	// upsert ke passengers (✅ sekarang PER-SEAT / PER-PENUMPANG)
	if err := berangkatUpsertPassengersPerSeat(tx, ds, p); err != nil {
		return err
	}

	// upsert ke trip_information (tetap seperti sebelumnya)
	if err := berangkatUpsertTripInformation(tx, ds, p); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Println("[BERANGKAT SYNC] DONE booking_id:", ds.BookingID, "dep_id:", ds.ID)
	return nil
}

// ===============================
// ✅ NEW: upsert passengers PER SEAT
// - Seat diambil dari: payload seats -> departure_settings.seat_numbers -> booking_seats.seat_code
// - Nama penumpang diambil dari booking_passengers (urut) -> fallback dari payload / gabungan lama
// ===============================
func berangkatUpsertPassengersPerSeat(tx *sql.Tx, ds DepartureSetting, p BookingSyncPayload) error {
	if !hasTable(tx, "passengers") {
		log.Println("[BERANGKAT SYNC] skip passengers: table passengers tidak ada")
		return fmt.Errorf("table passengers tidak ditemukan")
	}

	// date & time
	dateStr := berangkatFirstNonEmpty(p.Date, ds.DepartureDate)
	depTime := berangkatFirstNonEmpty(p.Time, ds.DepartureTime)

	// pickup & dropoff
	pickup := berangkatFirstNonEmpty(p.PickupLocation, ds.PickupAddress)
	dropoff := berangkatFirstNonEmpty(p.DropoffLocation, p.To, ds.RouteTo)

	// service type
	serviceType := berangkatFirstNonEmpty(p.Category, ds.ServiceType)

	driverName := strings.TrimSpace(ds.DriverName)
	vehicleCode := strings.TrimSpace(ds.VehicleCode)

	total := p.TotalAmount

	// e-ticket marker (tetap kompatibel cara lama)
	eticket := strings.TrimSpace(p.ETicketPhoto)
	if eticket == "" && ds.BookingID > 0 {
		eticket = fmt.Sprintf("ETICKET_INVOICE_FROM_BOOKING:%d", ds.BookingID)
	}

	// Seat list: payload -> ds.seat_numbers -> booking_seats.seat_code
	seatCodes := collectSeatCodes(tx, ds, p, dateStr, depTime)
	if len(seatCodes) == 0 {
		// fallback terakhir: tetap insert 1 row seperti behavior lama, agar tidak “hilang fitur”
		passengerName := berangkatFirstNonEmpty(p.CustomerName, p.PassengerName)
		passengerPhone := berangkatFirstNonEmpty(p.CustomerPhone, p.PassengerPhone)
		if namesJoined, phoneFirst := loadBookingPassengers(tx, ds.BookingID); namesJoined != "" {
			passengerName = namesJoined
			if passengerPhone == "" {
				passengerPhone = phoneFirst
			}
		}
		return upsertPassengerRow(tx, ds.BookingID, passengerName, passengerPhone, dateStr, depTime, pickup, dropoff, total, "", serviceType, eticket, driverName, vehicleCode, "")
	}

	// Ambil nama per penumpang (urut) dari booking_passengers
	nameList, phoneFirst := loadBookingPassengerNameList(tx, ds.BookingID)

	// fallback nama/phone dari payload
	fallbackName := berangkatFirstNonEmpty(p.CustomerName, p.PassengerName)
	fallbackPhone := berangkatFirstNonEmpty(p.CustomerPhone, p.PassengerPhone)
	if fallbackPhone == "" {
		fallbackPhone = phoneFirst
	}

	// Optional: jika sebelumnya pernah ada 1-row gabungan untuk booking ini,
	// kita TIDAK menghapus fitur apa pun, tapi kita akan buat/update row per-seat.
	// Jika tabel passengers punya banyak row, UI akan menampilkan semuanya.
	for i, seat := range seatCodes {
		name := fallbackName
		if i < len(nameList) && strings.TrimSpace(nameList[i]) != "" {
			name = strings.TrimSpace(nameList[i])
		}
		phone := fallbackPhone

		if err := upsertPassengerRow(tx, ds.BookingID, name, phone, dateStr, depTime, pickup, dropoff, total, seat, serviceType, eticket, driverName, vehicleCode, ""); err != nil {
			return err
		}
	}

	log.Println("[BERANGKAT SYNC] passengers per-seat OK. booking_id:", ds.BookingID, "count:", len(seatCodes))
	return nil
}

// upsertPassengerRow akan INSERT/UPDATE berdasarkan:
// - jika kolom selected_seats ada: (booking_id + selected_seats) -> per seat
// - jika tidak: fallback pakai (booking_id) -> tetap kompatibel
func upsertPassengerRow(
	tx *sql.Tx,
	bookingID int64,
	passengerName string,
	passengerPhone string,
	dateStr string,
	depTime string,
	pickup string,
	dropoff string,
	total any,
	seat string,
	serviceType string,
	eticket string,
	driverName string,
	vehicleCode string,
	notes string,
) error {
	if bookingID <= 0 {
		return fmt.Errorf("booking_id kosong")
	}

	hasSelectedSeats := hasColumn(tx, "passengers", "selected_seats")
	hasBookingID := hasColumn(tx, "passengers", "booking_id")

	if !hasBookingID {
		return fmt.Errorf("kolom booking_id tidak ada di passengers")
	}

	// cek exist
	var existingID sql.NullInt64
	if hasSelectedSeats {
		_ = tx.QueryRow(`SELECT id FROM passengers WHERE booking_id=? AND selected_seats=? LIMIT 1`, bookingID, seat).Scan(&existingID)
	} else {
		_ = tx.QueryRow(`SELECT id FROM passengers WHERE booking_id=? LIMIT 1`, bookingID).Scan(&existingID)
	}

	if !existingID.Valid || existingID.Int64 <= 0 {
		// INSERT
		_, err := tx.Exec(`
			INSERT INTO passengers
				(passenger_name, passenger_phone, date, departure_time, pickup_address, dropoff_address,
				 total_amount, selected_seats, service_type, eticket_photo,
				 driver_name, vehicle_code, notes, booking_id)
			VALUES
				(?, ?, ?, ?, ?, ?,
				 ?, ?, ?, ?,
				 ?, ?, ?, ?)
		`,
			passengerName, passengerPhone, dateStr, depTime, pickup, dropoff,
			total, seat, serviceType, eticket,
			driverName, vehicleCode, notes, bookingID,
		)
		if err != nil {
			log.Println("[BERANGKAT SYNC] insert passengers fail booking:", bookingID, "seat:", seat, "err:", err)
			return fmt.Errorf("insert passengers failed: %w", err)
		}
		return nil
	}

	// UPDATE
	if hasSelectedSeats {
		_, err := tx.Exec(`
			UPDATE passengers
			SET passenger_name=?,
				passenger_phone=?,
				date=?,
				departure_time=?,
				pickup_address=?,
				dropoff_address=?,
				total_amount=?,
				service_type=?,
				eticket_photo=?,
				driver_name=?,
				vehicle_code=?,
				notes=?
			WHERE booking_id=? AND selected_seats=?
		`,
			passengerName, passengerPhone, dateStr, depTime, pickup, dropoff,
			total, serviceType, eticket,
			driverName, vehicleCode, notes,
			bookingID, seat,
		)
		if err != nil {
			log.Println("[BERANGKAT SYNC] update passengers fail booking:", bookingID, "seat:", seat, "err:", err)
			return fmt.Errorf("update passengers failed: %w", err)
		}
		return nil
	}

	// fallback legacy update by booking_id only
	_, err := tx.Exec(`
		UPDATE passengers
		SET passenger_name=?,
			passenger_phone=?,
			date=?,
			departure_time=?,
			pickup_address=?,
			dropoff_address=?,
			total_amount=?,
			selected_seats=?,
			service_type=?,
			eticket_photo=?,
			driver_name=?,
			vehicle_code=?,
			notes=?
		WHERE booking_id=?
	`,
		passengerName, passengerPhone, dateStr, depTime, pickup, dropoff,
		total, seat, serviceType, eticket,
		driverName, vehicleCode, notes,
		bookingID,
	)
	if err != nil {
		log.Println("[BERANGKAT SYNC] update passengers legacy fail booking:", bookingID, "err:", err)
		return fmt.Errorf("update passengers failed: %w", err)
	}
	return nil
}

// collectSeatCodes: ambil seat dengan urutan prioritas:
// 1) payload SelectedSeats
// 2) departure_settings.seat_numbers (string)
// 3) booking_seats.seat_code (paling akurat)
func collectSeatCodes(tx *sql.Tx, ds DepartureSetting, p BookingSyncPayload, depDate string, depTime string) []string {
	seen := map[string]bool{}
	out := []string{}

	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}

	// 1) payload seats
	for _, s := range p.SelectedSeats {
		add(s)
	}

	// 2) seat_numbers string
	if strings.TrimSpace(ds.SeatNumbers) != "" {
		parts := strings.FieldsFunc(ds.SeatNumbers, func(r rune) bool {
			return r == ',' || r == ';' || r == '|' || r == '\n' || r == '\t'
		})
		for _, s := range parts {
			add(s)
		}
	}

	// 3) booking_seats.seat_code (✅ sesuai tabel kamu)
	if len(out) == 0 && ds.BookingID > 0 {
		for _, s := range loadBookingSeatCodes(tx, ds.BookingID, depDate, depTime) {
			add(s)
		}
	}

	return out
}

// loadBookingSeatCodes: ambil seat_code dari tabel booking_seats.
// Struktur tabel kamu: id, booking_id, route_from, route_to, trip_date, trip_time, seat_code, created_at
func loadBookingSeatCodes(tx *sql.Tx, bookingID int64, depDate string, depTime string) []string {
	if bookingID <= 0 {
		return nil
	}
	if !hasTable(tx, "booking_seats") {
		return nil
	}
	if !hasColumn(tx, "booking_seats", "booking_id") || !hasColumn(tx, "booking_seats", "seat_code") {
		return nil
	}

	dateOnly := normalizeDateOnly(depDate)
	timeHM := normalizeTimeHM(depTime)

	hasTripDate := hasColumn(tx, "booking_seats", "trip_date")
	hasTripTime := hasColumn(tx, "booking_seats", "trip_time")

	q := `SELECT seat_code FROM booking_seats WHERE booking_id=?`
	args := []any{bookingID}

	if hasTripDate && dateOnly != "" {
		q += ` AND (trip_date=? OR DATE(trip_date)=?)`
		args = append(args, depDate, dateOnly)
	}
	if hasTripTime && timeHM != "" {
		q += ` AND (trip_time=? OR LEFT(trip_time,5)=?)`
		args = append(args, depTime, timeHM)
	}

	q += ` ORDER BY id ASC`

	rows, err := tx.Query(q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	seen := map[string]bool{}
	out := []string{}
	for rows.Next() {
		var v sql.NullString
		if err := rows.Scan(&v); err != nil {
			continue
		}
		s := strings.TrimSpace(v.String)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func normalizeTimeHM(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if len(v) >= 5 {
		return v[:5]
	}
	return v
}

func berangkatUpsertTripInformation(tx *sql.Tx, ds DepartureSetting, p BookingSyncPayload) error {
	if !hasTable(tx, "trip_information") {
		return fmt.Errorf("table trip_information tidak ditemukan")
	}

	// trip_number harus stabil supaya upsert bisa jalan.
	// Pakai ds.TripNumber kalau ada; fallback: "TRIP-BOOKING-<id>"
	tripNumber := strings.TrimSpace(ds.TripNumber)
	if tripNumber == "" {
		tripNumber = fmt.Sprintf("TRIP-BOOKING-%d", ds.BookingID)
	}

	depDate := berangkatFirstNonEmpty(p.Date, ds.DepartureDate)
	depTime := berangkatFirstNonEmpty(p.Time, ds.DepartureTime)

	driverName := strings.TrimSpace(ds.DriverName)
	vehicleCode := strings.TrimSpace(ds.VehicleCode)

	// trip details dari booking (fallback ke route di departure_settings, terakhir trip number)
	tripDetails := ""
	if strings.TrimSpace(p.From) != "" || strings.TrimSpace(p.To) != "" {
		tripDetails = strings.TrimSpace(strings.TrimSpace(p.From) + " - " + strings.TrimSpace(p.To))
	} else if strings.TrimSpace(ds.RouteFrom) != "" || strings.TrimSpace(ds.RouteTo) != "" {
		tripDetails = strings.TrimSpace(strings.TrimSpace(ds.RouteFrom) + " - " + strings.TrimSpace(ds.RouteTo))
	} else {
		tripDetails = tripNumber
	}

	// surat jalan (biasanya URL)
	suratJalan := strings.TrimSpace(ds.SuratJalanFile)
	if suratJalan == "" && ds.BookingID > 0 {
		suratJalan = buildSuratJalanAPI(ds.BookingID)
	}

	hasTripNumber := hasColumn(tx, "trip_information", "trip_number")
	if !hasTripNumber {
		// tanpa trip_number tidak bisa upsert, skip tanpa mematikan flow penumpang
		log.Println("[BERANGKAT SYNC] skip trip_information: kolom trip_number tidak ada")
		return nil
	}

	// cek kolom lain yang ada
	hasTripDetails := hasColumn(tx, "trip_information", "trip_details")
	hasDepartureDate := hasColumn(tx, "trip_information", "departure_date")
	hasDepartureTime := hasColumn(tx, "trip_information", "departure_time")
	hasDriver := hasColumn(tx, "trip_information", "driver_name")
	hasVehicle := hasColumn(tx, "trip_information", "vehicle_code")
	hasSurat := hasColumn(tx, "trip_information", "e_surat_jalan")
	hasCreated := hasColumn(tx, "trip_information", "created_at")
	hasUpdated := hasColumn(tx, "trip_information", "updated_at")
	hasBookingID := hasColumn(tx, "trip_information", "booking_id")

	// Upsert by trip_number
	exists, err := berangkatExistsByTripNumber(tx, "trip_information", tripNumber)
	if err != nil {
		return err
	}

	now := time.Now().Format("2006-01-02 15:04:05")

	if !exists {
		cols := []string{"trip_number"}
		vals := []any{tripNumber}
		ph := []string{"?"}

		if hasTripDetails {
			cols = append(cols, "trip_details")
			vals = append(vals, tripDetails)
			ph = append(ph, "?")
		}
		if hasDepartureDate {
			cols = append(cols, "departure_date")
			vals = append(vals, depDate)
			ph = append(ph, "?")
		}
		if hasDepartureTime {
			cols = append(cols, "departure_time")
			vals = append(vals, depTime)
			ph = append(ph, "?")
		}
		if hasDriver {
			cols = append(cols, "driver_name")
			vals = append(vals, driverName)
			ph = append(ph, "?")
		}
		if hasVehicle {
			cols = append(cols, "vehicle_code")
			vals = append(vals, vehicleCode)
			ph = append(ph, "?")
		}
		if hasSurat {
			cols = append(cols, "e_surat_jalan")
			vals = append(vals, suratJalan)
			ph = append(ph, "?")
		}
		if hasCreated {
			cols = append(cols, "created_at")
			vals = append(vals, now)
			ph = append(ph, "?")
		}
		if hasUpdated {
			cols = append(cols, "updated_at")
			vals = append(vals, now)
			ph = append(ph, "?")
		}
		if hasBookingID {
			cols = append(cols, "booking_id")
			vals = append(vals, ds.BookingID)
			ph = append(ph, "?")
		}

		_, err := tx.Exec(`INSERT INTO trip_information (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(ph, ",")+`)`, vals...)
		if err != nil {
			log.Println("[BERANGKAT SYNC] insert trip_information fail trip:", tripNumber, "err:", err)
			return fmt.Errorf("insert trip_information failed: %w", err)
		}
		log.Println("[BERANGKAT SYNC] insert trip_information OK trip:", tripNumber)
		return nil
	}

	setParts := []string{}
	args := []any{}

	if hasTripDetails {
		setParts = append(setParts, "trip_details=?")
		args = append(args, tripDetails)
	}
	if hasDepartureDate {
		setParts = append(setParts, "departure_date=?")
		args = append(args, depDate)
	}
	if hasDepartureTime {
		setParts = append(setParts, "departure_time=?")
		args = append(args, depTime)
	}
	if hasDriver {
		setParts = append(setParts, "driver_name=?")
		args = append(args, driverName)
	}
	if hasVehicle {
		setParts = append(setParts, "vehicle_code=?")
		args = append(args, vehicleCode)
	}
	if hasSurat {
		setParts = append(setParts, "e_surat_jalan=?")
		args = append(args, suratJalan)
	}
	if hasBookingID {
		setParts = append(setParts, "booking_id=?")
		args = append(args, ds.BookingID)
	}
	if hasUpdated && len(setParts) > 0 {
		setParts = append(setParts, "updated_at=?")
		args = append(args, now)
	}

	if len(setParts) == 0 {
		return nil
	}

	args = append(args, tripNumber)

	_, err = tx.Exec(`UPDATE trip_information SET `+strings.Join(setParts, ", ")+` WHERE trip_number=?`, args...)
	if err != nil {
		log.Println("[BERANGKAT SYNC] update trip_information fail trip:", tripNumber, "err:", err)
		return fmt.Errorf("update trip_information failed: %w", err)
	}
	log.Println("[BERANGKAT SYNC] update trip_information OK trip:", tripNumber)
	return nil
}

func berangkatFirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func berangkatExistsByTripNumber(tx *sql.Tx, table string, tripNumber string) (bool, error) {
	tripNumber = strings.TrimSpace(tripNumber)
	if tripNumber == "" {
		return false, nil
	}
	var one int
	err := tx.QueryRow(`SELECT 1 FROM `+table+` WHERE trip_number=? LIMIT 1`, tripNumber).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func lookupBookingID(depID int) int64 {
	if depID <= 0 {
		return 0
	}
	if !hasTable(config.DB, "departure_settings") {
		return 0
	}
	// kolom yang kamu punya: booking_id
	if !hasColumn(config.DB, "departure_settings", "booking_id") {
		return 0
	}
	var bid sql.NullInt64
	err := config.DB.QueryRow(`SELECT booking_id FROM departure_settings WHERE id=? LIMIT 1`, depID).Scan(&bid)
	if err != nil {
		return 0
	}
	if bid.Valid && bid.Int64 > 0 {
		return bid.Int64
	}
	return 0
}

func lookupBookingIDFromTripNumber(tripNumber string) int64 {
	tripNumber = strings.TrimSpace(tripNumber)
	if tripNumber == "" {
		return 0
	}
	if !hasTable(config.DB, "departure_settings") {
		return 0
	}
	if !hasColumn(config.DB, "departure_settings", "trip_number") || !hasColumn(config.DB, "departure_settings", "booking_id") {
		return 0
	}
	var bid sql.NullInt64
	err := config.DB.QueryRow(`SELECT booking_id FROM departure_settings WHERE trip_number=? LIMIT 1`, tripNumber).Scan(&bid)
	if err != nil {
		return 0
	}
	if bid.Valid && bid.Int64 > 0 {
		return bid.Int64
	}
	return 0
}

func bookingExists(id int64) bool {
	if id <= 0 {
		return false
	}
	// cek bookings dulu, lalu reguler_bookings
	table := ""
	if hasTable(config.DB, "bookings") {
		table = "bookings"
	} else if hasTable(config.DB, "reguler_bookings") {
		table = "reguler_bookings"
	}
	if table == "" {
		return false
	}
	var one int
	err := config.DB.QueryRow(`SELECT 1 FROM `+table+` WHERE id=? LIMIT 1`, id).Scan(&one)
	return err == nil
}

// loadBookingPassengers (lama) tetap dipertahankan untuk kompatibilitas
func loadBookingPassengers(tx *sql.Tx, bookingID int64) (string, string) {
	if bookingID <= 0 {
		return "", ""
	}
	tables := []string{}
	if hasTable(tx, "booking_passengers") {
		tables = append(tables, "booking_passengers")
	}
	if hasTable(tx, "booking_passengers_reguler") {
		tables = append(tables, "booking_passengers_reguler")
	}
	if len(tables) == 0 {
		return "", ""
	}

	names := []string{}
	phoneFirst := ""

	for _, table := range tables {
		hasBookingID := hasColumn(tx, table, "booking_id")
		hasName := hasColumn(tx, table, "passenger_name") || hasColumn(tx, table, "name")
		if !hasBookingID || !hasName {
			continue
		}

		nameCol := "passenger_name"
		if !hasColumn(tx, table, "passenger_name") && hasColumn(tx, table, "name") {
			nameCol = "name"
		}

		hasPhone := hasColumn(tx, table, "passenger_phone") || hasColumn(tx, table, "phone")
		phoneCol := "passenger_phone"
		if !hasColumn(tx, table, "passenger_phone") && hasColumn(tx, table, "phone") {
			phoneCol = "phone"
		}

		q := `SELECT ` + nameCol
		if hasPhone {
			q += `, ` + phoneCol
		}
		q += ` FROM ` + table + ` WHERE booking_id=? ORDER BY id ASC`

		rows, err := tx.Query(q, bookingID)
		if err != nil {
			continue
		}
		defer rows.Close()

		for rows.Next() {
			var name sql.NullString
			var phone sql.NullString

			if hasPhone {
				if err := rows.Scan(&name, &phone); err != nil {
					continue
				}
			} else {
				if err := rows.Scan(&name); err != nil {
					continue
				}
			}

			nm := strings.TrimSpace(name.String)
			if nm != "" {
				names = append(names, nm)
			}
			if phoneFirst == "" && hasPhone {
				ph := strings.TrimSpace(phone.String)
				if ph != "" {
					phoneFirst = ph
				}
			}
		}
	}

	return strings.Join(names, ", "), phoneFirst
}

// ✅ NEW: ambil list nama (urut) untuk mapping seat->nama
func loadBookingPassengerNameList(tx *sql.Tx, bookingID int64) ([]string, string) {
	if bookingID <= 0 {
		return nil, ""
	}
	tables := []string{}
	if hasTable(tx, "booking_passengers") {
		tables = append(tables, "booking_passengers")
	}
	if hasTable(tx, "booking_passengers_reguler") {
		tables = append(tables, "booking_passengers_reguler")
	}
	if len(tables) == 0 {
		return nil, ""
	}

	out := []string{}
	phoneFirst := ""

	for _, table := range tables {
		hasBookingID := hasColumn(tx, table, "booking_id")
		hasName := hasColumn(tx, table, "passenger_name") || hasColumn(tx, table, "name")
		if !hasBookingID || !hasName {
			continue
		}

		nameCol := "passenger_name"
		if !hasColumn(tx, table, "passenger_name") && hasColumn(tx, table, "name") {
			nameCol = "name"
		}

		hasPhone := hasColumn(tx, table, "passenger_phone") || hasColumn(tx, table, "phone")
		phoneCol := "passenger_phone"
		if !hasColumn(tx, table, "passenger_phone") && hasColumn(tx, table, "phone") {
			phoneCol = "phone"
		}

		q := `SELECT ` + nameCol
		if hasPhone {
			q += `, ` + phoneCol
		}
		q += ` FROM ` + table + ` WHERE booking_id=? ORDER BY id ASC`

		rows, err := tx.Query(q, bookingID)
		if err != nil {
			continue
		}
		defer rows.Close()

		for rows.Next() {
			var name sql.NullString
			var phone sql.NullString

			if hasPhone {
				if err := rows.Scan(&name, &phone); err != nil {
					continue
				}
			} else {
				if err := rows.Scan(&name); err != nil {
					continue
				}
			}

			nm := strings.TrimSpace(name.String)
			if nm != "" {
				out = append(out, nm)
			}
			if phoneFirst == "" && hasPhone {
				ph := strings.TrimSpace(phone.String)
				if ph != "" {
					phoneFirst = ph
				}
			}
		}

		if len(out) > 0 {
			// sudah dapat dari 1 table, cukup
			break
		}
	}

	return out, phoneFirst
}

// ========== booking id fallback ==========

func lookupBookingIDFromPaymentValidations(phone string, pickup string, depDate string) int64 {
	phone = strings.TrimSpace(phone)
	pickup = strings.TrimSpace(pickup)
	depDate = strings.TrimSpace(depDate)
	if phone == "" || pickup == "" || depDate == "" {
		return 0
	}
	if !hasTable(config.DB, "payment_validations") {
		return 0
	}
	if !hasColumn(config.DB, "payment_validations", "booking_id") {
		return 0
	}

	dateOnly := depDate
	if len(dateOnly) > 10 {
		dateOnly = dateOnly[:10]
	}

	var bid sql.NullInt64
	err := config.DB.QueryRow(`
		SELECT booking_id
		FROM payment_validations
		WHERE phone=? AND pickup_address=? AND (date=? OR DATE(date)=?)
		ORDER BY id DESC
		LIMIT 1
	`, phone, pickup, depDate, dateOnly).Scan(&bid)
	if err != nil {
		return 0
	}
	if bid.Valid && bid.Int64 > 0 {
		return bid.Int64
	}
	return 0
}

func lookupBookingIDFromBookings(phone string, pickup string, depDate string) int64 {
	phone = strings.TrimSpace(phone)
	pickup = strings.TrimSpace(pickup)
	depDate = strings.TrimSpace(depDate)
	if phone == "" || pickup == "" || depDate == "" {
		return 0
	}

	table := ""
	if hasTable(config.DB, "bookings") {
		table = "bookings"
	} else if hasTable(config.DB, "reguler_bookings") {
		table = "reguler_bookings"
	}
	if table == "" {
		return 0
	}

	phoneCol := "phone"
	if !hasColumn(config.DB, table, "phone") && hasColumn(config.DB, table, "customer_phone") {
		phoneCol = "customer_phone"
	}
	pickCol := "pickup_address"
	if !hasColumn(config.DB, table, "pickup_address") && hasColumn(config.DB, table, "pickup_location") {
		pickCol = "pickup_location"
	}
	dateCol := "departure_date"
	if !hasColumn(config.DB, table, "departure_date") && hasColumn(config.DB, table, "date") {
		dateCol = "date"
	}

	dateOnly := depDate
	if len(dateOnly) > 10 {
		dateOnly = dateOnly[:10]
	}

	var id sql.NullInt64
	q := fmt.Sprintf(`
		SELECT id FROM %s
		WHERE %s=? AND %s=? AND (%s=? OR DATE(%s)=?)
		ORDER BY id DESC
		LIMIT 1
	`, table, phoneCol, pickCol, dateCol, dateCol)

	err := config.DB.QueryRow(q, phone, pickup, depDate, dateOnly).Scan(&id)
	if err != nil {
		return 0
	}
	if id.Valid && id.Int64 > 0 {
		return id.Int64
	}
	return 0
}
