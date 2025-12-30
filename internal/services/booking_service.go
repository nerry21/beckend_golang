package services

import (
	"database/sql"
	"fmt"
	"strings"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
	"backend/internal/domain"
	"backend/internal/domain/models"
	"backend/internal/repositories"
	"backend/internal/utils"
)

type BookingService struct {
	BookingRepo     repositories.BookingRepo
	BookingSeatRepo repositories.BookingSeatRepo
	DB              *sql.DB
}

func (s BookingService) db() *sql.DB {
	if s.DB != nil {
		return s.DB
	}
	return intconfig.DB
}

func (s BookingService) bookings() repositories.BookingRepo {
	if s.BookingRepo.DB != nil {
		return s.BookingRepo
	}
	return repositories.BookingRepo{DB: s.db()}
}

func (s BookingService) seats() repositories.BookingSeatRepo {
	if s.BookingSeatRepo.DB != nil {
		return s.BookingSeatRepo
	}
	return repositories.BookingSeatRepo{DB: s.db()}
}

// SavePassengerInputs stores per-seat passenger name and phone, preserving legacy fields.
func (s BookingService) SavePassengerInputs(bookingID int64, inputs []models.PassengerInput) error {
	if bookingID <= 0 {
		return domain.ValidationError{Field: "booking_id", Msg: "id tidak valid"}
	}
	booking, err := s.bookings().GetBookingByID(bookingID)
	if err != nil {
		return domain.NotFoundError{Resource: "booking", Err: err}
	}

	clean := make([]models.PassengerInput, 0, len(inputs))
	for _, in := range inputs {
		seat := strings.ToUpper(strings.TrimSpace(in.SeatCode))
		name := strings.TrimSpace(in.Name)
		phone := strings.TrimSpace(in.Phone)
		if name == "" && phone == "" {
			continue
		}
		if seat == "" {
			seat = "ALL"
		}
		clean = append(clean, models.PassengerInput{
			SeatCode: seat,
			Name:     name,
			Phone:    phone,
		})
	}
	if len(clean) == 0 {
		return domain.ValidationError{Field: "passengers", Msg: "data kosong"}
	}

	if err := s.ensureBookingPassengerTable(); err != nil {
		return domain.InternalError{Err: err}
	}

	db := s.db()
	withPaidPrice := intdb.HasColumn(db, "booking_passengers", "paid_price")
	placeholder := "(?,?,?,?)"
	if withPaidPrice {
		placeholder = "(?,?,?,?,?)"
	}
	stmt := `INSERT INTO booking_passengers (booking_id, seat_code, passenger_name, passenger_phone`
	if withPaidPrice {
		stmt += ", paid_price"
	}
	stmt += `) VALUES ` + placeholder + `
			 ON DUPLICATE KEY UPDATE passenger_name=VALUES(passenger_name), passenger_phone=VALUES(passenger_phone)`
	if withPaidPrice {
		stmt += `, paid_price=VALUES(paid_price)`
	}
	for _, p := range clean {
		var paid int64 = 0
		if withPaidPrice {
			paid = utils.ComputeFare(booking.RouteFrom, booking.RouteTo, booking.PricePerSeat)
		}
		if withPaidPrice {
			if _, err := db.Exec(stmt, bookingID, p.SeatCode, p.Name, p.Phone, paid); err != nil {
				return domain.InternalError{Err: err}
			}
		} else if _, err := db.Exec(stmt, bookingID, p.SeatCode, p.Name, p.Phone); err != nil {
			return domain.InternalError{Err: err}
		}
	}

	// Persist seats (route/time info best-effort only).
	seats := make([]models.BookingSeat, 0, len(clean))
	for _, p := range clean {
		seats = append(seats, models.BookingSeat{
			SeatCode:  p.SeatCode,
			RouteFrom: booking.RouteFrom,
			RouteTo:   booking.RouteTo,
			TripDate:  booking.TripDate,
			TripTime:  booking.TripTime,
		})
	}
	_ = s.seats().UpsertSeats(bookingID, seats)

	// Backward compatibility: set aggregated passenger name/phone when available.
	first := clean[0]
	update := models.BookingUpdate{
		PassengerName:  &first.Name,
		PassengerPhone: &first.Phone,
	}
	_ = s.bookings().UpdateBooking(bookingID, update)

	return nil
}

func (s BookingService) ensureBookingPassengerTable() error {
	db := s.db()
	if db == nil {
		return fmt.Errorf("db tidak tersedia")
	}
	if intdb.HasTable(db, "booking_passengers") {
		return nil
	}
	ddl := `
CREATE TABLE IF NOT EXISTS booking_passengers (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	booking_id BIGINT NOT NULL,
	seat_code VARCHAR(50) NOT NULL,
	passenger_name VARCHAR(255) NOT NULL,
	passenger_phone VARCHAR(100) NOT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	UNIQUE KEY uniq_booking_seat (booking_id, seat_code),
	KEY idx_booking (booking_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`
	_, err := db.Exec(ddl)
	return err
}
