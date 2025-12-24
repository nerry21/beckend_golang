// backend/handlers/reguler_payment_handler.go
package handlers

import (
	"backend/config"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ===============================
// REQUEST DTO
// ===============================

type SubmitPaymentRequest struct {
	PaymentMethod string `json:"paymentMethod"` // "transfer" | "qris"
	ProofFile     string `json:"proofFile"`     // base64/data-url string
	ProofFileName string `json:"proofFileName"` // nama file bukti
}

// ===============================
// GET /api/reguler/bookings/:id
// => dipakai FE untuk cek payment status/method + info ringkas booking
// ===============================

func GetRegulerBookingDetail(c *gin.Context) {
	bookingID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || bookingID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id tidak valid"})
		return
	}

	cols := []string{
		"id",
		"COALESCE(category,'')",
		"COALESCE(route_from,'')",
		"COALESCE(route_to,'')",
		"COALESCE(trip_date,'')",
		"COALESCE(trip_time,'')",
		"COALESCE(passenger_name,'')",
		"COALESCE(passenger_count,0)",
		"COALESCE(pickup_location,'')",
		"COALESCE(dropoff_location,'')",
		"COALESCE(total,0)",
	}

	hasPayMethod := hasColumn(config.DB, "bookings", "payment_method")
	hasPayStatus := hasColumn(config.DB, "bookings", "payment_status")

	if hasPayMethod {
		cols = append(cols, "COALESCE(payment_method,'')")
	}
	if hasPayStatus {
		cols = append(cols, "COALESCE(payment_status,'')")
	}

	query := "SELECT " + strings.Join(cols, ", ") + " FROM bookings WHERE id = ? LIMIT 1"

	var (
		id             int64
		category       string
		routeFrom      string
		routeTo        string
		tripDate       string
		tripTime       string
		passengerName  string
		passengerCount int
		pickup         string
		dropoff        string
		total          int64

		paymentMethod string
		paymentStatus string
	)

	args := []any{
		&id, &category, &routeFrom, &routeTo, &tripDate, &tripTime,
		&passengerName, &passengerCount, &pickup, &dropoff, &total,
	}
	if hasPayMethod {
		args = append(args, &paymentMethod)
	}
	if hasPayStatus {
		args = append(args, &paymentStatus)
	}

	if err := config.DB.QueryRow(query, bookingID).Scan(args...); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "booking tidak ditemukan"})
		return
	}

	if !hasPayMethod {
		paymentMethod = ""
	}
	if !hasPayStatus {
		paymentStatus = "Belum Bayar"
	}

	c.JSON(http.StatusOK, gin.H{
		"id":             id,
		"category":       category,
		"routeFrom":      routeFrom,
		"routeTo":        routeTo,
		"tripDate":       tripDate,
		"tripTime":       tripTime,
		"passengerName":  passengerName,
		"passengerCount": passengerCount,
		"pickupLocation": pickup,
		"dropoffLocation": dropoff,
		"total":          total,
		"paymentMethod":  paymentMethod,
		"paymentStatus":  paymentStatus,
	})
}

// ===============================
// POST /api/reguler/bookings/:id/submit-payment
// Transfer/QRIS: buat record payment_validations + update bookings => Menunggu Validasi
// ===============================

func SubmitRegulerPaymentProof(c *gin.Context) {
	bookingID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || bookingID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id tidak valid"})
		return
	}

	var req SubmitPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "payload tidak valid"})
		return
	}

	req.PaymentMethod = strings.TrimSpace(strings.ToLower(req.PaymentMethod))
	req.ProofFile = strings.TrimSpace(req.ProofFile)
	req.ProofFileName = strings.TrimSpace(req.ProofFileName)

	if req.PaymentMethod != "transfer" && req.PaymentMethod != "qris" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "paymentMethod harus 'transfer' atau 'qris'"})
		return
	}
	if req.ProofFile == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "proofFile wajib diisi"})
		return
	}
	if req.ProofFileName == "" {
		req.ProofFileName = "bukti-pembayaran"
	}

	// Ambil info booking ringkas untuk di-copy ke payment_validations
	var (
		customerName  string
		customerPhone string
		pickup        string
		bookingDate   string
	)

	_ = config.DB.QueryRow(`
		SELECT
			COALESCE(passenger_name,''),
			COALESCE(passenger_phone,''),
			COALESCE(pickup_location,''),
			COALESCE(trip_date,'')
		FROM bookings
		WHERE id = ?
		LIMIT 1
	`, bookingID).Scan(&customerName, &customerPhone, &pickup, &bookingDate)

	tx, err := config.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal mulai transaksi"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	// 1) insert payment_validations
	cols := []string{
		"customer_name",
		"customer_phone",
		"pickup_address",
		"booking_date",
		"payment_method",
		"payment_status",
		"proof_file",
		"proof_file_name",
	}
	vals := []string{"?", "?", "?", "?", "?", "?", "?", "?"}
	args := []any{
		customerName,
		customerPhone,
		pickup,
		nullIfEmpty(bookingDate),
		req.PaymentMethod,
		"Menunggu Validasi",
		req.ProofFile,
		req.ProofFileName,
	}

	// kalau table punya booking_id, isi biar relasi kuat
	if hasColumn(tx, "payment_validations", "booking_id") {
		cols = append(cols, "booking_id")
		vals = append(vals, "?")
		args = append(args, bookingID)
	}
	if hasColumn(tx, "payment_validations", "updated_at") {
		cols = append(cols, "updated_at")
		vals = append(vals, "?")
		args = append(args, time.Now())
	}

	insertSQL := "INSERT INTO payment_validations (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(vals, ", ") + ")"
	res, err := tx.Exec(insertSQL, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal simpan validasi pembayaran: " + err.Error()})
		return
	}

	validationID, _ := res.LastInsertId()

	// 2) update bookings => menunggu validasi + link validation id (jika kolom ada)
	updates := []string{}
	uargs := []any{}

	if hasColumn(tx, "bookings", "payment_method") {
		updates = append(updates, "payment_method = ?")
		uargs = append(uargs, req.PaymentMethod)
	}
	if hasColumn(tx, "bookings", "payment_status") {
		updates = append(updates, "payment_status = ?")
		uargs = append(uargs, "Menunggu Validasi")
	}
	if hasColumn(tx, "bookings", "payment_validation_id") {
		updates = append(updates, "payment_validation_id = ?")
		uargs = append(uargs, validationID)
	}
	if hasColumn(tx, "bookings", "updated_at") {
		updates = append(updates, "updated_at = ?")
		uargs = append(uargs, time.Now())
	}

	if len(updates) > 0 {
		uargs = append(uargs, bookingID)
		if _, err := tx.Exec("UPDATE bookings SET "+strings.Join(updates, ", ")+" WHERE id = ?", uargs...); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal update booking: " + err.Error()})
			return
		}
	}

	// Sinkronkan data perjalanan (tarif/penumpang) segera setelah masuk tahap validasi
	if err := SyncConfirmedRegulerBooking(tx, bookingID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "bukti tersimpan, tapi gagal sync data perjalanan: " + err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal commit transaksi: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Bukti pembayaran terkirim. Menunggu validasi admin.",
		"bookingId":     bookingID,
		"validationId":  validationID,
		"paymentStatus": "Menunggu Validasi",
	})
}

// ===============================
// POST /api/reguler/bookings/:id/confirm-cash
	// Cash: langsung Lunas => trigger SyncConfirmedRegulerBookingTx(tx, bookingID)
// ===============================

func ConfirmRegulerCash(c *gin.Context) {
	bookingID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || bookingID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id tidak valid"})
		return
	}

	if err := config.DB.Ping(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "DB ping gagal: " + err.Error()})
		return
	}

	tx, err := config.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal mulai transaksi"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	updates := []string{}
	args := []any{}

	if hasColumn(tx, "bookings", "payment_method") {
		updates = append(updates, "payment_method = ?")
		args = append(args, "cash")
	}
	if hasColumn(tx, "bookings", "payment_status") {
		updates = append(updates, "payment_status = ?")
		args = append(args, "Lunas")
	}
	if hasColumn(tx, "bookings", "updated_at") {
		updates = append(updates, "updated_at = ?")
		args = append(args, time.Now())
	}

	if len(updates) > 0 {
		args = append(args, bookingID)
		if _, err := tx.Exec("UPDATE bookings SET "+strings.Join(updates, ", ")+" WHERE id = ?", args...); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal update booking: " + err.Error()})
			return
		}
	}

	// Trigger auto-sync modul-modul setelah Lunas
	if err := SyncConfirmedRegulerBookingTx(tx, bookingID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal sync data perjalanan: " + err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal commit transaksi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Pembayaran cash dikonfirmasi. E-ticket & invoice siap ditampilkan.",
		"bookingId":     bookingID,
		"paymentStatus": "Lunas",
	})
}

// supaya file tetap compile kalau suatu saat Anda butuh *sql.Tx di helper lain
var _ = sql.Tx{}
