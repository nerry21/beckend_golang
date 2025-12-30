package services

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"

	legacy "backend/handlers"
	intconfig "backend/internal/config"
	intdb "backend/internal/db"
	"backend/internal/repositories"
	"backend/internal/utils"
)

// PassengerService memastikan data passengers selalu sinkron per-seat dari departure/return/booking.
type PassengerService struct {
	PassengerRepo   repositories.PassengerRepository
	BookingRepo     repositories.BookingRepository
	BookingSeatRepo repositories.BookingSeatRepository
	DB              *sql.DB
	RequestID       string
	FetchBooking    func(int64) (repositories.Booking, []repositories.BookingSeat, error)
}

func (s PassengerService) db() *sql.DB {
	if s.DB != nil {
		return s.DB
	}
	return intconfig.DB
}

// SyncFromDeparture membuat/memperbarui penumpang per seat berdasarkan departure_settings + booking.
func (s PassengerService) SyncFromDeparture(dep legacy.DepartureSetting) error {
	utils.LogEvent(s.RequestID, "passenger", "sync_departure", "booking_id="+strconv.FormatInt(dep.BookingID, 10))
	return s.syncFromTrip(dep, "berangkat")
}

// SyncFromReturn membuat/memperbarui penumpang per seat berdasarkan return_settings + booking.
func (s PassengerService) SyncFromReturn(ret legacy.DepartureSetting) error {
	utils.LogEvent(s.RequestID, "passenger", "sync_return", "booking_id="+strconv.FormatInt(ret.BookingID, 10))
	return s.syncFromTrip(ret, "pulang")
}

func (s PassengerService) syncFromTrip(dep legacy.DepartureSetting, tripRole string) error {
	if dep.BookingID <= 0 {
		return fmt.Errorf("booking_id kosong pada setting id %d", dep.ID)
	}

	booking, seats, err := s.loadBookingAndSeats(dep.BookingID)
	if err != nil {
		return err
	}

	seatByCode := map[string]repositories.BookingSeat{}
	seatCodes := []string{}
	for _, seat := range seats {
		code := strings.ToUpper(strings.TrimSpace(seat.SeatCode))
		if code == "" {
			continue
		}
		if _, ok := seatByCode[code]; !ok {
			seatByCode[code] = seat
			seatCodes = append(seatCodes, code)
		}
	}
	if len(seatCodes) == 0 && strings.TrimSpace(dep.SeatNumbers) != "" {
		for _, s := range splitSeats(dep.SeatNumbers) {
			code := strings.ToUpper(strings.TrimSpace(s))
			if code != "" && !contains(seatCodes, code) {
				seatCodes = append(seatCodes, code)
			}
		}
	}
	if len(seatCodes) == 0 {
		seatCodes = []string{""}
	}
	seatCount := len(seatCodes)
	if seatCount == 0 {
		seatCount = 1
	}

	passengerNames := s.loadPassengerInputs(dep.BookingID)
	baseName := firstNonEmptyStr(booking.BookingFor, booking.PassengerName, dep.BookingName)
	basePhone := firstNonEmptyStr(booking.PassengerPhone, dep.Phone)

	pickup := firstNonEmptyStr(booking.PickupLocation, dep.PickupAddress)
	dropoff := firstNonEmptyStr(booking.DropoffLocation, dep.RouteTo)
	serviceType := firstNonEmptyStr(booking.Category, dep.ServiceType)
	pricePerSeat := booking.PricePerSeat
	if pricePerSeat == 0 {
		if booking.Total > 0 && seatCount > 0 {
			pricePerSeat = booking.Total / int64(seatCount)
		} else {
			pricePerSeat = booking.Total
		}
	}

	eticket := fmt.Sprintf("ETICKET_INVOICE_FROM_BOOKING:%d", dep.BookingID)

	for _, code := range seatCodes {
		seatMeta := seatByCode[code]
		date := firstNonEmptyStr(seatMeta.TripDate, booking.TripDate, dep.DepartureDate)
		time := firstNonEmptyStr(seatMeta.TripTime, booking.TripTime, dep.DepartureTime)

		name := baseName
		phone := basePhone
		if p, ok := passengerNames[code]; ok {
			if strings.TrimSpace(p.name) != "" {
				name = p.name
			}
			if strings.TrimSpace(p.phone) != "" {
				phone = p.phone
			}
		} else if p, ok := passengerNames["ALL"]; ok {
			if strings.TrimSpace(p.name) != "" {
				name = p.name
			}
			if strings.TrimSpace(p.phone) != "" {
				phone = p.phone
			}
		}

		payload := repositories.PassengerSeatData{
			PassengerName:  name,
			PassengerPhone: phone,
			Date:           date,
			DepartureTime:  time,
			PickupAddress:  pickup,
			DropoffAddress: dropoff,
			TotalAmount:    pricePerSeat,
			SelectedSeat:   code,
			ServiceType:    serviceType,
			ETicketPhoto:   eticket,
			DriverName:     dep.DriverName,
			VehicleCode:    dep.VehicleCode,
			Notes:          "",
			TripRole:       tripRole,
		}

		if err := s.PassengerRepo.UpsertPassenger(dep.BookingID, payload); err != nil {
			log.Println("[PASSENGER SYNC] upsert seat gagal:", err)
			return err
		}
	}

	return nil
}

type passengerInput struct {
	name  string
	phone string
}

func (s PassengerService) loadPassengerInputs(bookingID int64) map[string]passengerInput {
	out := map[string]passengerInput{}
	db := s.db()
	if db == nil || !intdb.HasTable(db, "booking_passengers") {
		return out
	}
	if !intdb.HasColumn(db, "booking_passengers", "booking_id") {
		return out
	}

	rows, err := db.Query(`SELECT COALESCE(seat_code,''), COALESCE(passenger_name,''), COALESCE(passenger_phone,'') FROM booking_passengers WHERE booking_id=?`, bookingID)
	if err != nil {
		return out
	}
	defer rows.Close()

	for rows.Next() {
		var seat, name, phone string
		if err := rows.Scan(&seat, &name, &phone); err == nil {
			seat = strings.ToUpper(strings.TrimSpace(seat))
			if seat == "" {
				seat = "ALL"
			}
			out[seat] = passengerInput{name: strings.TrimSpace(name), phone: strings.TrimSpace(phone)}
		}
	}
	return out
}

func splitSeats(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == '\n' || r == '\t'
	})
	out := []string{}
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func contains(arr []string, val string) bool {
	for _, v := range arr {
		if v == val {
			return true
		}
	}
	return false
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (s PassengerService) loadBookingAndSeats(bookingID int64) (repositories.Booking, []repositories.BookingSeat, error) {
	if s.FetchBooking != nil {
		return s.FetchBooking(bookingID)
	}
	booking, err := s.BookingRepo.GetByID(bookingID)
	if err != nil {
		return booking, nil, err
	}
	seats, _ := s.BookingSeatRepo.GetSeats(bookingID)
	return booking, seats, nil
}
