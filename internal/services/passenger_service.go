package services

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
	"backend/internal/domain/models"
	"backend/internal/repositories"
	"backend/internal/utils"
)

// PassengerService memastikan data passenger_seats selalu sinkron per-seat dari departure/return/booking.
type PassengerService struct {
	PassengerRepo   repositories.PassengerRepository
	BookingRepo     repositories.BookingRepository
	BookingSeatRepo repositories.BookingSeatRepository
	DB              *sql.DB
	RequestID       string
	FetchBooking    func(int64) (repositories.Booking, []repositories.BookingSeat, error)
}

func (s PassengerService) db() *sql.DB {
	if s.DB != nil {
		return s.DB
	}
	return intconfig.DB
}

// SyncFromDeparture membuat/memperbarui penumpang per seat berdasarkan departure_settings + booking.
func (s PassengerService) SyncFromDeparture(dep models.DepartureSetting) error {
	utils.LogEvent(s.RequestID, "passenger", "sync_departure", "booking_id="+strconv.FormatInt(dep.BookingID, 10))
	return s.syncFromTrip(dep, "berangkat")
}

// SyncFromReturn membuat/memperbarui penumpang per seat berdasarkan return_settings + booking.
func (s PassengerService) SyncFromReturn(ret models.DepartureSetting) error {
	utils.LogEvent(s.RequestID, "passenger", "sync_return", "booking_id="+strconv.FormatInt(ret.BookingID, 10))
	return s.syncFromTrip(ret, "pulang")
}

// SyncFromBooking dipakai setelah SaveBookingPassengers agar invoice/e-ticket bisa dibuat sebelum Lunas.
func (s PassengerService) SyncFromBooking(bookingID int64) error {
	if bookingID <= 0 {
		return fmt.Errorf("booking_id tidak valid")
	}

	db := s.db()
	if db == nil {
		return fmt.Errorf("db tidak tersedia untuk sinkronisasi passenger_seats")
	}

	if !intdb.HasTable(db, "passenger_seats") {
		return fmt.Errorf("tabel passenger_seats belum tersedia, jalankan migrasi passenger_seats terlebih dahulu")
	}
	if !intdb.HasColumn(db, "passenger_seats", "booking_id") {
		return fmt.Errorf("schema passenger_seats belum siap: kolom booking_id belum ada")
	}

	booking, seats, err := s.loadBookingAndSeats(bookingID)
	if err != nil {
		return err
	}

	// ambil seatCodes dari booking_seats
	seatCodes := []string{}
	seen := map[string]bool{}
	for _, seat := range seats {
		code := strings.ToUpper(strings.TrimSpace(seat.SeatCode))
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		seatCodes = append(seatCodes, code)
	}

	// fallback seat list dari booking_passengers jika booking_seats belum ada
	inputs := s.loadPassengerInputs(bookingID)
	if len(seatCodes) == 0 {
		for code := range inputs.bySeat {
			upper := strings.ToUpper(strings.TrimSpace(code))
			if upper == "" || upper == "ALL" || seen[upper] {
				continue
			}
			seen[upper] = true
			seatCodes = append(seatCodes, upper)
		}
	}

	if len(seatCodes) == 0 {
		seatCodes = []string{"ALL"}
	}

	// ✅ bookingName fallback: kalau BookingFor/PassengerName kosong atau "self", ambil dari booking_passengers seat pertama
	displayName := firstNonEmptyStr(booking.BookingFor, booking.PassengerName)
	if strings.EqualFold(strings.TrimSpace(displayName), "self") || strings.TrimSpace(displayName) == "" {
		if nm := inputs.firstName(); nm != "" {
			displayName = nm
		}
	}

	dep := models.DepartureSetting{
		BookingID:      bookingID,
		BookingName:    displayName,
		Phone:          booking.PassengerPhone, // hanya untuk display, bukan fallback passenger seat
		PickupAddress:  booking.PickupLocation,
		DepartureDate:  booking.TripDate,
		DepartureTime:  booking.TripTime,
		ServiceType:    booking.Category,
		RouteFrom:      booking.RouteFrom,
		RouteTo:        booking.RouteTo,
		SeatNumbers:    strings.Join(seatCodes, ","),
		DriverName:     "",
		VehicleCode:    "",
		DepartureStatus: "",
	}

	return s.syncFromTrip(dep, "berangkat")
}

func (s PassengerService) syncFromTrip(dep models.DepartureSetting, tripRole string) error {
	if dep.BookingID <= 0 {
		return fmt.Errorf("booking_id kosong pada setting id %d", dep.ID)
	}

	db := s.db()
	if db == nil {
		return fmt.Errorf("db tidak tersedia untuk sinkronisasi passenger_seats")
	}
	if !intdb.HasTable(db, "passenger_seats") {
		return fmt.Errorf("tabel passenger_seats belum tersedia")
	}

	booking, seats, err := s.loadBookingAndSeats(dep.BookingID)
	if err != nil {
		return err
	}

	// seat meta dari booking_seats
	seatByCode := map[string]repositories.BookingSeat{}
	seatCodes := []string{}
	for _, seat := range seats {
		code := strings.ToUpper(strings.TrimSpace(seat.SeatCode))
		if code == "" {
			continue
		}
		if _, ok := seatByCode[code]; !ok {
			seatByCode[code] = seat
			seatCodes = append(seatCodes, code)
		}
	}

	// fallback seat list dari dep.SeatNumbers
	if len(seatCodes) == 0 && strings.TrimSpace(dep.SeatNumbers) != "" {
		for _, ss := range splitSeats(dep.SeatNumbers) {
			code := strings.ToUpper(strings.TrimSpace(ss))
			if code != "" && !contains(seatCodes, code) {
				seatCodes = append(seatCodes, code)
			}
		}
	}
	if len(seatCodes) == 0 {
		seatCodes = []string{"ALL"}
	}
	seatCount := len(seatCodes)

	// input penumpang dari booking_passengers
	inputs := s.loadPassengerInputs(dep.BookingID)

	// ✅ baseName: kalau dep.BookingName masih "self"/kosong, pakai passenger pertama dari booking_passengers
	baseName := firstNonEmptyStr(booking.BookingFor, booking.PassengerName, dep.BookingName)
	if strings.EqualFold(strings.TrimSpace(baseName), "self") || strings.TrimSpace(baseName) == "" {
		if nm := inputs.firstName(); nm != "" {
			baseName = nm
		}
	}

	// ✅ JANGAN fallback phone dari booking/dep ke passenger seat
	basePhone := ""

	// lokasi pickup/dropoff: pakai booking dulu, lalu dep
	pickup := firstNonEmptyStr(booking.PickupLocation, dep.PickupAddress)

	// untuk dropoff address, lebih aman pakai booking.dropoff_location dulu (kalau ada)
	dropoff := firstNonEmptyStr(booking.DropoffLocation, dep.RouteTo)

	serviceType := firstNonEmptyStr(booking.Category, dep.ServiceType)

	// rute untuk tarif: prioritas dari dep (return/departure settings), baru booking
	routeFrom := firstNonEmptyStr(dep.RouteFrom, booking.RouteFrom)
	routeTo := firstNonEmptyStr(dep.RouteTo, booking.RouteTo)

	// base price
	pricePerSeat := booking.PricePerSeat
	if pricePerSeat == 0 {
		if booking.Total > 0 && seatCount > 0 {
			pricePerSeat = booking.Total / int64(seatCount)
		} else {
			pricePerSeat = booking.Total
		}
	}

	eticket := fmt.Sprintf("ETICKET_INVOICE_FROM_BOOKING:%d", dep.BookingID)

	for _, code := range seatCodes {
		seatMeta := seatByCode[code]
		date := firstNonEmptyStr(seatMeta.TripDate, booking.TripDate, dep.DepartureDate)
		tm := firstNonEmptyStr(seatMeta.TripTime, booking.TripTime, dep.DepartureTime)

		name := baseName
		phone := basePhone

		// Ambil dari booking_passengers per seat (utama)
		if p, ok := inputs.bySeat[code]; ok {
			if strings.TrimSpace(p.name) != "" {
				name = p.name
			}
			if strings.TrimSpace(p.phone) != "" {
				phone = p.phone
			}
		} else if p, ok := inputs.bySeat["ALL"]; ok {
			if strings.TrimSpace(p.name) != "" {
				name = p.name
			}
			if strings.TrimSpace(p.phone) != "" {
				phone = p.phone
			}
		}

		// ✅ TotalAmount per seat ikut fare rules berdasar rute yang benar
		amount := utils.ComputeFare(routeFrom, routeTo, pricePerSeat)

		payload := repositories.PassengerSeatData{
			PassengerName:  name,
			PassengerPhone: phone,
			Date:           date,
			DepartureTime:  tm,
			PickupAddress:  pickup,
			DropoffAddress: dropoff,
			TotalAmount:    amount,
			SelectedSeat:   code,
			ServiceType:    serviceType,
			ETicketPhoto:   eticket,
			DriverName:     strings.TrimSpace(dep.DriverName),
			VehicleCode:    strings.TrimSpace(dep.VehicleCode),
			Notes:          "",
			TripRole:       tripRole,
		}

		if err := s.upsertPassengerSeat(db, dep.BookingID, payload); err != nil {
			log.Println("[PASSENGER SYNC] upsert passenger_seats gagal:", err)
			return err
		}
	}

	return nil
}

type passengerInput struct {
	name  string
	phone string
}

type passengerInputs struct {
	bySeat map[string]passengerInput
	order  []string // urutan seat berdasarkan id ASC
}

func (p passengerInputs) firstName() string {
	for _, seat := range p.order {
		if v, ok := p.bySeat[seat]; ok {
			if strings.TrimSpace(v.name) != "" {
				return strings.TrimSpace(v.name)
			}
		}
	}
	// fallback lain
	if v, ok := p.bySeat["ALL"]; ok {
		return strings.TrimSpace(v.name)
	}
	return ""
}

func (s PassengerService) loadPassengerInputs(bookingID int64) passengerInputs {
	out := passengerInputs{
		bySeat: map[string]passengerInput{},
		order:  []string{},
	}

	db := s.db()
	if db == nil || !intdb.HasTable(db, "booking_passengers") {
		return out
	}
	if !intdb.HasColumn(db, "booking_passengers", "booking_id") {
		return out
	}

	// ✅ ORDER BY id ASC biar seat pertama konsisten jadi "pemesan"
	rows, err := db.Query(`
		SELECT COALESCE(seat_code,''), COALESCE(passenger_name,''), COALESCE(passenger_phone,'')
		FROM booking_passengers
		WHERE booking_id=?
		ORDER BY id ASC`, bookingID)
	if err != nil {
		return out
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var seat, name, phone string
		if err := rows.Scan(&seat, &name, &phone); err == nil {
			seat = strings.ToUpper(strings.TrimSpace(seat))
			if seat == "" {
				seat = "ALL"
			}
			out.bySeat[seat] = passengerInput{
				name:  strings.TrimSpace(name),
				phone: strings.TrimSpace(phone),
			}
			if !seen[seat] {
				seen[seat] = true
				out.order = append(out.order, seat)
			}
		}
	}
	return out
}

// ✅ UPSERT dinamis ke passenger_seats (deteksi kolom otomatis)
func (s PassengerService) upsertPassengerSeat(db *sql.DB, bookingID int64, p repositories.PassengerSeatData) error {
	if db == nil {
		return fmt.Errorf("db nil")
	}
	if !intdb.HasTable(db, "passenger_seats") {
		return fmt.Errorf("tabel passenger_seats belum tersedia")
	}

	seatCol := ""
	switch {
	case intdb.HasColumn(db, "passenger_seats", "seat_code"):
		seatCol = "seat_code"
	case intdb.HasColumn(db, "passenger_seats", "selected_seats"):
		seatCol = "selected_seats"
	default:
		return fmt.Errorf("schema passenger_seats belum siap: kolom seat_code/selected_seats tidak ditemukan")
	}

	cols := make([]string, 0, 20)
	vals := make([]any, 0, 20)
	updates := make([]string, 0, 20)

	add := func(col string, v any, allowUpdate bool) {
		if !intdb.HasColumn(db, "passenger_seats", col) {
			return
		}
		cols = append(cols, col)
		vals = append(vals, v)
		if allowUpdate {
			updates = append(updates, fmt.Sprintf("%s = VALUES(%s)", col, col))
		}
	}

	normalizeDate := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}
		if i := strings.Index(s, "T"); i > 0 {
			return s[:i]
		}
		return s
	}

	seat := strings.ToUpper(strings.TrimSpace(p.SelectedSeat))
	if seat == "" {
		seat = "ALL"
	}

	add("booking_id", bookingID, false)
	add(seatCol, seat, false)

	add("passenger_name", strings.TrimSpace(p.PassengerName), true)
	add("passenger_phone", strings.TrimSpace(p.PassengerPhone), true)

	nd := normalizeDate(p.Date)
	add("trip_date", nd, true)
	add("date", nd, true)

	add("trip_time", strings.TrimSpace(p.DepartureTime), true)
	add("departure_time", strings.TrimSpace(p.DepartureTime), true)

	add("pickup_location", strings.TrimSpace(p.PickupAddress), true)
	add("pickup_address", strings.TrimSpace(p.PickupAddress), true)

	add("dropoff_location", strings.TrimSpace(p.DropoffAddress), true)
	add("dropoff_address", strings.TrimSpace(p.DropoffAddress), true)

	add("paid_price", p.TotalAmount, true)
	add("total_amount", p.TotalAmount, true)

	add("service_type", strings.TrimSpace(p.ServiceType), true)
	add("driver_name", strings.TrimSpace(p.DriverName), true)
	add("vehicle_code", strings.TrimSpace(p.VehicleCode), true)
	add("notes", strings.TrimSpace(p.Notes), true)
	add("trip_role", strings.TrimSpace(p.TripRole), true)
	add("eticket_photo", strings.TrimSpace(p.ETicketPhoto), true)

	if len(cols) < 2 {
		return fmt.Errorf("kolom insert passenger_seats tidak cukup (booking_id dan seat)")
	}

	placeholders := make([]string, len(cols))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	onDup := ""
	if len(updates) > 0 {
		if intdb.HasColumn(db, "passenger_seats", "id") {
			onDup = " ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id), " + strings.Join(updates, ", ")
		} else {
			onDup = " ON DUPLICATE KEY UPDATE " + strings.Join(updates, ", ")
		}
	}

	q := fmt.Sprintf(
		"INSERT INTO passenger_seats (%s) VALUES (%s)%s",
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
		onDup,
	)

	_, err := db.Exec(q, vals...)
	return err
}

func splitSeats(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == '\n' || r == '\t'
	})
	out := []string{}
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func contains(arr []string, val string) bool {
	for _, v := range arr {
		if v == val {
			return true
		}
	}
	return false
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (s PassengerService) loadBookingAndSeats(bookingID int64) (repositories.Booking, []repositories.BookingSeat, error) {
	if s.FetchBooking != nil {
		return s.FetchBooking(bookingID)
	}
	booking, err := s.BookingRepo.GetByID(bookingID)
	if err != nil {
		return booking, nil, err
	}
	seats, _ := s.BookingSeatRepo.GetSeats(bookingID)
	return booking, seats, nil
}
