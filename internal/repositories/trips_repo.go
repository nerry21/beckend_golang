package repositories

import (
	"database/sql"
	"fmt"
	"strings"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
)

type TripFinance struct {
	ID             int64  `json:"id"`
	BookingID      int64  `json:"booking_id"`
	TripRole       string `json:"trip_role"`
	RouteFrom      string `json:"route_from"`
	RouteTo        string `json:"route_to"`
	DepartureDate  string `json:"departure_date"`
	DepartureTime  string `json:"departure_time"`
	ServiceType    string `json:"service_type"`
	DriverName     string `json:"driver_name"`
	VehicleCode    string `json:"vehicle_code"`
	PassengerCount int    `json:"passenger_count"`
}

type TripsRepository struct {
	DB *sql.DB
}

func (r TripsRepository) db() *sql.DB {
	if r.DB != nil {
		return r.DB
	}
	return intconfig.DB
}

// ListFinanceTrips returns trips from departure_settings or return_settings based on tripRole.
// tripRole: "berangkat" uses departure_settings; "pulang" uses return_settings.
func (r TripsRepository) ListFinanceTrips(tripRole, startDate, endDate string) ([]TripFinance, error) {
	role := strings.ToLower(strings.TrimSpace(tripRole))
	table := "departure_settings"
	if role == "pulang" {
		table = "return_settings"
	}

	db := r.db()
	if db == nil || !intdb.HasTable(db, table) {
		return []TripFinance{}, nil
	}

	hasDate := intdb.HasColumn(db, table, "departure_date")
	hasTime := intdb.HasColumn(db, table, "departure_time")
	hasRouteFrom := intdb.HasColumn(db, table, "route_from")
	hasRouteTo := intdb.HasColumn(db, table, "route_to")
	hasPassengerCount := intdb.HasColumn(db, table, "passenger_count")
	hasServiceType := intdb.HasColumn(db, table, "service_type")
	hasDriverName := intdb.HasColumn(db, table, "driver_name")
	hasVehicleCode := intdb.HasColumn(db, table, "vehicle_code")
	hasBookingID := intdb.HasColumn(db, table, "booking_id")

	cols := []string{"id"}
	if hasBookingID {
		cols = append(cols, "COALESCE(booking_id,0)")
	} else {
		cols = append(cols, "0")
	}
	if hasRouteFrom {
		cols = append(cols, "COALESCE(route_from,'')")
	} else {
		cols = append(cols, "''")
	}
	if hasRouteTo {
		cols = append(cols, "COALESCE(route_to,'')")
	} else {
		cols = append(cols, "''")
	}
	if hasDate {
		cols = append(cols, "COALESCE(departure_date,'')")
	} else {
		cols = append(cols, "''")
	}
	if hasTime {
		cols = append(cols, "COALESCE(departure_time,'')")
	} else {
		cols = append(cols, "''")
	}
	if hasServiceType {
		cols = append(cols, "COALESCE(service_type,'')")
	} else {
		cols = append(cols, "''")
	}
	if hasDriverName {
		cols = append(cols, "COALESCE(driver_name,'')")
	} else {
		cols = append(cols, "''")
	}
	if hasVehicleCode {
		cols = append(cols, "COALESCE(vehicle_code,'')")
	} else {
		cols = append(cols, "''")
	}
	if hasPassengerCount {
		cols = append(cols, "COALESCE(passenger_count,0)")
	} else {
		cols = append(cols, "0")
	}

	where := []string{"1=1"}
	args := []any{}
	if hasDate && strings.TrimSpace(startDate) != "" {
		where = append(where, "departure_date>=?")
		args = append(args, strings.TrimSpace(startDate))
	}
	if hasDate && strings.TrimSpace(endDate) != "" {
		where = append(where, "departure_date<=?")
		args = append(args, strings.TrimSpace(endDate))
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE %s ORDER BY departure_date ASC, id ASC`, strings.Join(cols, ","), table, strings.Join(where, " AND "))

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []TripFinance{}
	for rows.Next() {
		var rec TripFinance
		if err := rows.Scan(
			&rec.ID,
			&rec.BookingID,
			&rec.RouteFrom,
			&rec.RouteTo,
			&rec.DepartureDate,
			&rec.DepartureTime,
			&rec.ServiceType,
			&rec.DriverName,
			&rec.VehicleCode,
			&rec.PassengerCount,
		); err != nil {
			return out, err
		}
		rec.TripRole = role
		out = append(out, rec)
	}
	return out, rows.Err()
}
