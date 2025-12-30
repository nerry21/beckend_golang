package handlers

import (
	"bytes"
	"io"
	"strconv"

	legacy "backend/handlers"
	"backend/internal/http/middleware"
	"backend/internal/repositories"
	"backend/internal/services"

	"github.com/gin-gonic/gin"
)

var (
	GetRegulerStops           = legacy.GetRegulerStops
	GetRegulerSeats           = legacy.GetRegulerSeats
	GetRegulerQuote           = legacy.GetRegulerQuote
	CreateRegulerBooking      = legacy.CreateRegulerBooking
	GetRegulerSuratJalan      = legacy.GetRegulerSuratJalan
	GetRegulerBookingDetail   = legacy.GetRegulerBookingDetail
	SubmitRegulerPaymentProof = legacy.SubmitRegulerPaymentProof
)

// ConfirmRegulerCash diperkecil: panggil handler lama lalu pastikan departure terbentuk.
func ConfirmRegulerCash(c *gin.Context) {
	raw, _ := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(raw))
	legacy.ConfirmRegulerCash(c)

	idParam := c.Param("id")
	bookingID, _ := strconv.ParseInt(idParam, 10, 64)
	if bookingID <= 0 {
		return
	}

	reqID := middleware.GetRequestID(c)
	svc := services.PaymentService{
		PaymentRepo:     repositories.PaymentRepository{},
		BookingRepo:     repositories.BookingRepository{},
		BookingSeatRepo: repositories.BookingSeatRepository{},
		RequestID:       reqID,
		DepartureSvc:    services.DepartureService{Repo: repositories.DepartureRepository{}, RequestID: reqID},
		PassengerSvc:    services.PassengerService{PassengerRepo: repositories.PassengerRepository{}, BookingRepo: repositories.BookingRepository{}, BookingSeatRepo: repositories.BookingSeatRepository{}},
	}
	_ = svc.ValidateLunas(bookingID, raw)
}
