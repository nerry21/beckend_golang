package handlers

import (
	"log"
	"net/http"
	"strconv"

	intconfig "backend/internal/config"

	"github.com/gin-gonic/gin"
)

type Driver struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Phone          string `json:"phone"`
	Role           string `json:"role"`
	VehicleType    string `json:"vehicleType"`
	VehicleAssigned string `json:"vehicleAssigned"`
	CreatedAt      string `json:"createdAt"`
	Photo          string `json:"photo"`
}

// GET /api/drivers
func GetDrivers(c *gin.Context) {
	rows, err := intconfig.DB.Query(`
		SELECT
			id,
			COALESCE(name, ''),
			COALESCE(phone, ''),
			COALESCE(role, ''),
			COALESCE(vehicle_type, ''),
			COALESCE(vehicle_assigned, ''),
			COALESCE(DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s'), ''),
			COALESCE(photo, '')
		FROM drivers
		ORDER BY id DESC
	`)
	if err != nil {
		log.Println("GetDrivers query error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengambil data driver: " + err.Error()})
		return
	}
	defer rows.Close()

	drivers := []Driver{}
	for rows.Next() {
		var d Driver
		if err := rows.Scan(
			&d.ID,
			&d.Name,
			&d.Phone,
			&d.Role,
			&d.VehicleType,
			&d.VehicleAssigned,
			&d.CreatedAt,
			&d.Photo,
		); err != nil {
			log.Println("GetDrivers scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data driver: " + err.Error()})
			return
		}
		drivers = append(drivers, d)
	}

	if err := rows.Err(); err != nil {
		log.Println("GetDrivers rows error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data driver: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, drivers)
}

// POST /api/drivers
func CreateDriver(c *gin.Context) {
	var input Driver
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("CreateDriver bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	res, err := intconfig.DB.Exec(`
		INSERT INTO drivers (name, phone, role, vehicle_type, vehicle_assigned, photo)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		input.Name,
		input.Phone,
		input.Role,
		input.VehicleType,
		input.VehicleAssigned,
		input.Photo,
	)
	if err != nil {
		log.Println("CreateDriver insert error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membuat driver baru: " + err.Error()})
		return
	}

	id, _ := res.LastInsertId()
	input.ID = int(id)

	_ = intconfig.DB.QueryRow(`
		SELECT
			COALESCE(DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s'), ''),
			COALESCE(photo, ''),
			COALESCE(vehicle_type, ''),
			COALESCE(vehicle_assigned, '')
		FROM drivers
		WHERE id = ?
	`, id).Scan(&input.CreatedAt, &input.Photo, &input.VehicleType, &input.VehicleAssigned)

	c.JSON(http.StatusCreated, input)
}

// PUT /api/drivers/:id
func UpdateDriver(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	var input Driver
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("UpdateDriver bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	if _, err = intconfig.DB.Exec(`
		UPDATE drivers
		SET
			name             = ?,
			phone            = ?,
			role             = ?,
			vehicle_type      = ?,
			vehicle_assigned = ?,
			photo            = ?
		WHERE id = ?
	`,
		input.Name,
		input.Phone,
		input.Role,
		input.VehicleType,
		input.VehicleAssigned,
		input.Photo,
		id,
	); err != nil {
		log.Println("UpdateDriver update error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengupdate driver: " + err.Error()})
		return
	}

	var out Driver
	err = intconfig.DB.QueryRow(`
		SELECT
			id,
			COALESCE(name, ''),
			COALESCE(phone, ''),
			COALESCE(role, ''),
			COALESCE(vehicle_type, ''),
			COALESCE(vehicle_assigned, ''),
			COALESCE(DATE_FORMAT(created_at, '%Y-%m-%d %H:%i:%s'), ''),
			COALESCE(photo, '')
		FROM drivers
		WHERE id = ?
	`, id).Scan(
		&out.ID,
		&out.Name,
		&out.Phone,
		&out.Role,
		&out.VehicleType,
		&out.VehicleAssigned,
		&out.CreatedAt,
		&out.Photo,
	)
	if err != nil {
		log.Println("UpdateDriver readback error:", err)
		input.ID = id
		c.JSON(http.StatusOK, input)
		return
	}

	c.JSON(http.StatusOK, out)
}

// DELETE /api/drivers/:id
func DeleteDriver(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	res, err := intconfig.DB.Exec(`DELETE FROM drivers WHERE id = ?`, id)
	if err != nil {
		log.Println("DeleteDriver delete error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menghapus driver: " + err.Error()})
		return
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "driver tidak ditemukan"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "driver berhasil dihapus"})
}
