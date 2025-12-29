// backend/handlers/reguler_payment_handler.go
package handlers

import (
	"backend/config"
	"database/sql"
	"fmt"
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
// Helpers: ambil kolom yang EXIST di DB
// ===============================

func firstExistingCol(table string, candidates ...string) string {
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if hasColumn(config.DB, table, c) {
			return c
		}
	}
	return ""
}

func strExpr(col string) string {
	if strings.TrimSpace(col) == "" {
		return "''"
	}
	// CAST supaya aman untuk DATE/DATETIME/INT juga
	return "COALESCE(CAST(" + col + " AS CHAR),'')"
}

func intExpr(col string) string {
	if strings.TrimSpace(col) == "" {
		return "0"
	}
	return "COALESCE(" + col + ",0)"
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

	// Booking columns (dinamis, karena nama kolom tiap project beda)
	colCategory := firstExistingCol("bookings", "category", "service_type", "serviceType", "layanan")
	colFrom := firstExistingCol("bookings", "route_from", "origin", "from_city", "from", "routeFrom")
	colTo := firstExistingCol("bookings", "route_to", "destination", "to_city", "to", "routeTo")
	colTripDate := firstExistingCol("bookings", "trip_date", "departure_date", "departureDate", "booking_date", "tanggal_pemesanan", "created_at")
	colTripTime := firstExistingCol("bookings", "trip_time", "departure_time", "departureTime")
	colPassengerName := firstExistingCol("bookings", "passenger_name", "booking_name", "bookingName", "customer_name", "nama_pemesan")
	colPassengerCount := firstExistingCol("bookings", "passenger_count", "passengerCount", "jumlah_penumpang")
	colPickup := firstExistingCol("bookings", "pickup_location", "pickup_address", "pickupAddress", "alamat_jemput")
	colDropoff := firstExistingCol("bookings", "dropoff_location", "dropoff_address", "dropoffAddress", "alamat_turun")
	colTotal := firstExistingCol("bookings", "total", "total_amount", "grand_total", "totalHarga", "total_harga")

	cols := []string{
		"id",
		strExpr(colCategory),
		strExpr(colFrom),
		strExpr(colTo),
		strExpr(colTripDate),
		strExpr(colTripTime),
		strExpr(colPassengerName),
		intExpr(colPassengerCount),
		strExpr(colPickup),
		strExpr(colDropoff),
		intExpr(colTotal),
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
		"id":              id,
		"category":        category,
		"routeFrom":       routeFrom,
		"routeTo":         routeTo,
		"tripDate":        tripDate,
		"tripTime":        tripTime,
		"passengerName":   passengerName,
		"passengerCount":  passengerCount,
		"pickupLocation":  pickup,
		"dropoffLocation": dropoff,
		"total":           total,
		"paymentMethod":   paymentMethod,
		"paymentStatus":   paymentStatus,
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

	// ✅ Ambil info booking ringkas untuk di-copy ke payment_validations (DINAMIS & error tidak dibuang)
	var (
		customerName  string
		customerPhone string
		pickup        string
		bookingDate   string
		routeFrom     string
		routeTo       string
		tripRole      string
	)

	colCustomerName := firstExistingCol("bookings", "passenger_name", "booking_name", "bookingName", "customer_name", "nama_pemesan")
	colCustomerPhone := firstExistingCol("bookings", "passenger_phone", "phone", "customer_phone", "no_hp", "hp")
	colPickup := firstExistingCol("bookings", "pickup_location", "pickup_address", "pickupAddress", "alamat_jemput")
	colBookingDate := firstExistingCol("bookings", "trip_date", "departure_date", "departureDate", "booking_date", "tanggal_pemesanan", "created_at")
	colFrom := firstExistingCol("bookings", "route_from", "origin", "from", "routeFrom")
	colTo := firstExistingCol("bookings", "route_to", "destination", "to", "routeTo")
	colTripRole := firstExistingCol("bookings", "trip_role", "role_trip", "tripRole")

	readSQL := fmt.Sprintf(
		`SELECT %s,%s,%s,%s,%s,%s,%s FROM bookings WHERE id=? LIMIT 1`,
		strExpr(colCustomerName),
		strExpr(colCustomerPhone),
		strExpr(colPickup),
		strExpr(colBookingDate),
		strExpr(colFrom),
		strExpr(colTo),
		strExpr(colTripRole),
	)

	if err := config.DB.QueryRow(readSQL, bookingID).Scan(
		&customerName, &customerPhone, &pickup, &bookingDate, &routeFrom, &routeTo, &tripRole,
	); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"message": "booking tidak ditemukan untuk submit pembayaran"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal baca data booking: " + err.Error()})
		return
	}

	// ✅ trip_role harus kosong saat Menunggu Validasi (nunggu admin edit manual)
	tripRole = ""

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

	// ✅ origin/destination ikut dikirim jika kolom ada di payment_validations
	if hasColumn(tx, "payment_validations", "origin") {
		cols = append(cols, "origin")
		vals = append(vals, "?")
		args = append(args, routeFrom)
	}
	if hasColumn(tx, "payment_validations", "destination") {
		cols = append(cols, "destination")
		vals = append(vals, "?")
		args = append(args, routeTo)
	}

	if hasColumn(tx, "payment_validations", "trip_role") {
		cols = append(cols, "trip_role")
		vals = append(vals, "?")
		args = append(args, "")
	}

	// relasi kuat ke booking
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
	// trip_role dikosongkan sampai user set manual di validasi
	if hasColumn(tx, "bookings", "trip_role") {
		updates = append(updates, "trip_role = ?")
		uargs = append(uargs, "")
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
	tripRole := ""
	if hasColumn(tx, "bookings", "trip_role") {
		_ = tx.QueryRow(`SELECT COALESCE(trip_role,'') FROM bookings WHERE id=? LIMIT 1`, bookingID).Scan(&tripRole)
	}
	var syncErr error
	if strings.EqualFold(tripRole, "kepulangan") {
		syncErr = SyncReturnBooking(tx, bookingID)
	} else {
		syncErr = SyncConfirmedRegulerBookingTx(tx, bookingID)
	}
	if syncErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal sync data perjalanan: " + syncErr.Error()})
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
