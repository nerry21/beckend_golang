package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
	"backend/internal/http/middleware"
	"backend/internal/repositories"
	"backend/internal/services"

	"github.com/gin-gonic/gin"
)

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

const autoSyncOnManualPaidEdit = false

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

func GetPaymentValidations(c *gin.Context) {
	rows, err := intconfig.DB.Query(`
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

func CreatePaymentValidation(c *gin.Context) {
	raw, _ := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(raw))

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

	if intdb.HasColumn(intconfig.DB, "payment_validations", "booking_id") && input.BookingID != nil && *input.BookingID > 0 {
		cols = append(cols, "booking_id")
		args = append(args, *input.BookingID)
	}

	if intdb.HasColumn(intconfig.DB, "payment_validations", "origin") {
		cols = append(cols, "origin")
		args = append(args, input.Origin)
	}
	if intdb.HasColumn(intconfig.DB, "payment_validations", "destination") {
		cols = append(cols, "destination")
		args = append(args, input.Destination)
	}

	if intdb.HasColumn(intconfig.DB, "payment_validations", "trip_role") {
		cols = append(cols, "trip_role")
		args = append(args, input.TripRole)
	}

	ph := make([]string, 0, len(cols))
	for range cols {
		ph = append(ph, "?")
	}

	q := `INSERT INTO payment_validations (` + strings.Join(cols, ",") + `) VALUES (` + strings.Join(ph, ",") + `)`
	res, err := intconfig.DB.Exec(q, args...)
	if err != nil {
		log.Println("CreatePaymentValidation insert error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal membuat data: " + err.Error()})
		return
	}

	id, _ := res.LastInsertId()
	input.ID = id
	_ = intconfig.DB.QueryRow(`SELECT COALESCE(created_at,'') FROM payment_validations WHERE id=?`, id).Scan(&input.CreatedAt)

	c.JSON(http.StatusCreated, input)
	triggerValidatePayment(c, raw)
}

func DeletePaymentValidation(c *gin.Context) {
	id64, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id64 <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	tx, err := intconfig.DB.Begin()
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

	var bookingID int64
	if intdb.HasColumn(tx, "payment_validations", "booking_id") {
		if err := tx.QueryRow(`SELECT COALESCE(booking_id,0) FROM payment_validations WHERE id=? LIMIT 1`, id64).Scan(&bookingID); err != nil && err != sql.ErrNoRows {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal baca booking_id: " + err.Error()})
			return
		}
	} else if intdb.HasColumn(tx, "bookings", "payment_validation_id") {
		if err := tx.QueryRow(`SELECT COALESCE(id,0) FROM bookings WHERE payment_validation_id=? LIMIT 1`, id64).Scan(&bookingID); err != nil && err != sql.ErrNoRows {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal baca booking dari payment_validation_id: " + err.Error()})
			return
		}
	}

	if _, err := tx.Exec(`DELETE FROM payment_validations WHERE id=?`, id64); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal menghapus data: " + err.Error()})
		return
	}

	if bookingID > 0 {
		bSet := []string{}
		bArgs := []any{}

		if intdb.HasColumn(tx, "bookings", "payment_validation_id") {
			bSet = append(bSet, "payment_validation_id = NULL")
		}
		if intdb.HasColumn(tx, "bookings", "trip_role") {
			bSet = append(bSet, "trip_role = ?")
			bArgs = append(bArgs, "")
		}
		if intdb.HasColumn(tx, "bookings", "updated_at") {
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

func UpdatePaymentValidation(c *gin.Context) {
	id64, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id64 <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	raw, _ := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(raw))

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
	input.TripRole = normalizeTripRolePV(input.TripRole)
	input.ProofFileName = strings.TrimSpace(input.ProofFileName)

	if input.PaymentStatus == "" {
		input.PaymentStatus = "Menunggu Validasi"
	}

	tx, err := intconfig.DB.Begin()
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

	var bookingID int64
	var existingPayMethod sql.NullString

	if intdb.HasColumn(tx, "payment_validations", "booking_id") {
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

	if intdb.HasColumn(tx, "payment_validations", "origin") {
		sets = append(sets, "origin = ?")
		args = append(args, input.Origin)
	}
	if intdb.HasColumn(tx, "payment_validations", "destination") {
		sets = append(sets, "destination = ?")
		args = append(args, input.Destination)
	}
	if intdb.HasColumn(tx, "payment_validations", "trip_role") {
		sets = append(sets, "trip_role = ?")
		args = append(args, input.TripRole)
	}
	if intdb.HasColumn(tx, "payment_validations", "updated_at") {
		sets = append(sets, "updated_at = ?")
		args = append(args, time.Now())
	}

	args = append(args, id64)
	if _, err := tx.Exec(`UPDATE payment_validations SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
		log.Println("UpdatePaymentValidation update error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal mengupdate data: " + err.Error()})
		return
	}

	if bookingID > 0 {
		bSets := []string{}
		bArgs := []any{}

		if intdb.HasColumn(tx, "bookings", "payment_method") && strings.TrimSpace(input.PaymentMethod) != "" {
			bSets = append(bSets, "payment_method = ?")
			bArgs = append(bArgs, input.PaymentMethod)
		}
		if intdb.HasColumn(tx, "bookings", "trip_role") {
			bSets = append(bSets, "trip_role = ?")
			bArgs = append(bArgs, input.TripRole)
		}
		if intdb.HasColumn(tx, "bookings", "updated_at") {
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

		if autoSyncOnManualPaidEdit && isValidationPaid(input.PaymentStatus) && strings.TrimSpace(input.TripRole) != "" {
			reqID := middleware.GetRequestID(c)
			svc := services.PaymentService{
				PaymentRepo:     repositories.PaymentRepository{},
				BookingRepo:     repositories.BookingRepository{},
				BookingSeatRepo: repositories.BookingSeatRepository{},
				RequestID:       reqID,
				DepartureSvc:    services.DepartureService{Repo: repositories.DepartureRepository{}, BookingRepo: repositories.BookingRepository{}, SeatRepo: repositories.BookingSeatRepository{}, RequestID: reqID},
				ReturnSvc:       services.ReturnService{Repo: repositories.ReturnRepository{}, BookingRepo: repositories.BookingRepository{}, SeatRepo: repositories.BookingSeatRepository{}, RequestID: reqID},
				PassengerSvc:    services.PassengerService{PassengerRepo: repositories.PassengerRepository{}, BookingRepo: repositories.BookingRepository{}, BookingSeatRepo: repositories.BookingSeatRepository{}},
			}
			_ = svc.ValidatePayment(bookingID, raw)
		}
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "gagal commit transaksi: " + err.Error()})
		return
	}
	committed = true

	input.ID = id64
	c.JSON(http.StatusOK, input)
	triggerValidatePayment(c, raw)
}

func ApprovePaymentValidation(c *gin.Context) {
	updatePaymentValidationStatus(c, true)
}

func RejectPaymentValidation(c *gin.Context) {
	updatePaymentValidationStatus(c, false)
}

func updatePaymentValidationStatus(c *gin.Context, approved bool) {
	validationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || validationID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id validasi tidak valid"})
		return
	}

	tx, err := intconfig.DB.Begin()
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

	var bookingID int64
	var payMethod sql.NullString
	var tripRole string

	if intdb.HasColumn(tx, "payment_validations", "booking_id") {
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
		if intdb.HasColumn(tx, "bookings", "payment_validation_id") {
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

	setParts := []string{"payment_status = ?"}
	args := []any{newPVStatus}

	if intdb.HasColumn(tx, "payment_validations", "trip_role") {
		setParts = append(setParts, "trip_role = ?")
		args = append(args, tripRole)
	}
	if intdb.HasColumn(tx, "payment_validations", "updated_at") {
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

	bSet := []string{}
	bArgs := []any{}

	if intdb.HasColumn(tx, "bookings", "payment_status") {
		bSet = append(bSet, "payment_status = ?")
		bArgs = append(bArgs, newBookingStatus)
	}
	if intdb.HasColumn(tx, "bookings", "payment_method") && payMethod.Valid && strings.TrimSpace(payMethod.String) != "" {
		bSet = append(bSet, "payment_method = ?")
		bArgs = append(bArgs, strings.TrimSpace(payMethod.String))
	}
	if intdb.HasColumn(tx, "bookings", "trip_role") {
		bSet = append(bSet, "trip_role = ?")
		bArgs = append(bArgs, tripRole)
	}
	if intdb.HasColumn(tx, "bookings", "updated_at") {
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

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal commit transaksi: " + err.Error()})
		return
	}
	committed = true

	if approved {
		reqID := middleware.GetRequestID(c)
		svc := services.PaymentService{
			PaymentRepo:     repositories.PaymentRepository{},
			BookingRepo:     repositories.BookingRepository{},
			BookingSeatRepo: repositories.BookingSeatRepository{},
			RequestID:       reqID,
			DepartureSvc:    services.DepartureService{Repo: repositories.DepartureRepository{}, BookingRepo: repositories.BookingRepository{}, SeatRepo: repositories.BookingSeatRepository{}, RequestID: reqID},
			ReturnSvc:       services.ReturnService{Repo: repositories.ReturnRepository{}, BookingRepo: repositories.BookingRepository{}, SeatRepo: repositories.BookingSeatRepository{}, RequestID: reqID},
			PassengerSvc:    services.PassengerService{PassengerRepo: repositories.PassengerRepository{}, BookingRepo: repositories.BookingRepository{}, BookingSeatRepo: repositories.BookingSeatRepository{}},
		}
		_ = svc.ValidatePayment(bookingID, nil)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Status validasi diperbarui",
		"validationId":  validationID,
		"bookingId":     bookingID,
		"paymentStatus": newBookingStatus,
	})
}

func triggerValidatePayment(c *gin.Context, raw []byte) {
	var payload map[string]any
	_ = json.Unmarshal(raw, &payload)

	var bookingID int64
	for _, k := range []string{"booking_id", "bookingId"} {
		if v, ok := payload[k]; ok {
			switch val := v.(type) {
			case float64:
				bookingID = int64(val)
			case int64:
				bookingID = val
			case int:
				bookingID = int64(val)
			case string:
				if n, err := strconv.ParseInt(val, 10, 64); err == nil {
					bookingID = n
				}
			}
		}
	}

	if bookingID <= 0 {
		return
	}

	reqID := middleware.GetRequestID(c)
	svc := services.PaymentService{
		PaymentRepo:     repositories.PaymentRepository{},
		BookingRepo:     repositories.BookingRepository{},
		BookingSeatRepo: repositories.BookingSeatRepository{},
		RequestID:       reqID,
		DepartureSvc:    services.DepartureService{Repo: repositories.DepartureRepository{}, BookingRepo: repositories.BookingRepository{}, SeatRepo: repositories.BookingSeatRepository{}, RequestID: reqID},
		ReturnSvc:       services.ReturnService{Repo: repositories.ReturnRepository{}, BookingRepo: repositories.BookingRepository{}, SeatRepo: repositories.BookingSeatRepository{}, RequestID: reqID},
		PassengerSvc:    services.PassengerService{PassengerRepo: repositories.PassengerRepository{}, BookingRepo: repositories.BookingRepository{}, BookingSeatRepo: repositories.BookingSeatRepository{}},
	}
	if err := svc.ValidatePayment(bookingID, raw); err != nil {
		RespondError(c, http.StatusInternalServerError, "gagal memvalidasi pembayaran", err)
	}
}
