package repositories

import (
	"database/sql"
	"fmt"
	"strings"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
)

// Booking represents minimal booking data we need for departure creation.
type Booking struct {
	ID              int64
	Category        string
	RouteFrom       string
	RouteTo         string
	TripDate        string
	TripTime        string
	PassengerName   string
	PassengerCount  int
	PickupLocation  string
	DropoffLocation string
	PricePerSeat    int64
	Total           int64
	BookingFor      string
	PassengerPhone  string
	PaymentMethod   string
	PaymentStatus   string
}

type BookingRepository struct {
	DB *sql.DB
}

func (r BookingRepository) GetByID(id int64) (Booking, error) {
	if id <= 0 {
		return Booking{}, fmt.Errorf("id tidak valid")
	}
	table := "bookings"
	if !intdb.HasTable(intconfig.DB, table) {
		return Booking{}, fmt.Errorf("tabel bookings tidak ditemukan")
	}

	sel := func(col string, def string) string {
		if intdb.HasColumn(intconfig.DB, table, col) {
			return "COALESCE(" + col + ", '')"
		}
		return def
	}

	numSel := func(col string) string {
		if intdb.HasColumn(intconfig.DB, table, col) {
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
			%s,
			%s, %s
		FROM %s
		WHERE id=? LIMIT 1
	`,
		sel("category", "''"),         // 2
		sel("route_from", "''"),       // 3
		sel("route_to", "''"),         // 4
		sel("trip_date", "''"),        // 5
		sel("trip_time", "''"),        // 6
		sel("passenger_name", "''"),   // 7
		numSel("passenger_count"),     // 8
		sel("pickup_location", "''"),  // 9
		sel("dropoff_location", "''"), // 10
		numSel("price_per_seat"),      // 11
		numSel("total"),               // 12
		sel("booking_for", "''"),      // 13
		sel("passenger_phone", "''"),  // 14
		table,
	)

	var b Booking
	var count int
	var price, total int64
	if err := intconfig.DB.QueryRow(query, id).Scan(
		&b.ID,
		&b.Category,
		&b.RouteFrom,
		&b.RouteTo,
		&b.TripDate,
		&b.TripTime,
		&b.PassengerName,
		&count,
		&b.PickupLocation,
		&b.DropoffLocation,
		&price,
		&total,
		&b.BookingFor,
		&b.PassengerPhone,
	); err != nil {
		return Booking{}, err
	}

	b.PassengerCount = count
	b.PricePerSeat = price
	b.Total = total

	if intdb.HasColumn(intconfig.DB, table, "payment_method") {
		_ = intconfig.DB.QueryRow(`SELECT COALESCE(payment_method,'') FROM `+table+` WHERE id=? LIMIT 1`, id).Scan(&b.PaymentMethod)
	}
	if intdb.HasColumn(intconfig.DB, table, "payment_status") {
		_ = intconfig.DB.QueryRow(`SELECT COALESCE(payment_status,'') FROM `+table+` WHERE id=? LIMIT 1`, id).Scan(&b.PaymentStatus)
	}

	return b, nil
}

// UpdatePaymentStatus sets payment_status (and optionally payment_method) on bookings.
func (r BookingRepository) UpdatePaymentStatus(id int64, status, method string) error {
	if id <= 0 {
		return fmt.Errorf("id tidak valid")
	}
	table := "bookings"
	if !intdb.HasTable(intconfig.DB, table) || !intdb.HasColumn(intconfig.DB, table, "payment_status") {
		return nil
	}

	sets := []string{"payment_status=?"}
	args := []any{strings.TrimSpace(status)}
	if method != "" && intdb.HasColumn(intconfig.DB, table, "payment_method") {
		sets = append(sets, "payment_method=?")
		args = append(args, method)
	}
	if intdb.HasColumn(intconfig.DB, table, "updated_at") {
		sets = append(sets, "updated_at=NOW()")
	}
	args = append(args, id)
	_, err := intconfig.DB.Exec(`UPDATE `+table+` SET `+strings.Join(sets, ",")+` WHERE id=?`, args...)
	return err
}
