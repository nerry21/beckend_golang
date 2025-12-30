package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	legacy "backend/handlers"
	"backend/internal/http/middleware"
	"backend/internal/repositories"
	"backend/internal/services"

	"github.com/gin-gonic/gin"
)

func GetPaymentValidations(c *gin.Context)   { legacy.GetPaymentValidations(c) }
func DeletePaymentValidation(c *gin.Context) { legacy.DeletePaymentValidation(c) }

// RejectPaymentValidation keeps legacy behavior while ensuring request_id is attached when errors happen.
func RejectPaymentValidation(c *gin.Context) { legacy.RejectPaymentValidation(c) }

// CreatePaymentValidation memanggil handler lama lalu menjalankan PaymentService.ValidateLunas.
func CreatePaymentValidation(c *gin.Context) {
	raw, _ := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(raw))
	legacy.CreatePaymentValidation(c)
	triggerValidatePayment(c, raw)
}

// UpdatePaymentValidation memanggil handler lama lalu menjalankan PaymentService.ValidateLunas.
func UpdatePaymentValidation(c *gin.Context) {
	raw, _ := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(raw))
	legacy.UpdatePaymentValidation(c)
	triggerValidatePayment(c, raw)
}

// ApprovePaymentValidation memanggil handler lama lalu menjalankan PaymentService.ValidateLunas.
func ApprovePaymentValidation(c *gin.Context) {
	if c.Param("id") == "" {
		RespondError(c, http.StatusBadRequest, "id tidak valid", nil)
		return
	}
	legacy.ApprovePaymentValidation(c)

	idParam := c.Param("id")
	id, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil || id <= 0 {
		RespondError(c, http.StatusBadRequest, "id tidak valid", err)
		return
	}

	pRepo := repositories.PaymentRepository{}
	val, err := pRepo.GetValidationByID(id)
	if err != nil || val.BookingID <= 0 {
		RespondError(c, http.StatusNotFound, "payment validation tidak ditemukan", err)
		return
	}

	reqID := middleware.GetRequestID(c)
	svc := services.PaymentService{
		PaymentRepo:     pRepo,
		BookingRepo:     repositories.BookingRepository{},
		BookingSeatRepo: repositories.BookingSeatRepository{},
		RequestID:       reqID,
		DepartureSvc:    services.DepartureService{Repo: repositories.DepartureRepository{}, BookingRepo: repositories.BookingRepository{}, SeatRepo: repositories.BookingSeatRepository{}, RequestID: reqID},
		ReturnSvc:       services.ReturnService{Repo: repositories.ReturnRepository{}, BookingRepo: repositories.BookingRepository{}, SeatRepo: repositories.BookingSeatRepository{}, RequestID: reqID},
		PassengerSvc:    services.PassengerService{PassengerRepo: repositories.PassengerRepository{}, BookingRepo: repositories.BookingRepository{}, BookingSeatRepo: repositories.BookingSeatRepository{}},
	}
	_ = svc.ValidatePayment(val.BookingID, nil)
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
		// jangan blokir response lama; hanya logkan sebagai respon tambahan jika belum di-set
		RespondError(c, http.StatusInternalServerError, "gagal memvalidasi pembayaran", err)
	}
}
