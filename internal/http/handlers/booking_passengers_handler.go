package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
	"backend/internal/utils"

	"github.com/gin-gonic/gin"
)

type bookingPassengerResponse struct {
	SeatCode       string `json:"seat_code"`
	PassengerName  string `json:"passenger_name"`
	PassengerPhone string `json:"passenger_phone"`
	PaidPrice      int64  `json:"paid_price"`
}

// GetBookingPassengers returns per-seat passengers for a booking.
func GetBookingPassengers(c *gin.Context) {
	idStr := c.Param("id")
	bookingID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || bookingID <= 0 {
		respondError(c, http.StatusBadRequest, "invalid_booking_id", "id booking tidak valid", nil)
		return
	}

	db := intconfig.DB
	var (
		routeFrom, routeTo, tripDate, tripTime, bookingPhone, paymentStatus sql.NullString
		pricePerSeat                                                         sql.NullInt64
	)
	if err := db.QueryRow(`
		SELECT route_from, route_to, trip_date, trip_time, COALESCE(passenger_phone,''), COALESCE(payment_status,''), COALESCE(price_per_seat,0)
		FROM bookings WHERE id=? LIMIT 1`, bookingID).Scan(
		&routeFrom, &routeTo, &tripDate, &tripTime, &bookingPhone, &paymentStatus, &pricePerSeat,
	); err != nil {
		if err == sql.ErrNoRows {
			respondError(c, http.StatusNotFound, "booking_not_found", "booking tidak ditemukan", err)
			return
		}
		respondError(c, http.StatusInternalServerError, "db_error", "gagal membaca booking", err)
		return
	}

	bookingPhoneStr := strings.TrimSpace(bookingPhone.String)
	// fallback phone dari payment_validations.customer_phone
	var validationPhone sql.NullString
	_ = db.QueryRow(`SELECT customer_phone FROM payment_validations WHERE booking_id=? ORDER BY id DESC LIMIT 1`, bookingID).Scan(&validationPhone)
	if bookingPhoneStr == "" && validationPhone.Valid {
		bookingPhoneStr = strings.TrimSpace(validationPhone.String)
	}

	// seat list dari booking_seats
	rows, err := db.Query(`SELECT seat_code FROM booking_seats WHERE booking_id=? ORDER BY id ASC`, bookingID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "db_error", "gagal membaca seat", err)
		return
	}
	defer rows.Close()
	seatList := []string{}
	for rows.Next() {
		var code sql.NullString
		if err := rows.Scan(&code); err == nil {
			seat := strings.ToUpper(strings.TrimSpace(code.String))
			if seat != "" {
				seatList = append(seatList, seat)
			}
		}
	}

	// map per-seat dari booking_passengers
	passengerMap := map[string]bookingPassengerResponse{}
	withPassengerPhone := intdb.HasColumn(db, "booking_passengers", "passenger_phone")
	withPaidPrice := intdb.HasColumn(db, "booking_passengers", "paid_price")

	if intdb.HasTable(db, "booking_passengers") {
		phoneSel := "''"
		if withPassengerPhone {
			phoneSel = "COALESCE(passenger_phone,'')"
		}
		priceSel := "0"
		if withPaidPrice {
			priceSel = "COALESCE(paid_price,0)"
		}
		query := `SELECT seat_code, COALESCE(passenger_name,''), ` + phoneSel + `, ` + priceSel + ` FROM booking_passengers WHERE booking_id=?`
		pRows, err := db.Query(query, bookingID)
		if err == nil {
			defer pRows.Close()
			for pRows.Next() {
				var seat, name, phone sql.NullString
				var paid sql.NullInt64
				if err := pRows.Scan(&seat, &name, &phone, &paid); err == nil {
					seatCode := strings.ToUpper(strings.TrimSpace(seat.String))
					if seatCode == "" {
						continue
					}
					passengerMap[seatCode] = bookingPassengerResponse{
						SeatCode:       seatCode,
						PassengerName:  strings.TrimSpace(name.String),
						PassengerPhone: strings.TrimSpace(phone.String),
						PaidPrice:      paid.Int64,
					}
				}
			}
		}
	}

	// susun output per seat (jika seatList kosong, gunakan map key)
	if len(seatList) == 0 {
		for k := range passengerMap {
			seatList = append(seatList, k)
		}
	}

	resp := []bookingPassengerResponse{}
	baseFare := pricePerSeat.Int64
	for _, seat := range seatList {
		item := passengerMap[seat]
		item.SeatCode = seat
		if item.PaidPrice == 0 {
			item.PaidPrice = utils.ComputeFare(routeFrom.String, routeTo.String, baseFare)
		}
		resp = append(resp, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"booking_id":     bookingID,
		"customer_phone": bookingPhoneStr,
		"payment_status": strings.TrimSpace(paymentStatus.String),
		"route_from":     routeFrom.String,
		"route_to":       routeTo.String,
		"trip_date":      tripDate.String,
		"trip_time":      tripTime.String,
		"passengers":     resp,
	})
}
