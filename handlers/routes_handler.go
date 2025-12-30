package handlers

import (
	"database/sql"
	"net/http"

	"backend/config" // GANTI kalau nama module kamu bukan "backend"
	"github.com/gin-gonic/gin"
)

// struct untuk respon JSON ke frontend
type Route struct {
	ID              uint    `json:"id"`
	ServiceTypeID   uint8   `json:"service_type_id"`
	ServiceTypeCode string  `json:"service_type_code"`
	ServiceTypeName string  `json:"service_type_name"`
	Origin          string  `json:"origin"`
	Destination     string  `json:"destination"`
	BasePrice       float64 `json:"base_price"`
	Description     *string `json:"description,omitempty"`
}

// GET /api/routes
func GetRoutes(c *gin.Context) {
	query := `
        SELECT 
            r.id,
            r.service_type_id,
            st.code,
            st.name,
            r.origin,
            r.destination,
            r.base_price,
            r.description
        FROM routes r
        JOIN service_types st ON r.service_type_id = st.id
        ORDER BY r.id DESC
    `

	rows, err := config.DB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "gagal mengambil data routes: " + err.Error(),
		})
		return
	}
	defer rows.Close()

	// penting: inisialisasi slice kosong, bukan nil
	routes := make([]Route, 0)

	for rows.Next() {
		var rt Route
		var desc sql.NullString

		// scan database row ke struct
		if err := rows.Scan(
			&rt.ID,
			&rt.ServiceTypeID,
			&rt.ServiceTypeCode,
			&rt.ServiceTypeName,
			&rt.Origin,
			&rt.Destination,
			&rt.BasePrice,
			&desc,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "gagal membaca data route: " + err.Error(),
			})
			return
		}

		if desc.Valid {
			rt.Description = &desc.String
		} else {
			rt.Description = nil
		}

		routes = append(routes, rt)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "error saat iterasi data: " + err.Error(),
		})
		return
	}

	// kirim semua routes sebagai JSON
	c.JSON(http.StatusOK, routes)
}
