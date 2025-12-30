package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	intconfig "backend/internal/config"
	"backend/internal/http/middleware"
	"backend/internal/repositories"
	"backend/internal/services"

	"github.com/gin-gonic/gin"
)

// GetPassengerETicketPDF returns per-passenger e-ticket (inline).
func GetPassengerETicketPDF(c *gin.Context) {
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || pid <= 0 {
		RespondError(c, http.StatusBadRequest, "id passenger tidak valid", err)
		return
	}

	paid, payErr := isPaymentLunas(pid)
	if payErr != nil {
		RespondError(c, http.StatusInternalServerError, "payment_check_failed", payErr.Error(), payErr)
		return
	}
	if !paid {
		RespondError(c, http.StatusForbidden, "payment_pending", "pembayaran belum lunas", nil)
		return
	}

	svc := services.DocsService{
		PassengerRepo: repositories.PassengerRepository{},
		SeatRepo:      repositories.BookingSeatRepo{},
		BookingRepo:   repositories.BookingRepository{},
		RequestID:     middleware.GetRequestID(c),
	}
	pdfBytes, filename, err := svc.GenerateETicket(pid)
	if err != nil {
		RespondError(c, http.StatusNotFound, "data passenger tidak ditemukan", err)
		return
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", `inline; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

// GetPassengerInvoicePDF returns per-passenger invoice (inline).
func GetPassengerInvoicePDF(c *gin.Context) {
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || pid <= 0 {
		RespondError(c, http.StatusBadRequest, "id passenger tidak valid", err)
		return
	}

	paid, payErr := isPaymentLunas(pid)
	if payErr != nil {
		RespondError(c, http.StatusInternalServerError, "payment_check_failed", payErr.Error(), payErr)
		return
	}
	if !paid {
		RespondError(c, http.StatusForbidden, "payment_pending", "pembayaran belum lunas", nil)
		return
	}

	svc := services.DocsService{
		PassengerRepo: repositories.PassengerRepository{},
		SeatRepo:      repositories.BookingSeatRepo{},
		BookingRepo:   repositories.BookingRepository{},
	}
	pdfBytes, filename, err := svc.GenerateInvoice(pid)
	if err != nil {
		RespondError(c, http.StatusNotFound, "data passenger tidak ditemukan", err)
		return
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", `inline; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

// isPaymentLunas checks payment status from bookings or payment_validations.
func isPaymentLunas(passengerID int64) (bool, error) {
	db := intconfig.DB
	// dapatkan booking_id dari passengers
	var bookingID sql.NullInt64
	_ = db.QueryRow(`SELECT booking_id FROM passengers WHERE id=?`, passengerID).Scan(&bookingID)
	if !bookingID.Valid || bookingID.Int64 == 0 {
		return false, nil
	}

	// cek bookings.payment_status
	var payStatus sql.NullString
	_ = db.QueryRow(`SELECT COALESCE(payment_status,'') FROM bookings WHERE id=?`, bookingID.Int64).Scan(&payStatus)
	if strings.EqualFold(strings.TrimSpace(payStatus.String), "lunas") {
		return true, nil
	}

	// cek payment_validations.payment_status
	var pvStatus sql.NullString
	_ = db.QueryRow(`SELECT payment_status FROM payment_validations WHERE booking_id=? ORDER BY id DESC LIMIT 1`, bookingID.Int64).Scan(&pvStatus)
	if strings.EqualFold(strings.TrimSpace(pvStatus.String), "lunas") {
		return true, nil
	}
	return false, nil
}
