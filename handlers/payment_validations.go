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

type PaymentValidation struct {
	ID        int64  `json:"id"`
	BookingID *int64 `json:"bookingId,omitempty"`

	CustomerName  string `json:"customerName"`
	CustomerPhone string `json:"customerPhone"`
	PickupAddress string `json:"pickupAddress"`
	BookingDate   string `json:"bookingDate"`

	PaymentMethod string `json:"paymentMethod"`
	PaymentStatus string `json:"paymentStatus"`

	ProofFile     string `json:"proofFile"`
	ProofFileName string `json:"proofFileName"`

	CreatedAt string `json:"createdAt"`
}

// ===============================
// GET /api/payment-validations
// ===============================
func GetPaymentValidations(c *gin.Context) {
	hasBookingID := hasColumn(config.DB, "payment_validations", "booking_id")

	cols := []string{"id"}
	if hasBookingID {
		cols = append(cols, "booking_id")
	}
	cols = append(cols,
		"COALESCE(customer_name,'')",
		"COALESCE(customer_phone,'')",
		"COALESCE(pickup_address,'')",
		"COALESCE(booking_date,'')",
		"COALESCE(payment_method,'')",
		"COALESCE(payment_status,'')",
		"COALESCE(proof_file,'')",
		"COALESCE(proof_file_name,'')",
		"COALESCE(created_at,'')",
	)

	q := "SELECT " + strings.Join(cols, ",") + " FROM payment_validations ORDER BY id DESC"
	rows, err := config.DB.Query(q)
	if err != nil {
		log.Println("GetPaymentValidations query error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengambil data: " + err.Error()})
		return
	}
	defer rows.Close()

	list := make([]PaymentValidation, 0)
	for rows.Next() {
		var p PaymentValidation
		var bid sql.NullInt64

		dests := []any{&p.ID}
		if hasBookingID {
			dests = append(dests, &bid)
		}
		dests = append(dests,
			&p.CustomerName,
			&p.CustomerPhone,
			&p.PickupAddress,
			&p.BookingDate,
			&p.PaymentMethod,
			&p.PaymentStatus,
			&p.ProofFile,
			&p.ProofFileName,
			&p.CreatedAt,
		)

		if err := rows.Scan(dests...); err != nil {
			log.Println("GetPaymentValidations scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data: " + err.Error()})
			return
		}

		if bid.Valid {
			v := bid.Int64
			p.BookingID = &v
		}

		list = append(list, p)
	}

	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membaca data: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, list)
}

// ===============================
// POST /api/payment-validations
// ===============================
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
	input.BookingDate = strings.TrimSpace(input.BookingDate)
	input.PaymentMethod = strings.TrimSpace(input.PaymentMethod)
	input.PaymentStatus = strings.TrimSpace(input.PaymentStatus)
	input.ProofFileName = strings.TrimSpace(input.ProofFileName)

	if input.PaymentStatus == "" {
		input.PaymentStatus = "Menunggu Validasi"
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

// ===============================
// PUT /api/payment-validations/:id
// ===============================
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
	input.BookingDate = strings.TrimSpace(input.BookingDate)
	input.PaymentMethod = strings.TrimSpace(input.PaymentMethod)
	input.PaymentStatus = strings.TrimSpace(input.PaymentStatus)
	input.ProofFileName = strings.TrimSpace(input.ProofFileName)

	if input.PaymentStatus == "" {
		input.PaymentStatus = "Menunggu Validasi"
	}

	_, err = config.DB.Exec(`
		UPDATE payment_validations
		SET
			customer_name    = ?,
			customer_phone   = ?,
			pickup_address   = ?,
			booking_date     = ?,
			payment_method   = ?,
			payment_status   = ?,
			proof_file       = ?,
			proof_file_name  = ?
		WHERE id = ?
	`,
		input.CustomerName,
		input.CustomerPhone,
		input.PickupAddress,
		nullIfEmpty(input.BookingDate),
		input.PaymentMethod,
		input.PaymentStatus,
		input.ProofFile,
		input.ProofFileName,
		id64,
	)
	if err != nil {
		log.Println("UpdatePaymentValidation update error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengupdate data: " + err.Error()})
		return
	}

	input.ID = id64
	c.JSON(http.StatusOK, input)
}

// ===============================
// DELETE /api/payment-validations/:id
// ===============================
func DeletePaymentValidation(c *gin.Context) {
	id64, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id64 <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	res, err := config.DB.Exec(`DELETE FROM payment_validations WHERE id = ?`, id64)
	if err != nil {
		log.Println("DeletePaymentValidation delete error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menghapus data: " + err.Error()})
		return
	}

	aff, _ := res.RowsAffected()
	if aff == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "data tidak ditemukan"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "data berhasil dihapus"})
}

// ===============================
// PUT /api/payment-validations/:id/approve
// PUT /api/payment-validations/:id/reject
// ===============================

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
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal mulai transaksi"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	// ambil booking_id + payment_method dari payment_validations jika ada
	var (
		bookingID  int64
		payMethod  sql.NullString
		hasBookCol = hasColumn(tx, "payment_validations", "booking_id")
	)

	if hasBookCol {
		var bid sql.NullInt64
		_ = tx.QueryRow(
			`SELECT booking_id, payment_method FROM payment_validations WHERE id = ? LIMIT 1`,
			validationID,
		).Scan(&bid, &payMethod)
		if bid.Valid {
			bookingID = bid.Int64
		}
	}

	// fallback: cari dari bookings.payment_validation_id
	if bookingID <= 0 && hasColumn(tx, "bookings", "payment_validation_id") {
		_ = tx.QueryRow(
			`SELECT id FROM bookings WHERE payment_validation_id = ? LIMIT 1`,
			validationID,
		).Scan(&bookingID)
	}

	if bookingID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "booking_id tidak ditemukan untuk validasi ini"})
		return
	}

	newPVStatus := "Ditolak"
	newBookingStatus := "Ditolak"
	if approved {
		newPVStatus = "Sukses"
		// ✅ ini yang bikin invoice/e-ticket/surat jalan tampil:
		newBookingStatus = "Lunas"
	}

	// update payment_validations
	setParts := []string{"payment_status = ?"}
	args := []any{newPVStatus}

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

	// approve => sync data perjalanan + penumpang + dokumen (kalau implementasinya ada)
	if approved {
		if err := SyncConfirmedRegulerBooking(tx, bookingID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "approve sukses, tapi gagal sync: " + err.Error()})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal commit transaksi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Status validasi diperbarui",
		"validationId":  validationID,
		"bookingId":     bookingID,
		"paymentStatus": newBookingStatus, // ✅ yang dipakai UI booking sebaiknya ini
	})
}
