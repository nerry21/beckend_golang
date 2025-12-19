package handlers

import (
	"backend/config"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// SeatsString menyimpan seat dalam bentuk string, tapi bisa menerima JSON array ataupun string.
type SeatsString string

// UnmarshalJSON: bisa terima "1A, 2B" ATAU ["1A","2B"]
func (s *SeatsString) UnmarshalJSON(data []byte) error {
	// null -> kosong
	if string(data) == "null" {
		*s = ""
		return nil
	}

	// jika bentuknya array JSON
	if len(data) > 0 && data[0] == '[' {
		var arr []string
		if err := json.Unmarshal(data, &arr); err != nil {
			return err
		}
		// normalize: trim + join
		out := make([]string, 0, len(arr))
		for _, it := range arr {
			it = strings.TrimSpace(it)
			if it != "" {
				out = append(out, it)
			}
		}
		*s = SeatsString(strings.Join(out, ", "))
		return nil
	}

	// kalau bukan array, anggap string biasa
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*s = SeatsString(strings.TrimSpace(str))
	return nil
}

type Passenger struct {
	ID             int         `json:"id"`
	PassengerName  string      `json:"passengerName"`
	PassengerPhone string      `json:"passengerPhone"`
	Date           string      `json:"date"`          // "2025-12-17"
	DepartureTime  string      `json:"departureTime"` // "13:30"
	PickupAddress  string      `json:"pickupAddress"`
	DropoffAddress string      `json:"dropoffAddress"`
	TotalAmount    int64       `json:"totalAmount"`    // ✅ int64 (uang)
	SelectedSeats  SeatsString `json:"selectedSeats"`  // disimpan di DB sebagai string "1A, 2B"
	ServiceType    string      `json:"serviceType"`    // Reguler, Dropping, Rental, Paket Barang
	ETicketPhoto   string      `json:"eTicketPhoto"`   // base64 image / url / marker BOOKING:<id>
	DriverName     string      `json:"driverName"`
	VehicleCode    string      `json:"vehicleCode"`
	Notes          string      `json:"notes"`
	CreatedAt      string      `json:"createdAt"`

	// ============================
	// ✅ Tambahan untuk integrasi booking -> passenger
	// (akan terisi jika kolom tersedia di DB)
	// ============================
	BookingID          int64  `json:"bookingId,omitempty"`
	ETicketInvoiceHint string `json:"eTicketInvoiceHint,omitempty"` // contoh: "ETICKET_INVOICE_FROM_BOOKING:123"
	BookingHint        string `json:"bookingHint,omitempty"`
	SuratJalanAPI      string `json:"suratJalanApi,omitempty"`
}

// helper: kirim NULL ke DB kalau string kosong (khusus sql.NullString)
// ✅ Rename agar tidak bentrok dengan nullIfEmpty di db_helpers.go
func nullStringIfEmpty(s string) sql.NullString {
	s = strings.TrimSpace(s)
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// ==============================
// ✅ Helpers tambahan (tanpa menghapus kode lama)
// ==============================
func firstExistingColumn(db *sql.DB, table string, candidates []string) string {
	for _, cc := range candidates {
		if hasColumn(db, table, cc) {
			return cc
		}
	}
	return ""
}

func parseBookingIdFromHint(h string) int64 {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}

	// format umum dari sync: ETICKET_INVOICE_FROM_BOOKING:123
	// ambil angka setelah ":" terakhir
	last := strings.LastIndex(h, ":")
	if last < 0 || last == len(h)-1 {
		return 0
	}
	idStr := strings.TrimSpace(h[last+1:])
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func makeBookingMarker(id int64) string {
	if id <= 0 {
		return ""
	}
	return "BOOKING:" + strconv.FormatInt(id, 10)
}

// GET /api/passengers
func GetPassengers(c *gin.Context) {
	// ✅ Tambahan: deteksi kolom-kolom hint jika tersedia (tanpa menghapus query lama)
	table := "passengers"
	if hasTable(config.DB, table) {
		// kandidat berbagai kemungkinan nama kolom
		colBookingID := firstExistingColumn(config.DB, table, []string{"booking_id", "bookingId"})
		colHint := firstExistingColumn(config.DB, table, []string{"eticket_invoice_hint", "e_ticket_invoice_hint", "eticketInvoiceHint"})
		colBookingHint := firstExistingColumn(config.DB, table, []string{"booking_hint", "bookingHint"})
		colSuratJalanAPI := firstExistingColumn(config.DB, table, []string{"surat_jalan_api", "suratJalanApi", "e_surat_jalan_api"})

		// Kalau ada minimal satu kolom hint, pakai query extended
		if colBookingID != "" || colHint != "" || colBookingHint != "" || colSuratJalanAPI != "" {
			// build SELECT extra fields (COALESCE)
			extraSelect := ""
			// urutan scan harus konsisten
			if colBookingID != "" {
				extraSelect += ", COALESCE(" + colBookingID + ", 0)"
			}
			if colHint != "" {
				extraSelect += ", COALESCE(" + colHint + ", '')"
			}
			if colBookingHint != "" {
				extraSelect += ", COALESCE(" + colBookingHint + ", '')"
			}
			if colSuratJalanAPI != "" {
				extraSelect += ", COALESCE(" + colSuratJalanAPI + ", '')"
			}

			rows, err := config.DB.Query(`
				SELECT
					id,
					COALESCE(passenger_name, ''),
					COALESCE(passenger_phone, ''),
					COALESCE(date, ''),
					COALESCE(departure_time, ''),
					COALESCE(pickup_address, ''),
					COALESCE(dropoff_address, ''),
					COALESCE(total_amount, 0),
					COALESCE(selected_seats, ''),
					COALESCE(service_type, ''),
					COALESCE(eticket_photo, ''),
					COALESCE(driver_name, ''),
					COALESCE(vehicle_code, ''),
					COALESCE(notes, ''),
					COALESCE(created_at, '')
					` + extraSelect + `
				FROM passengers
				ORDER BY id DESC
			`)
			if err != nil {
				log.Println("GetPassengers (extended) query error:", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengambil data penumpang: " + err.Error()})
				return
			}
			defer rows.Close()

			passengers := []Passenger{}
			for rows.Next() {
				var p Passenger
				var seatsStr string

				// variabel scan tambahan
				var bookingID int64
				var hint, bookingHint, suratAPI string

				// buat list args scan dinamis
				scanArgs := []any{
					&p.ID,
					&p.PassengerName,
					&p.PassengerPhone,
					&p.Date,
					&p.DepartureTime,
					&p.PickupAddress,
					&p.DropoffAddress,
					&p.TotalAmount,
					&seatsStr,
					&p.ServiceType,
					&p.ETicketPhoto,
					&p.DriverName,
					&p.VehicleCode,
					&p.Notes,
					&p.CreatedAt,
				}

				if colBookingID != "" {
					scanArgs = append(scanArgs, &bookingID)
				}
				if colHint != "" {
					scanArgs = append(scanArgs, &hint)
				}
				if colBookingHint != "" {
					scanArgs = append(scanArgs, &bookingHint)
				}
				if colSuratJalanAPI != "" {
					scanArgs = append(scanArgs, &suratAPI)
				}

				if err := rows.Scan(scanArgs...); err != nil {
					log.Println("GetPassengers (extended) scan error:", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data penumpang: " + err.Error()})
					return
				}

				p.SelectedSeats = SeatsString(seatsStr)

				// set fields tambahan
				if colBookingID != "" {
					p.BookingID = bookingID
				}
				if colHint != "" {
					p.ETicketInvoiceHint = strings.TrimSpace(hint)
				}
				if colBookingHint != "" {
					p.BookingHint = strings.TrimSpace(bookingHint)
				}
				if colSuratJalanAPI != "" {
					p.SuratJalanAPI = strings.TrimSpace(suratAPI)
				}

				// ✅ Jika eticket_photo kosong tetapi ada bookingId/hint -> isi marker BOOKING:<id>
				if strings.TrimSpace(p.ETicketPhoto) == "" {
					bid := p.BookingID
					if bid <= 0 {
						bid = parseBookingIdFromHint(p.ETicketInvoiceHint)
					}
					if bid > 0 {
						p.ETicketPhoto = makeBookingMarker(bid)
					}
				}

				passengers = append(passengers, p)
			}

			if err := rows.Err(); err != nil {
				log.Println("GetPassengers (extended) rows error:", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data penumpang: " + err.Error()})
				return
			}

			c.JSON(http.StatusOK, passengers)
			return
		}
	}

	// ============================
	// ✅ KODE LAMA (TIDAK DIKURANGI) - fallback jika kolom hint tidak ada
	// ============================
	rows, err := config.DB.Query(`
		SELECT
			id,
			COALESCE(passenger_name, ''),
			COALESCE(passenger_phone, ''),
			COALESCE(date, ''),
			COALESCE(departure_time, ''),
			COALESCE(pickup_address, ''),
			COALESCE(dropoff_address, ''),
			COALESCE(total_amount, 0),
			COALESCE(selected_seats, ''),
			COALESCE(service_type, ''),
			COALESCE(eticket_photo, ''),
			COALESCE(driver_name, ''),
			COALESCE(vehicle_code, ''),
			COALESCE(notes, ''),
			COALESCE(created_at, '')
		FROM passengers
		ORDER BY id DESC
	`)
	if err != nil {
		log.Println("GetPassengers query error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengambil data penumpang: " + err.Error()})
		return
	}
	defer rows.Close()

	passengers := []Passenger{}
	for rows.Next() {
		var p Passenger
		var seatsStr string

		if err := rows.Scan(
			&p.ID,
			&p.PassengerName,
			&p.PassengerPhone,
			&p.Date,
			&p.DepartureTime,
			&p.PickupAddress,
			&p.DropoffAddress,
			&p.TotalAmount,
			&seatsStr,
			&p.ServiceType,
			&p.ETicketPhoto,
			&p.DriverName,
			&p.VehicleCode,
			&p.Notes,
			&p.CreatedAt,
		); err != nil {
			log.Println("GetPassengers scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data penumpang: " + err.Error()})
			return
		}

		p.SelectedSeats = SeatsString(seatsStr)
		passengers = append(passengers, p)
	}

	if err := rows.Err(); err != nil {
		log.Println("GetPassengers rows error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data penumpang: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, passengers)
}

// POST /api/passengers
func CreatePassenger(c *gin.Context) {
	var input Passenger
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("CreatePassenger bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	input.PassengerName = strings.TrimSpace(input.PassengerName)
	input.PassengerPhone = strings.TrimSpace(input.PassengerPhone)

	if input.PassengerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nama penumpang wajib diisi"})
		return
	}
	if input.PassengerPhone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no hp penumpang wajib diisi"})
		return
	}

	res, err := config.DB.Exec(`
		INSERT INTO passengers
			(passenger_name, passenger_phone, date, departure_time,
			 pickup_address, dropoff_address, total_amount, selected_seats,
			 service_type, eticket_photo,
			 driver_name, vehicle_code, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		input.PassengerName,
		input.PassengerPhone,
		nullStringIfEmpty(input.Date),
		nullStringIfEmpty(input.DepartureTime),
		strings.TrimSpace(input.PickupAddress),
		strings.TrimSpace(input.DropoffAddress),
		input.TotalAmount,
		string(input.SelectedSeats), // simpan ke DB sebagai string
		strings.TrimSpace(input.ServiceType),
		input.ETicketPhoto,
		strings.TrimSpace(input.DriverName),
		strings.TrimSpace(input.VehicleCode),
		strings.TrimSpace(input.Notes),
	)
	if err != nil {
		log.Println("CreatePassenger insert error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membuat penumpang baru: " + err.Error()})
		return
	}

	lastID, _ := res.LastInsertId()
	input.ID = int(lastID)
	_ = config.DB.QueryRow(
		"SELECT COALESCE(created_at, '') FROM passengers WHERE id = ?",
		lastID,
	).Scan(&input.CreatedAt)

	c.JSON(http.StatusCreated, input)
}

// PUT /api/passengers/:id
func UpdatePassenger(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	var input Passenger
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("UpdatePassenger bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	input.PassengerName = strings.TrimSpace(input.PassengerName)
	input.PassengerPhone = strings.TrimSpace(input.PassengerPhone)

	if input.PassengerName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nama penumpang wajib diisi"})
		return
	}
	if input.PassengerPhone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no hp penumpang wajib diisi"})
		return
	}

	_, err = config.DB.Exec(`
		UPDATE passengers
		SET
			passenger_name  = ?,
			passenger_phone = ?,
			date            = ?,
			departure_time  = ?,
			pickup_address  = ?,
			dropoff_address = ?,
			total_amount    = ?,
			selected_seats  = ?,
			service_type    = ?,
			eticket_photo   = ?,
			driver_name     = ?,
			vehicle_code    = ?,
			notes           = ?
		WHERE id = ?
	`,
		input.PassengerName,
		input.PassengerPhone,
		nullStringIfEmpty(input.Date),
		nullStringIfEmpty(input.DepartureTime),
		strings.TrimSpace(input.PickupAddress),
		strings.TrimSpace(input.DropoffAddress),
		input.TotalAmount,
		string(input.SelectedSeats),
		strings.TrimSpace(input.ServiceType),
		input.ETicketPhoto,
		strings.TrimSpace(input.DriverName),
		strings.TrimSpace(input.VehicleCode),
		strings.TrimSpace(input.Notes),
		id,
	)
	if err != nil {
		log.Println("UpdatePassenger update error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengupdate penumpang: " + err.Error()})
		return
	}

	input.ID = id
	c.JSON(http.StatusOK, input)
}

// DELETE /api/passengers/:id
func DeletePassenger(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	res, err := config.DB.Exec(`DELETE FROM passengers WHERE id = ?`, id)
	if err != nil {
		log.Println("DeletePassenger delete error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menghapus data penumpang: " + err.Error()})
		return
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "penumpang tidak ditemukan"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "penumpang berhasil dihapus"})
}
