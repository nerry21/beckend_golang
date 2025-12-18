package handlers

import (
    "database/sql"
    "log"
    "net/http"

    "backend/config"
    "github.com/gin-gonic/gin"
)

type TripInformation struct {
    ID            uint   `json:"id"`
    TripNumber    string `json:"tripNumber"`
    DepartureDate string `json:"departureDate"` // "YYYY-MM-DD"
    DriverName    string `json:"driverName"`
    VehicleCode   string `json:"vehicleCode"`
    LicensePlate  string `json:"licensePlate"`
    ESuratJalan   string `json:"eSuratJalan"`
}

// GET /api/trip-information
func GetTripInformation(c *gin.Context) {
    query := `
        SELECT
            id,
            trip_number,
            departure_date,
            driver_name,
            vehicle_code,
            license_plate,
            e_surat_jalan
        FROM trip_information
        ORDER BY departure_date DESC, id DESC
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
        var eSurat sql.NullString

        if err := rows.Scan(
            &t.ID,
            &t.TripNumber,
            &t.DepartureDate,
            &t.DriverName,
            &t.VehicleCode,
            &t.LicensePlate,
            &eSurat,
        ); err != nil {
            log.Printf("GetTripInformation - scan error: %v", err)
            c.JSON(http.StatusInternalServerError, gin.H{
                "error": "gagal membaca data trip_information: " + err.Error(),
            })
            return
        }

        if eSurat.Valid {
            t.ESuratJalan = eSurat.String
        } else {
            t.ESuratJalan = ""
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
func CreateTripInformation(c *gin.Context) {
    var input TripInformation
    if err := c.ShouldBindJSON(&input); err != nil {
        log.Printf("CreateTripInformation - bind error: %v", err)
        c.JSON(http.StatusBadRequest, gin.H{"error": "input tidak valid: " + err.Error()})
        return
    }

    query := `
        INSERT INTO trip_information
            (trip_number, departure_date, driver_name, vehicle_code, license_plate, e_surat_jalan)
        VALUES (?, ?, ?, ?, ?, ?)
    `

    res, err := config.DB.Exec(query,
        input.TripNumber,
        input.DepartureDate,
        input.DriverName,
        input.VehicleCode,
        input.LicensePlate,
        input.ESuratJalan,
    )
    if err != nil {
        log.Printf("CreateTripInformation - DB insert error: %v", err)
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "gagal menyimpan trip_information: " + err.Error(),
        })
        return
    }

    id, _ := res.LastInsertId()
    input.ID = uint(id)

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

    query := `
        UPDATE trip_information
        SET trip_number = ?, departure_date = ?, driver_name = ?,
            vehicle_code = ?, license_plate = ?, e_surat_jalan = ?
        WHERE id = ?
    `

    _, err := config.DB.Exec(query,
        input.TripNumber,
        input.DepartureDate,
        input.DriverName,
        input.VehicleCode,
        input.LicensePlate,
        input.ESuratJalan,
        id,
    )
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
