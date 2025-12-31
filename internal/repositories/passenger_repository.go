package repositories

import (
	"database/sql"
	"strings"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
)

type PassengerSeatData struct {
	PassengerName  string
	PassengerPhone string
	Date           string
	DepartureTime  string
	PickupAddress  string
	DropoffAddress string
	TotalAmount    int64
	SelectedSeat   string
	ServiceType    string
	ETicketPhoto   string
	DriverName     string
	VehicleCode    string
	Notes          string
	TripRole       string
}

type PassengerRepository struct {
	DB *sql.DB
}

type PassengerDetail struct {
	ID             int64
	BookingID      int64
	TripRole       string
	SelectedSeat   string
	PassengerName  string
	PassengerPhone string
	Date           string
	DepartureTime  string
	PickupAddress  string
	DropoffAddress string
	ServiceType    string
	DriverName     string
	VehicleCode    string
	TotalAmount    int64
}

func (r PassengerRepository) db() *sql.DB {
	if r.DB != nil {
		return r.DB
	}
	return intconfig.DB
}

// GetByID fetches one passenger row adaptively.
func (r PassengerRepository) GetByID(id int64) (PassengerDetail, error) {
	table := "passengers"
	db := r.db()
	if !intdb.HasTable(db, table) || id <= 0 {
		return PassengerDetail{}, sql.ErrNoRows
	}
	if !intdb.HasColumn(db, table, "id") || !intdb.HasColumn(db, table, "booking_id") {
		return PassengerDetail{}, sql.ErrNoRows
	}

	hasTripRole := intdb.HasColumn(db, table, "trip_role")
	hasDate := intdb.HasColumn(db, table, "date")
	hasDepartureTime := intdb.HasColumn(db, table, "departure_time")
	hasPickup := intdb.HasColumn(db, table, "pickup_address")
	hasDropoff := intdb.HasColumn(db, table, "dropoff_address")
	hasServiceType := intdb.HasColumn(db, table, "service_type")
	hasDriver := intdb.HasColumn(db, table, "driver_name")
	hasVehicle := intdb.HasColumn(db, table, "vehicle_code")
	hasTotal := intdb.HasColumn(db, table, "total_amount")

	tripRoleSel := "''"
	if hasTripRole {
		tripRoleSel = "COALESCE(trip_role,'')"
	}
	dateSel := "''"
	if hasDate {
		dateSel = "COALESCE(date,'')"
	}
	timeSel := "''"
	if hasDepartureTime {
		timeSel = "COALESCE(departure_time,'')"
	}
	pickupSel := "''"
	if hasPickup {
		pickupSel = "COALESCE(pickup_address,'')"
	}
	dropoffSel := "''"
	if hasDropoff {
		dropoffSel = "COALESCE(dropoff_address,'')"
	}
	serviceSel := "''"
	if hasServiceType {
		serviceSel = "COALESCE(service_type,'')"
	}
	driverSel := "''"
	if hasDriver {
		driverSel = "COALESCE(driver_name,'')"
	}
	vehicleSel := "''"
	if hasVehicle {
		vehicleSel = "COALESCE(vehicle_code,'')"
	}
	totalSel := "0"
	if hasTotal {
		totalSel = "COALESCE(total_amount,0)"
	}

	var det PassengerDetail
	if err := db.QueryRow(`
		SELECT id,
			   COALESCE(booking_id,0),
			   COALESCE(selected_seats,''),
			   COALESCE(passenger_name,''),
			   COALESCE(passenger_phone,''),
			   `+tripRoleSel+`,
			   `+dateSel+`,
			   `+timeSel+`,
			   `+pickupSel+`,
			   `+dropoffSel+`,
			   `+serviceSel+`,
			   `+driverSel+`,
			   `+vehicleSel+`,
			   `+totalSel+`
		FROM `+table+`
		WHERE id=? LIMIT 1`, id).Scan(
		&det.ID,
		&det.BookingID,
		&det.SelectedSeat,
		&det.PassengerName,
		&det.PassengerPhone,
		&det.TripRole,
		&det.Date,
		&det.DepartureTime,
		&det.PickupAddress,
		&det.DropoffAddress,
		&det.ServiceType,
		&det.DriverName,
		&det.VehicleCode,
		&det.TotalAmount,
	); err != nil {
		return PassengerDetail{}, err
	}
	return det, nil
}

// UpsertPassenger melakukan insert/update berdasarkan booking_id + selected_seats (+ trip_role jika ada kolom).
func (r PassengerRepository) UpsertPassenger(bookingID int64, data PassengerSeatData) error {
	table := "passengers"
	db := r.db()
	if !intdb.HasTable(db, table) {
		return nil
	}
	if bookingID <= 0 || !intdb.HasColumn(db, table, "booking_id") || !intdb.HasColumn(db, table, "selected_seats") {
		return nil
	}

	tripRole := strings.TrimSpace(data.TripRole)
	hasTripRole := intdb.HasColumn(db, table, "trip_role")

	// cek existing
	var existingID sql.NullInt64
	if hasTripRole {
		_ = db.QueryRow(`SELECT id FROM `+table+` WHERE booking_id=? AND selected_seats=? AND trip_role=? LIMIT 1`, bookingID, strings.TrimSpace(data.SelectedSeat), tripRole).Scan(&existingID)
	} else {
		_ = db.QueryRow(`SELECT id FROM `+table+` WHERE booking_id=? AND selected_seats=? LIMIT 1`, bookingID, strings.TrimSpace(data.SelectedSeat)).Scan(&existingID)
	}

	if !existingID.Valid || existingID.Int64 == 0 {
		cols := []string{"booking_id", "selected_seats"}
		vals := []any{bookingID, strings.TrimSpace(data.SelectedSeat)}
		if hasTripRole {
			cols = append(cols, "trip_role")
			vals = append(vals, tripRole)
		}
		cols = append(cols, "passenger_name", "passenger_phone", "date", "departure_time", "pickup_address", "dropoff_address", "total_amount", "service_type", "eticket_photo", "driver_name", "vehicle_code", "notes")
		vals = append(vals,
			strings.TrimSpace(data.PassengerName),
			strings.TrimSpace(data.PassengerPhone),
			nullableString(data.Date),
			nullableString(data.DepartureTime),
			strings.TrimSpace(data.PickupAddress),
			strings.TrimSpace(data.DropoffAddress),
			data.TotalAmount,
			strings.TrimSpace(data.ServiceType),
			strings.TrimSpace(data.ETicketPhoto),
			strings.TrimSpace(data.DriverName),
			strings.TrimSpace(data.VehicleCode),
			strings.TrimSpace(data.Notes),
		)

		placeholders := make([]string, len(cols))
		for i := range placeholders {
			placeholders[i] = "?"
		}

		_, err := db.Exec(`INSERT INTO `+table+` (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(placeholders, ",")+`)`, vals...)
		return err
	}

	// UPDATE
	setTripRole := ""
	args := []any{
		strings.TrimSpace(data.PassengerName),
		strings.TrimSpace(data.PassengerPhone),
		nullableString(data.Date),
		nullableString(data.DepartureTime),
		strings.TrimSpace(data.PickupAddress),
		strings.TrimSpace(data.DropoffAddress),
		data.TotalAmount,
		strings.TrimSpace(data.ServiceType),
		strings.TrimSpace(data.ETicketPhoto),
		strings.TrimSpace(data.DriverName),
		strings.TrimSpace(data.VehicleCode),
		strings.TrimSpace(data.Notes),
	}
	whereArgs := []any{bookingID, strings.TrimSpace(data.SelectedSeat)}
	if hasTripRole {
		setTripRole = ", trip_role=?"
		whereArgs = append(whereArgs, tripRole)
		args = append(args, tripRole)
	}
	_, err := db.Exec(`
		UPDATE `+table+`
		SET passenger_name=?,
		    passenger_phone=?,
		    date=?,
		    departure_time=?,
		    pickup_address=?,
		    dropoff_address=?,
		    total_amount=?,
		    service_type=?,
		    eticket_photo=?,
		    driver_name=?,
		    vehicle_code=?,
		    notes=?`+setTripRole+`
		WHERE booking_id=? AND selected_seats=?`+func() string {
		if hasTripRole {
			return " AND trip_role=?"
		}
		return ""
	}(),
		append(args, whereArgs...)...,
	)
	return err
}

// UpsertSeat kept for compatibility (uses trip_role empty).
func (r PassengerRepository) UpsertSeat(bookingID int64, data PassengerSeatData) error {
	return r.UpsertPassenger(bookingID, data)
}

type PassengerFilter struct {
	BookingID int64
	TripRole  string
}

type PassengerRecord struct {
	ID             int64
	BookingID      int64
	TripRole       string
	SelectedSeat   string
	PassengerName  string
	PassengerPhone string
}

// ListPassengers returns passengers filtered by booking_id/trip_role when available.
func (r PassengerRepository) ListPassengers(f PassengerFilter) ([]PassengerRecord, error) {
	table := "passengers"
	db := r.db()
	if !intdb.HasTable(db, table) {
		return []PassengerRecord{}, nil
	}
	if !intdb.HasColumn(db, table, "booking_id") || !intdb.HasColumn(db, table, "selected_seats") {
		return []PassengerRecord{}, nil
	}

	where := []string{"1=1"}
	args := []any{}
	if f.BookingID > 0 {
		where = append(where, "booking_id=?")
		args = append(args, f.BookingID)
	}
	hasTripRole := intdb.HasColumn(db, table, "trip_role")
	if hasTripRole && strings.TrimSpace(f.TripRole) != "" {
		where = append(where, "trip_role=?")
		args = append(args, strings.TrimSpace(f.TripRole))
	}

	tripRoleSel := "''"
	if hasTripRole {
		tripRoleSel = "COALESCE(trip_role,'')"
	}

	rows, err := db.Query(`SELECT id, booking_id, COALESCE(selected_seats,''), `+tripRoleSel+`, COALESCE(passenger_name,''), COALESCE(passenger_phone,'') FROM `+table+` WHERE `+strings.Join(where, " AND ")+` ORDER BY id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PassengerRecord{}
	for rows.Next() {
		var rec PassengerRecord
		if err := rows.Scan(&rec.ID, &rec.BookingID, &rec.SelectedSeat, &rec.TripRole, &rec.PassengerName, &rec.PassengerPhone); err != nil {
			return out, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func nullableString(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
