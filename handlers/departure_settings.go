// backend/handlers/departure_settings.go
package handlers

import (
	"backend/config"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type DepartureSetting struct {
	ID              int    `json:"id"`
	BookingName     string `json:"bookingName"`
	Phone           string `json:"phone"`
	PickupAddress   string `json:"pickupAddress"`
	DepartureDate   string `json:"departureDate"`
	SeatNumbers     string `json:"seatNumbers"`
	PassengerCount  string `json:"passengerCount"`
	ServiceType     string `json:"serviceType"`
	DriverName      string `json:"driverName"`
	VehicleCode     string `json:"vehicleCode"`
	SuratJalanFile  string `json:"suratJalanFile"`
	SuratJalanFileName string `json:"suratJalanFileName"`
	DepartureStatus string `json:"departureStatus"`
	CreatedAt       string `json:"createdAt"`
}

// GET /api/departure-settings
func GetDepartureSettings(c *gin.Context) {
	rows, err := config.DB.Query(`
		SELECT
			id,
			booking_name,
			phone,
			COALESCE(pickup_address, ''),
			COALESCE(departure_date, ''),
			COALESCE(seat_numbers, ''),
			COALESCE(passenger_count, 0),
			service_type,
			COALESCE(driver_name, ''),
			COALESCE(vehicle_code, ''),
			COALESCE(surat_jalan_file, ''),
			COALESCE(surat_jalan_file_name, ''),
			COALESCE(departure_status, ''),
			COALESCE(created_at, '')
		FROM departure_settings
		ORDER BY id DESC
	`)
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
		); err != nil {
			log.Println("GetDepartureSettings scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data: " + err.Error()})
			return
		}
		d.PassengerCount = strconv.Itoa(countInt)
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

	count, _ := strconv.Atoi(input.PassengerCount)

	res, err := config.DB.Exec(`
		INSERT INTO departure_settings
			(booking_name, phone, pickup_address, departure_date,
			 seat_numbers, passenger_count, service_type,
			 driver_name, vehicle_code, surat_jalan_file,
			 surat_jalan_file_name, departure_status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		input.BookingName,
		input.Phone,
		input.PickupAddress,
		nullIfEmpty(input.DepartureDate),
		input.SeatNumbers,
		count,
		input.ServiceType,
		input.DriverName,
		input.VehicleCode,
		input.SuratJalanFile,
		input.SuratJalanFileName,
		input.DepartureStatus,
	)
	if err != nil {
		log.Println("CreateDepartureSetting insert error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membuat data: " + err.Error()})
		return
	}

	id, _ := res.LastInsertId()
	input.ID = int(id)
	input.PassengerCount = strconv.Itoa(count)
	_ = config.DB.QueryRow("SELECT COALESCE(created_at, '') FROM departure_settings WHERE id = ?", id).
		Scan(&input.CreatedAt)

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

	count, _ := strconv.Atoi(input.PassengerCount)

	_, err = config.DB.Exec(`
		UPDATE departure_settings
		SET
			booking_name    = ?,
			phone           = ?,
			pickup_address  = ?,
			departure_date  = ?,
			seat_numbers    = ?,
			passenger_count = ?,
			service_type    = ?,
			driver_name     = ?,
			vehicle_code    = ?,
			surat_jalan_file = ?,
			surat_jalan_file_name = ?,
			departure_status = ?
		WHERE id = ?
	`,
		input.BookingName,
		input.Phone,
		input.PickupAddress,
		nullIfEmpty(input.DepartureDate),
		input.SeatNumbers,
		count,
		input.ServiceType,
		input.DriverName,
		input.VehicleCode,
		input.SuratJalanFile,
		input.SuratJalanFileName,
		input.DepartureStatus,
		id,
	)
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

	res, err := config.DB.Exec(`DELETE FROM departure_settings WHERE id = ?`, id)
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
