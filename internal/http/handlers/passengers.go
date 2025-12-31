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
	Date           string      `json:"date"`
	DepartureTime  string      `json:"departureTime"`
	PickupAddress  string      `json:"pickupAddress"`
	DropoffAddress string      `json:"dropoffAddress"`
	TotalAmount    int64       `json:"totalAmount"`
	SelectedSeats  SeatsString `json:"selectedSeats"`
	ServiceType    string      `json:"serviceType"`
	ETicketPhoto   string      `json:"eTicketPhoto"`
	DriverName     string      `json:"driverName"`
	VehicleCode    string      `json:"vehicleCode"`
	VehicleType    string      `json:"vehicleType,omitempty"`
	Notes          string      `json:"notes"`
	CreatedAt      string      `json:"createdAt"`
}

// helper: kirim NULL ke DB kalau string kosong (khusus sql.NullString)
func nullStringIfEmpty(s string) sql.NullString {
	s = strings.TrimSpace(s)
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// GET /api/passengers
func GetPassengers(c *gin.Context) {
	vehicleTypeSel := "''"
	if intdb.HasColumn(intconfig.DB, "passengers", "vehicle_type") {
		vehicleTypeSel = "COALESCE(vehicle_type,'')"
	}
	routeFromSel := "''"
	if intdb.HasColumn(intconfig.DB, "passengers", "route_from") {
		routeFromSel = "COALESCE(route_from,'')"
	}
	routeToSel := "''"
	if intdb.HasColumn(intconfig.DB, "passengers", "route_to") {
		routeToSel = "COALESCE(route_to,'')"
	}

	bookingIDSel := ""
	if intdb.HasColumn(intconfig.DB, "passengers", "booking_id") {
		bookingIDSel = ", COALESCE(booking_id,0)"
	}

	query := fmt.Sprintf(`
		SELECT
			id,
			COALESCE(passenger_name, ''),
			COALESCE(passenger_phone, ''),
			COALESCE(date, ''),
			COALESCE(departure_time, ''),
			COALESCE(pickup_address, ''),
			COALESCE(dropoff_address, ''),
			COALESCE(total_amount, 0),
			COALESCE(selected_seats, ''),
			COALESCE(service_type, ''),
			COALESCE(eticket_photo, ''),
			COALESCE(driver_name, ''),
			COALESCE(vehicle_code, ''),
			%s,
			%s,
			%s,
			COALESCE(notes, ''),
			COALESCE(created_at, '')%s
		FROM passengers
		ORDER BY id DESC
	`, routeFromSel, routeToSel, vehicleTypeSel, bookingIDSel)

	rows, err := intconfig.DB.Query(query)
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
		var bookingID sql.NullInt64
		var routeFrom string
		var routeTo string

		dests := []any{
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
			&routeFrom,
			&routeTo,
			&p.VehicleType,
			&p.Notes,
			&p.CreatedAt,
		}
		if bookingIDSel != "" {
			dests = append(dests, &bookingID)
		}

		if err := rows.Scan(dests...); err != nil {
			log.Println("GetPassengers scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data penumpang: " + err.Error()})
			return
		}

		p.SelectedSeats = SeatsString(seatsStr)
		if strings.TrimSpace(p.VehicleCode) == "" && strings.TrimSpace(p.VehicleType) != "" {
			p.VehicleCode = p.VehicleType
		}
		if bookingID.Valid && strings.TrimSpace(p.DriverName) == "" && strings.TrimSpace(p.VehicleCode) == "" && strings.TrimSpace(p.VehicleType) == "" {
			d, v := findDriverVehicleForBooking(bookingID.Int64)
			if strings.TrimSpace(p.DriverName) == "" {
				p.DriverName = d
			}
			if strings.TrimSpace(p.VehicleCode) == "" {
				p.VehicleCode = v
			}
		}
		if strings.TrimSpace(p.DriverName) == "" && strings.TrimSpace(p.VehicleCode) == "" {
			from := strings.TrimSpace(routeFrom)
			to := strings.TrimSpace(routeTo)
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

func findDriverVehicleForBooking(bookingID int64) (string, string) {
	if bookingID <= 0 || !intdb.HasTable(intconfig.DB, "departure_settings") {
		return "", ""
	}

	var dsID int64
	if intdb.HasColumn(intconfig.DB, "departure_settings", "booking_id") {
		_ = intconfig.DB.QueryRow(`SELECT id FROM departure_settings WHERE booking_id=? LIMIT 1`, bookingID).Scan(&dsID)
	} else if intdb.HasColumn(intconfig.DB, "departure_settings", "reguler_booking_id") {
		_ = intconfig.DB.QueryRow(`SELECT id FROM departure_settings WHERE reguler_booking_id=? LIMIT 1`, bookingID).Scan(&dsID)
	}
	if dsID == 0 {
		return "", ""
	}

	var driver sql.NullString
	if intdb.HasColumn(intconfig.DB, "departure_settings", "driver_name") {
		_ = intconfig.DB.QueryRow(`SELECT COALESCE(driver_name,'') FROM departure_settings WHERE id=?`, dsID).Scan(&driver)
	} else if intdb.HasColumn(intconfig.DB, "departure_settings", "driver") {
		_ = intconfig.DB.QueryRow(`SELECT COALESCE(driver,'') FROM departure_settings WHERE id=?`, dsID).Scan(&driver)
	}

	var vehicle sql.NullString
	switch {
	case intdb.HasColumn(intconfig.DB, "departure_settings", "vehicle_type"):
		_ = intconfig.DB.QueryRow(`SELECT COALESCE(vehicle_type,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicle)
	case intdb.HasColumn(intconfig.DB, "departure_settings", "vehicle_name"):
		_ = intconfig.DB.QueryRow(`SELECT COALESCE(vehicle_name,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicle)
	case intdb.HasColumn(intconfig.DB, "departure_settings", "vehicle"):
		_ = intconfig.DB.QueryRow(`SELECT COALESCE(vehicle,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicle)
	case intdb.HasColumn(intconfig.DB, "departure_settings", "vehicle_code"):
		_ = intconfig.DB.QueryRow(`SELECT COALESCE(vehicle_code,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicle)
	case intdb.HasColumn(intconfig.DB, "departure_settings", "car_code"):
		_ = intconfig.DB.QueryRow(`SELECT COALESCE(car_code,'') FROM departure_settings WHERE id=?`, dsID).Scan(&vehicle)
	}

	d := strings.TrimSpace(driver.String)
	v := strings.TrimSpace(vehicle.String)
	if v == "" && d != "" {
		v = loadDriverVehicleTypeByDriverName(d)
	}

	return d, v
}

func findDriverVehicleByTrip(dateStr, timeStr, from, to string) (string, string) {
	if !intdb.HasTable(intconfig.DB, "departure_settings") {
		return "", ""
	}

	dateOnly := normalizeDateOnly(dateStr)
	timeOnly := normalizeTripTime(timeStr)

	cond := []string{}
	args := []any{}
	if intdb.HasColumn(intconfig.DB, "departure_settings", "departure_date") && dateOnly != "" {
		cond = append(cond, "DATE(COALESCE(departure_date,''))=?")
		args = append(args, dateOnly)
	}
	if intdb.HasColumn(intconfig.DB, "departure_settings", "departure_time") && timeOnly != "" {
		cond = append(cond, "LEFT(COALESCE(departure_time,''),5)=?")
		args = append(args, timeOnly)
	}
	if intdb.HasColumn(intconfig.DB, "departure_settings", "route_from") && strings.TrimSpace(from) != "" {
		cond = append(cond, "LOWER(TRIM(route_from))=?")
		args = append(args, strings.ToLower(strings.TrimSpace(from)))
	}
	if intdb.HasColumn(intconfig.DB, "departure_settings", "route_to") && strings.TrimSpace(to) != "" {
		cond = append(cond, "LOWER(TRIM(route_to))=?")
		args = append(args, strings.ToLower(strings.TrimSpace(to)))
	}

	if len(cond) == 0 {
		return "", ""
	}

	driverSel := "''"
	if intdb.HasColumn(intconfig.DB, "departure_settings", "driver_name") {
		driverSel = "COALESCE(driver_name,'')"
	} else if intdb.HasColumn(intconfig.DB, "departure_settings", "driver") {
		driverSel = "COALESCE(driver,'')"
	}

	vehicleSel := "''"
	switch {
	case intdb.HasColumn(intconfig.DB, "departure_settings", "vehicle_type"):
		vehicleSel = "COALESCE(vehicle_type,'')"
	case intdb.HasColumn(intconfig.DB, "departure_settings", "vehicle_name"):
		vehicleSel = "COALESCE(vehicle_name,'')"
	case intdb.HasColumn(intconfig.DB, "departure_settings", "vehicle"):
		vehicleSel = "COALESCE(vehicle,'')"
	case intdb.HasColumn(intconfig.DB, "departure_settings", "vehicle_code"):
		vehicleSel = "COALESCE(vehicle_code,'')"
	case intdb.HasColumn(intconfig.DB, "departure_settings", "car_code"):
		vehicleSel = "COALESCE(car_code,'')"
	}

	q := fmt.Sprintf(`SELECT %s, %s FROM departure_settings WHERE %s ORDER BY id DESC LIMIT 1`, driverSel, vehicleSel, strings.Join(cond, " AND "))

	var d sql.NullString
	var v sql.NullString
	if err := intconfig.DB.QueryRow(q, args...).Scan(&d, &v); err != nil {
		return "", ""
	}

	driver := strings.TrimSpace(d.String)
	vehicle := strings.TrimSpace(v.String)
	if vehicle == "" && driver != "" {
		vehicle = loadDriverVehicleTypeByDriverName(driver)
	}
	return driver, vehicle
}

// POST /api/passengers
func CreatePassenger(c *gin.Context) {
	var input Passenger
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("CreatePassenger bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	input.PassengerName = strings.TrimSpace(input.PassengerName)
	input.PassengerPhone = strings.TrimSpace(input.PassengerPhone)

	if input.PassengerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nama penumpang wajib diisi"})
		return
	}
	if input.PassengerPhone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no hp penumpang wajib diisi"})
		return
	}

	res, err := intconfig.DB.Exec(`
		INSERT INTO passengers
			(passenger_name, passenger_phone, date, departure_time,
			 pickup_address, dropoff_address, total_amount, selected_seats,
			 service_type, eticket_photo,
			 driver_name, vehicle_code, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		input.PassengerName,
		input.PassengerPhone,
		nullStringIfEmpty(input.Date),
		nullStringIfEmpty(input.DepartureTime),
		strings.TrimSpace(input.PickupAddress),
		strings.TrimSpace(input.DropoffAddress),
		input.TotalAmount,
		string(input.SelectedSeats),
		strings.TrimSpace(input.ServiceType),
		input.ETicketPhoto,
		strings.TrimSpace(input.DriverName),
		strings.TrimSpace(input.VehicleCode),
		strings.TrimSpace(input.Notes),
	)
	if err != nil {
		log.Println("CreatePassenger insert error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membuat penumpang baru: " + err.Error()})
		return
	}

	lastID, _ := res.LastInsertId()
	input.ID = int(lastID)
	_ = intconfig.DB.QueryRow(
		"SELECT COALESCE(created_at, '') FROM passengers WHERE id = ?",
		lastID,
	).Scan(&input.CreatedAt)

	c.JSON(http.StatusCreated, input)
}

// PUT /api/passengers/:id
func UpdatePassenger(c *gin.Context) {
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

	if input.PassengerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nama penumpang wajib diisi"})
		return
	}
	if input.PassengerPhone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no hp penumpang wajib diisi"})
		return
	}

	if _, err = intconfig.DB.Exec(`
		UPDATE passengers
		SET
			passenger_name  = ?,
			passenger_phone = ?,
			date            = ?,
			departure_time  = ?,
			pickup_address  = ?,
			dropoff_address = ?,
			total_amount    = ?,
			selected_seats  = ?,
			service_type    = ?,
			eticket_photo   = ?,
			driver_name     = ?,
			vehicle_code    = ?,
			notes           = ?
		WHERE id = ?
	`,
		input.PassengerName,
		input.PassengerPhone,
		nullStringIfEmpty(input.Date),
		nullStringIfEmpty(input.DepartureTime),
		strings.TrimSpace(input.PickupAddress),
		strings.TrimSpace(input.DropoffAddress),
		input.TotalAmount,
		string(input.SelectedSeats),
		strings.TrimSpace(input.ServiceType),
		input.ETicketPhoto,
		strings.TrimSpace(input.DriverName),
		strings.TrimSpace(input.VehicleCode),
		strings.TrimSpace(input.Notes),
		id,
	); err != nil {
		log.Println("UpdatePassenger update error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengupdate penumpang: " + err.Error()})
		return
	}

	input.ID = id
	c.JSON(http.StatusOK, input)
}

// DELETE /api/passengers/:id
func DeletePassenger(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	res, err := intconfig.DB.Exec(`DELETE FROM passengers WHERE id = ?`, id)
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
