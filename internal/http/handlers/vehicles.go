package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	intconfig "backend/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
)

type vehicle struct {
	ID          uint64 `json:"id"`
	VehicleCode string `json:"vehicleCode"`
	PlateNumber string `json:"plateNumber"`
	Color       string `json:"color,omitempty"`
	Kilometers  *int   `json:"kilometers,omitempty"`
	LastService string `json:"lastService,omitempty"`
}

type vehiclePayload struct {
	VehicleCode string `json:"vehicleCode" binding:"required"`
	PlateNumber string `json:"plateNumber" binding:"required"`
	Color       string `json:"color"`
	Kilometers  *int   `json:"kilometers"`
	LastService string `json:"lastService"`
}

// GET /api/vehicles?q=LK&page=1&limit=50
func GetVehicles(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	pageStr := strings.TrimSpace(c.Query("page"))
	limitStr := strings.TrimSpace(c.Query("limit"))

	var (
		rows *sql.Rows
		err  error
	)

	baseSelect := `
		SELECT 
			id,
			vehicle_code,
			plate_number,
			COALESCE(color,'') AS color,
			kilometers,
			CASE 
				WHEN last_service IS NULL THEN NULL
				ELSE DATE_FORMAT(last_service, '%Y-%m-%d')
			END AS last_service
		FROM vehicles
	`

	where := ""
	args := []any{}
	if q != "" {
		where = " WHERE (vehicle_code LIKE ? OR plate_number LIKE ?) "
		like := "%" + q + "%"
		args = append(args, like, like)
	}

	order := " ORDER BY id DESC "

	if pageStr != "" && limitStr != "" {
		page, _ := strconv.Atoi(pageStr)
		limit, _ := strconv.Atoi(limitStr)
		if page < 1 {
			page = 1
		}
		if limit < 1 {
			limit = 50
		}
		if limit > 200 {
			limit = 200
		}
		offset := (page - 1) * limit
		query := baseSelect + where + order + " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
		rows, err = intconfig.DB.Query(query, args...)
	} else {
		query := baseSelect + where + order
		rows, err = intconfig.DB.Query(query, args...)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengambil data kendaraan: " + err.Error()})
		return
	}
	defer rows.Close()

	list := []vehicle{}
	for rows.Next() {
		var v vehicle
		var km sql.NullInt64
		var last sql.NullString
		var color string
		if err := rows.Scan(
			&v.ID,
			&v.VehicleCode,
			&v.PlateNumber,
			&color,
			&km,
			&last,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal scan data kendaraan: " + err.Error()})
			return
		}
		v.Color = color
		if km.Valid {
			x := int(km.Int64)
			v.Kilometers = &x
		} else {
			v.Kilometers = nil
		}
		if last.Valid {
			v.LastService = last.String
		} else {
			v.LastService = ""
		}
		list = append(list, v)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "error iterasi rows: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, list)
}

// POST /api/vehicles
func CreateVehicle(c *gin.Context) {
	var payload vehiclePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "data tidak valid", "detail": err.Error()})
		return
	}

	vehicleCode := strings.TrimSpace(payload.VehicleCode)
	plateNumber := strings.TrimSpace(payload.PlateNumber)
	if vehicleCode == "" || plateNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "vehicleCode dan plateNumber wajib diisi"})
		return
	}

	var lastService any = nil
	if strings.TrimSpace(payload.LastService) != "" {
		if _, err := time.Parse("2006-01-02", payload.LastService); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "format lastService harus YYYY-MM-DD"})
			return
		}
		lastService = payload.LastService
	}

	res, err := intconfig.DB.Exec(`
		INSERT INTO vehicles (vehicle_code, plate_number, color, kilometers, last_service)
		VALUES (?, ?, ?, ?, ?)
	`, vehicleCode, plateNumber, vehicleNullIfEmpty(payload.Color), payload.Kilometers, lastService)
	if err != nil {
		if me, ok := err.(*mysql.MySQLError); ok && me.Number == 1062 {
			c.JSON(http.StatusConflict, gin.H{"error": "Kode Mobil atau Plat Mobil sudah terdaftar (duplikat)."})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menambah kendaraan: " + err.Error()})
		return
	}

	id, _ := res.LastInsertId()
	c.JSON(http.StatusCreated, gin.H{"message": "kendaraan berhasil ditambahkan", "id": id})
}

// PUT /api/vehicles/:id
func UpdateVehicle(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	var payload vehiclePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "data tidak valid", "detail": err.Error()})
		return
	}

	vehicleCode := strings.TrimSpace(payload.VehicleCode)
	plateNumber := strings.TrimSpace(payload.PlateNumber)
	if vehicleCode == "" || plateNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "vehicleCode dan plateNumber wajib diisi"})
		return
	}

	var lastService any = nil
	if strings.TrimSpace(payload.LastService) != "" {
		if _, err := time.Parse("2006-01-02", payload.LastService); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "format lastService harus YYYY-MM-DD"})
			return
		}
		lastService = payload.LastService
	}

	res, err := intconfig.DB.Exec(`
		UPDATE vehicles
		SET vehicle_code = ?, plate_number = ?, color = ?, kilometers = ?, last_service = ?
		WHERE id = ?
	`, vehicleCode, plateNumber, vehicleNullIfEmpty(payload.Color), payload.Kilometers, lastService, id)
	if err != nil {
		if me, ok := err.(*mysql.MySQLError); ok && me.Number == 1062 {
			c.JSON(http.StatusConflict, gin.H{"error": "Kode Mobil atau Plat Mobil sudah terdaftar (duplikat)."})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal update kendaraan: " + err.Error()})
		return
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "kendaraan tidak ditemukan"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "kendaraan berhasil diupdate"})
}

// DELETE /api/vehicles/:id
func DeleteVehicle(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	res, err := intconfig.DB.Exec(`DELETE FROM vehicles WHERE id = ?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal hapus kendaraan: " + err.Error()})
		return
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "kendaraan tidak ditemukan"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "kendaraan berhasil dihapus"})
}

func vehicleNullIfEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}
