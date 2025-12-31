package repositories

import (
	"database/sql"
	"strings"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
)

type BookingSeat struct {
	SeatCode  string
	RouteFrom string
	RouteTo   string
	TripDate  string
	TripTime  string
}

type BookingSeatRepository struct {
	DB *sql.DB
}

func (r BookingSeatRepository) GetSeats(bookingID int64) ([]BookingSeat, error) {
	if bookingID <= 0 {
		return nil, nil
	}
	table := "booking_seats"
	if !intdb.HasTable(intconfig.DB, table) {
		return nil, nil
	}
	if !intdb.HasColumn(intconfig.DB, table, "booking_id") || !intdb.HasColumn(intconfig.DB, table, "seat_code") {
		return nil, nil
	}

	cols := []string{"seat_code"}
	if intdb.HasColumn(intconfig.DB, table, "route_from") {
		cols = append(cols, "route_from")
	} else {
		cols = append(cols, "''")
	}
	if intdb.HasColumn(intconfig.DB, table, "route_to") {
		cols = append(cols, "route_to")
	} else {
		cols = append(cols, "''")
	}
	if intdb.HasColumn(intconfig.DB, table, "trip_date") {
		cols = append(cols, "trip_date")
	} else {
		cols = append(cols, "''")
	}
	if intdb.HasColumn(intconfig.DB, table, "trip_time") {
		cols = append(cols, "trip_time")
	} else {
		cols = append(cols, "''")
	}

	query := `SELECT ` + strings.Join(cols, ",") + ` FROM ` + table + ` WHERE booking_id=? ORDER BY id ASC`
	rows, err := intconfig.DB.Query(query, bookingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []BookingSeat{}
	for rows.Next() {
		var s, rf, rt, td, tt sql.NullString
		if err := rows.Scan(&s, &rf, &rt, &td, &tt); err != nil {
			return out, err
		}
		out = append(out, BookingSeat{
			SeatCode:  strings.TrimSpace(s.String),
			RouteFrom: strings.TrimSpace(rf.String),
			RouteTo:   strings.TrimSpace(rt.String),
			TripDate:  strings.TrimSpace(td.String),
			TripTime:  strings.TrimSpace(tt.String),
		})
	}
	return out, rows.Err()
}
