package services

import (
	"database/sql"
	"fmt"
	"log"
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

func normalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return ""
	}
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "\t", "")
	phone = strings.ReplaceAll(phone, "\n", "")
	phone = strings.ReplaceAll(phone, "\r", "")
	return phone
}

// SavePassengerInputs stores per-seat passenger name and phone.
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
		if seat == "" {
			seat = "ALL"
		}

		name := strings.TrimSpace(in.Name)
		phone := normalizePhone(in.Phone)

		// PENTING: jangan fallback phone booking ke phone penumpang.
		// Jika kosong, biarkan kosong (atau bisa divalidasi wajib di frontend).
		clean = append(clean, models.PassengerInput{
			SeatCode: seat,
			Name:     name,
			Phone:    phone,
		})
	}

	if len(clean) == 0 {
		log.Printf("[SavePassengerInputs] booking=%d inputs=%d clean=0", bookingID, len(inputs))
		return domain.ValidationError{Field: "passengers", Msg: "data kosong"}
	}

	// pastikan table booking_passengers ada + kolom penting ada
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

	tx, err := db.Begin()
	if err != nil {
		return domain.InternalError{Err: fmt.Errorf("begin tx: %w", err)}
	}

	for _, p := range clean {
		if p.Name == "" {
			p.Name = ""
		}
		if p.Phone == "" {
			p.Phone = ""
		}

		var paid int64 = 0
		if withPaidPrice {
			paid = utils.ComputeFare(booking.RouteFrom, booking.RouteTo, booking.PricePerSeat)
		}

		if withPaidPrice {
			if _, err := tx.Exec(stmt, bookingID, p.SeatCode, p.Name, p.Phone, paid); err != nil {
				_ = tx.Rollback()
				return domain.InternalError{Err: fmt.Errorf("insert booking_passengers booking=%d seat=%s: %w", bookingID, p.SeatCode, err)}
			}
		} else {
			if _, err := tx.Exec(stmt, bookingID, p.SeatCode, p.Name, p.Phone); err != nil {
				_ = tx.Rollback()
				return domain.InternalError{Err: fmt.Errorf("insert booking_passengers booking=%d seat=%s: %w", bookingID, p.SeatCode, err)}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return domain.InternalError{Err: fmt.Errorf("commit tx: %w", err)}
	}

	// Persist seats (best-effort)
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

	// Backward compatibility: jangan overwrite booking passenger_phone jadi kosong.
	first := clean[0]
	for _, x := range clean {
		if strings.TrimSpace(x.Name) != "" || strings.TrimSpace(x.Phone) != "" {
			first = x
			break
		}
	}

	var upd models.BookingUpdate
	if strings.TrimSpace(first.Name) != "" {
		upd.PassengerName = &first.Name
	}
	if strings.TrimSpace(first.Phone) != "" {
		upd.PassengerPhone = &first.Phone
	}
	// kalau kosong, biarkan nil supaya tidak menghapus data pemesan di booking.
	_ = s.bookings().UpdateBooking(bookingID, upd)

	ps := PassengerService{
		PassengerRepo:   repositories.PassengerRepository{DB: s.db()},
		BookingRepo:     repositories.BookingRepository{DB: s.db()},
		BookingSeatRepo: repositories.BookingSeatRepository{DB: s.db()},
	}

	// Best-effort sync
	if err := ps.SyncFromBooking(bookingID); err != nil {
		log.Printf("[SavePassengerInputs] WARN SyncFromBooking failed booking=%d: %v", bookingID, err)
	}

	return nil
}

func (s BookingService) ensureBookingPassengerTable() error {
	db := s.db()
	if db == nil {
		return fmt.Errorf("db tidak tersedia")
	}

	if intdb.HasTable(db, "booking_passengers") {
		if !intdb.HasColumn(db, "booking_passengers", "paid_price") {
			if _, err := db.Exec(`ALTER TABLE booking_passengers ADD COLUMN paid_price BIGINT NULL DEFAULT NULL`); err != nil {
				return fmt.Errorf("alter booking_passengers add paid_price: %w", err)
			}
		}
		return nil
	}

	ddl := `
CREATE TABLE IF NOT EXISTS booking_passengers (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	booking_id BIGINT NOT NULL,
	seat_code VARCHAR(50) NOT NULL,
	passenger_name VARCHAR(255) NOT NULL,
	passenger_phone VARCHAR(100) NOT NULL,
	paid_price BIGINT NULL DEFAULT NULL,
	created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	UNIQUE KEY uniq_booking_seat (booking_id, seat_code),
	KEY idx_booking (booking_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`
	_, err := db.Exec(ddl)
	return err
}
