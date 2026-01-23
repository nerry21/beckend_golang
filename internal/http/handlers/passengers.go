package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"

	"github.com/gin-gonic/gin"
)

// SeatsString menyimpan seat dalam bentuk string, tapi bisa menerima JSON array ataupun string.
type SeatsString string

// UnmarshalJSON: bisa terima "1A, 2B" ATAU ["1A","2B"]
func (s *SeatsString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*s = ""
		return nil
	}
	if len(data) > 0 && data[0] == '[' {
		var arr []string
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		out := make([]string, 0, len(arr))
		for _, it := range arr {
			it = strings.TrimSpace(it)
			if it != "" {
				out = append(out, it)
			}
		}
		*s = SeatsString(strings.Join(out, ", "))
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*s = SeatsString(strings.TrimSpace(str))
	return nil
}

type Passenger struct {
	ID             int         `json:"id"`
	PassengerName  string      `json:"passengerName"`
	PassengerPhone string      `json:"passengerPhone"`
	Date           string      `json:"date"`          // bisa date/trip_date
	DepartureTime  string      `json:"departureTime"` // bisa departure_time/trip_time
	PickupAddress  string      `json:"pickupAddress"` // bisa pickup_address/pickup_location
	DropoffAddress string      `json:"dropoffAddress"`
	TotalAmount    int64       `json:"totalAmount"` // bisa total_amount/paid_price
	SelectedSeats  SeatsString `json:"selectedSeats"` // bisa selected_seats/seat_code
	ServiceType    string      `json:"serviceType"`
	ETicketPhoto   string      `json:"eTicketPhoto"`
	DriverName     string      `json:"driverName"`
	VehicleCode    string      `json:"vehicleCode"`
	VehicleType    string      `json:"vehicleType,omitempty"`
	BookingID      int64       `json:"bookingId,omitempty"`
	RouteFrom      string      `json:"routeFrom,omitempty"`
	RouteTo        string      `json:"routeTo,omitempty"`
	PaymentStatus  string      `json:"paymentStatus,omitempty"`
	Notes          string      `json:"notes"`
	CreatedAt      string      `json:"createdAt"`
}

// pilih tabel penumpang yang aktif (baru dulu)
func passengerTableName() string {
	db := intconfig.DB
	if db == nil {
		return "passengers"
	}
	if intdb.HasTable(db, "passenger_seats") {
		return "passenger_seats"
	}
	return "passengers"
}

// helper: kirim NULL ke DB kalau string kosong (khusus sql.NullString)
func nullStringIfEmpty(s string) sql.NullString {
	s = strings.TrimSpace(s)
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

func normalizeDateOnly(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func normalizeTripTime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) >= 5 {
		return s[:5]
	}
	return s
}

// GET /api/passengers
func GetPassengers(c *gin.Context) {
	db := intconfig.DB
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db tidak tersedia"})
		return
	}

	tbl := passengerTableName()

	bookingParam := strings.TrimSpace(c.Query("bookingId"))
	if bookingParam == "" {
		bookingParam = strings.TrimSpace(c.Query("booking_id"))
	}

	// Deteksi kolom-kolom yang mungkin beda nama
	seatCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "seat_code"):
		seatCol = "seat_code"
	case intdb.HasColumn(db, tbl, "selected_seats"):
		seatCol = "selected_seats"
	default:
		seatCol = "" // nanti pakai '' di SELECT
	}

	dateCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "trip_date"):
		dateCol = "trip_date"
	case intdb.HasColumn(db, tbl, "date"):
		dateCol = "date"
	default:
		dateCol = ""
	}

	timeCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "trip_time"):
		timeCol = "trip_time"
	case intdb.HasColumn(db, tbl, "departure_time"):
		timeCol = "departure_time"
	default:
		timeCol = ""
	}

	pickupCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "pickup_location"):
		pickupCol = "pickup_location"
	case intdb.HasColumn(db, tbl, "pickup_address"):
		pickupCol = "pickup_address"
	default:
		pickupCol = ""
	}

	dropoffCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "dropoff_location"):
		dropoffCol = "dropoff_location"
	case intdb.HasColumn(db, tbl, "dropoff_address"):
		dropoffCol = "dropoff_address"
	default:
		dropoffCol = ""
	}

	amountCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "paid_price"):
		amountCol = "paid_price"
	case intdb.HasColumn(db, tbl, "total_amount"):
		amountCol = "total_amount"
	default:
		amountCol = ""
	}

	serviceTypeCol := ""
	if intdb.HasColumn(db, tbl, "service_type") {
		serviceTypeCol = "service_type"
	}

	eticketCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "eticket_photo"):
		eticketCol = "eticket_photo"
	case intdb.HasColumn(db, tbl, "e_ticket_photo"):
		eticketCol = "e_ticket_photo"
	default:
		eticketCol = ""
	}

	driverCol := ""
	if intdb.HasColumn(db, tbl, "driver_name") {
		driverCol = "driver_name"
	}

	vehicleCodeCol := ""
	if intdb.HasColumn(db, tbl, "vehicle_code") {
		vehicleCodeCol = "vehicle_code"
	}

	vehicleTypeCol := ""
	if intdb.HasColumn(db, tbl, "vehicle_type") {
		vehicleTypeCol = "vehicle_type"
	}

	routeFromCol := ""
	if intdb.HasColumn(db, tbl, "route_from") {
		routeFromCol = "route_from"
	}
	routeToCol := ""
	if intdb.HasColumn(db, tbl, "route_to") {
		routeToCol = "route_to"
	}

	paymentStatusCol := ""
	if intdb.HasColumn(db, tbl, "payment_status") {
		paymentStatusCol = "payment_status"
	}

	notesCol := ""
	if intdb.HasColumn(db, tbl, "notes") {
		notesCol = "notes"
	}
	createdAtCol := ""
	if intdb.HasColumn(db, tbl, "created_at") {
		createdAtCol = "created_at"
	}

	hasBookingColumn := intdb.HasColumn(db, tbl, "booking_id")

	bookingFilter := ""
	args := []any{}
	if bookingParam != "" {
		bookingID, err := strconv.Atoi(bookingParam)
		if err != nil || bookingID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bookingId tidak valid"})
			return
		}
		if !hasBookingColumn {
			c.JSON(http.StatusBadRequest, gin.H{"error": "kolom booking_id tidak tersedia di tabel " + tbl})
			return
		}
		bookingFilter = "WHERE booking_id = ?"
		args = append(args, bookingID)
	}

	// helper untuk SELECT aman
	selStr := func(col string) string {
		if col == "" {
			return "''"
		}
		return fmt.Sprintf("COALESCE(%s,'')", col)
	}
	selInt := func(col string) string {
		if col == "" {
			return "0"
		}
		return fmt.Sprintf("COALESCE(%s,0)", col)
	}

	bookingIDSel := "0"
	if hasBookingColumn {
		bookingIDSel = "COALESCE(booking_id,0)"
	}

	query := fmt.Sprintf(`
		SELECT
			COALESCE(id,0),
			%s,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s
		FROM %s
		%s
		ORDER BY id DESC
	`,
		selStr("passenger_name"),
		selStr("passenger_phone"),
		selStr(dateCol),
		selStr(timeCol),
		selStr(pickupCol),
		selStr(dropoffCol),
		selInt(amountCol),
		selStr(seatCol),
		selStr(serviceTypeCol),
		selStr(eticketCol),
		selStr(driverCol),
		selStr(vehicleCodeCol),
		selStr(routeFromCol),
		selStr(routeToCol),
		selStr(vehicleTypeCol),
		selStr(paymentStatusCol),
		selStr(notesCol),
		selStr(createdAtCol),
		bookingIDSel,
		tbl,
		bookingFilter,
	)

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Println("GetPassengers query error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengambil data penumpang: " + err.Error()})
		return
	}
	defer rows.Close()

	passengers := []Passenger{}
	for rows.Next() {
		var p Passenger
		var seatsStr string
		if err := rows.Scan(
			&p.ID,
			&p.PassengerName,
			&p.PassengerPhone,
			&p.Date,
			&p.DepartureTime,
			&p.PickupAddress,
			&p.DropoffAddress,
			&p.TotalAmount,
			&seatsStr,
			&p.ServiceType,
			&p.ETicketPhoto,
			&p.DriverName,
			&p.VehicleCode,
			&p.RouteFrom,
			&p.RouteTo,
			&p.VehicleType,
			&p.PaymentStatus,
			&p.Notes,
			&p.CreatedAt,
			&p.BookingID,
		); err != nil {
			log.Println("GetPassengers scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data penumpang: " + err.Error()})
			return
		}

		// normalize seat (kalau ternyata kebaca array / string campuran)
		normalizedSeats := normalizeSeatsUnique(parseSeatsFlexible(seatsStr))
		if len(normalizedSeats) > 0 {
			// untuk tabel baru seat_code, tetap kita simpan string seat saja (ambil pertama)
			// tapi kalau memang multi-seat, kita gabungkan
			p.SelectedSeats = SeatsString(strings.Join(normalizedSeats, ","))
		} else {
			p.SelectedSeats = SeatsString(strings.TrimSpace(seatsStr))
		}

		// fallback vehicle_code dari vehicle_type jika kosong
		if strings.TrimSpace(p.VehicleCode) == "" && strings.TrimSpace(p.VehicleType) != "" {
			p.VehicleCode = p.VehicleType
		}

		// fallback driver/vehicle dari departure_settings berdasar booking
		if p.BookingID > 0 && strings.TrimSpace(p.DriverName) == "" && strings.TrimSpace(p.VehicleCode) == "" && strings.TrimSpace(p.VehicleType) == "" {
			d, v := findDriverVehicleForBooking(p.BookingID)
			if strings.TrimSpace(p.DriverName) == "" {
				p.DriverName = d
			}
			if strings.TrimSpace(p.VehicleCode) == "" {
				p.VehicleCode = v
			}
		}

		// fallback driver/vehicle berdasar trip meta jika belum juga
		if strings.TrimSpace(p.DriverName) == "" && strings.TrimSpace(p.VehicleCode) == "" {
			from := strings.TrimSpace(p.RouteFrom)
			to := strings.TrimSpace(p.RouteTo)
			if from == "" {
				from = p.PickupAddress
			}
			if to == "" {
				to = p.DropoffAddress
			}
			d, v := findDriverVehicleByTrip(p.Date, p.DepartureTime, from, to)
			if strings.TrimSpace(p.DriverName) == "" {
				p.DriverName = d
			}
			if strings.TrimSpace(p.VehicleCode) == "" {
				p.VehicleCode = v
			}
		}

		passengers = append(passengers, p)
	}

	if err := rows.Err(); err != nil {
		log.Println("GetPassengers rows error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data penumpang: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, passengers)
}

// POST /api/passengers
func CreatePassenger(c *gin.Context) {
	db := intconfig.DB
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db tidak tersedia"})
		return
	}

	tbl := passengerTableName()

	var input Passenger
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("CreatePassenger bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	input.PassengerName = strings.TrimSpace(input.PassengerName)
	input.PassengerPhone = strings.TrimSpace(input.PassengerPhone)

	// ✅ phone boleh kosong (karena dari booking_passengers kadang belum diisi)
	if input.PassengerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nama penumpang wajib diisi"})
		return
	}

	// detect seat column
	seatCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "seat_code"):
		seatCol = "seat_code"
	case intdb.HasColumn(db, tbl, "selected_seats"):
		seatCol = "selected_seats"
	}

	// detect other columns
	dateCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "trip_date"):
		dateCol = "trip_date"
	case intdb.HasColumn(db, tbl, "date"):
		dateCol = "date"
	}
	timeCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "trip_time"):
		timeCol = "trip_time"
	case intdb.HasColumn(db, tbl, "departure_time"):
		timeCol = "departure_time"
	}
	pickupCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "pickup_location"):
		pickupCol = "pickup_location"
	case intdb.HasColumn(db, tbl, "pickup_address"):
		pickupCol = "pickup_address"
	}
	dropoffCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "dropoff_location"):
		dropoffCol = "dropoff_location"
	case intdb.HasColumn(db, tbl, "dropoff_address"):
		dropoffCol = "dropoff_address"
	}
	amountCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "paid_price"):
		amountCol = "paid_price"
	case intdb.HasColumn(db, tbl, "total_amount"):
		amountCol = "total_amount"
	}

	cols := []string{}
	args := []any{}

	add := func(col string, val any) {
		if col == "" || !intdb.HasColumn(db, tbl, col) {
			return
		}
		cols = append(cols, col)
		args = append(args, val)
	}

	add("passenger_name", input.PassengerName)
	add("passenger_phone", input.PassengerPhone)

	// tanggal/jam/lokasi/seat/amount (pakai yang tersedia)
	if dateCol != "" {
		add(dateCol, nullStringIfEmpty(normalizeDateOnly(input.Date)))
	}
	if timeCol != "" {
		add(timeCol, nullStringIfEmpty(normalizeTripTime(input.DepartureTime)))
	}
	if pickupCol != "" {
		add(pickupCol, strings.TrimSpace(input.PickupAddress))
	}
	if dropoffCol != "" {
		add(dropoffCol, strings.TrimSpace(input.DropoffAddress))
	}
	if amountCol != "" {
		add(amountCol, input.TotalAmount)
	}
	if seatCol != "" {
		seat := strings.TrimSpace(string(input.SelectedSeats))
		if seat == "" {
			seat = "ALL"
		}
		add(seatCol, seat)
	}

	add("service_type", strings.TrimSpace(input.ServiceType))
	// e-ticket
	if intdb.HasColumn(db, tbl, "eticket_photo") {
		add("eticket_photo", input.ETicketPhoto)
	} else if intdb.HasColumn(db, tbl, "e_ticket_photo") {
		add("e_ticket_photo", input.ETicketPhoto)
	}

	add("driver_name", strings.TrimSpace(input.DriverName))
	add("vehicle_code", strings.TrimSpace(input.VehicleCode))
	add("vehicle_type", strings.TrimSpace(input.VehicleType))
	add("route_from", strings.TrimSpace(input.RouteFrom))
	add("route_to", strings.TrimSpace(input.RouteTo))
	add("payment_status", strings.TrimSpace(input.PaymentStatus))
	add("notes", strings.TrimSpace(input.Notes))

	// booking_id optional tapi disarankan ada
	if intdb.HasColumn(db, tbl, "booking_id") && input.BookingID > 0 {
		add("booking_id", input.BookingID)
	}

	if len(cols) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "schema tabel " + tbl + " tidak cocok / kolom tidak ditemukan"})
		return
	}

	placeholders := make([]string, len(cols))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(`INSERT INTO %s (%s) VALUES (%s)`, tbl, strings.Join(cols, ", "), strings.Join(placeholders, ", "))
	res, err := db.Exec(query, args...)
	if err != nil {
		log.Println("CreatePassenger insert error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membuat penumpang baru: " + err.Error()})
		return
	}

	lastID, _ := res.LastInsertId()
	input.ID = int(lastID)

	// created_at optional
	if intdb.HasColumn(db, tbl, "created_at") {
		_ = db.QueryRow(fmt.Sprintf("SELECT COALESCE(created_at,'') FROM %s WHERE id=?", tbl), lastID).Scan(&input.CreatedAt)
	}

	c.JSON(http.StatusCreated, input)
}

// PUT /api/passengers/:id
func UpdatePassenger(c *gin.Context) {
	db := intconfig.DB
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db tidak tersedia"})
		return
	}

	tbl := passengerTableName()

	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	var input Passenger
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("UpdatePassenger bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	input.PassengerName = strings.TrimSpace(input.PassengerName)
	input.PassengerPhone = strings.TrimSpace(input.PassengerPhone)

	// ✅ phone boleh kosong
	if input.PassengerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nama penumpang wajib diisi"})
		return
	}

	// detect seat/date/time/location columns
	seatCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "seat_code"):
		seatCol = "seat_code"
	case intdb.HasColumn(db, tbl, "selected_seats"):
		seatCol = "selected_seats"
	}
	dateCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "trip_date"):
		dateCol = "trip_date"
	case intdb.HasColumn(db, tbl, "date"):
		dateCol = "date"
	}
	timeCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "trip_time"):
		timeCol = "trip_time"
	case intdb.HasColumn(db, tbl, "departure_time"):
		timeCol = "departure_time"
	}
	pickupCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "pickup_location"):
		pickupCol = "pickup_location"
	case intdb.HasColumn(db, tbl, "pickup_address"):
		pickupCol = "pickup_address"
	}
	dropoffCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "dropoff_location"):
		dropoffCol = "dropoff_location"
	case intdb.HasColumn(db, tbl, "dropoff_address"):
		dropoffCol = "dropoff_address"
	}
	amountCol := ""
	switch {
	case intdb.HasColumn(db, tbl, "paid_price"):
		amountCol = "paid_price"
	case intdb.HasColumn(db, tbl, "total_amount"):
		amountCol = "total_amount"
	}

	sets := []string{}
	args := []any{}

	addSet := func(col string, val any) {
		if col == "" || !intdb.HasColumn(db, tbl, col) {
			return
		}
		sets = append(sets, fmt.Sprintf("%s = ?", col))
		args = append(args, val)
	}

	addSet("passenger_name", input.PassengerName)
	addSet("passenger_phone", input.PassengerPhone)

	if dateCol != "" {
		addSet(dateCol, nullStringIfEmpty(normalizeDateOnly(input.Date)))
	}
	if timeCol != "" {
		addSet(timeCol, nullStringIfEmpty(normalizeTripTime(input.DepartureTime)))
	}
	if pickupCol != "" {
		addSet(pickupCol, strings.TrimSpace(input.PickupAddress))
	}
	if dropoffCol != "" {
		addSet(dropoffCol, strings.TrimSpace(input.DropoffAddress))
	}
	if amountCol != "" {
		addSet(amountCol, input.TotalAmount)
	}
	if seatCol != "" {
		seat := strings.TrimSpace(string(input.SelectedSeats))
		if seat == "" {
			seat = "ALL"
		}
		addSet(seatCol, seat)
	}

	addSet("service_type", strings.TrimSpace(input.ServiceType))

	// e-ticket
	if intdb.HasColumn(db, tbl, "eticket_photo") {
		addSet("eticket_photo", input.ETicketPhoto)
	} else if intdb.HasColumn(db, tbl, "e_ticket_photo") {
		addSet("e_ticket_photo", input.ETicketPhoto)
	}

	addSet("driver_name", strings.TrimSpace(input.DriverName))
	addSet("vehicle_code", strings.TrimSpace(input.VehicleCode))
	addSet("vehicle_type", strings.TrimSpace(input.VehicleType))
	addSet("route_from", strings.TrimSpace(input.RouteFrom))
	addSet("route_to", strings.TrimSpace(input.RouteTo))
	addSet("payment_status", strings.TrimSpace(input.PaymentStatus))
	addSet("notes", strings.TrimSpace(input.Notes))

	if intdb.HasColumn(db, tbl, "booking_id") && input.BookingID > 0 {
		addSet("booking_id", input.BookingID)
	}

	if len(sets) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tidak ada kolom yang bisa diupdate pada tabel " + tbl})
		return
	}

	args = append(args, id)

	query := fmt.Sprintf(`UPDATE %s SET %s WHERE id = ?`, tbl, strings.Join(sets, ", "))
	if _, err = db.Exec(query, args...); err != nil {
		log.Println("UpdatePassenger update error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengupdate penumpang: " + err.Error()})
		return
	}

	input.ID = id
	c.JSON(http.StatusOK, input)
}

// DELETE /api/passengers/:id
func DeletePassenger(c *gin.Context) {
	db := intconfig.DB
	if db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db tidak tersedia"})
		return
	}

	tbl := passengerTableName()

	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	res, err := db.Exec(fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, tbl), id)
	if err != nil {
		log.Println("DeletePassenger delete error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menghapus data penumpang: " + err.Error()})
		return
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "penumpang tidak ditemukan"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "penumpang berhasil dihapus"})
}

// =======================
// helpers (shared logic)
// =======================

func loadDriverVehicleTypeByDriverName(driverName string) string {
	n := strings.ToLower(strings.TrimSpace(driverName))
	if n == "" {
		return ""
	}

	if intdb.HasTable(intconfig.DB, "driver_accounts") && intdb.HasColumn(intconfig.DB, "driver_accounts", "vehicle_type") {
		var vt sql.NullString
		_ = intconfig.DB.QueryRow(
			`SELECT COALESCE(vehicle_type,'') FROM driver_accounts WHERE LOWER(TRIM(driver_name)) = ? LIMIT 1`,
			n,
		).Scan(&vt)
		if strings.TrimSpace(vt.String) != "" {
			return strings.TrimSpace(vt.String)
		}
	}

	if intdb.HasTable(intconfig.DB, "drivers") && intdb.HasColumn(intconfig.DB, "drivers", "vehicle_type") {
		var vt sql.NullString
		_ = intconfig.DB.QueryRow(
			`SELECT COALESCE(vehicle_type,'') FROM drivers WHERE LOWER(TRIM(name)) = ? LIMIT 1`,
			n,
		).Scan(&vt)
		return strings.TrimSpace(vt.String)
	}

	return ""
}

func parseSeatsFlexible(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return arr
	}

	var s string
	if err := json.Unmarshal([]byte(raw), &s); err == nil {
		raw = s
	}

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

func findDriverVehicleForBooking(bookingID int64) (string, string) {
	db := intconfig.DB
	if bookingID <= 0 || db == nil || !intdb.HasTable(db, "departure_settings") {
		return "", ""
	}

	var dsID int64
	switch {
	case intdb.HasColumn(db, "departure_settings", "booking_id"):
		_ = db.QueryRow(`SELECT id FROM departure_settings WHERE booking_id=? LIMIT 1`, bookingID).Scan(&dsID)
	case intdb.HasColumn(db, "departure_settings", "reguler_booking_id"):
		_ = db.QueryRow(`SELECT id FROM departure_settings WHERE reguler_booking_id=? LIMIT 1`, bookingID).Scan(&dsID)
	default:
		return "", ""
	}
	if dsID == 0 {
		return "", ""
	}

	// driver
	var driver sql.NullString
	switch {
	case intdb.HasColumn(db, "departure_settings", "driver_name"):
		_ = db.QueryRow(`SELECT COALESCE(driver_name,'') FROM departure_settings WHERE id=?`, dsID).Scan(&driver)
	case intdb.HasColumn(db, "departure_settings", "driver"):
		_ = db.QueryRow(`SELECT COALESCE(driver,'') FROM departure_settings WHERE id=?`, dsID).Scan(&driver)
	default:
		driver = sql.NullString{String: "", Valid: true}
	}

	// vehicle
	var vehicle sql.NullString
	switch {
	case intdb.HasColumn(db, "departure_settings", "vehicle_type"):
		_ = db.QueryRow(`SELECT COALESCE(vehicle_type,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicle)
	case intdb.HasColumn(db, "departure_settings", "vehicle_name"):
		_ = db.QueryRow(`SELECT COALESCE(vehicle_name,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicle)
	case intdb.HasColumn(db, "departure_settings", "vehicle"):
		_ = db.QueryRow(`SELECT COALESCE(vehicle,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicle)
	case intdb.HasColumn(db, "departure_settings", "vehicle_code"):
		_ = db.QueryRow(`SELECT COALESCE(vehicle_code,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicle)
	case intdb.HasColumn(db, "departure_settings", "car_code"):
		_ = db.QueryRow(`SELECT COALESCE(car_code,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicle)
	default:
		vehicle = sql.NullString{String: "", Valid: true}
	}

	d := strings.TrimSpace(driver.String)
	v := strings.TrimSpace(vehicle.String)
	if v == "" && d != "" {
		v = loadDriverVehicleTypeByDriverName(d)
	}
	return d, v
}

func findDriverVehicleByTrip(dateStr, timeStr, from, to string) (string, string) {
	db := intconfig.DB
	if db == nil || !intdb.HasTable(db, "departure_settings") {
		return "", ""
	}

	dateOnly := normalizeDateOnly(dateStr)
	timeOnly := normalizeTripTime(timeStr)

	cond := []string{}
	args := []any{}

	if intdb.HasColumn(db, "departure_settings", "departure_date") && dateOnly != "" {
		cond = append(cond, "DATE(COALESCE(departure_date,''))=?")
		args = append(args, dateOnly)
	}
	if intdb.HasColumn(db, "departure_settings", "departure_time") && timeOnly != "" {
		cond = append(cond, "LEFT(COALESCE(departure_time,''),5)=?")
		args = append(args, timeOnly)
	}
	if intdb.HasColumn(db, "departure_settings", "route_from") && strings.TrimSpace(from) != "" {
		cond = append(cond, "LOWER(TRIM(route_from))=?")
		args = append(args, strings.ToLower(strings.TrimSpace(from)))
	}
	if intdb.HasColumn(db, "departure_settings", "route_to") && strings.TrimSpace(to) != "" {
		cond = append(cond, "LOWER(TRIM(route_to))=?")
		args = append(args, strings.ToLower(strings.TrimSpace(to)))
	}

	if len(cond) == 0 {
		return "", ""
	}

	driverSel := "''"
	if intdb.HasColumn(db, "departure_settings", "driver_name") {
		driverSel = "COALESCE(driver_name,'')"
	} else if intdb.HasColumn(db, "departure_settings", "driver") {
		driverSel = "COALESCE(driver,'')"
	}

	vehicleSel := "''"
	switch {
	case intdb.HasColumn(db, "departure_settings", "vehicle_type"):
		vehicleSel = "COALESCE(vehicle_type,'')"
	case intdb.HasColumn(db, "departure_settings", "vehicle_name"):
		vehicleSel = "COALESCE(vehicle_name,'')"
	case intdb.HasColumn(db, "departure_settings", "vehicle"):
		vehicleSel = "COALESCE(vehicle,'')"
	case intdb.HasColumn(db, "departure_settings", "vehicle_code"):
		vehicleSel = "COALESCE(vehicle_code,'')"
	case intdb.HasColumn(db, "departure_settings", "car_code"):
		vehicleSel = "COALESCE(car_code,'')"
	}

	q := fmt.Sprintf(
		`SELECT %s, %s FROM departure_settings WHERE %s ORDER BY id DESC LIMIT 1`,
		driverSel, vehicleSel, strings.Join(cond, " AND "),
	)

	var d sql.NullString
	var v sql.NullString
	if err := db.QueryRow(q, args...).Scan(&d, &v); err != nil {
		return "", ""
	}

	driver := strings.TrimSpace(d.String)
	vehicle := strings.TrimSpace(v.String)
	if vehicle == "" && driver != "" {
		vehicle = loadDriverVehicleTypeByDriverName(driver)
	}
	return driver, vehicle
}
