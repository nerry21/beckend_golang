package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
	"backend/internal/http/middleware"
	"backend/internal/repositories"
	"backend/internal/services"
	"time"

	"github.com/gin-gonic/gin"
)

type ReturnSettingDTO = DepartureSettingDTO

// GET /api/return-settings
func GetReturnSettings(c *gin.Context) {
	table := "return_settings"
	if !intdb.HasTable(intconfig.DB, table) {
		c.JSON(http.StatusOK, []ReturnSettingDTO{})
		return
	}

	driverVehicleMap := loadDriverVehicleTypes()

	tripNoSel := "''"
	if intdb.HasColumn(intconfig.DB, table, "trip_number") {
		tripNoSel = "COALESCE(trip_number,'')"
	}
	bookingIDSel := "0"
	if intdb.HasColumn(intconfig.DB, table, "booking_id") {
		bookingIDSel = "COALESCE(booking_id,0)"
	}
	depTimeSel := "''"
	if intdb.HasColumn(intconfig.DB, table, "departure_time") {
		depTimeSel = "COALESCE(departure_time,'')"
	}
	routeFromSel := "''"
	if intdb.HasColumn(intconfig.DB, table, "route_from") {
		routeFromSel = "COALESCE(route_from,'')"
	}
	routeToSel := "''"
	if intdb.HasColumn(intconfig.DB, table, "route_to") {
		routeToSel = "COALESCE(route_to,'')"
	}
	vehicleTypeSel := "''"
	if intdb.HasColumn(intconfig.DB, table, "vehicle_type") {
		vehicleTypeSel = "COALESCE(vehicle_type,'')"
	}

	rows, err := intconfig.DB.Query(fmt.Sprintf(`
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

	var list []ReturnSettingDTO
	for rows.Next() {
		var d ReturnSettingDTO
		var countInt int

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
			&d.BookingID,
			&d.DepartureTime,
			&d.RouteFrom,
			&d.RouteTo,
			&d.VehicleType,
		); err != nil {
			log.Println("GetReturnSettings scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data: " + err.Error()})
			return
		}

		d.PassengerCount = strconv.Itoa(countInt)
		applyDepartureFallbacks((*DepartureSettingDTO)(&d), driverVehicleMap)

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
	if !intdb.HasTable(intconfig.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel return_settings tidak ditemukan"})
		return
	}

	driverVehicleMap := loadDriverVehicleTypes()

	tripNoSel := "''"
	if intdb.HasColumn(intconfig.DB, table, "trip_number") {
		tripNoSel = "COALESCE(trip_number,'')"
	}
	bookingIDSel := "0"
	if intdb.HasColumn(intconfig.DB, table, "booking_id") {
		bookingIDSel = "COALESCE(booking_id,0)"
	}
	depTimeSel := "''"
	if intdb.HasColumn(intconfig.DB, table, "departure_time") {
		depTimeSel = "COALESCE(departure_time,'')"
	}
	routeFromSel := "''"
	if intdb.HasColumn(intconfig.DB, table, "route_from") {
		routeFromSel = "COALESCE(route_from,'')"
	}
	routeToSel := "''"
	if intdb.HasColumn(intconfig.DB, table, "route_to") {
		routeToSel = "COALESCE(route_to,'')"
	}
	vehicleTypeSel := "''"
	if intdb.HasColumn(intconfig.DB, table, "vehicle_type") {
		vehicleTypeSel = "COALESCE(vehicle_type,'')"
	}

	row := intconfig.DB.QueryRow(fmt.Sprintf(`
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

	var d ReturnSettingDTO
	var countInt int
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
		&d.BookingID,
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

	d.PassengerCount = strconv.Itoa(countInt)
	applyDepartureFallbacks((*DepartureSettingDTO)(&d), driverVehicleMap)

	c.JSON(http.StatusOK, d)
}

// POST /api/return-settings
func CreateReturnSetting(c *gin.Context) {
	var input ReturnSettingDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("CreateReturnSetting bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	table := "return_settings"
	if !intdb.HasTable(intconfig.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel return_settings tidak ditemukan"})
		return
	}

	if strings.TrimSpace(input.VehicleType) == "" && strings.TrimSpace(input.DriverName) != "" && intdb.HasColumn(intconfig.DB, table, "vehicle_type") {
		input.VehicleType = lookupDriverVehicleType(input.DriverName)
	}

	count, _ := strconv.Atoi(input.PassengerCount)

	cols := []string{}
	vals := []any{}

	add := func(col string, val any) {
		if intdb.HasColumn(intconfig.DB, table, col) {
			cols = append(cols, col)
			vals = append(vals, val)
		}
	}

	add("booking_name", input.BookingName)
	add("phone", input.Phone)
	add("pickup_address", input.PickupAddress)
	add("departure_date", nullIfEmpty(input.DepartureDate))
	add("seat_numbers", input.SeatNumbers)
	if intdb.HasColumn(intconfig.DB, table, "passenger_count") {
		cols = append(cols, "passenger_count")
		vals = append(vals, count)
	}
	add("service_type", input.ServiceType)
	add("driver_name", input.DriverName)
	add("vehicle_code", input.VehicleCode)
	add("vehicle_type", input.VehicleType)
	add("surat_jalan_file", input.SuratJalanFile)
	add("surat_jalan_file_name", input.SuratJalanFileName)
	add("departure_status", input.DepartureStatus)
	add("departure_time", nullIfEmpty(input.DepartureTime))
	add("route_from", input.RouteFrom)
	add("route_to", input.RouteTo)
	add("trip_number", input.TripNumber)
	if intdb.HasColumn(intconfig.DB, table, "booking_id") {
		cols = append(cols, "booking_id")
		vals = append(vals, input.BookingID)
	}
	if intdb.HasColumn(intconfig.DB, table, "created_at") {
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
		if cols[i] == "booking_id" {
			ph[i] = "NULLIF(?,0)"
		}
	}

	res, err := intconfig.DB.Exec(
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

	if intdb.HasColumn(intconfig.DB, table, "created_at") {
		_ = intconfig.DB.QueryRow("SELECT COALESCE(created_at, '') FROM "+table+" WHERE id = ? LIMIT 1", id).Scan(&input.CreatedAt)
	}

	c.JSON(http.StatusCreated, input)
}

// PUT /api/return-settings/:id
func UpdateReturnSetting(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil || id <= 0 {
		RespondError(c, http.StatusBadRequest, "id tidak valid", err)
		return
	}

	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "gagal membaca payload", err)
		return
	}
	if len(raw) == 0 {
		RespondError(c, http.StatusBadRequest, "payload tidak boleh kosong", nil)
		return
	}

	svc := services.ReturnService{
		Repo:        repositories.ReturnRepository{},
		BookingRepo: repositories.BookingRepository{},
		SeatRepo:    repositories.BookingSeatRepository{},
		RequestID:   middleware.GetRequestID(c),
	}
	ret, err := svc.MarkPulang(id, raw)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "gagal memperbarui kepulangan", err)
		return
	}

	// ReturnService output shape sama dengan departure; gunakan mapper ulang.
	c.JSON(http.StatusOK, toDepartureDTO(ret))
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
	if !intdb.HasTable(intconfig.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel return_settings tidak ditemukan"})
		return
	}

	res, err := intconfig.DB.Exec(`DELETE FROM `+table+` WHERE id = ?`, id)
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
