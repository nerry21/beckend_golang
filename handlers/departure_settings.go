// backend/handlers/departure_settings.go
package handlers

import (
	"backend/config"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type DepartureSetting struct {
	ID                 int    `json:"id"`
	BookingName        string `json:"bookingName"`
	Phone              string `json:"phone"`
	PickupAddress      string `json:"pickupAddress"`
	DepartureDate      string `json:"departureDate"` // "YYYY-MM-DD" atau datetime string
	SeatNumbers        string `json:"seatNumbers"`
	PassengerCount     string `json:"passengerCount"`
	ServiceType        string `json:"serviceType"`
	DriverName         string `json:"driverName"`
	VehicleCode        string `json:"vehicleCode"`
	SuratJalanFile     string `json:"suratJalanFile"`
	SuratJalanFileName string `json:"suratJalanFileName"`
	DepartureStatus    string `json:"departureStatus"`
	CreatedAt          string `json:"createdAt"`

	// ✅ tambahan (tidak menghapus field lama) - opsional sesuai kolom DB
	DepartureTime string `json:"departureTime"`
	RouteFrom     string `json:"routeFrom"`
	RouteTo       string `json:"routeTo"`
	TripNumber    string `json:"tripNumber"`
	BookingID     int64  `json:"bookingId"`
}

// GET /api/departure-settings
func GetDepartureSettings(c *gin.Context) {
	table := "departure_settings"
	if !hasTable(config.DB, table) {
		c.JSON(http.StatusOK, []DepartureSetting{})
		return
	}

	// kolom opsional (agar kompatibel berbagai skema)
	tripNoSel := "''"
	if hasColumn(config.DB, table, "trip_number") {
		tripNoSel = "COALESCE(trip_number,'')"
	}
	bookingIDSel := "0"
	if hasColumn(config.DB, table, "booking_id") {
		bookingIDSel = "COALESCE(booking_id,0)"
	}
	depTimeSel := "''"
	if hasColumn(config.DB, table, "departure_time") {
		depTimeSel = "COALESCE(departure_time,'')"
	}
	routeFromSel := "''"
	if hasColumn(config.DB, table, "route_from") {
		routeFromSel = "COALESCE(route_from,'')"
	}
	routeToSel := "''"
	if hasColumn(config.DB, table, "route_to") {
		routeToSel = "COALESCE(route_to,'')"
	}

	rows, err := config.DB.Query(fmt.Sprintf(`
		SELECT
			id,
			COALESCE(booking_name,''),
			COALESCE(phone,''),
			COALESCE(pickup_address,''),
			COALESCE(departure_date,''),
			COALESCE(seat_numbers,''),
			COALESCE(passenger_count, 0),
			COALESCE(service_type,''),
			COALESCE(driver_name,''),
			COALESCE(vehicle_code,''),
			COALESCE(surat_jalan_file,''),
			COALESCE(surat_jalan_file_name,''),
			COALESCE(departure_status,''),
			COALESCE(created_at,''),
			%s AS trip_number,
			%s AS booking_id,
			%s AS departure_time,
			%s AS route_from,
			%s AS route_to
		FROM %s
		ORDER BY id DESC
	`, tripNoSel, bookingIDSel, depTimeSel, routeFromSel, routeToSel, table))
	if err != nil {
		log.Println("GetDepartureSettings query error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengambil data: " + err.Error()})
		return
	}
	defer rows.Close()

	var list []DepartureSetting
	for rows.Next() {
		var d DepartureSetting
		var countInt int
		var bookingID int64

		if err := rows.Scan(
			&d.ID,
			&d.BookingName,
			&d.Phone,
			&d.PickupAddress,
			&d.DepartureDate,
			&d.SeatNumbers,
			&countInt,
			&d.ServiceType,
			&d.DriverName,
			&d.VehicleCode,
			&d.SuratJalanFile,
			&d.SuratJalanFileName,
			&d.DepartureStatus,
			&d.CreatedAt,
			&d.TripNumber,
			&bookingID,
			&d.DepartureTime,
			&d.RouteFrom,
			&d.RouteTo,
		); err != nil {
			log.Println("GetDepartureSettings scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data: " + err.Error()})
			return
		}

		d.BookingID = bookingID
		d.PassengerCount = strconv.Itoa(countInt)

		// ✅ Auto isi Surat Jalan bila kosong (dari trip_information / fallback booking)
		if strings.TrimSpace(d.SuratJalanFile) == "" {
			if s := strings.TrimSpace(getTripESuratJalanDB(config.DB, d.TripNumber)); s != "" {
				d.SuratJalanFile = s
			} else if d.BookingID > 0 {
				// fallback endpoint booking (biasanya bisa preview sebagai <img>)
				d.SuratJalanFile = buildSuratJalanAPI(d.BookingID)
			}
		}

		list = append(list, d)
	}

	c.JSON(http.StatusOK, list)
}

// POST /api/departure-settings
func CreateDepartureSetting(c *gin.Context) {
	var input DepartureSetting
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("CreateDepartureSetting bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	table := "departure_settings"
	if !hasTable(config.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel departure_settings tidak ditemukan"})
		return
	}

	count, _ := strconv.Atoi(input.PassengerCount)

	cols := []string{}
	vals := []any{}

	// base kolom (kompatibel kode lama)
	if hasColumn(config.DB, table, "booking_name") {
		cols = append(cols, "booking_name")
		vals = append(vals, input.BookingName)
	}
	if hasColumn(config.DB, table, "phone") {
		cols = append(cols, "phone")
		vals = append(vals, input.Phone)
	}
	if hasColumn(config.DB, table, "pickup_address") {
		cols = append(cols, "pickup_address")
		vals = append(vals, input.PickupAddress)
	}
	if hasColumn(config.DB, table, "departure_date") {
		cols = append(cols, "departure_date")
		vals = append(vals, nullIfEmpty(input.DepartureDate))
	}
	if hasColumn(config.DB, table, "seat_numbers") {
		cols = append(cols, "seat_numbers")
		vals = append(vals, input.SeatNumbers)
	}
	if hasColumn(config.DB, table, "passenger_count") {
		cols = append(cols, "passenger_count")
		vals = append(vals, count)
	}
	if hasColumn(config.DB, table, "service_type") {
		cols = append(cols, "service_type")
		vals = append(vals, input.ServiceType)
	}
	if hasColumn(config.DB, table, "driver_name") {
		cols = append(cols, "driver_name")
		vals = append(vals, input.DriverName)
	}
	if hasColumn(config.DB, table, "vehicle_code") {
		cols = append(cols, "vehicle_code")
		vals = append(vals, input.VehicleCode)
	}
	if hasColumn(config.DB, table, "surat_jalan_file") {
		cols = append(cols, "surat_jalan_file")
		vals = append(vals, input.SuratJalanFile)
	}
	if hasColumn(config.DB, table, "surat_jalan_file_name") {
		cols = append(cols, "surat_jalan_file_name")
		vals = append(vals, input.SuratJalanFileName)
	}
	if hasColumn(config.DB, table, "departure_status") {
		cols = append(cols, "departure_status")
		vals = append(vals, input.DepartureStatus)
	}

	// ✅ kolom tambahan opsional
	if hasColumn(config.DB, table, "departure_time") {
		cols = append(cols, "departure_time")
		vals = append(vals, nullIfEmpty(input.DepartureTime))
	}
	if hasColumn(config.DB, table, "route_from") {
		cols = append(cols, "route_from")
		vals = append(vals, input.RouteFrom)
	}
	if hasColumn(config.DB, table, "route_to") {
		cols = append(cols, "route_to")
		vals = append(vals, input.RouteTo)
	}
	if hasColumn(config.DB, table, "trip_number") {
		cols = append(cols, "trip_number")
		vals = append(vals, input.TripNumber)
	}
	if hasColumn(config.DB, table, "booking_id") {
		cols = append(cols, "booking_id")
		vals = append(vals, input.BookingID)
	}
	if hasColumn(config.DB, table, "created_at") {
		cols = append(cols, "created_at")
		vals = append(vals, time.Now())
	}

	if len(cols) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tidak ada kolom yang bisa di-insert"})
		return
	}

	ph := make([]string, len(cols))
	for i := range ph {
		ph[i] = "?"
	}

	res, err := config.DB.Exec(
		`INSERT INTO `+table+` (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(ph, ",")+`)`,
		vals...,
	)
	if err != nil {
		log.Println("CreateDepartureSetting insert error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membuat data: " + err.Error()})
		return
	}

	id, _ := res.LastInsertId()
	input.ID = int(id)
	input.PassengerCount = strconv.Itoa(count)

	// ambil created_at jika ada
	if hasColumn(config.DB, table, "created_at") {
		_ = config.DB.QueryRow("SELECT COALESCE(created_at, '') FROM "+table+" WHERE id = ? LIMIT 1", id).Scan(&input.CreatedAt)
	}

	c.JSON(http.StatusCreated, input)
}

// PUT /api/departure-settings/:id
func UpdateDepartureSetting(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	var input DepartureSetting
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("UpdateDepartureSetting bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	table := "departure_settings"
	if !hasTable(config.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel departure_settings tidak ditemukan"})
		return
	}

	count, _ := strconv.Atoi(input.PassengerCount)

	sets := []string{}
	args := []any{}

	// base kolom
	if hasColumn(config.DB, table, "booking_name") {
		sets = append(sets, "booking_name=?")
		args = append(args, input.BookingName)
	}
	if hasColumn(config.DB, table, "phone") {
		sets = append(sets, "phone=?")
		args = append(args, input.Phone)
	}
	if hasColumn(config.DB, table, "pickup_address") {
		sets = append(sets, "pickup_address=?")
		args = append(args, input.PickupAddress)
	}
	if hasColumn(config.DB, table, "departure_date") {
		sets = append(sets, "departure_date=?")
		args = append(args, nullIfEmpty(input.DepartureDate))
	}
	if hasColumn(config.DB, table, "seat_numbers") {
		sets = append(sets, "seat_numbers=?")
		args = append(args, input.SeatNumbers)
	}
	if hasColumn(config.DB, table, "passenger_count") {
		sets = append(sets, "passenger_count=?")
		args = append(args, count)
	}
	if hasColumn(config.DB, table, "service_type") {
		sets = append(sets, "service_type=?")
		args = append(args, input.ServiceType)
	}
	if hasColumn(config.DB, table, "driver_name") {
		sets = append(sets, "driver_name=?")
		args = append(args, input.DriverName)
	}
	if hasColumn(config.DB, table, "vehicle_code") {
		sets = append(sets, "vehicle_code=?")
		args = append(args, input.VehicleCode)
	}
	if hasColumn(config.DB, table, "surat_jalan_file") {
		sets = append(sets, "surat_jalan_file=?")
		args = append(args, input.SuratJalanFile)
	}
	if hasColumn(config.DB, table, "surat_jalan_file_name") {
		sets = append(sets, "surat_jalan_file_name=?")
		args = append(args, input.SuratJalanFileName)
	}
	if hasColumn(config.DB, table, "departure_status") {
		sets = append(sets, "departure_status=?")
		args = append(args, input.DepartureStatus)
	}

	// kolom tambahan opsional
	if hasColumn(config.DB, table, "departure_time") {
		sets = append(sets, "departure_time=?")
		args = append(args, nullIfEmpty(input.DepartureTime))
	}
	if hasColumn(config.DB, table, "route_from") {
		sets = append(sets, "route_from=?")
		args = append(args, input.RouteFrom)
	}
	if hasColumn(config.DB, table, "route_to") {
		sets = append(sets, "route_to=?")
		args = append(args, input.RouteTo)
	}
	if hasColumn(config.DB, table, "trip_number") {
		sets = append(sets, "trip_number=?")
		args = append(args, input.TripNumber)
	}
	if hasColumn(config.DB, table, "booking_id") {
		sets = append(sets, "booking_id=?")
		args = append(args, input.BookingID)
	}
	if hasColumn(config.DB, table, "updated_at") {
		sets = append(sets, "updated_at=?")
		args = append(args, time.Now())
	}

	if len(sets) == 0 {
		c.JSON(http.StatusOK, input)
		return
	}

	args = append(args, id)
	_, err = config.DB.Exec(`UPDATE `+table+` SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...)
	if err != nil {
		log.Println("UpdateDepartureSetting update error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengupdate data: " + err.Error()})
		return
	}

	input.ID = id
	input.PassengerCount = strconv.Itoa(count)
	c.JSON(http.StatusOK, input)
}

// DELETE /api/departure-settings/:id
func DeleteDepartureSetting(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	table := "departure_settings"
	if !hasTable(config.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel departure_settings tidak ditemukan"})
		return
	}

	res, err := config.DB.Exec(`DELETE FROM `+table+` WHERE id = ?`, id)
	if err != nil {
		log.Println("DeleteDepartureSetting delete error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menghapus data: " + err.Error()})
		return
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "data tidak ditemukan"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "data berhasil dihapus"})
}

// helper: ambil e_surat_jalan dari trip_information (best effort)
func getTripESuratJalanDB(q queryRower, tripNo string) string {
	if q == nil || strings.TrimSpace(tripNo) == "" {
		return ""
	}

	if !hasTable(q, "trip_information") {
		return ""
	}

	// cari kolom e_surat_jalan yang tersedia
	candidates := []string{"e_surat_jalan", "eSuratJalan", "surat_jalan", "suratJalan"}
	col := ""
	for _, cc := range candidates {
		if hasColumn(q, "trip_information", cc) {
			col = cc
			break
		}
	}
	if col == "" {
		return ""
	}

	var s sql.NullString
	_ = q.QueryRow(
		"SELECT COALESCE("+col+",'') FROM trip_information WHERE trip_number=? ORDER BY id DESC LIMIT 1",
		tripNo,
	).Scan(&s)
	return strings.TrimSpace(s.String)
}
