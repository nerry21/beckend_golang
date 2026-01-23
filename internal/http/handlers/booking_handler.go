// backend/internal/http/handlers/booking_handler.go
package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"backend/internal/domain/models"
	"backend/internal/services"

	"github.com/gin-gonic/gin"
)

// Stringish menoleransi string/number/bool menjadi string.
type Stringish string

func (s *Stringish) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch {
	case string(b) == "null" || len(b) == 0:
		*s = ""
		return nil
	case len(b) >= 2 && b[0] == '"' && b[len(b)-1] == '"':
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		*s = Stringish(str)
		return nil
	default:
		// number/bool/object -> stringify best-effort
		*s = Stringish(strings.Trim(string(b), `"`))
		return nil
	}
}

func (s Stringish) String() string { return string(s) }

type passengerPayload struct {
	// seat
	SeatCode      Stringish `json:"seat_code"`
	SeatCodeCamel Stringish `json:"seatCode"`
	SeatCodeAlt   Stringish `json:"seat"`

	// name
	Name           Stringish `json:"name"`
	PassengerName  Stringish `json:"passengerName"`
	PassengerName2 Stringish `json:"passenger_name"`

	// phone
	Phone           Stringish `json:"phone"`
	PassengerPhone  Stringish `json:"passengerPhone"`
	PassengerPhone2 Stringish `json:"passenger_phone"`
}

type bookingPassengersEnvelope struct {
	Passengers []passengerPayload `json:"passengers"`

	// legacy single-object (support snake + camel)
	SeatCode      Stringish `json:"seat_code"`
	SeatCodeCamel Stringish `json:"seatCode"`
	SeatCodeAlt   Stringish `json:"seat"`

	Name           Stringish `json:"name"`
	PassengerName  Stringish `json:"passengerName"`
	PassengerName2 Stringish `json:"passenger_name"`

	Phone           Stringish `json:"phone"`
	PassengerPhone  Stringish `json:"passengerPhone"`
	PassengerPhone2 Stringish `json:"passenger_phone"`
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
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		respondError(c, http.StatusBadRequest, "invalid_json", "payload tidak valid", nil)
		return
	}

	firstNonEmpty := func(vals ...Stringish) string {
		for _, v := range vals {
			s := strings.TrimSpace(v.String())
			if s != "" {
				return s
			}
		}
		return ""
	}

	normalizePhone := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}
		s = strings.ReplaceAll(s, " ", "")
		s = strings.ReplaceAll(s, "\t", "")
		s = strings.ReplaceAll(s, "\n", "")
		s = strings.ReplaceAll(s, "\r", "")
		return s
	}

	var passengers []passengerPayload
	var envelope bookingPassengersEnvelope

	// Parse berdasar tipe JSON (object vs array)
	switch raw[0] {
	case '[':
		var direct []passengerPayload
		if err := json.Unmarshal(raw, &direct); err != nil {
			respondError(c, http.StatusBadRequest, "invalid_json", "payload tidak valid", nil)
			return
		}
		passengers = direct

	case '{':
		if err := json.Unmarshal(raw, &envelope); err != nil {
			respondError(c, http.StatusBadRequest, "invalid_json", "payload tidak valid", nil)
			return
		}
		passengers = envelope.Passengers

		// fallback: extract via map (kalau key beda casing)
		if len(passengers) == 0 {
			var m map[string]json.RawMessage
			if err := json.Unmarshal(raw, &m); err == nil {
				for _, k := range []string{"passengers", "Passengers", "data", "items"} {
					if b, ok := m[k]; ok && len(bytes.TrimSpace(b)) > 0 {
						var arr []passengerPayload
						if err := json.Unmarshal(b, &arr); err == nil && len(arr) > 0 {
							passengers = arr
							break
						}
					}
				}
			}
		}

	default:
		respondError(c, http.StatusBadRequest, "invalid_json", "payload tidak valid", nil)
		return
	}

	// fallback: legacy single-object (snake/camel)
	if len(passengers) == 0 {
		legacyName := firstNonEmpty(envelope.Name, envelope.PassengerName, envelope.PassengerName2)
		legacyPhone := firstNonEmpty(envelope.Phone, envelope.PassengerPhone, envelope.PassengerPhone2)
		legacySeat := firstNonEmpty(envelope.SeatCode, envelope.SeatCodeCamel, envelope.SeatCodeAlt)
		legacySeat = strings.ToUpper(strings.TrimSpace(legacySeat))
		if legacySeat == "" {
			legacySeat = "ALL"
		}

		if legacyName != "" || legacyPhone != "" {
			passengers = []passengerPayload{{
				SeatCode:        Stringish(legacySeat),
				Name:            envelope.Name,
				PassengerName:   envelope.PassengerName,
				PassengerName2:  envelope.PassengerName2,
				Phone:           envelope.Phone,
				PassengerPhone:  envelope.PassengerPhone,
				PassengerPhone2: envelope.PassengerPhone2,
			}}
		}
	}

	if len(passengers) == 0 {
		respondError(c, http.StatusBadRequest, "validation_error", "passengers: data kosong", nil)
		return
	}

	inputs := make([]models.PassengerInput, 0, len(passengers))
	echo := make([]gin.H, 0, len(passengers))

	for _, p := range passengers {
		seat := strings.ToUpper(strings.TrimSpace(firstNonEmpty(p.SeatCode, p.SeatCodeCamel, p.SeatCodeAlt)))
		if seat == "" {
			seat = "ALL"
		}
		name := strings.TrimSpace(firstNonEmpty(p.Name, p.PassengerName, p.PassengerName2))
		phone := normalizePhone(firstNonEmpty(p.Phone, p.PassengerPhone, p.PassengerPhone2))

		// jangan drop row (biar tidak jadi kosong total)
		inputs = append(inputs, models.PassengerInput{
			SeatCode: seat,
			Name:     name,
			Phone:    phone,
		})

		echo = append(echo, gin.H{
			"seat":  seat,
			"name":  name,
			"phone": phone,
		})
	}

	if len(inputs) == 0 {
		respondError(c, http.StatusBadRequest, "validation_error", "passengers: data kosong", nil)
		return
	}

	sample := inputs[0]
	log.Printf("[SaveBookingPassengers] booking=%d parsed=%d inputs=%d sampleSeat=%s sampleName=%s samplePhone=%s",
		bookingID, len(passengers), len(inputs), sample.SeatCode, sample.Name, sample.Phone)

	svc := services.BookingService{}
	if err := svc.SavePassengerInputs(bookingID, inputs); err != nil {
		RespondDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":         true,
		"saved":      len(inputs),
		"booking_id": bookingID,
		"passengers": echo, // ✅ biar gampang debug payload yang masuk
	})
}
