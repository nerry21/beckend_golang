package repositories

import (
	"database/sql"
	"fmt"
	"strings"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
	"backend/internal/domain/models"
)

type BookingSeatRepo struct {
	DB *sql.DB
}

func (r BookingSeatRepo) db() *sql.DB {
	if r.DB != nil {
		return r.DB
	}
	return intconfig.DB
}

// ListByBookingID returns seats for a booking, tolerant to missing columns.
func (r BookingSeatRepo) ListByBookingID(bookingID int64) ([]models.BookingSeat, error) {
	if bookingID <= 0 {
		return nil, fmt.Errorf("id tidak valid")
	}
	db := r.db()
	table := "booking_seats"
	if db == nil || !intdb.HasTable(db, table) {
		return []models.BookingSeat{}, nil
	}
	if !intdb.HasColumn(db, table, "booking_id") || !intdb.HasColumn(db, table, "seat_code") {
		return []models.BookingSeat{}, nil
	}

	cols := []string{"seat_code"}
	if intdb.HasColumn(db, table, "route_from") {
		cols = append(cols, "route_from")
	} else {
		cols = append(cols, "''")
	}
	if intdb.HasColumn(db, table, "route_to") {
		cols = append(cols, "route_to")
	} else {
		cols = append(cols, "''")
	}
	if intdb.HasColumn(db, table, "trip_date") {
		cols = append(cols, "trip_date")
	} else {
		cols = append(cols, "''")
	}
	if intdb.HasColumn(db, table, "trip_time") {
		cols = append(cols, "trip_time")
	} else {
		cols = append(cols, "''")
	}

	query := `SELECT ` + strings.Join(cols, ",") + ` FROM ` + table + ` WHERE booking_id=? ORDER BY id ASC`
	rows, err := db.Query(query, bookingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.BookingSeat{}
	for rows.Next() {
		var seat, rf, rt, td, tt sql.NullString
		if err := rows.Scan(&seat, &rf, &rt, &td, &tt); err != nil {
			return out, err
		}
		out = append(out, models.BookingSeat{
			SeatCode:  strings.TrimSpace(seat.String),
			RouteFrom: strings.TrimSpace(rf.String),
			RouteTo:   strings.TrimSpace(rt.String),
			TripDate:  strings.TrimSpace(td.String),
			TripTime:  strings.TrimSpace(tt.String),
		})
	}
	return out, rows.Err()
}

// UpsertSeats ensures seat rows exist; updates route/time if columns available.
func (r BookingSeatRepo) UpsertSeats(bookingID int64, seats []models.BookingSeat) error {
	if bookingID <= 0 || len(seats) == 0 {
		return nil
	}
	db := r.db()
	table := "booking_seats"
	if db == nil || !intdb.HasTable(db, table) {
		return nil
	}
	if !intdb.HasColumn(db, table, "booking_id") || !intdb.HasColumn(db, table, "seat_code") {
		return nil
	}

	hasRouteFrom := intdb.HasColumn(db, table, "route_from")
	hasRouteTo := intdb.HasColumn(db, table, "route_to")
	hasTripDate := intdb.HasColumn(db, table, "trip_date")
	hasTripTime := intdb.HasColumn(db, table, "trip_time")

	baseCols := []string{"booking_id", "seat_code"}
	valPlaceholder := []string{"?", "?"}
	updateSets := []string{}

	if hasRouteFrom {
		baseCols = append(baseCols, "route_from")
		valPlaceholder = append(valPlaceholder, "?")
		updateSets = append(updateSets, "route_from=VALUES(route_from)")
	}
	if hasRouteTo {
		baseCols = append(baseCols, "route_to")
		valPlaceholder = append(valPlaceholder, "?")
		updateSets = append(updateSets, "route_to=VALUES(route_to)")
	}
	if hasTripDate {
		baseCols = append(baseCols, "trip_date")
		valPlaceholder = append(valPlaceholder, "?")
		updateSets = append(updateSets, "trip_date=VALUES(trip_date)")
	}
	if hasTripTime {
		baseCols = append(baseCols, "trip_time")
		valPlaceholder = append(valPlaceholder, "?")
		updateSets = append(updateSets, "trip_time=VALUES(trip_time)")
	}

	stmt := `INSERT INTO ` + table + ` (` + strings.Join(baseCols, ",") + `) VALUES (` + strings.Join(valPlaceholder, ",") + `)`
	if len(updateSets) > 0 {
		stmt += ` ON DUPLICATE KEY UPDATE ` + strings.Join(updateSets, ",")
	}

	for _, seat := range seats {
		args := []any{bookingID, strings.TrimSpace(strings.ToUpper(seat.SeatCode))}
		if hasRouteFrom {
			args = append(args, strings.TrimSpace(seat.RouteFrom))
		}
		if hasRouteTo {
			args = append(args, strings.TrimSpace(seat.RouteTo))
		}
		if hasTripDate {
			args = append(args, strings.TrimSpace(seat.TripDate))
		}
		if hasTripTime {
			args = append(args, strings.TrimSpace(seat.TripTime))
		}
		if _, err := db.Exec(stmt, args...); err != nil {
			return err
		}
	}
	return nil
}

// GetSeatByCode fetches a single seat row by booking_id + seat_code.
func (r BookingSeatRepo) GetSeatByCode(bookingID int64, seatCode string) (models.BookingSeat, error) {
	var out models.BookingSeat
	if bookingID <= 0 || strings.TrimSpace(seatCode) == "" {
		return out, sql.ErrNoRows
	}
	db := r.db()
	table := "booking_seats"
	if db == nil || !intdb.HasTable(db, table) || !intdb.HasColumn(db, table, "booking_id") || !intdb.HasColumn(db, table, "seat_code") {
		return out, sql.ErrNoRows
	}
	queryCols := []string{"seat_code"}
	dest := []any{&out.SeatCode}
	if intdb.HasColumn(db, table, "route_from") {
		queryCols = append(queryCols, "route_from")
		dest = append(dest, &out.RouteFrom)
	}
	if intdb.HasColumn(db, table, "route_to") {
		queryCols = append(queryCols, "route_to")
		dest = append(dest, &out.RouteTo)
	}
	if intdb.HasColumn(db, table, "trip_date") {
		queryCols = append(queryCols, "trip_date")
		dest = append(dest, &out.TripDate)
	}
	if intdb.HasColumn(db, table, "trip_time") {
		queryCols = append(queryCols, "trip_time")
		dest = append(dest, &out.TripTime)
	}
	if err := db.QueryRow(`SELECT `+strings.Join(queryCols, ",")+` FROM `+table+` WHERE booking_id=? AND seat_code=? LIMIT 1`, bookingID, strings.TrimSpace(seatCode)).Scan(dest...); err != nil {
		return out, err
	}
	return out, nil
}
