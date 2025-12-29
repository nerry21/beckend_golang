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

// ReturnSetting memakai struktur yang sama dengan DepartureSetting (kolom 1:1)
type ReturnSetting = DepartureSetting

// GET /api/return-settings
func GetReturnSettings(c *gin.Context) {
	table := "return_settings"
	if !hasTable(config.DB, table) {
		c.JSON(http.StatusOK, []ReturnSetting{})
		return
	}

	driverVehicleMap := loadDriverVehicleTypes()

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
	vehicleTypeSel := "''"
	if hasColumn(config.DB, table, "vehicle_type") {
		vehicleTypeSel = "COALESCE(vehicle_type,'')"
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
			%s AS route_to,
			%s AS vehicle_type
		FROM %s
		ORDER BY id DESC
	`, tripNoSel, bookingIDSel, depTimeSel, routeFromSel, routeToSel, vehicleTypeSel, table))
	if err != nil {
		log.Println("GetReturnSettings query error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengambil data: " + err.Error()})
		return
	}
	defer rows.Close()

	var list []ReturnSetting
	for rows.Next() {
		var d ReturnSetting
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
			&d.VehicleType,
		); err != nil {
			log.Println("GetReturnSettings scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data: " + err.Error()})
			return
		}

		d.BookingID = bookingID
		d.PassengerCount = strconv.Itoa(countInt)

		if strings.TrimSpace(d.SuratJalanFile) == "" {
			if s := strings.TrimSpace(getTripESuratJalanDB(config.DB, d.TripNumber)); s != "" {
				d.SuratJalanFile = s
			} else if d.BookingID > 0 {
				d.SuratJalanFile = buildSuratJalanAPI(d.BookingID)
			}
		}

		if strings.TrimSpace(d.VehicleType) == "" && strings.TrimSpace(d.DriverName) != "" {
			if vt := driverVehicleMap[strings.ToLower(strings.TrimSpace(d.DriverName))]; vt != "" {
				d.VehicleType = vt
			}
		}

		list = append(list, d)
	}

	c.JSON(http.StatusOK, list)
}

// GET /api/return-settings/:id
func GetReturnSettingByID(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	table := "return_settings"
	if !hasTable(config.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel return_settings tidak ditemukan"})
		return
	}

	driverVehicleMap := loadDriverVehicleTypes()

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
	vehicleTypeSel := "''"
	if hasColumn(config.DB, table, "vehicle_type") {
		vehicleTypeSel = "COALESCE(vehicle_type,'')"
	}

	row := config.DB.QueryRow(fmt.Sprintf(`
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
			%s AS route_to,
			%s AS vehicle_type
		FROM %s
		WHERE id = ?
		LIMIT 1
	`, tripNoSel, bookingIDSel, depTimeSel, routeFromSel, routeToSel, vehicleTypeSel, table), id)

	var d ReturnSetting
	var countInt int
	var bookingID int64
	if err := row.Scan(
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
		&d.VehicleType,
	); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "data tidak ditemukan"})
			return
		}
		log.Println("GetReturnSettingByID scan error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data: " + err.Error()})
		return
	}

	d.BookingID = bookingID
	d.PassengerCount = strconv.Itoa(countInt)

	if strings.TrimSpace(d.SuratJalanFile) == "" {
		if s := strings.TrimSpace(getTripESuratJalanDB(config.DB, d.TripNumber)); s != "" {
			d.SuratJalanFile = s
		} else if d.BookingID > 0 {
			d.SuratJalanFile = buildSuratJalanAPI(d.BookingID)
		}
	}

	if strings.TrimSpace(d.VehicleType) == "" && strings.TrimSpace(d.DriverName) != "" {
		if vt := driverVehicleMap[strings.ToLower(strings.TrimSpace(d.DriverName))]; vt != "" {
			d.VehicleType = vt
		}
	}

	c.JSON(http.StatusOK, d)
}

// POST /api/return-settings
func CreateReturnSetting(c *gin.Context) {
	var input ReturnSetting
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("CreateReturnSetting bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	table := "return_settings"
	if !hasTable(config.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel return_settings tidak ditemukan"})
		return
	}

	if strings.TrimSpace(input.VehicleType) == "" && strings.TrimSpace(input.DriverName) != "" && hasColumn(config.DB, table, "vehicle_type") {
		input.VehicleType = lookupDriverVehicleType(input.DriverName)
	}

	count, _ := strconv.Atoi(input.PassengerCount)

	cols := []string{}
	vals := []any{}

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
	if hasColumn(config.DB, table, "vehicle_type") {
		cols = append(cols, "vehicle_type")
		vals = append(vals, input.VehicleType)
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
	for i, c := range cols {
		if c == "booking_id" {
			ph[i] = "NULLIF(?,0)"
		}
	}

	res, err := config.DB.Exec(
		`INSERT INTO `+table+` (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(ph, ",")+`)`,
		vals...,
	)
	if err != nil {
		log.Println("CreateReturnSetting insert error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membuat data: " + err.Error()})
		return
	}

	id, _ := res.LastInsertId()
	input.ID = int(id)
	input.PassengerCount = strconv.Itoa(count)

	if hasColumn(config.DB, table, "created_at") {
		_ = config.DB.QueryRow("SELECT COALESCE(created_at, '') FROM "+table+" WHERE id = ? LIMIT 1", id).Scan(&input.CreatedAt)
	}

	// sinkronkan driver/vehicle kepulangan ke trip & passengers (best effort)
	if input.BookingID > 0 || strings.TrimSpace(input.TripNumber) != "" {
		go syncReturnDriverVehicleFromSettings(input.BookingID, input.TripNumber, input.DriverName, input.VehicleCode, input.VehicleType)
	}

	c.JSON(http.StatusCreated, input)
}

// PUT /api/return-settings/:id
func UpdateReturnSetting(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	var input ReturnSetting
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("UpdateReturnSetting bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	table := "return_settings"
	if !hasTable(config.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel return_settings tidak ditemukan"})
		return
	}

	count, _ := strconv.Atoi(input.PassengerCount)

	sets := []string{}
	args := []any{}

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
	if hasColumn(config.DB, table, "vehicle_type") {
		sets = append(sets, "vehicle_type=?")
		args = append(args, input.VehicleType)
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
		sets = append(sets, "booking_id=NULLIF(?,0)")
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
		log.Println("UpdateReturnSetting update error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengupdate data: " + err.Error()})
		return
	}

	input.ID = id
	input.PassengerCount = strconv.Itoa(count)

	// sinkronkan driver/vehicle kepulangan ke trip & passengers (best effort)
	tripNum := strings.TrimSpace(input.TripNumber)
	if tripNum == "" {
		_ = config.DB.QueryRow(`SELECT COALESCE(trip_number,'') FROM `+table+` WHERE id=? LIMIT 1`, id).Scan(&tripNum)
	}
	if input.BookingID > 0 || tripNum != "" {
		go syncReturnDriverVehicleFromSettings(input.BookingID, tripNum, input.DriverName, input.VehicleCode, input.VehicleType)
	}

	c.JSON(http.StatusOK, input)
}

// DELETE /api/return-settings/:id
func DeleteReturnSetting(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	table := "return_settings"
	if !hasTable(config.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel return_settings tidak ditemukan"})
		return
	}

	res, err := config.DB.Exec(`DELETE FROM `+table+` WHERE id = ?`, id)
	if err != nil {
		log.Println("DeleteReturnSetting delete error:", err)
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
