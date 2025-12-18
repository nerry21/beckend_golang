package handlers

import (
	"backend/config"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Driver struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	Phone           string `json:"phone"`
	Role            string `json:"role"`
	VehicleAssigned string `json:"vehicleAssigned"`
	CreatedAt       string `json:"createdAt"`
	Photo           string `json:"photo"`
}

// GET /api/drivers
func GetDrivers(c *gin.Context) {
	rows, err := config.DB.Query(`
		SELECT
			id,
			name,
			phone,
			role,
			COALESCE(vehicle_assigned, ''),
			COALESCE(created_at, '')
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
			&d.VehicleAssigned,
			&d.CreatedAt,
		); err != nil {
			log.Println("GetDrivers scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data driver: " + err.Error()})
			return
		}
		drivers = append(drivers, d)
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

	res, err := config.DB.Exec(`
		INSERT INTO drivers (name, phone, role, vehicle_assigned)
		VALUES (?, ?, ?, ?)
	`,
		input.Name,
		input.Phone,
		input.Role,
		input.VehicleAssigned,
	)
	if err != nil {
		log.Println("CreateDriver insert error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membuat driver baru: " + err.Error()})
		return
	}

	id, _ := res.LastInsertId()
	input.ID = int(id)
	_ = config.DB.QueryRow("SELECT COALESCE(created_at, '') FROM drivers WHERE id = ?", id).Scan(&input.CreatedAt)

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

	_, err = config.DB.Exec(`
		UPDATE drivers
		SET
			name             = ?,
			phone            = ?,
			role             = ?,
			vehicle_assigned = ?
		WHERE id = ?
	`,
		input.Name,
		input.Phone,
		input.Role,
		input.VehicleAssigned,
		id,
	)
	if err != nil {
		log.Println("UpdateDriver update error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengupdate driver: " + err.Error()})
		return
	}

	input.ID = id
	c.JSON(http.StatusOK, input)
}

// DELETE /api/drivers/:id
func DeleteDriver(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	res, err := config.DB.Exec(`DELETE FROM drivers WHERE id = ?`, id)
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
