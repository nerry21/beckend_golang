package repositories

import (
	"database/sql"
	"fmt"
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
	if db == nil || !intdb.HasTable(db, table) || id <= 0 {
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

	det.SelectedSeat = strings.ToUpper(strings.TrimSpace(det.SelectedSeat))
	det.PassengerName = strings.TrimSpace(det.PassengerName)
	det.PassengerPhone = strings.TrimSpace(det.PassengerPhone)
	return det, nil
}

// UpsertPassenger insert/update by booking_id + selected_seats (+ trip_role jika ada kolom).
// ✅ Adaptif terhadap kolom yang tersedia.
func (r PassengerRepository) UpsertPassenger(bookingID int64, data PassengerSeatData) error {
	table := "passengers"
	db := r.db()
	if db == nil {
		return fmt.Errorf("db tidak tersedia untuk passengers")
	}
	if !intdb.HasTable(db, table) {
		return fmt.Errorf("tabel passengers belum tersedia, jalankan migrasi passengers")
	}
	if bookingID <= 0 {
		return fmt.Errorf("booking_id tidak valid untuk passengers")
	}
	if !intdb.HasColumn(db, table, "booking_id") || !intdb.HasColumn(db, table, "selected_seats") {
		return fmt.Errorf("schema passengers belum siap: kolom booking_id/selected_seats tidak ditemukan")
	}

	seatCode := strings.ToUpper(strings.TrimSpace(data.SelectedSeat))
	if seatCode == "" {
		return fmt.Errorf("selected_seats kosong, lewati sinkronisasi seat")
	}

	// cek kolom-kolom opsional
	hasTripRole := intdb.HasColumn(db, table, "trip_role")
	hasName := intdb.HasColumn(db, table, "passenger_name")
	hasPhone := intdb.HasColumn(db, table, "passenger_phone")
	hasDate := intdb.HasColumn(db, table, "date")
	hasTime := intdb.HasColumn(db, table, "departure_time")
	hasPickup := intdb.HasColumn(db, table, "pickup_address")
	hasDropoff := intdb.HasColumn(db, table, "dropoff_address")
	hasTotal := intdb.HasColumn(db, table, "total_amount")
	hasService := intdb.HasColumn(db, table, "service_type")
	hasETicket := intdb.HasColumn(db, table, "eticket_photo")
	hasDriver := intdb.HasColumn(db, table, "driver_name")
	hasVehicle := intdb.HasColumn(db, table, "vehicle_code")
	hasNotes := intdb.HasColumn(db, table, "notes")

	if !hasName || !hasPhone {
		return fmt.Errorf("schema passengers belum siap: kolom passenger_name/passenger_phone tidak ditemukan")
	}

	tripRole := strings.TrimSpace(data.TripRole)

	// cek existing
	var existingID sql.NullInt64
	if hasTripRole {
		_ = db.QueryRow(`SELECT id FROM `+table+` WHERE booking_id=? AND selected_seats=? AND trip_role=? LIMIT 1`, bookingID, seatCode, tripRole).Scan(&existingID)
	} else {
		_ = db.QueryRow(`SELECT id FROM `+table+` WHERE booking_id=? AND selected_seats=? LIMIT 1`, bookingID, seatCode).Scan(&existingID)
	}

	// build insert
	buildInsert := func() (string, []any) {
		cols := []string{"booking_id", "selected_seats"}
		vals := []any{bookingID, seatCode}

		if hasTripRole {
			cols = append(cols, "trip_role")
			vals = append(vals, tripRole)
		}

		cols = append(cols, "passenger_name", "passenger_phone")
		vals = append(vals, strings.TrimSpace(data.PassengerName), strings.TrimSpace(data.PassengerPhone))

		if hasDate {
			cols = append(cols, "date")
			vals = append(vals, nullableString(data.Date))
		}
		if hasTime {
			cols = append(cols, "departure_time")
			vals = append(vals, nullableString(data.DepartureTime))
		}
		if hasPickup {
			cols = append(cols, "pickup_address")
			vals = append(vals, strings.TrimSpace(data.PickupAddress))
		}
		if hasDropoff {
			cols = append(cols, "dropoff_address")
			vals = append(vals, strings.TrimSpace(data.DropoffAddress))
		}
		if hasTotal {
			cols = append(cols, "total_amount")
			vals = append(vals, data.TotalAmount)
		}
		if hasService {
			cols = append(cols, "service_type")
			vals = append(vals, strings.TrimSpace(data.ServiceType))
		}
		if hasETicket {
			cols = append(cols, "eticket_photo")
			vals = append(vals, strings.TrimSpace(data.ETicketPhoto))
		}
		if hasDriver {
			cols = append(cols, "driver_name")
			vals = append(vals, strings.TrimSpace(data.DriverName))
		}
		if hasVehicle {
			cols = append(cols, "vehicle_code")
			vals = append(vals, strings.TrimSpace(data.VehicleCode))
		}
		if hasNotes {
			cols = append(cols, "notes")
			vals = append(vals, strings.TrimSpace(data.Notes))
		}

		ph := make([]string, len(cols))
		for i := range ph {
			ph[i] = "?"
		}

		q := `INSERT INTO ` + table + ` (` + strings.Join(cols, ",") + `) VALUES (` + strings.Join(ph, ",") + `)`
		return q, vals
	}

	// build update
	buildUpdate := func() (string, []any) {
		set := []string{}
		args := []any{}

		set = append(set, "passenger_name=?")
		args = append(args, strings.TrimSpace(data.PassengerName))

		set = append(set, "passenger_phone=?")
		args = append(args, strings.TrimSpace(data.PassengerPhone))

		if hasDate {
			set = append(set, "date=?")
			args = append(args, nullableString(data.Date))
		}
		if hasTime {
			set = append(set, "departure_time=?")
			args = append(args, nullableString(data.DepartureTime))
		}
		if hasPickup {
			set = append(set, "pickup_address=?")
			args = append(args, strings.TrimSpace(data.PickupAddress))
		}
		if hasDropoff {
			set = append(set, "dropoff_address=?")
			args = append(args, strings.TrimSpace(data.DropoffAddress))
		}
		if hasTotal {
			set = append(set, "total_amount=?")
			args = append(args, data.TotalAmount)
		}
		if hasService {
			set = append(set, "service_type=?")
			args = append(args, strings.TrimSpace(data.ServiceType))
		}
		if hasETicket {
			set = append(set, "eticket_photo=?")
			args = append(args, strings.TrimSpace(data.ETicketPhoto))
		}
		if hasDriver {
			set = append(set, "driver_name=?")
			args = append(args, strings.TrimSpace(data.DriverName))
		}
		if hasVehicle {
			set = append(set, "vehicle_code=?")
			args = append(args, strings.TrimSpace(data.VehicleCode))
		}
		if hasNotes {
			set = append(set, "notes=?")
			args = append(args, strings.TrimSpace(data.Notes))
		}

		if hasTripRole {
			set = append(set, "trip_role=?")
			args = append(args, tripRole)
		}

		where := " WHERE booking_id=? AND selected_seats=?"
		args = append(args, bookingID, seatCode)
		if hasTripRole {
			where += " AND trip_role=?"
			args = append(args, tripRole)
		}

		q := `UPDATE ` + table + ` SET ` + strings.Join(set, ", ") + where
		return q, args
	}

	if !existingID.Valid || existingID.Int64 == 0 {
		q, vals := buildInsert()
		_, err := db.Exec(q, vals...)
		return err
	}

	q, args := buildUpdate()
	_, err := db.Exec(q, args...)
	return err
}

// UpsertSeat kept for compatibility.
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

func (r PassengerRepository) ListPassengers(f PassengerFilter) ([]PassengerRecord, error) {
	table := "passengers"
	db := r.db()
	if db == nil || !intdb.HasTable(db, table) {
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

	rows, err := db.Query(
		`SELECT id, booking_id, COALESCE(selected_seats,''), `+tripRoleSel+`, COALESCE(passenger_name,''), COALESCE(passenger_phone,'')
		 FROM `+table+` WHERE `+strings.Join(where, " AND ")+` ORDER BY id ASC`, args...,
	)
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
		rec.SelectedSeat = strings.ToUpper(strings.TrimSpace(rec.SelectedSeat))
		rec.PassengerName = strings.TrimSpace(rec.PassengerName)
		rec.PassengerPhone = strings.TrimSpace(rec.PassengerPhone)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func nullableString(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.TrimSpace(s)
}
