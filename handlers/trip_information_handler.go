package handlers

import (
	"backend/config"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// TripInformation: respons untuk halaman Trip Information (Informasi Perjalanan)
// Catatan:
// - departure_time optional (tergantung schema DB)
// - e_surat_jalan biasanya berupa endpoint API (mis: /api/reguler/bookings/{id}/surat-jalan)
type TripInformation struct {
	ID            uint   `json:"id"`
	TripNumber    string `json:"tripNumber"`
	DepartureDate string `json:"departureDate"`
	DepartureTime string `json:"departureTime"`
	DriverName    string `json:"driverName"`
	VehicleCode   string `json:"vehicleCode"`
	LicensePlate  string `json:"licensePlate"`
	ESuratJalan   string `json:"eSuratJalan"`
}

// ==============================
// ✅ MySQL named lock (anti race condition)
// Digunakan agar create/upsert tidak bikin trip_information dobel.
// ==============================

func acquireNamedLockDB(tx *sql.Tx, key string, timeoutSec int) error {
	if tx == nil || key == "" {
		return errors.New("acquireNamedLockDB: invalid args")
	}
	var got sql.NullInt64
	if err := tx.QueryRow(`SELECT GET_LOCK(?, ?)`, key, timeoutSec).Scan(&got); err != nil {
		return err
	}
	if !got.Valid || got.Int64 != 1 {
		return errors.New("cannot acquire lock")
	}
	return nil
}

func releaseNamedLockDB(tx *sql.Tx, key string) {
	if tx == nil || key == "" {
		return
	}
	_, _ = tx.Exec(`SELECT RELEASE_LOCK(?)`, key)
}

// GET /api/trip-information
// ✅ Perubahan utama:
// - DEDUP: kalau ada baris ganda dengan trip_number yang sama, API hanya mengembalikan 1 baris (id terbesar).
// - departureTime dikirim kalau kolomnya ada.
func GetTripInformation(c *gin.Context) {
	// selalu kirim departureTime, jika kolom tidak ada => ''
	depTimeSel := `'' AS departure_time`
	if hasColumn(config.DB, "trip_information", "departure_time") {
		depTimeSel = `ti.departure_time`
	}

	query := `
		SELECT
			ti.id,
			ti.trip_number,
			ti.departure_date,
			` + depTimeSel + `,
			COALESCE(ti.driver_name,''),
			COALESCE(ti.vehicle_code,''),
			COALESCE(ti.license_plate,''),
			COALESCE(ti.e_surat_jalan,'')
		FROM trip_information ti
		JOIN (
			SELECT trip_number, MAX(id) AS id
			FROM trip_information
			GROUP BY trip_number
		) x ON x.id = ti.id
		ORDER BY ti.departure_date DESC, ti.id DESC
	`

	rows, err := config.DB.Query(query)
	if err != nil {
		log.Printf("GetTripInformation - query error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "gagal mengambil data trip_information: " + err.Error(),
		})
		return
	}
	defer rows.Close()

	trips := make([]TripInformation, 0)

	for rows.Next() {
		var t TripInformation
		var depDate sql.NullString
		var depTime sql.NullString
		var driver sql.NullString
		var vehicle sql.NullString
		var plate sql.NullString
		var eSurat sql.NullString

		if err := rows.Scan(
			&t.ID,
			&t.TripNumber,
			&depDate,
			&depTime,
			&driver,
			&vehicle,
			&plate,
			&eSurat,
		); err != nil {
			log.Printf("GetTripInformation - scan error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "gagal membaca data trip_information: " + err.Error(),
			})
			return
		}

		if depDate.Valid {
			t.DepartureDate = normalizeTripDate(depDate.String)
		}
		if depTime.Valid {
			t.DepartureTime = normalizeTripTime(depTime.String)
		}
		if driver.Valid {
			t.DriverName = driver.String
		}
		if vehicle.Valid {
			t.VehicleCode = vehicle.String
		}
		if plate.Valid {
			t.LicensePlate = plate.String
		}
		if eSurat.Valid {
			t.ESuratJalan = eSurat.String
		}

		trips = append(trips, t)
	}

	if err := rows.Err(); err != nil {
		log.Printf("GetTripInformation - rows error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "error saat iterasi trip_information: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, trips)
}

// POST /api/trip-information
// ✅ Perubahan utama:
// - Jika trip_number sudah ada, maka UPDATE record existing (bukan INSERT baru)
//   supaya tidak dobel.
func CreateTripInformation(c *gin.Context) {
	var input TripInformation
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("CreateTripInformation - bind error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "input tidak valid: " + err.Error()})
		return
	}
	if input.TripNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tripNumber wajib diisi"})
		return
	}

	// ✅ NORMALIZE (anti format aneh seperti 2025-12-22T00:00:00Z / 08:00 WIB)
	input.DepartureDate = normalizeTripDate(input.DepartureDate)
	input.DepartureTime = normalizeTripTime(input.DepartureTime)

	tx, err := config.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mulai transaksi: " + err.Error()})
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	lockKey := "trip_information:" + input.TripNumber
	if err := acquireNamedLockDB(tx, lockKey, 5); err == nil {
		defer releaseNamedLockDB(tx, lockKey)
	}

	var existingID int64
	_ = tx.QueryRow(`SELECT id FROM trip_information WHERE trip_number=? ORDER BY id DESC LIMIT 1`, input.TripNumber).Scan(&existingID)

	// kalau sudah ada, update saja
	if existingID > 0 {
		sets := []string{}
		args := []any{}

		if hasColumn(tx, "trip_information", "departure_date") {
			sets = append(sets, "departure_date=?")
			args = append(args, input.DepartureDate)
		}
		if hasColumn(tx, "trip_information", "departure_time") {
			sets = append(sets, "departure_time=?")
			args = append(args, input.DepartureTime)
		}
		if hasColumn(tx, "trip_information", "driver_name") {
			sets = append(sets, "driver_name=?")
			args = append(args, input.DriverName)
		}
		if hasColumn(tx, "trip_information", "vehicle_code") {
			sets = append(sets, "vehicle_code=?")
			args = append(args, input.VehicleCode)
		}
		if hasColumn(tx, "trip_information", "license_plate") {
			sets = append(sets, "license_plate=?")
			args = append(args, input.LicensePlate)
		}
		if hasColumn(tx, "trip_information", "e_surat_jalan") {
			sets = append(sets, "e_surat_jalan=?")
			args = append(args, input.ESuratJalan)
		}
		if hasColumn(tx, "trip_information", "updated_at") {
			sets = append(sets, "updated_at=?")
			args = append(args, time.Now())
		}

		if len(sets) > 0 {
			args = append(args, existingID)
			if _, err := tx.Exec(`UPDATE trip_information SET `+joinComma(sets)+` WHERE id=?`, args...); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal update trip_information: " + err.Error()})
				return
			}
		}

		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal commit: " + err.Error()})
			return
		}
		committed = true
		input.ID = uint(existingID)
		c.JSON(http.StatusOK, input)
		return
	}

	// insert baru
	cols := []string{"trip_number"}
	vals := []any{input.TripNumber}

	if hasColumn(tx, "trip_information", "departure_date") {
		cols = append(cols, "departure_date")
		vals = append(vals, input.DepartureDate)
	}
	if hasColumn(tx, "trip_information", "departure_time") {
		cols = append(cols, "departure_time")
		vals = append(vals, input.DepartureTime)
	}
	if hasColumn(tx, "trip_information", "driver_name") {
		cols = append(cols, "driver_name")
		vals = append(vals, input.DriverName)
	}
	if hasColumn(tx, "trip_information", "vehicle_code") {
		cols = append(cols, "vehicle_code")
		vals = append(vals, input.VehicleCode)
	}
	if hasColumn(tx, "trip_information", "license_plate") {
		cols = append(cols, "license_plate")
		vals = append(vals, input.LicensePlate)
	}
	if hasColumn(tx, "trip_information", "e_surat_jalan") {
		cols = append(cols, "e_surat_jalan")
		vals = append(vals, input.ESuratJalan)
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

	res, err := tx.Exec(`INSERT INTO trip_information (`+joinComma(cols)+`) VALUES (`+joinComma(ph)+`)`, vals...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal insert trip_information: " + err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	input.ID = uint(id)

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal commit: " + err.Error()})
		return
	}
	committed = true

	c.JSON(http.StatusCreated, input)
}

// PUT /api/trip-information/:id
func UpdateTripInformation(c *gin.Context) {
	id := c.Param("id")

	var input TripInformation
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("UpdateTripInformation - bind error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "input tidak valid: " + err.Error()})
		return
	}

	// ✅ NORMALIZE (anti format aneh)
	input.DepartureDate = normalizeTripDate(input.DepartureDate)
	input.DepartureTime = normalizeTripTime(input.DepartureTime)

	sets := []string{}
	args := []any{}

	// jangan ubah trip_number di update by id (biar aman)
	if hasColumn(config.DB, "trip_information", "departure_date") {
		sets = append(sets, "departure_date=?")
		args = append(args, input.DepartureDate)
	}
	if hasColumn(config.DB, "trip_information", "departure_time") {
		sets = append(sets, "departure_time=?")
		args = append(args, input.DepartureTime)
	}
	if hasColumn(config.DB, "trip_information", "driver_name") {
		sets = append(sets, "driver_name=?")
		args = append(args, input.DriverName)
	}
	if hasColumn(config.DB, "trip_information", "vehicle_code") {
		sets = append(sets, "vehicle_code=?")
		args = append(args, input.VehicleCode)
	}
	if hasColumn(config.DB, "trip_information", "license_plate") {
		sets = append(sets, "license_plate=?")
		args = append(args, input.LicensePlate)
	}
	if hasColumn(config.DB, "trip_information", "e_surat_jalan") {
		sets = append(sets, "e_surat_jalan=?")
		args = append(args, input.ESuratJalan)
	}
	if hasColumn(config.DB, "trip_information", "updated_at") {
		sets = append(sets, "updated_at=?")
		args = append(args, time.Now())
	}

	if len(sets) == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "tidak ada kolom yang bisa diupdate"})
		return
	}

	args = append(args, id)
	_, err := config.DB.Exec(`UPDATE trip_information SET `+joinComma(sets)+` WHERE id=?`, args...)
	if err != nil {
		log.Printf("UpdateTripInformation - DB update error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "gagal mengupdate trip_information: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "trip_information terupdate"})
}

// DELETE /api/trip-information/:id
func DeleteTripInformation(c *gin.Context) {
	id := c.Param("id")

	_, err := config.DB.Exec("DELETE FROM trip_information WHERE id = ?", id)
	if err != nil {
		log.Printf("DeleteTripInformation - DB delete error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "gagal menghapus trip_information: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "trip_information terhapus"})
}

// helper kecil supaya tidak import strings hanya untuk Join
func joinComma(arr []string) string {
	if len(arr) == 0 {
		return ""
	}
	out := arr[0]
	for i := 1; i < len(arr); i++ {
		out += ", " + arr[i]
	}
	return out
}
