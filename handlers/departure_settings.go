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
	VehicleType        string `json:"vehicleType"`
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

	// Derived dari data driver (fallback jika kolom kosong)
	DriverVehicleType string `json:"-"`
}

// GET /api/departure-settings
func GetDepartureSettings(c *gin.Context) {
	table := "departure_settings"
	if !hasTable(config.DB, table) {
		c.JSON(http.StatusOK, []DepartureSetting{})
		return
	}

	// cache driver vehicle type by name (lowercase)
	driverVehicleMap := loadDriverVehicleTypes()

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
			&d.VehicleType,
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

		// fallback jenis kendaraan dari tabel drivers (jika kolom kosong)
		if strings.TrimSpace(d.VehicleType) == "" && strings.TrimSpace(d.DriverName) != "" {
			if vt := driverVehicleMap[strings.ToLower(strings.TrimSpace(d.DriverName))]; vt != "" {
				d.VehicleType = vt
			}
		}

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
	if !hasTable(config.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel departure_settings tidak ditemukan"})
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

	var d DepartureSetting
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
		log.Println("GetDepartureSettingByID scan error:", err)
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

	// fallback isi vehicleType dari tabel drivers jika kosong
	if strings.TrimSpace(input.VehicleType) == "" && strings.TrimSpace(input.DriverName) != "" && hasColumn(config.DB, table, "vehicle_type") {
		input.VehicleType = lookupDriverVehicleType(input.DriverName)
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

	// バ. Sync ke akun driver (best effort, tidak blokir respon)
	go syncDepartureToDriverAccount(input)
	// バ. Sync ke trips (laporan keuangan) supaya finansial ikut terisi
	go syncDepartureToTrips(input)

	// ✅ ambil data terbaru dari DB untuk kebutuhan sync (booking_id/trip_number bisa tidak ada di payload)
	syncDep := input
	if dep, derr := getDepartureSettingForSyncByID(input.ID); derr == nil {
		syncDep = dep
	}

	go func(dep DepartureSetting) {
		if err := SyncAfterDepartureBerangkat(dep); err != nil {
			log.Println("[SYNC BERANGKAT] error:", err)
		}
	}(syncDep)

	c.JSON(http.StatusCreated, input)
}

// PUT /api/departure-settings/:id
func UpdateDepartureSetting(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil || id <= 0 {
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

	// ✅ FIX UTAMA:
	// UI saat klik "Berangkat" sering hanya mengirim sebagian field (atau field lain kosong).
	// Jika kita UPDATE langsung pakai `input`, maka kolom penting (seat_numbers, booking_id, pickup_address, dll)
	// bisa ketimpa jadi kosong/0 → SyncAfterDepartureBerangkat gagal → passengers & trip_information jadi kosong.
	existing, eerr := getDepartureSettingForSyncByID(id)
	if eerr != nil {
		if eerr == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "data tidak ditemukan"})
			return
		}
		log.Println("UpdateDepartureSetting load existing error:", eerr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data existing: " + eerr.Error()})
		return
	}

	// merge: hanya overwrite jika payload mengisi nilai non-kosong (bookingId hanya jika >0)
	merged := existing

	if strings.TrimSpace(input.BookingName) != "" {
		merged.BookingName = input.BookingName
	}
	if strings.TrimSpace(input.Phone) != "" {
		merged.Phone = input.Phone
	}
	if strings.TrimSpace(input.PickupAddress) != "" {
		merged.PickupAddress = input.PickupAddress
	}
	if strings.TrimSpace(input.DepartureDate) != "" {
		merged.DepartureDate = input.DepartureDate
	}
	if strings.TrimSpace(input.SeatNumbers) != "" {
		merged.SeatNumbers = input.SeatNumbers
	}
	if strings.TrimSpace(input.PassengerCount) != "" {
		merged.PassengerCount = input.PassengerCount
	}
	if strings.TrimSpace(input.ServiceType) != "" {
		merged.ServiceType = input.ServiceType
	}
	if strings.TrimSpace(input.DriverName) != "" {
		merged.DriverName = input.DriverName
	}
	if strings.TrimSpace(input.VehicleCode) != "" {
		merged.VehicleCode = input.VehicleCode
	}
	if strings.TrimSpace(input.VehicleType) != "" {
		merged.VehicleType = input.VehicleType
	}
	if strings.TrimSpace(input.SuratJalanFile) != "" {
		merged.SuratJalanFile = input.SuratJalanFile
	}
	if strings.TrimSpace(input.SuratJalanFileName) != "" {
		merged.SuratJalanFileName = input.SuratJalanFileName
	}
	if strings.TrimSpace(input.DepartureStatus) != "" {
		merged.DepartureStatus = input.DepartureStatus
	}

	// kolom tambahan opsional
	if strings.TrimSpace(input.DepartureTime) != "" {
		merged.DepartureTime = input.DepartureTime
	}
	if strings.TrimSpace(input.RouteFrom) != "" {
		merged.RouteFrom = input.RouteFrom
	}
	if strings.TrimSpace(input.RouteTo) != "" {
		merged.RouteTo = input.RouteTo
	}
	if strings.TrimSpace(input.TripNumber) != "" {
		merged.TripNumber = input.TripNumber
	}

	// booking_id: hanya update kalau benar-benar dikirim valid (>0)
	if input.BookingID > 0 {
		merged.BookingID = input.BookingID
	}

	// normalisasi passenger_count
	count, _ := strconv.Atoi(strings.TrimSpace(merged.PassengerCount))
	merged.PassengerCount = strconv.Itoa(count)

	sets := []string{}
	args := []any{}

	// base kolom
	if hasColumn(config.DB, table, "booking_name") {
		sets = append(sets, "booking_name=?")
		args = append(args, merged.BookingName)
	}
	if hasColumn(config.DB, table, "phone") {
		sets = append(sets, "phone=?")
		args = append(args, merged.Phone)
	}
	if hasColumn(config.DB, table, "pickup_address") {
		sets = append(sets, "pickup_address=?")
		args = append(args, merged.PickupAddress)
	}
	if hasColumn(config.DB, table, "departure_date") {
		sets = append(sets, "departure_date=?")
		args = append(args, nullIfEmpty(merged.DepartureDate))
	}
	if hasColumn(config.DB, table, "seat_numbers") {
		sets = append(sets, "seat_numbers=?")
		args = append(args, merged.SeatNumbers)
	}
	if hasColumn(config.DB, table, "passenger_count") {
		sets = append(sets, "passenger_count=?")
		args = append(args, count)
	}
	if hasColumn(config.DB, table, "service_type") {
		sets = append(sets, "service_type=?")
		args = append(args, merged.ServiceType)
	}
	if hasColumn(config.DB, table, "driver_name") {
		sets = append(sets, "driver_name=?")
		args = append(args, merged.DriverName)
	}
	if hasColumn(config.DB, table, "vehicle_code") {
		sets = append(sets, "vehicle_code=?")
		args = append(args, merged.VehicleCode)
	}
	if hasColumn(config.DB, table, "vehicle_type") {
		sets = append(sets, "vehicle_type=?")
		args = append(args, merged.VehicleType)
	}
	if hasColumn(config.DB, table, "surat_jalan_file") {
		sets = append(sets, "surat_jalan_file=?")
		args = append(args, merged.SuratJalanFile)
	}
	if hasColumn(config.DB, table, "surat_jalan_file_name") {
		sets = append(sets, "surat_jalan_file_name=?")
		args = append(args, merged.SuratJalanFileName)
	}
	if hasColumn(config.DB, table, "departure_status") {
		sets = append(sets, "departure_status=?")
		args = append(args, merged.DepartureStatus)
	}

	// kolom tambahan opsional
	if hasColumn(config.DB, table, "departure_time") {
		sets = append(sets, "departure_time=?")
		args = append(args, nullIfEmpty(merged.DepartureTime))
	}
	if hasColumn(config.DB, table, "route_from") {
		sets = append(sets, "route_from=?")
		args = append(args, merged.RouteFrom)
	}
	if hasColumn(config.DB, table, "route_to") {
		sets = append(sets, "route_to=?")
		args = append(args, merged.RouteTo)
	}
	if hasColumn(config.DB, table, "trip_number") {
		sets = append(sets, "trip_number=?")
		args = append(args, merged.TripNumber)
	}

	// booking_id: jangan di-null-kan karena payload 0; pakai merged.BookingID
	if hasColumn(config.DB, table, "booking_id") {
		sets = append(sets, "booking_id=NULLIF(?,0)")
		args = append(args, merged.BookingID)
	}

	if hasColumn(config.DB, table, "updated_at") {
		sets = append(sets, "updated_at=?")
		args = append(args, time.Now())
	}

	if len(sets) == 0 {
		merged.ID = id
		c.JSON(http.StatusOK, merged)
		return
	}

	args = append(args, id)
	_, err = config.DB.Exec(`UPDATE `+table+` SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...)
	if err != nil {
		log.Println("UpdateDepartureSetting update error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengupdate data: " + err.Error()})
		return
	}

	merged.ID = id
	merged.PassengerCount = strconv.Itoa(count)

	// ✅ ambil data terbaru dari DB untuk kebutuhan sync (setelah UPDATE)
	syncDep := merged
	if dep, derr := getDepartureSettingForSyncByID(id); derr == nil {
		syncDep = dep
	}

	// バ. Sync ke akun driver (best effort, tidak blokir respon)
	go syncDepartureToDriverAccount(syncDep)
	// バ. Sync ke trips (laporan keuangan) supaya finansial ikut terisi
	go syncDepartureToTrips(syncDep)
	go func(dep DepartureSetting) {
		if err := SyncAfterDepartureBerangkat(dep); err != nil {
			log.Println("[SYNC BERANGKAT] error:", err)
		}
	}(syncDep)

	c.JSON(http.StatusOK, merged)
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

// getDepartureSettingForSyncByID mengambil data departure_settings versi TERBARU dari DB.
// Tujuan: untuk proses sync (Berangkat) yang membutuhkan booking_id/trip_number yang sering
// tidak ikut terkirim dari UI saat update.
func getDepartureSettingForSyncByID(id int) (DepartureSetting, error) {
	var d DepartureSetting
	if id <= 0 {
		return d, fmt.Errorf("id tidak valid")
	}

	table := "departure_settings"
	if !hasTable(config.DB, table) {
		return d, fmt.Errorf("tabel departure_settings tidak ditemukan")
	}

	// cache driver vehicle type by name (lowercase)
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
			COALESCE(passenger_count,0),
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
		WHERE id=?
		LIMIT 1
	`, tripNoSel, bookingIDSel, depTimeSel, routeFromSel, routeToSel, vehicleTypeSel, table), id)

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
		return d, err
	}

	d.BookingID = bookingID
	d.PassengerCount = strconv.Itoa(countInt)

	// ✅ Auto isi Surat Jalan bila kosong (dari trip_information / fallback booking)
	if strings.TrimSpace(d.SuratJalanFile) == "" {
		if s := strings.TrimSpace(getTripESuratJalanDB(config.DB, d.TripNumber)); s != "" {
			d.SuratJalanFile = s
		} else if d.BookingID > 0 {
			d.SuratJalanFile = buildSuratJalanAPI(d.BookingID)
		}
	}

	// fallback jenis kendaraan dari tabel drivers (jika kolom kosong)
	if strings.TrimSpace(d.VehicleType) == "" && strings.TrimSpace(d.DriverName) != "" {
		if vt := driverVehicleMap[strings.ToLower(strings.TrimSpace(d.DriverName))]; vt != "" {
			d.VehicleType = vt
		}
	}

	return d, nil
}

// syncDepartureToDriverAccount menyalin data pengaturan keberangkatan ke driver_accounts.
// Best effort: tidak gagal-kan request utama jika tabel/kolom tidak ada.
func syncDepartureToDriverAccount(dep DepartureSetting) {
	table := "driver_accounts"
	if !hasTable(config.DB, table) {
		return
	}

	if strings.TrimSpace(dep.VehicleType) == "" && strings.TrimSpace(dep.DriverName) != "" {
		dep.VehicleType = lookupDriverVehicleType(dep.DriverName)
	}

	// gunakan default pembayaran jika tidak tersedia di payload keberangkatan
	paymentMethod := "Cash"
	paymentStatus := "Belum Sukses"
	if strings.EqualFold(dep.DepartureStatus, "Berangkat") {
		paymentStatus = "Pembayaran Sukses"
	}

	count, _ := strconv.Atoi(dep.PassengerCount)

	// build payload sesuai kolom yang ada
	buildMap := func() (cols []string, vals []any) {
		if hasColumn(config.DB, table, "driver_name") {
			cols = append(cols, "driver_name")
			vals = append(vals, dep.DriverName)
		}
		if hasColumn(config.DB, table, "vehicle_type") {
			cols = append(cols, "vehicle_type")
			vals = append(vals, dep.VehicleType)
		}
		if hasColumn(config.DB, table, "booking_name") {
			cols = append(cols, "booking_name")
			vals = append(vals, dep.BookingName)
		}
		if hasColumn(config.DB, table, "phone") {
			cols = append(cols, "phone")
			vals = append(vals, dep.Phone)
		}
		if hasColumn(config.DB, table, "pickup_address") {
			cols = append(cols, "pickup_address")
			vals = append(vals, dep.PickupAddress)
		}
		if hasColumn(config.DB, table, "departure_date") {
			cols = append(cols, "departure_date")
			vals = append(vals, nullIfEmpty(dep.DepartureDate))
		}
		if hasColumn(config.DB, table, "seat_numbers") {
			cols = append(cols, "seat_numbers")
			vals = append(vals, dep.SeatNumbers)
		}
		if hasColumn(config.DB, table, "passenger_count") {
			cols = append(cols, "passenger_count")
			vals = append(vals, count)
		}
		if hasColumn(config.DB, table, "service_type") {
			cols = append(cols, "service_type")
			vals = append(vals, dep.ServiceType)
		}
		if hasColumn(config.DB, table, "payment_method") {
			cols = append(cols, "payment_method")
			vals = append(vals, paymentMethod)
		}
		if hasColumn(config.DB, table, "payment_status") {
			cols = append(cols, "payment_status")
			vals = append(vals, paymentStatus)
		}
		if hasColumn(config.DB, table, "departure_status") {
			cols = append(cols, "departure_status")
			vals = append(vals, dep.DepartureStatus)
		}
		if hasColumn(config.DB, table, "departure_setting_id") {
			cols = append(cols, "departure_setting_id")
			vals = append(vals, dep.ID)
		}
		return
	}

	cols, vals := buildMap()
	if len(cols) == 0 {
		return
	}

	// coba update dulu (berdasarkan departure_setting_id jika ada, jika tidak pakai kombinasi unik sederhana)
	where := []string{}
	whereArgs := []any{}
	if hasColumn(config.DB, table, "departure_setting_id") && dep.ID > 0 {
		where = append(where, "departure_setting_id=?")
		whereArgs = append(whereArgs, dep.ID)
	}
	if len(where) == 0 {
		if strings.TrimSpace(dep.BookingName) != "" && hasColumn(config.DB, table, "booking_name") {
			where = append(where, "booking_name=?")
			whereArgs = append(whereArgs, dep.BookingName)
		}
		if strings.TrimSpace(dep.DepartureDate) != "" && hasColumn(config.DB, table, "departure_date") {
			where = append(where, "departure_date=?")
			whereArgs = append(whereArgs, nullIfEmpty(dep.DepartureDate))
		}
		if strings.TrimSpace(dep.DriverName) != "" && hasColumn(config.DB, table, "driver_name") {
			where = append(where, "driver_name=?")
			whereArgs = append(whereArgs, dep.DriverName)
		}
	}

	// Jika tidak ada klausa WHERE yang aman, fallback ke insert saja
	if len(where) > 0 {
		setParts := make([]string, 0, len(cols))
		for _, c := range cols {
			setParts = append(setParts, c+"=?")
		}
		args := append([]any{}, vals...)
		args = append(args, whereArgs...)

		res, err := config.DB.Exec(
			`UPDATE `+table+` SET `+strings.Join(setParts, ", ")+` WHERE `+strings.Join(where, " AND "),
			args...,
		)
		if err == nil {
			if rows, _ := res.RowsAffected(); rows > 0 {
				return
			}
		}
	}

	// insert baru
	placeholders := make([]string, len(cols))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	_, _ = config.DB.Exec(
		`INSERT INTO `+table+` (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(placeholders, ",")+`)`,
		vals...,
	)
}

// syncDepartureToTrips melengkapi/menambah data trips (laporan keuangan) dengan driver/unit/status keberangkatan.
func syncDepartureToTrips(dep DepartureSetting) {
	table := "trips"
	if !hasTable(config.DB, table) {
		return
	}

	// hanya sinkron saat keberangkatan sudah dikonfirmasi berangkat
	if !strings.EqualFold(dep.DepartureStatus, "Berangkat") {
		return
	}

	// ambil data booking untuk isi tanggal/rute/total invoice setelah validasi pembayaran
	var booking BookingSyncPayload
	var hasBooking bool
	if dep.BookingID > 0 {
		if b, ok := loadBookingFinancePayload(dep.BookingID); ok {
			booking = b
			hasBooking = true
			if !isPaidSuccess(b.PaymentStatus, b.PaymentMethod) {
				// laporan keuangan hanya diisi kalau pembayaran sudah tervalidasi
				return
			}
		}
	}

	// merge sumber data (booking > pengaturan keberangkatan)
	depDate := strings.TrimSpace(dep.DepartureDate)
	depTime := strings.TrimSpace(dep.DepartureTime)
	depOrigin := strings.TrimSpace(dep.RouteFrom)
	depDest := strings.TrimSpace(dep.RouteTo)
	depCategory := strings.TrimSpace(dep.ServiceType)
	if hasBooking {
		if strings.TrimSpace(booking.Date) != "" {
			depDate = booking.Date
		}
		if strings.TrimSpace(booking.Time) != "" {
			depTime = booking.Time
		}
		if strings.TrimSpace(booking.From) != "" {
			depOrigin = booking.From
		}
		if strings.TrimSpace(booking.To) != "" {
			depDest = booking.To
		}
		if strings.TrimSpace(booking.Category) != "" {
			depCategory = booking.Category
		}
	}
	if depOrigin == "" {
		depOrigin = dep.PickupAddress
	}

	day, month, year := parseDayMonthYear(depDate)

	carCode := strings.ToUpper(strings.TrimSpace(dep.VehicleCode))
	if carCode == "" {
		carCode = "AUTO"
	}

	driverName := strings.TrimSpace(dep.DriverName)
	vehicleName := strings.TrimSpace(dep.VehicleType)
	if vehicleName == "" {
		vehicleName = strings.TrimSpace(dep.VehicleCode)
	}

	// agregasi invoice penumpang & jumlah kursi pada slot tanggal+jam yang sama
	agg := aggregatePaidBookings(depDate, depTime, depOrigin, depDest)

	deptPassengerCount := agg.DeptCount
	deptPassengerFare := agg.DeptTotal
	if deptPassengerCount == 0 && hasBooking {
		deptPassengerCount = len(normalizeSeatsUnique(booking.SelectedSeats))
		if deptPassengerCount == 0 {
			deptPassengerCount = 1
		}
		deptPassengerFare = booking.TotalAmount
	}
	if deptPassengerCount == 0 {
		deptPassengerCount, _ = strconv.Atoi(dep.PassengerCount)
	}

	retPassengerCount := agg.RetCount
	retPassengerFare := agg.RetTotal
	retOrigin := ""
	retDest := ""
	retCategory := depCategory
	if retPassengerCount > 0 {
		retOrigin = depDest
		retDest = depOrigin
	}

	paymentStatus := "Belum Lunas"
	if hasBooking {
		paymentStatus = strings.TrimSpace(booking.PaymentStatus)
		if paymentStatus == "" {
			paymentStatus = "Lunas"
		}
	}

	// No Order format LKT/XX/KODE (urut per hari per kode mobil)
	tripNoCandidate := strings.TrimSpace(dep.TripNumber)
	autoTripNo := ""
	if hasBooking {
		autoTripNo = autoTripNumber(depDate, depTime, depOrigin, depDest)
	}
	orderNo := tripNoCandidate
	if orderNo == "" {
		orderNo = autoTripNo
	}
	if gen := buildOrderNumberForTrip(carCode, day, month, year); gen != "" {
		orderNo = gen
	}

	var existingID int64
	if hasColumn(config.DB, table, "order_no") {
		for _, cand := range []string{orderNo, tripNoCandidate, autoTripNo, strings.TrimSpace(dep.BookingName)} {
			if cand == "" {
				continue
			}
			_ = config.DB.QueryRow(`SELECT id FROM `+table+` WHERE order_no=? LIMIT 1`, cand).Scan(&existingID)
			if existingID > 0 {
				if orderNo == "" {
					orderNo = cand
				}
				break
			}
		}
	}
	if existingID == 0 && hasColumn(config.DB, table, "car_code") && hasColumn(config.DB, table, "day") && hasColumn(config.DB, table, "month") && hasColumn(config.DB, table, "year") {
		_ = config.DB.QueryRow(
			`SELECT id FROM `+table+` WHERE car_code=? AND day=? AND month=? AND year=? ORDER BY id DESC LIMIT 1`,
			carCode, day, month, year,
		).Scan(&existingID)
	}
	// fallback: jika sudah ada baris AUTO/kosong di tanggal yang sama, pakai itu agar tidak dobel
	if existingID == 0 && hasColumn(config.DB, table, "car_code") && hasColumn(config.DB, table, "day") && hasColumn(config.DB, table, "month") && hasColumn(config.DB, table, "year") {
		_ = config.DB.QueryRow(
			`SELECT id FROM `+table+` WHERE (car_code='' OR car_code IS NULL OR UPPER(car_code)='AUTO') AND day=? AND month=? AND year=? ORDER BY id DESC LIMIT 1`,
			day, month, year,
		).Scan(&existingID)
	}

	if existingID > 0 {
		// Jangan menimpa nominal/jumlah yang sudah terisi jika hitungan baru nol
		var oldDeptFare, oldRetFare int64
		var oldDeptCount, oldRetCount int
		oldPayStatus := ""
		selCols := []string{}
		destPtrs := []any{}
		if hasColumn(config.DB, table, "dept_passenger_fare") {
			selCols = append(selCols, "COALESCE(dept_passenger_fare,0)")
			destPtrs = append(destPtrs, &oldDeptFare)
		}
		if hasColumn(config.DB, table, "ret_passenger_fare") {
			selCols = append(selCols, "COALESCE(ret_passenger_fare,0)")
			destPtrs = append(destPtrs, &oldRetFare)
		}
		if hasColumn(config.DB, table, "dept_passenger_count") {
			selCols = append(selCols, "COALESCE(dept_passenger_count,0)")
			destPtrs = append(destPtrs, &oldDeptCount)
		}
		if hasColumn(config.DB, table, "ret_passenger_count") {
			selCols = append(selCols, "COALESCE(ret_passenger_count,0)")
			destPtrs = append(destPtrs, &oldRetCount)
		}
		if hasColumn(config.DB, table, "payment_status") {
			selCols = append(selCols, "COALESCE(payment_status,'')")
			destPtrs = append(destPtrs, &oldPayStatus)
		}
		if len(selCols) > 0 {
			_ = config.DB.QueryRow(`SELECT `+strings.Join(selCols, ",")+` FROM `+table+` WHERE id=? LIMIT 1`, existingID).Scan(destPtrs...)
		}
		if deptPassengerFare == 0 && oldDeptFare > 0 {
			deptPassengerFare = oldDeptFare
		}
		if retPassengerFare == 0 && oldRetFare > 0 {
			retPassengerFare = oldRetFare
		}
		if deptPassengerCount == 0 && oldDeptCount > 0 {
			deptPassengerCount = oldDeptCount
		}
		if retPassengerCount == 0 && oldRetCount > 0 {
			retPassengerCount = oldRetCount
		}
		// jangan turunkan status yang sudah Lunas
		if strings.EqualFold(oldPayStatus, "Lunas") {
			paymentStatus = oldPayStatus
		}

		setParts := []string{}
		args := []any{}

		for col, val := range map[string]any{
			"day":                  day,
			"month":                month,
			"year":                 year,
			"car_code":             carCode,
			"vehicle_name":         vehicleName,
			"driver_name":          driverName,
			"order_no":             orderNo,
			"dept_origin":          depOrigin,
			"dept_dest":            depDest,
			"dept_category":        depCategory,
			"dept_passenger_count": deptPassengerCount,
			"dept_passenger_fare":  deptPassengerFare,
			"ret_origin":           retOrigin,
			"ret_dest":             retDest,
			"ret_category":         retCategory,
			"ret_passenger_count":  retPassengerCount,
			"ret_passenger_fare":   retPassengerFare,
			"payment_status":       paymentStatus,
		} {
			if hasColumn(config.DB, table, col) {
				setParts = append(setParts, col+"=?")
				args = append(args, val)
			}
		}

		if len(setParts) > 0 {
			args = append(args, existingID)
			_, _ = config.DB.Exec(`UPDATE `+table+` SET `+strings.Join(setParts, ", ")+` WHERE id=?`, args...)
		}
		// isi kategori kepulangan untuk trip sebelumnya jika kosong
		patchReturnCategoryForPreviousTrip(carCode, driverName, day, month, year, depCategory)
		return
	}

	// jika belum ada, insert baris baru (nominal awal mengikuti invoice; admin dihitung otomatis oleh trips handler)
	if !hasColumn(config.DB, table, "order_no") {
		return
	}

	_, _ = config.DB.Exec(`
		INSERT INTO `+table+` (
			day, month, year, car_code, vehicle_name, driver_name, order_no,
			dept_origin, dept_dest, dept_category, dept_passenger_count, dept_passenger_fare, dept_package_count, dept_package_fare,
			ret_origin, ret_dest, ret_category, ret_passenger_count, ret_passenger_fare, ret_package_count, ret_package_fare,
			other_income, bbm_fee, meal_fee, courier_fee, tol_parkir_fee, payment_status,
			dept_admin_percent_override, ret_admin_percent_override
		) VALUES (
			?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?
		)
	`,
		day, month, year, carCode, vehicleName, driverName, orderNo,
		depOrigin, depDest, depCategory, deptPassengerCount, deptPassengerFare, 0, 0,
		retOrigin, retDest, retCategory, retPassengerCount, retPassengerFare, 0, 0,
		0, 0, 0, 0, 0, paymentStatus,
		nil, nil,
	)
	// isi kategori kepulangan untuk trip sebelumnya jika kosong
	patchReturnCategoryForPreviousTrip(carCode, driverName, day, month, year, depCategory)
}

// loadDriverVehicleTypes memuat map nama driver (lowercase) -> vehicle_type dari tabel drivers
func loadDriverVehicleTypes() map[string]string {
	m := make(map[string]string)
	if !hasTable(config.DB, "drivers") {
		return m
	}
	rows, err := config.DB.Query(`SELECT COALESCE(LOWER(TRIM(name)),'') AS name_lc, COALESCE(vehicle_type,'') FROM drivers`)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var n, vt string
		if err := rows.Scan(&n, &vt); err == nil && n != "" && vt != "" {
			m[n] = vt
		}
	}
	return m
}

// lookupDriverVehicleType mencari vehicle_type berdasarkan nama driver (case-insensitive)
func lookupDriverVehicleType(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" || !hasTable(config.DB, "drivers") {
		return ""
	}
	var vt sql.NullString
	_ = config.DB.QueryRow(
		`SELECT COALESCE(vehicle_type,'') FROM drivers WHERE LOWER(TRIM(name)) = ? LIMIT 1`,
		n,
	).Scan(&vt)
	return strings.TrimSpace(vt.String)
}

// patchReturnCategoryForPreviousTrip mengisi ret_category trip sebelumnya (car_code + driver sama, tanggal <= ini) jika masih kosong.
func patchReturnCategoryForPreviousTrip(carCode, driverName string, day, month, year int, category string) {
	table := "trips"
	if strings.TrimSpace(category) == "" || !hasTable(config.DB, table) || !hasColumn(config.DB, table, "ret_category") {
		return
	}

	car := strings.ToUpper(strings.TrimSpace(carCode))
	if car == "" {
		return
	}
	driver := strings.TrimSpace(driverName)

	where := []string{"UPPER(car_code) = ?"}
	args := []any{car}

	if driver != "" && hasColumn(config.DB, table, "driver_name") {
		where = append(where, "TRIM(driver_name) = ?")
		args = append(args, driver)
	}

	if hasColumn(config.DB, table, "year") && hasColumn(config.DB, table, "month") && hasColumn(config.DB, table, "day") {
		where = append(where, "(year < ? OR (year = ? AND (month < ? OR (month = ? AND day <= ?))))")
		args = append(args, year, year, month, month, day)
	}

	q := `SELECT id, COALESCE(ret_category,'') FROM ` + table + ` WHERE ` + strings.Join(where, " AND ") + ` ORDER BY year DESC, month DESC, day DESC, id DESC LIMIT 1`

	var id int64
	var existingCat string
	if err := config.DB.QueryRow(q, args...).Scan(&id, &existingCat); err != nil || id == 0 {
		return
	}
	if strings.TrimSpace(existingCat) != "" {
		return
	}

	_, _ = config.DB.Exec(`UPDATE `+table+` SET ret_category=? WHERE id=?`, category, id)
}

// =====================
// Sync Berangkat -> passengers & trip_information
// =====================

// syncDepartureToPassengersAndTripInformation sudah digantikan dengan SyncAfterDepartureBerangkat di departure_berangkat_sync.go.
// Dibiarkan kosong untuk kompatibilitas lama; jangan dipanggil lagi.
// func syncDepartureToPassengersAndTripInformation(dep DepartureSetting) {}
