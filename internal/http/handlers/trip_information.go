package handlers

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"

	"github.com/gin-gonic/gin"
)

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

type TripInformationList = []TripInformation

// ==============================
// Named lock helpers
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
func GetTripInformation(c *gin.Context) {
	depTimeSel := `'' AS departure_time`
	if intdb.HasColumn(intconfig.DB, "trip_information", "departure_time") {
		depTimeSel = `ti.departure_time`
	}
	routeFromSel := "''"
	routeToSel := "''"
	if intdb.HasColumn(intconfig.DB, "trip_information", "route_from") {
		routeFromSel = "COALESCE(ti.route_from,'')"
	}
	if intdb.HasColumn(intconfig.DB, "trip_information", "route_to") {
		routeToSel = "COALESCE(ti.route_to,'')"
	}

	query := `
		SELECT
			ti.id,
			ti.trip_number,
			ti.departure_date,
			` + depTimeSel + `,
			` + routeFromSel + `,
			` + routeToSel + `,
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

	rows, err := intconfig.DB.Query(query)
	if err != nil {
		log.Printf("GetTripInformation - query error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "gagal mengambil data trip_information: " + err.Error(),
		})
		return
	}
	defer rows.Close()

	trips := make(TripInformationList, 0)

	for rows.Next() {
		var t TripInformation
		var depDate sql.NullString
		var depTime sql.NullString
		var routeFrom sql.NullString
		var routeTo sql.NullString
		var driver sql.NullString
		var vehicle sql.NullString
		var plate sql.NullString
		var eSurat sql.NullString

		if err := rows.Scan(
			&t.ID,
			&t.TripNumber,
			&depDate,
			&depTime,
			&routeFrom,
			&routeTo,
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
			t.DepartureDate = normalizeDateOnly(depDate.String)
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

		if strings.TrimSpace(t.DriverName) == "" || strings.TrimSpace(t.VehicleCode) == "" {
			rFrom := strings.TrimSpace(routeFrom.String)
			rTo := strings.TrimSpace(routeTo.String)
			driverName, vehicleCode, license := findDepartureDriverVehicleBySchedule(t.DepartureDate, t.DepartureTime, rFrom, rTo)
			if strings.TrimSpace(t.DriverName) == "" && driverName != "" {
				t.DriverName = driverName
			}
			if strings.TrimSpace(t.VehicleCode) == "" && vehicleCode != "" {
				t.VehicleCode = vehicleCode
			}
			if strings.TrimSpace(t.LicensePlate) == "" && license != "" {
				t.LicensePlate = license
			}
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

func findDepartureDriverVehicleBySchedule(dateStr, timeStr, from, to string) (string, string, string) {
	if !intdb.HasTable(intconfig.DB, "departure_settings") {
		return "", "", ""
	}

	dateOnly := normalizeDateOnly(dateStr)
	timeOnly := normalizeTripTime(timeStr)

	conds := []string{}
	args := []any{}

	if intdb.HasColumn(intconfig.DB, "departure_settings", "departure_date") && strings.TrimSpace(dateOnly) != "" {
		conds = append(conds, "DATE(COALESCE(departure_date,''))=?")
		args = append(args, dateOnly)
	}
	if intdb.HasColumn(intconfig.DB, "departure_settings", "departure_time") && strings.TrimSpace(timeOnly) != "" {
		conds = append(conds, "LEFT(COALESCE(departure_time,''),5)=?")
		args = append(args, timeOnly)
	}
	if intdb.HasColumn(intconfig.DB, "departure_settings", "route_from") && strings.TrimSpace(from) != "" {
		conds = append(conds, "LOWER(TRIM(route_from))=?")
		args = append(args, strings.ToLower(strings.TrimSpace(from)))
	}
	if intdb.HasColumn(intconfig.DB, "departure_settings", "route_to") && strings.TrimSpace(to) != "" {
		conds = append(conds, "LOWER(TRIM(route_to))=?")
		args = append(args, strings.ToLower(strings.TrimSpace(to)))
	}

	if len(conds) == 0 {
		return "", "", ""
	}

	driverSel := "''"
	if intdb.HasColumn(intconfig.DB, "departure_settings", "driver_name") {
		driverSel = "COALESCE(driver_name,'')"
	} else if intdb.HasColumn(intconfig.DB, "departure_settings", "driver") {
		driverSel = "COALESCE(driver,'')"
	}

	vehicleSel := "''"
	switch {
	case intdb.HasColumn(intconfig.DB, "departure_settings", "vehicle_code"):
		vehicleSel = "COALESCE(vehicle_code,'')"
	case intdb.HasColumn(intconfig.DB, "departure_settings", "car_code"):
		vehicleSel = "COALESCE(car_code,'')"
	case intdb.HasColumn(intconfig.DB, "departure_settings", "vehicle_type"):
		vehicleSel = "COALESCE(vehicle_type,'')"
	case intdb.HasColumn(intconfig.DB, "departure_settings", "vehicle_name"):
		vehicleSel = "COALESCE(vehicle_name,'')"
	case intdb.HasColumn(intconfig.DB, "departure_settings", "vehicle"):
		vehicleSel = "COALESCE(vehicle,'')"
	}

	plateSel := "''"
	if intdb.HasColumn(intconfig.DB, "departure_settings", "license_plate") {
		plateSel = "COALESCE(license_plate,'')"
	}

	q := `SELECT ` + driverSel + `, ` + vehicleSel + `, ` + plateSel + ` FROM departure_settings WHERE ` + strings.Join(conds, " AND ") + ` ORDER BY id DESC LIMIT 1`

	var d, v, p sql.NullString
	if err := intconfig.DB.QueryRow(q, args...).Scan(&d, &v, &p); err != nil {
		return "", "", ""
	}

	driver := strings.TrimSpace(d.String)
	vehicle := strings.TrimSpace(v.String)
	plate := strings.TrimSpace(p.String)

	if vehicle == "" && driver != "" {
		vehicle = loadDriverVehicleTypeByDriverName(driver)
	}
	return driver, vehicle, plate
}

// POST /api/trip-information
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

	input.DepartureDate = normalizeDateOnly(input.DepartureDate)
	input.DepartureTime = normalizeTripTime(input.DepartureTime)

	tx, err := intconfig.DB.Begin()
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

	if existingID > 0 {
		sets := []string{}
		args := []any{}

		if intdb.HasColumn(tx, "trip_information", "departure_date") {
			sets = append(sets, "departure_date=?")
			args = append(args, input.DepartureDate)
		}
		if intdb.HasColumn(tx, "trip_information", "departure_time") {
			sets = append(sets, "departure_time=?")
			args = append(args, input.DepartureTime)
		}
		if intdb.HasColumn(tx, "trip_information", "driver_name") {
			sets = append(sets, "driver_name=?")
			args = append(args, input.DriverName)
		}
		if intdb.HasColumn(tx, "trip_information", "vehicle_code") {
			sets = append(sets, "vehicle_code=?")
			args = append(args, input.VehicleCode)
		}
		if intdb.HasColumn(tx, "trip_information", "license_plate") {
			sets = append(sets, "license_plate=?")
			args = append(args, input.LicensePlate)
		}
		if intdb.HasColumn(tx, "trip_information", "e_surat_jalan") {
			sets = append(sets, "e_surat_jalan=?")
			args = append(args, input.ESuratJalan)
		}
		if intdb.HasColumn(tx, "trip_information", "updated_at") {
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

	cols := []string{"trip_number"}
	vals := []any{input.TripNumber}

	if intdb.HasColumn(tx, "trip_information", "departure_date") {
		cols = append(cols, "departure_date")
		vals = append(vals, input.DepartureDate)
	}
	if intdb.HasColumn(tx, "trip_information", "departure_time") {
		cols = append(cols, "departure_time")
		vals = append(vals, input.DepartureTime)
	}
	if intdb.HasColumn(tx, "trip_information", "driver_name") {
		cols = append(cols, "driver_name")
		vals = append(vals, input.DriverName)
	}
	if intdb.HasColumn(tx, "trip_information", "vehicle_code") {
		cols = append(cols, "vehicle_code")
		vals = append(vals, input.VehicleCode)
	}
	if intdb.HasColumn(tx, "trip_information", "license_plate") {
		cols = append(cols, "license_plate")
		vals = append(vals, input.LicensePlate)
	}
	if intdb.HasColumn(tx, "trip_information", "e_surat_jalan") {
		cols = append(cols, "e_surat_jalan")
		vals = append(vals, input.ESuratJalan)
	}
	if intdb.HasColumn(tx, "trip_information", "created_at") {
		cols = append(cols, "created_at")
		vals = append(vals, time.Now())
	}
	if intdb.HasColumn(tx, "trip_information", "updated_at") {
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

	input.DepartureDate = normalizeDateOnly(input.DepartureDate)
	input.DepartureTime = normalizeTripTime(input.DepartureTime)

	sets := []string{}
	args := []any{}

	if intdb.HasColumn(intconfig.DB, "trip_information", "departure_date") {
		sets = append(sets, "departure_date=?")
		args = append(args, input.DepartureDate)
	}
	if intdb.HasColumn(intconfig.DB, "trip_information", "departure_time") {
		sets = append(sets, "departure_time=?")
		args = append(args, input.DepartureTime)
	}
	if intdb.HasColumn(intconfig.DB, "trip_information", "driver_name") {
		sets = append(sets, "driver_name=?")
		args = append(args, input.DriverName)
	}
	if intdb.HasColumn(intconfig.DB, "trip_information", "vehicle_code") {
		sets = append(sets, "vehicle_code=?")
		args = append(args, input.VehicleCode)
	}
	if intdb.HasColumn(intconfig.DB, "trip_information", "license_plate") {
		sets = append(sets, "license_plate=?")
		args = append(args, input.LicensePlate)
	}
	if intdb.HasColumn(intconfig.DB, "trip_information", "e_surat_jalan") {
		sets = append(sets, "e_surat_jalan=?")
		args = append(args, input.ESuratJalan)
	}
	if intdb.HasColumn(intconfig.DB, "trip_information", "updated_at") {
		sets = append(sets, "updated_at=?")
		args = append(args, time.Now())
	}

	if len(sets) == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "tidak ada kolom yang bisa diupdate"})
		return
	}

	args = append(args, id)
	if _, err := intconfig.DB.Exec(`UPDATE trip_information SET `+joinComma(sets)+` WHERE id=?`, args...); err != nil {
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

	if _, err := intconfig.DB.Exec("DELETE FROM trip_information WHERE id = ?", id); err != nil {
		log.Printf("DeleteTripInformation - DB delete error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "gagal menghapus trip_information: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "trip_information terhapus"})
}

// GET /api/trip-information/:id/surat-jalan
func GetTripSuratJalan(c *gin.Context) {
	id64, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id64 <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	table := "trip_information"
	if !intdb.HasTable(intconfig.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel trip_information tidak ditemukan"})
		return
	}

	candidates := []string{
		"e_surat_jalan",
		"eSuratJalan",
		"surat_jalan",
		"suratJalan",
		"e_surat_jalan_json",
		"surat_jalan_json",
	}

	col := ""
	for _, cc := range candidates {
		if intdb.HasColumn(intconfig.DB, table, cc) {
			col = cc
			break
		}
	}
	if col == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "kolom surat jalan tidak ditemukan di trip_information"})
		return
	}

	var raw sql.NullString
	q := "SELECT COALESCE(" + col + ",'') FROM " + table + " WHERE id=? LIMIT 1"
	if err := intconfig.DB.QueryRow(q, id64).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "trip tidak ditemukan"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s := strings.TrimSpace(raw.String)
	if s == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "surat jalan belum tersedia"})
		return
	}

	accept := strings.ToLower(strings.TrimSpace(c.GetHeader("Accept")))

	if strings.HasPrefix(strings.ToLower(s), "data:") {
		if tryServeRawByAccept(c, accept, s) {
			return
		}
		if strings.HasPrefix(strings.ToLower(s), "data:image/") {
			c.JSON(http.StatusOK, gin.H{
				"__type": "image",
				"src":    s,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"__type": "file",
			"src":    s,
		})
		return
	}

	if json.Valid([]byte(s)) {
		var anyPayload any
		if err := json.Unmarshal([]byte(s), &anyPayload); err == nil {
			if src := extractSrcFromAny(anyPayload); src != "" && strings.HasPrefix(strings.ToLower(src), "data:") {
				if tryServeRawByAccept(c, accept, src) {
					return
				}
			}

			c.JSON(http.StatusOK, anyPayload)
			return
		}
	}

	if looksLikeBase64(s) {
		mime := detectMimeFromBase64Prefix(s)
		if mime == "" {
			mime = "image/png"
		}
		dataURL := "data:" + mime + ";base64," + s

		if tryServeRawByAccept(c, accept, dataURL) {
			return
		}

		if strings.HasPrefix(mime, "image/") {
			c.JSON(http.StatusOK, gin.H{
				"__type": "image",
				"src":    dataURL,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"__type": "file",
			"mime":   mime,
			"src":    dataURL,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"__type": "raw",
		"raw":    s,
	})
}

// Helpers for surat jalan handling

func tryServeRawByAccept(c *gin.Context, accept string, dataURL string) bool {
	mime, b64, ok := parseDataURL(dataURL)
	if !ok {
		return false
	}

	wantImage := strings.Contains(accept, "image/")
	wantPDF := strings.Contains(accept, "application/pdf")

	if strings.HasPrefix(mime, "image/") && wantImage {
		if bs, err := base64.StdEncoding.DecodeString(b64); err == nil {
			c.Data(http.StatusOK, mime, bs)
			return true
		}
	}

	if mime == "application/pdf" && wantPDF {
		if bs, err := base64.StdEncoding.DecodeString(b64); err == nil {
			c.Data(http.StatusOK, mime, bs)
			return true
		}
	}

	return false
}

func parseDataURL(s string) (mime string, b64 string, ok bool) {
	ss := strings.TrimSpace(s)
	if !strings.HasPrefix(strings.ToLower(ss), "data:") {
		return "", "", false
	}
	parts := strings.SplitN(ss, ",", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	meta := strings.ToLower(strings.TrimSpace(parts[0]))
	b64 = strings.TrimSpace(parts[1])

	mime = strings.TrimPrefix(meta, "data:")
	mime = strings.TrimSpace(mime)
	mime = strings.SplitN(mime, ";", 2)[0]
	if mime == "" {
		return "", "", false
	}
	return mime, b64, true
}

func extractSrcFromAny(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if s, ok := m["src"].(string); ok {
		return strings.TrimSpace(s)
	}
	if s, ok := m["eSuratJalan"].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func looksLikeBase64(s string) bool {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return false
	}
	if strings.ContainsAny(ss, " \n\r\t") {
		return false
	}
	if len(ss) < 80 {
		return false
	}
	return true
}

func detectMimeFromBase64Prefix(b64 string) string {
	ss := strings.TrimSpace(b64)

	if strings.HasPrefix(ss, "iVBORw0KGgo") {
		return "image/png"
	}
	if strings.HasPrefix(ss, "/9j/") {
		return "image/jpeg"
	}
	if strings.HasPrefix(ss, "R0lGOD") {
		return "image/gif"
	}
	if strings.HasPrefix(ss, "UklGR") {
		return "image/webp"
	}
	if strings.HasPrefix(ss, "JVBERi0") {
		return "application/pdf"
	}

	return ""
}

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
