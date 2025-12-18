// backend/handlers/driver_accounts.go
package handlers

import (
	"backend/config"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type DriverAccount struct {
	ID              int    `json:"id"`
	DriverName      string `json:"driverName"`
	BookingName     string `json:"bookingName"`
	Phone           string `json:"phone"`
	PickupAddress   string `json:"pickupAddress"`
	DepartureDate   string `json:"departureDate"`
	SeatNumbers     string `json:"seatNumbers"`
	PassengerCount  string `json:"passengerCount"`
	ServiceType     string `json:"serviceType"`
	PaymentMethod   string `json:"paymentMethod"`
	PaymentStatus   string `json:"paymentStatus"`
	DepartureStatus string `json:"departureStatus"`
	CreatedAt       string `json:"createdAt"`
}

// GET /api/driver-accounts
func GetDriverAccounts(c *gin.Context) {
	rows, err := config.DB.Query(`
		SELECT
			id,
			driver_name,
			booking_name,
			phone,
			COALESCE(pickup_address, ''),
			COALESCE(departure_date, ''),
			COALESCE(seat_numbers, ''),
			COALESCE(passenger_count, 0),
			service_type,
			payment_method,
			payment_status,
			departure_status,
			COALESCE(created_at, '')
		FROM driver_accounts
		ORDER BY id DESC
	`)
	if err != nil {
		log.Println("GetDriverAccounts query error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengambil data: " + err.Error()})
		return
	}
	defer rows.Close()

	var list []DriverAccount
	for rows.Next() {
		var d DriverAccount
		var countInt int
		if err := rows.Scan(
			&d.ID,
			&d.DriverName,
			&d.BookingName,
			&d.Phone,
			&d.PickupAddress,
			&d.DepartureDate,
			&d.SeatNumbers,
			&countInt,
			&d.ServiceType,
			&d.PaymentMethod,
			&d.PaymentStatus,
			&d.DepartureStatus,
			&d.CreatedAt,
		); err != nil {
			log.Println("GetDriverAccounts scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data: " + err.Error()})
			return
		}
		d.PassengerCount = strconv.Itoa(countInt)
		list = append(list, d)
	}

	c.JSON(http.StatusOK, list)
}

// POST /api/driver-accounts
func CreateDriverAccount(c *gin.Context) {
	var input DriverAccount
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("CreateDriverAccount bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	count, _ := strconv.Atoi(input.PassengerCount)

	res, err := config.DB.Exec(`
		INSERT INTO driver_accounts
			(driver_name, booking_name, phone, pickup_address,
			 departure_date, seat_numbers, passenger_count,
			 service_type, payment_method, payment_status,
			 departure_status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		input.DriverName,
		input.BookingName,
		input.Phone,
		input.PickupAddress,
		nullIfEmpty(input.DepartureDate),
		input.SeatNumbers,
		count,
		input.ServiceType,
		input.PaymentMethod,
		input.PaymentStatus,
		input.DepartureStatus,
	)
	if err != nil {
		log.Println("CreateDriverAccount insert error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membuat data: " + err.Error()})
		return
	}

	id, _ := res.LastInsertId()
	input.ID = int(id)
	input.PassengerCount = strconv.Itoa(count)
	_ = config.DB.QueryRow("SELECT COALESCE(created_at, '') FROM driver_accounts WHERE id = ?", id).
		Scan(&input.CreatedAt)

	c.JSON(http.StatusCreated, input)
}

// PUT /api/driver-accounts/:id
func UpdateDriverAccount(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	var input DriverAccount
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("UpdateDriverAccount bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	count, _ := strconv.Atoi(input.PassengerCount)

	_, err = config.DB.Exec(`
		UPDATE driver_accounts
		SET
			driver_name      = ?,
			booking_name     = ?,
			phone            = ?,
			pickup_address   = ?,
			departure_date   = ?,
			seat_numbers     = ?,
			passenger_count  = ?,
			service_type     = ?,
			payment_method   = ?,
			payment_status   = ?,
			departure_status = ?
		WHERE id = ?
	`,
		input.DriverName,
		input.BookingName,
		input.Phone,
		input.PickupAddress,
		nullIfEmpty(input.DepartureDate),
		input.SeatNumbers,
		count,
		input.ServiceType,
		input.PaymentMethod,
		input.PaymentStatus,
		input.DepartureStatus,
		id,
	)
	if err != nil {
		log.Println("UpdateDriverAccount update error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengupdate data: " + err.Error()})
		return
	}

	input.ID = id
	input.PassengerCount = strconv.Itoa(count)
	c.JSON(http.StatusOK, input)
}

// DELETE /api/driver-accounts/:id
func DeleteDriverAccount(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	res, err := config.DB.Exec(`DELETE FROM driver_accounts WHERE id = ?`, id)
	if err != nil {
		log.Println("DeleteDriverAccount delete error:", err)
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
