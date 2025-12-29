// backend/handlers/payment_validations.go
package handlers

import (
	"backend/config"
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// NOTE:
// Jangan auto-sync ke departure_settings/return_settings saat admin hanya meng-edit data validasi.
// Sync ke Pengaturan Keberangkatan/Kepulangan HARUS terjadi saat endpoint Approve dipanggil.
// Jika diaktifkan, update manual status=paid + trip_role terisi bisa menyebabkan data dobel.
const autoSyncOnManualPaidEdit = false

type PaymentValidation struct {
	ID        int64  `json:"id"`
	BookingID *int64 `json:"bookingId,omitempty"`

	CustomerName  string `json:"customerName"`
	CustomerPhone string `json:"customerPhone"`
	PickupAddress string `json:"pickupAddress"`
	Origin        string `json:"origin"`
	Destination   string `json:"destination"`
	BookingDate   string `json:"bookingDate"`

	PaymentMethod string `json:"paymentMethod"`
	PaymentStatus string `json:"paymentStatus"`
	TripRole      string `json:"tripRole"`

	ProofFile     string `json:"proofFile"`
	ProofFileName string `json:"proofFileName"`

	CreatedAt string `json:"createdAt"`
}

func isValidationPaid(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return s == "sukses" || s == "lunas" || s == "paid" || s == "approve" || s == "approved" || s == "pembayaran sukses"
}

// Normalisasi role supaya konsisten dengan booking_sync.go (Keberangkatan/Kepulangan)
func normalizeTripRolePV(role string) string {
	r := strings.ToLower(strings.TrimSpace(role))
	switch r {
	case "keberangkatan", "berangkat":
		return "Keberangkatan"
	case "kepulangan", "pulang":
		return "Kepulangan"
	default:
		return strings.TrimSpace(role)
	}
}

// cek apakah setting sudah ada untuk booking tertentu pada table tertentu
// - support booking_id atau reguler_booking_id
// - kalau table punya kolom trip_role, akan cek per-role (lebih aman)
func settingExistsForBooking(tx *sql.Tx, table string, bookingID int64, role string) (bool, error) {
	if tx == nil || bookingID <= 0 {
		return false, nil
	}
	if !hasTable(tx, table) {
		return false, nil
	}

	bookingCol := ""
	if hasColumn(tx, table, "booking_id") {
		bookingCol = "booking_id"
	} else if hasColumn(tx, table, "reguler_booking_id") {
		bookingCol = "reguler_booking_id"
	} else {
		// tidak bisa cek idempotensi
		return false, nil
	}

	role = strings.TrimSpace(role)
	if role != "" && hasColumn(tx, table, "trip_role") {
		var one int
		err := tx.QueryRow(
			`SELECT 1 FROM `+table+` WHERE `+bookingCol+`=? AND LOWER(COALESCE(trip_role,''))=LOWER(?) LIMIT 1`,
			bookingID, role,
		).Scan(&one)
		if err == sql.ErrNoRows {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	}

	var one int
	err := tx.QueryRow(
		`SELECT 1 FROM `+table+` WHERE `+bookingCol+`=? LIMIT 1`,
		bookingID,
	).Scan(&one)

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GET /api/payment-validations
func GetPaymentValidations(c *gin.Context) {
	rows, err := config.DB.Query(`
		SELECT 
			id,
			COALESCE(customer_name,''),
			COALESCE(customer_phone,''),
			COALESCE(pickup_address,''),
			COALESCE(origin,''),
			COALESCE(destination,''),
			COALESCE(booking_date,''),
			COALESCE(payment_method,''),
			COALESCE(payment_status,''),
			COALESCE(trip_role,''),
			COALESCE(proof_file,''),
			COALESCE(proof_file_name,''),
			COALESCE(created_at,'')
		FROM payment_validations
		ORDER BY id DESC
	`)
	if err != nil {
		log.Println("GetPaymentValidations query error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengambil data: " + err.Error()})
		return
	}
	defer rows.Close()

	list := []PaymentValidation{}
	for rows.Next() {
		var pv PaymentValidation
		if err := rows.Scan(
			&pv.ID,
			&pv.CustomerName,
			&pv.CustomerPhone,
			&pv.PickupAddress,
			&pv.Origin,
			&pv.Destination,
			&pv.BookingDate,
			&pv.PaymentMethod,
			&pv.PaymentStatus,
			&pv.TripRole,
			&pv.ProofFile,
			&pv.ProofFileName,
			&pv.CreatedAt,
		); err != nil {
			log.Println("GetPaymentValidations scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data: " + err.Error()})
			return
		}
		list = append(list, pv)
	}

	c.JSON(http.StatusOK, list)
}

// POST /api/payment-validations
func CreatePaymentValidation(c *gin.Context) {
	var input PaymentValidation
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("CreatePaymentValidation bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	input.CustomerName = strings.TrimSpace(input.CustomerName)
	input.CustomerPhone = strings.TrimSpace(input.CustomerPhone)
	input.PickupAddress = strings.TrimSpace(input.PickupAddress)
	input.Origin = strings.TrimSpace(input.Origin)
	input.Destination = strings.TrimSpace(input.Destination)
	input.BookingDate = strings.TrimSpace(input.BookingDate)
	input.PaymentMethod = strings.TrimSpace(input.PaymentMethod)
	input.PaymentStatus = strings.TrimSpace(input.PaymentStatus)
	input.TripRole = strings.TrimSpace(input.TripRole)
	input.ProofFileName = strings.TrimSpace(input.ProofFileName)

	if input.PaymentStatus == "" {
		input.PaymentStatus = "Menunggu Validasi"
	}

	// ✅ REQUIREMENT:
	// Saat dibuat dari booking (umumnya Menunggu Validasi), trip_role harus kosong menunggu edit manual.
	if strings.EqualFold(strings.TrimSpace(input.PaymentStatus), "menunggu validasi") || !isValidationPaid(input.PaymentStatus) {
		input.TripRole = ""
	} else {
		input.TripRole = normalizeTripRolePV(input.TripRole)
	}

	cols := []string{
		"customer_name", "customer_phone", "pickup_address", "booking_date",
		"payment_method", "payment_status", "proof_file", "proof_file_name",
	}
	args := []any{
		input.CustomerName,
		input.CustomerPhone,
		input.PickupAddress,
		nullIfEmpty(input.BookingDate),
		input.PaymentMethod,
		input.PaymentStatus,
		input.ProofFile,
		input.ProofFileName,
	}

	if hasColumn(config.DB, "payment_validations", "booking_id") && input.BookingID != nil && *input.BookingID > 0 {
		cols = append(cols, "booking_id")
		args = append(args, *input.BookingID)
	}

	if hasColumn(config.DB, "payment_validations", "origin") {
		cols = append(cols, "origin")
		args = append(args, input.Origin)
	}
	if hasColumn(config.DB, "payment_validations", "destination") {
		cols = append(cols, "destination")
		args = append(args, input.Destination)
	}

	if hasColumn(config.DB, "payment_validations", "trip_role") {
		cols = append(cols, "trip_role")
		args = append(args, input.TripRole) // kosong saat menunggu validasi
	}

	ph := make([]string, 0, len(cols))
	for range cols {
		ph = append(ph, "?")
	}

	q := `INSERT INTO payment_validations (` + strings.Join(cols, ",") + `) VALUES (` + strings.Join(ph, ",") + `)`
	res, err := config.DB.Exec(q, args...)
	if err != nil {
		log.Println("CreatePaymentValidation insert error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membuat data: " + err.Error()})
		return
	}

	id, _ := res.LastInsertId()
	input.ID = id
	_ = config.DB.QueryRow(`SELECT COALESCE(created_at,'') FROM payment_validations WHERE id=?`, id).Scan(&input.CreatedAt)

	c.JSON(http.StatusCreated, input)
}

// DELETE /api/payment-validations/:id
func DeletePaymentValidation(c *gin.Context) {
	id64, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id64 <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

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

	// Ambil booking_id kalau ada (untuk bersihkan relasi di bookings)
	var bookingID int64
	if hasColumn(tx, "payment_validations", "booking_id") {
		if err := tx.QueryRow(`SELECT COALESCE(booking_id,0) FROM payment_validations WHERE id=? LIMIT 1`, id64).Scan(&bookingID); err != nil && err != sql.ErrNoRows {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal baca booking_id: " + err.Error()})
			return
		}
	} else if hasColumn(tx, "bookings", "payment_validation_id") {
		if err := tx.QueryRow(`SELECT COALESCE(id,0) FROM bookings WHERE payment_validation_id=? LIMIT 1`, id64).Scan(&bookingID); err != nil && err != sql.ErrNoRows {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal baca booking dari payment_validation_id: " + err.Error()})
			return
		}
	}

	// Hapus validasi
	if _, err := tx.Exec(`DELETE FROM payment_validations WHERE id=?`, id64); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menghapus data: " + err.Error()})
		return
	}

	// Bersihkan bookings agar tidak menggantung ke validasi yang sudah dihapus
	if bookingID > 0 {
		bSet := []string{}
		bArgs := []any{}

		if hasColumn(tx, "bookings", "payment_validation_id") {
			bSet = append(bSet, "payment_validation_id = NULL")
		}
		if hasColumn(tx, "bookings", "trip_role") {
			bSet = append(bSet, "trip_role = ?")
			bArgs = append(bArgs, "")
		}
		if hasColumn(tx, "bookings", "updated_at") {
			bSet = append(bSet, "updated_at = ?")
			bArgs = append(bArgs, time.Now())
		}

		if len(bSet) > 0 {
			bArgs = append(bArgs, bookingID)
			q := `UPDATE bookings SET ` + strings.Join(bSet, ", ") + ` WHERE id=?`
			if _, err := tx.Exec(q, bArgs...); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "validasi terhapus, tapi gagal update booking: " + err.Error()})
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal commit transaksi: " + err.Error()})
		return
	}
	committed = true

	c.JSON(http.StatusOK, gin.H{"message": "data validasi pembayaran berhasil dihapus"})
}

// PUT /api/payment-validations/:id
func UpdatePaymentValidation(c *gin.Context) {
	id64, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id64 <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	var input PaymentValidation
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Println("UpdatePaymentValidation bind error:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload tidak valid: " + err.Error()})
		return
	}

	input.CustomerName = strings.TrimSpace(input.CustomerName)
	input.CustomerPhone = strings.TrimSpace(input.CustomerPhone)
	input.PickupAddress = strings.TrimSpace(input.PickupAddress)
	input.Origin = strings.TrimSpace(input.Origin)
	input.Destination = strings.TrimSpace(input.Destination)
	input.BookingDate = strings.TrimSpace(input.BookingDate)
	input.PaymentMethod = strings.TrimSpace(input.PaymentMethod)
	input.PaymentStatus = strings.TrimSpace(input.PaymentStatus)
	input.TripRole = normalizeTripRolePV(input.TripRole) // ✅ boleh diisi walau masih menunggu
	input.ProofFileName = strings.TrimSpace(input.ProofFileName)

	if input.PaymentStatus == "" {
		input.PaymentStatus = "Menunggu Validasi"
	}

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

	// Ambil booking_id + payment_method lama sebagai fallback
	var bookingID int64
	var existingPayMethod sql.NullString

	if hasColumn(tx, "payment_validations", "booking_id") {
		if err := tx.QueryRow(`SELECT COALESCE(booking_id,0), COALESCE(payment_method,'') FROM payment_validations WHERE id=? LIMIT 1`, id64).
			Scan(&bookingID, &existingPayMethod); err != nil && err != sql.ErrNoRows {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal baca data lama: " + err.Error()})
			return
		}
	} else {
		if err := tx.QueryRow(`SELECT COALESCE(payment_method,'') FROM payment_validations WHERE id=? LIMIT 1`, id64).
			Scan(&existingPayMethod); err != nil && err != sql.ErrNoRows {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal baca data lama: " + err.Error()})
			return
		}
	}

	if strings.TrimSpace(input.PaymentMethod) == "" && existingPayMethod.Valid {
		input.PaymentMethod = strings.TrimSpace(existingPayMethod.String)
	}

	sets := []string{
		"customer_name    = ?",
		"customer_phone   = ?",
		"pickup_address   = ?",
		"booking_date     = ?",
		"payment_method   = ?",
		"payment_status   = ?",
		"proof_file       = ?",
		"proof_file_name  = ?",
	}
	args := []any{
		input.CustomerName,
		input.CustomerPhone,
		input.PickupAddress,
		nullIfEmpty(input.BookingDate),
		input.PaymentMethod,
		input.PaymentStatus,
		input.ProofFile,
		input.ProofFileName,
	}

	if hasColumn(tx, "payment_validations", "origin") {
		sets = append(sets, "origin = ?")
		args = append(args, input.Origin)
	}
	if hasColumn(tx, "payment_validations", "destination") {
		sets = append(sets, "destination = ?")
		args = append(args, input.Destination)
	}
	if hasColumn(tx, "payment_validations", "trip_role") {
		sets = append(sets, "trip_role = ?")
		args = append(args, input.TripRole) // ✅ tidak dihapus otomatis
	}
	if hasColumn(tx, "payment_validations", "updated_at") {
		sets = append(sets, "updated_at = ?")
		args = append(args, time.Now())
	}

	args = append(args, id64)
	if _, err := tx.Exec(`UPDATE payment_validations SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
		log.Println("UpdatePaymentValidation update error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengupdate data: " + err.Error()})
		return
	}

	// ✅ Sync ringan ke bookings (TANPA mengubah payment_status):
	// - simpan trip_role & payment_method agar approve mudah
	if bookingID > 0 {
		bSets := []string{}
		bArgs := []any{}

		if hasColumn(tx, "bookings", "payment_method") && strings.TrimSpace(input.PaymentMethod) != "" {
			bSets = append(bSets, "payment_method = ?")
			bArgs = append(bArgs, input.PaymentMethod)
		}
		if hasColumn(tx, "bookings", "trip_role") {
			bSets = append(bSets, "trip_role = ?")
			bArgs = append(bArgs, input.TripRole)
		}
		if hasColumn(tx, "bookings", "updated_at") {
			bSets = append(bSets, "updated_at = ?")
			bArgs = append(bArgs, time.Now())
		}

		if len(bSets) > 0 {
			bArgs = append(bArgs, bookingID)
			if _, err := tx.Exec(`UPDATE bookings SET `+strings.Join(bSets, ", ")+` WHERE id=?`, bArgs...); err != nil {
				log.Println("UpdatePaymentValidation booking sync error:", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal update booking: " + err.Error()})
				return
			}
		}

		// Auto-sync saat edit tetap dimatikan untuk mencegah dobel
		if autoSyncOnManualPaidEdit && isValidationPaid(input.PaymentStatus) && strings.TrimSpace(input.TripRole) != "" {
			var syncErr error
			if strings.EqualFold(input.TripRole, "kepulangan") {
				syncErr = SyncReturnBooking(tx, bookingID)
			} else {
				syncErr = SyncConfirmedRegulerBooking(tx, bookingID)
			}
			if syncErr != nil {
				log.Println("UpdatePaymentValidation sync error:", syncErr)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "update sukses, tapi gagal sinkronisasi: " + syncErr.Error()})
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal commit transaksi: " + err.Error()})
		return
	}
	committed = true

	input.ID = id64
	c.JSON(http.StatusOK, input)
}

// PUT /api/payment-validations/:id/approve
// PUT /api/payment-validations/:id/reject
func ApprovePaymentValidation(c *gin.Context) { updatePaymentValidationStatus(c, true) }
func RejectPaymentValidation(c *gin.Context)  { updatePaymentValidationStatus(c, false) }

func updatePaymentValidationStatus(c *gin.Context, approved bool) {
	validationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || validationID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id validasi tidak valid"})
		return
	}

	tx, err := config.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal mulai transaksi: " + err.Error()})
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Ambil booking_id, payment_method, trip_role dari payment_validations
	var bookingID int64
	var payMethod sql.NullString
	var tripRole string

	if hasColumn(tx, "payment_validations", "booking_id") {
		if err := tx.QueryRow(
			`SELECT COALESCE(booking_id,0), COALESCE(payment_method,''), COALESCE(trip_role,'') 
			 FROM payment_validations WHERE id=? LIMIT 1`, validationID).
			Scan(&bookingID, &payMethod, &tripRole); err != nil && err != sql.ErrNoRows {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal baca validasi: " + err.Error()})
			return
		}
	} else {
		if err := tx.QueryRow(
			`SELECT COALESCE(payment_method,''), COALESCE(trip_role,'') 
			 FROM payment_validations WHERE id=? LIMIT 1`, validationID).
			Scan(&payMethod, &tripRole); err != nil && err != sql.ErrNoRows {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal baca validasi: " + err.Error()})
			return
		}
		// fallback: cari booking_id via bookings.payment_validation_id kalau ada
		if hasColumn(tx, "bookings", "payment_validation_id") {
			if err := tx.QueryRow(`SELECT COALESCE(id,0) FROM bookings WHERE payment_validation_id=? LIMIT 1`, validationID).
				Scan(&bookingID); err != nil && err != sql.ErrNoRows {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal cari booking: " + err.Error()})
				return
			}
		}
	}

	if bookingID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "booking_id tidak ditemukan untuk validasi ini"})
		return
	}

	tripRole = normalizeTripRolePV(tripRole)

	// ✅ role wajib untuk approve (agar tidak salah kirim)
	if approved && strings.TrimSpace(tripRole) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Role trip belum diisi. Mohon edit validasi pembayaran dan pilih Keberangkatan/Kepulangan terlebih dahulu."})
		return
	}

	newPVStatus := "Ditolak"
	newBookingStatus := "Ditolak"
	if approved {
		newPVStatus = "Sukses"
		newBookingStatus = "Lunas"
	}

	// update payment_validations
	setParts := []string{"payment_status = ?"}
	args := []any{newPVStatus}

	if hasColumn(tx, "payment_validations", "trip_role") {
		setParts = append(setParts, "trip_role = ?")
		args = append(args, tripRole)
	}
	if hasColumn(tx, "payment_validations", "updated_at") {
		setParts = append(setParts, "updated_at = ?")
		args = append(args, time.Now())
	}

	args = append(args, validationID)
	if _, err := tx.Exec(
		"UPDATE payment_validations SET "+strings.Join(setParts, ", ")+" WHERE id = ?",
		args...,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal update validasi: " + err.Error()})
		return
	}

	// update bookings
	bSet := []string{}
	bArgs := []any{}

	if hasColumn(tx, "bookings", "payment_status") {
		bSet = append(bSet, "payment_status = ?")
		bArgs = append(bArgs, newBookingStatus)
	}
	if hasColumn(tx, "bookings", "payment_method") && payMethod.Valid && strings.TrimSpace(payMethod.String) != "" {
		bSet = append(bSet, "payment_method = ?")
		bArgs = append(bArgs, strings.TrimSpace(payMethod.String))
	}
	if hasColumn(tx, "bookings", "trip_role") {
		bSet = append(bSet, "trip_role = ?")
		bArgs = append(bArgs, tripRole)
	}
	if hasColumn(tx, "bookings", "updated_at") {
		bSet = append(bSet, "updated_at = ?")
		bArgs = append(bArgs, time.Now())
	}

	if len(bSet) > 0 {
		bArgs = append(bArgs, bookingID)
		if _, err := tx.Exec(
			"UPDATE bookings SET "+strings.Join(bSet, ", ")+" WHERE id = ?",
			bArgs...,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal update booking: " + err.Error()})
			return
		}
	}

	// approve => sync sesuai role, anti dobel per tabel tujuan
	if approved {
		if strings.EqualFold(tripRole, "kepulangan") {
			// ✅ hanya cek return_settings, bukan departure_settings
			exists, exErr := settingExistsForBooking(tx, "return_settings", bookingID, "Kepulangan")
			if exErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal cek return_settings: " + exErr.Error()})
				return
			}
			if !exists {
				if err := SyncReturnBooking(tx, bookingID); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": "approve sukses, tapi gagal sync return: " + err.Error()})
					return
				}
			}
		} else {
			// default: Keberangkatan
			exists, exErr := settingExistsForBooking(tx, "departure_settings", bookingID, "Keberangkatan")
			if exErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal cek departure_settings: " + exErr.Error()})
				return
			}
			if !exists {
				if err := SyncConfirmedRegulerBooking(tx, bookingID); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"message": "approve sukses, tapi gagal sync departure: " + err.Error()})
					return
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal commit transaksi: " + err.Error()})
		return
	}
	committed = true

	c.JSON(http.StatusOK, gin.H{
		"message":       "Status validasi diperbarui",
		"validationId":  validationID,
		"bookingId":     bookingID,
		"paymentStatus": newBookingStatus,
	})
}
