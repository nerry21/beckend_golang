package repositories

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
	"backend/internal/domain/models"
)

type BookingRepo struct {
	DB *sql.DB
}

func (r BookingRepo) db() *sql.DB {
	if r.DB != nil {
		return r.DB
	}
	return intconfig.DB
}

// GetBookingByID fetches booking record with adaptive column mapping.
func (r BookingRepo) GetBookingByID(id int64) (models.Booking, error) {
	if id <= 0 {
		return models.Booking{}, fmt.Errorf("id tidak valid")
	}
	db := r.db()
	table := "bookings"
	if db == nil || !intdb.HasTable(db, table) {
		return models.Booking{}, fmt.Errorf("tabel bookings tidak ditemukan")
	}

	sel := func(col, def string) string {
		if intdb.HasColumn(db, table, col) {
			return "COALESCE(" + col + ", '')"
		}
		return def
	}
	numSel := func(col string) string {
		if intdb.HasColumn(db, table, col) {
			return "COALESCE(" + col + ", 0)"
		}
		return "0"
	}

	query := fmt.Sprintf(`
		SELECT
			id,
			%s, %s, %s, %s,
			%s, %s,
			%s, %s,
			%s, %s,
			%s
		FROM %s
		WHERE id=? LIMIT 1
	`,
		sel("category", "''"),        // 2
		sel("route_from", "''"),      // 3
		sel("route_to", "''"),        // 4
		sel("trip_date", "''"),       // 5
		sel("trip_time", "''"),       // 6
		sel("passenger_name", "''"),  // 7
		numSel("passenger_count"),    // 8
		sel("booking_for", "''"),     // 9
		sel("passenger_phone", "''"), // 10
		sel("payment_method", "''"),  // 11
		sel("payment_status", "''"),  // 12
		table,
	)

	var b models.Booking
	var passengerCount int
	if err := db.QueryRow(query, id).Scan(
		&b.ID,
		&b.Category,
		&b.RouteFrom,
		&b.RouteTo,
		&b.TripDate,
		&b.TripTime,
		&b.PassengerName,
		&passengerCount,
		&b.BookingFor,
		&b.PassengerPhone,
		&b.PaymentMethod,
		&b.PaymentStatus,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.Booking{}, fmt.Errorf("booking tidak ditemukan")
		}
		return models.Booking{}, err
	}
	b.PassengerCount = passengerCount
	return b, nil
}

// UpdateBooking performs PATCH-style updates based on key presence.
func (r BookingRepo) UpdateBooking(id int64, upd models.BookingUpdate) error {
	if id <= 0 {
		return fmt.Errorf("id tidak valid")
	}
	db := r.db()
	table := "bookings"
	if db == nil || !intdb.HasTable(db, table) {
		return nil
	}
	sets := []string{}
	args := []any{}

	if upd.PassengerName != nil && intdb.HasColumn(db, table, "passenger_name") {
		sets = append(sets, "passenger_name=?")
		args = append(args, strings.TrimSpace(*upd.PassengerName))
	}
	if upd.PassengerPhone != nil && intdb.HasColumn(db, table, "passenger_phone") {
		sets = append(sets, "passenger_phone=?")
		args = append(args, strings.TrimSpace(*upd.PassengerPhone))
	}
	if len(sets) == 0 {
		return nil
	}
	if intdb.HasColumn(db, table, "updated_at") {
		sets = append(sets, "updated_at=NOW()")
	}
	args = append(args, id)
	_, err := db.Exec(`UPDATE `+table+` SET `+strings.Join(sets, ",")+` WHERE id=?`, args...)
	return err
}
