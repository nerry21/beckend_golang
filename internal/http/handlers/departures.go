package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
	"backend/internal/domain/models"
	"backend/internal/http/middleware"
	"backend/internal/repositories"
	"backend/internal/services"

	"github.com/gin-gonic/gin"
)

type DepartureSettingDTO struct {
	ID                 int    `json:"id"`
	BookingName        string `json:"bookingName"`
	Phone              string `json:"phone"`
	PickupAddress      string `json:"pickupAddress"`
	DepartureDate      string `json:"departureDate"`
	SeatNumbers        string `json:"seatNumbers"`
	PassengerCount     string `json:"passengerCount"`
	ServiceType        string `json:"serviceType"`
	DriverName         string `json:"driverName"`
	VehicleCode        string `json:"vehicleCode"`
	VehicleType        string `json:"vehicleType"`
	SuratJalanFile     string `json:"suratJalanFile"`
	SuratJalanFileName string `json:"suratJalanFileName"`
	DepartureStatus    string `json:"departureStatus"`
	DepartureTime      string `json:"departureTime"`
	RouteFrom          string `json:"routeFrom"`
	RouteTo            string `json:"routeTo"`
	TripNumber         string `json:"tripNumber"`
	BookingID          int64  `json:"bookingId"`
	CreatedAt          string `json:"createdAt"`
}

func toDepartureDTO(dep models.DepartureSetting) DepartureSettingDTO {
	return DepartureSettingDTO{
		ID:                 dep.ID,
		BookingName:        strings.TrimSpace(dep.BookingName),
		Phone:              strings.TrimSpace(dep.Phone),
		PickupAddress:      strings.TrimSpace(dep.PickupAddress),
		DepartureDate:      strings.TrimSpace(dep.DepartureDate),
		SeatNumbers:        strings.TrimSpace(dep.SeatNumbers),
		PassengerCount:     strings.TrimSpace(dep.PassengerCount),
		ServiceType:        strings.TrimSpace(dep.ServiceType),
		DriverName:         strings.TrimSpace(dep.DriverName),
		VehicleCode:        strings.TrimSpace(dep.VehicleCode),
		VehicleType:        strings.TrimSpace(dep.VehicleType),
		SuratJalanFile:     strings.TrimSpace(dep.SuratJalanFile),
		SuratJalanFileName: strings.TrimSpace(dep.SuratJalanFileName),
		DepartureStatus:    strings.TrimSpace(dep.DepartureStatus),
		DepartureTime:      strings.TrimSpace(dep.DepartureTime),
		RouteFrom:          strings.TrimSpace(dep.RouteFrom),
		RouteTo:            strings.TrimSpace(dep.RouteTo),
		TripNumber:         strings.TrimSpace(dep.TripNumber),
		BookingID:          dep.BookingID,
		CreatedAt:          strings.TrimSpace(dep.CreatedAt),
	}
}

// GET /api/departure-settings
func GetDepartureSettings(c *gin.Context) {
	table := "departure_settings"
	if !intdb.HasTable(intconfig.DB, table) {
		c.JSON(http.StatusOK, []DepartureSettingDTO{})
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
		log.Println("GetDepartureSettings query error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengambil data: " + err.Error()})
		return
	}
	defer rows.Close()

	var list []DepartureSettingDTO
	for rows.Next() {
		var d DepartureSettingDTO
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
			log.Println("GetDepartureSettings scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data: " + err.Error()})
			return
		}

		d.PassengerCount = strconv.Itoa(countInt)
		applyDepartureFallbacks(&d, driverVehicleMap)
		list = append(list, d)
	}

	c.JSON(http.StatusOK, list)
}

// GET /api/departure-settings/:id
func GetDepartureSettingByID(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	table := "departure_settings"
	if !intdb.HasTable(intconfig.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel departure_settings tidak ditemukan"})
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

	var d DepartureSettingDTO
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
		log.Println("GetDepartureSettingByID scan error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data: " + err.Error()})
		return
	}

	d.PassengerCount = strconv.Itoa(countInt)
	applyDepartureFallbacks(&d, driverVehicleMap)

	c.JSON(http.StatusOK, d)
}

// POST /api/departure-settings
func CreateDepartureSetting(c *gin.Context) {
	var input DepartureSettingDTO
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("CreateDepartureSetting bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	table := "departure_settings"
	if !intdb.HasTable(intconfig.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel departure_settings tidak ditemukan"})
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
		log.Println("CreateDepartureSetting insert error:", err)
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

// PUT /api/departure-settings/:id
// keep MarkBerangkat flow via service for parity with previous behavior.
func UpdateDepartureSetting(c *gin.Context) {
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

	svc := services.DepartureService{
		Repo:        repositories.DepartureRepository{DB: nil},
		BookingRepo: repositories.BookingRepository{},
		SeatRepo:    repositories.BookingSeatRepository{},
		RequestID:   middleware.GetRequestID(c),
	}
	dep, err := svc.MarkBerangkat(id, raw)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "gagal memperbarui keberangkatan", err)
		return
	}

	c.JSON(http.StatusOK, toDepartureDTO(dep))
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
	if !intdb.HasTable(intconfig.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel departure_settings tidak ditemukan"})
		return
	}

	res, err := intconfig.DB.Exec(`DELETE FROM `+table+` WHERE id = ?`, id)
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

func applyDepartureFallbacks(d *DepartureSettingDTO, driverVehicleMap map[string]string) {
	if strings.TrimSpace(d.SuratJalanFile) == "" {
		if s := strings.TrimSpace(getTripESuratJalanDB(intconfig.DB, d.TripNumber)); s != "" {
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
}

// helper: ambil e_surat_jalan dari trip_information (best effort)
func getTripESuratJalanDB(q intdb.QueryRower, tripNo string) string {
	if q == nil || strings.TrimSpace(tripNo) == "" {
		return ""
	}

	if !intdb.HasTable(q, "trip_information") {
		return ""
	}

	candidates := []string{"e_surat_jalan", "eSuratJalan", "surat_jalan", "suratJalan"}
	col := ""
	for _, cc := range candidates {
		if intdb.HasColumn(q, "trip_information", cc) {
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

func buildSuratJalanAPI(bookingID int64) string {
	return fmt.Sprintf("http://localhost:8080/api/reguler/bookings/%d/surat-jalan", bookingID)
}

// loadDriverVehicleTypes memuat map nama driver (lowercase) -> vehicle_type dari tabel drivers
func loadDriverVehicleTypes() map[string]string {
	if intconfig.DB == nil || !intdb.HasTable(intconfig.DB, "drivers") {
		return map[string]string{}
	}

	cols := []string{"name"}
	if intdb.HasColumn(intconfig.DB, "drivers", "vehicle_type") {
		cols = append(cols, "vehicle_type")
	}
	if len(cols) == 1 {
		return map[string]string{}
	}

	q := fmt.Sprintf(`SELECT %s FROM drivers`, strings.Join(cols, ","))
	rows, err := intconfig.DB.Query(q)
	if err != nil {
		log.Println("loadDriverVehicleTypes query error:", err)
		return map[string]string{}
	}
	defer rows.Close()

	type pair struct {
		Name        string
		VehicleType string
	}
	list := []pair{}

	for rows.Next() {
		var name, vt sql.NullString
		if err := rows.Scan(&name, &vt); err != nil {
			return map[string]string{}
		}
		list = append(list, pair{
			Name:        strings.TrimSpace(name.String),
			VehicleType: strings.TrimSpace(vt.String),
		})
	}

	m := map[string]string{}
	for _, p := range list {
		if p.Name == "" || p.VehicleType == "" {
			continue
		}
		m[strings.ToLower(p.Name)] = p.VehicleType
	}
	return m
}
