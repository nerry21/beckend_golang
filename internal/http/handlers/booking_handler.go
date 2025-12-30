package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"backend/internal/domain/models"
	"backend/internal/services"

	"github.com/gin-gonic/gin"
)

type passengerPayload struct {
	SeatCode string `json:"seat_code"`
	Name     string `json:"name"`
	Phone    string `json:"phone"`
}

// SaveBookingPassengers stores passenger name+phone per seat for a booking.
func SaveBookingPassengers(c *gin.Context) {
	idStr := c.Param("id")
	bookingID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || bookingID <= 0 {
		respondError(c, http.StatusBadRequest, "invalid_booking_id", "id booking tidak valid", nil)
		return
	}

	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_body", "gagal membaca payload", nil)
		return
	}

	var envelope struct {
		Passengers []passengerPayload `json:"passengers"`
		Name       string             `json:"name"`      // legacy
		Phone      string             `json:"phone"`     // legacy
		SeatCode   string             `json:"seat_code"` // legacy single-seat
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		respondError(c, http.StatusBadRequest, "invalid_json", "payload tidak valid", nil)
		return
	}

	passengers := envelope.Passengers
	if len(passengers) == 0 {
		var direct []passengerPayload
		if err := json.Unmarshal(raw, &direct); err == nil && len(direct) > 0 {
			passengers = direct
		}
	}
	if len(passengers) == 0 && (envelope.Name != "" || envelope.Phone != "") {
		passengers = []passengerPayload{{
			SeatCode: envelope.SeatCode,
			Name:     envelope.Name,
			Phone:    envelope.Phone,
		}}
	}

	inputs := make([]models.PassengerInput, 0, len(passengers))
	for _, p := range passengers {
		inputs = append(inputs, models.PassengerInput{
			SeatCode: p.SeatCode,
			Name:     p.Name,
			Phone:    p.Phone,
		})
	}

	svc := services.BookingService{}
	if err := svc.SavePassengerInputs(bookingID, inputs); err != nil {
		RespondDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":         true,
		"saved":      len(inputs),
		"booking_id": bookingID,
	})
}
