package repositories

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
)

type TripInformation struct {
	TripNumber    string
	TripDetails   string
	DepartureDate string
	DepartureTime string
	DriverName    string
	VehicleCode   string
	LicensePlate  string
	ESuratJalan   string
	BookingID     int64
	TripRole      string
}

type TripInformationRepository struct {
	DB *sql.DB
}

// Upsert melakukan insert/update trip_information berdasarkan trip_number.
// Menghindari overwrite nilai kosong; hanya field yang terisi yang di-set.
func (r TripInformationRepository) Upsert(info TripInformation) error {
	table := "trip_information"
	if !intdb.HasTable(intconfig.DB, table) {
		return nil
	}

	tripNumber := strings.TrimSpace(info.TripNumber)
	if tripNumber == "" {
		return fmt.Errorf("trip_number kosong")
	}

	var existingID int64
	if intdb.HasColumn(intconfig.DB, table, "trip_number") {
		_ = intconfig.DB.QueryRow(`SELECT id FROM `+table+` WHERE trip_number=? LIMIT 1`, tripNumber).Scan(&existingID)
	}

	now := time.Now()

	if existingID == 0 {
		cols := []string{"trip_number"}
		vals := []any{tripNumber}

		add := func(col string, val any) {
			switch v := val.(type) {
			case string:
				if strings.TrimSpace(v) == "" {
					return
				}
			case int64:
				if v == 0 {
					return
				}
			}
			if intdb.HasColumn(intconfig.DB, table, col) {
				cols = append(cols, col)
				vals = append(vals, val)
			}
		}

		add("trip_details", info.TripDetails)
		add("departure_date", info.DepartureDate)
		add("departure_time", info.DepartureTime)
		add("driver_name", info.DriverName)
		add("vehicle_code", info.VehicleCode)
		add("license_plate", info.LicensePlate)
		add("e_surat_jalan", info.ESuratJalan)
		if info.BookingID > 0 {
			add("booking_id", info.BookingID)
		}
		if intdb.HasColumn(intconfig.DB, table, "created_at") {
			cols = append(cols, "created_at")
			vals = append(vals, now)
		}
		if intdb.HasColumn(intconfig.DB, table, "updated_at") {
			cols = append(cols, "updated_at")
			vals = append(vals, now)
		}

		ph := make([]string, len(cols))
		for i := range ph {
			if cols[i] == "booking_id" {
				ph[i] = "NULLIF(?,0)"
			} else {
				ph[i] = "?"
			}
		}

		_, err := intconfig.DB.Exec(`INSERT INTO `+table+` (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(ph, ",")+`)`, vals...)
		return err
	}

	sets := []string{}
	args := []any{}
	addSet := func(col string, val any) {
		switch v := val.(type) {
		case string:
			if strings.TrimSpace(v) == "" {
				return
			}
		case int64:
			if v == 0 {
				return
			}
		}
		if intdb.HasColumn(intconfig.DB, table, col) {
			if col == "booking_id" {
				sets = append(sets, col+"=NULLIF(?,0)")
			} else {
				sets = append(sets, col+"=?")
			}
			args = append(args, val)
		}
	}

	addSet("trip_details", info.TripDetails)
	addSet("departure_date", info.DepartureDate)
	addSet("departure_time", info.DepartureTime)
	addSet("driver_name", info.DriverName)
	addSet("vehicle_code", info.VehicleCode)
	addSet("license_plate", info.LicensePlate)
	addSet("e_surat_jalan", info.ESuratJalan)
	if info.BookingID > 0 {
		addSet("booking_id", info.BookingID)
	}
	if intdb.HasColumn(intconfig.DB, table, "updated_at") {
		sets = append(sets, "updated_at=?")
		args = append(args, now)
	}

	if len(sets) == 0 {
		return nil
	}

	args = append(args, existingID)
	_, err := intconfig.DB.Exec(`UPDATE `+table+` SET `+strings.Join(sets, ",")+` WHERE id=?`, args...)
	return err
}

// UpsertTripInfo upserts by booking_id (+ trip_role if available), falling back to trip_number when necessary.
func (r TripInformationRepository) UpsertTripInfo(info TripInformation) error {
	table := "trip_information"
	if !intdb.HasTable(intconfig.DB, table) {
		return nil
	}
	db := intconfig.DB
	tripRole := strings.TrimSpace(info.TripRole)
	hasTripRole := intdb.HasColumn(db, table, "trip_role")

	var existingID int64
	if info.BookingID > 0 && intdb.HasColumn(db, table, "booking_id") {
		if hasTripRole && tripRole != "" {
			_ = db.QueryRow(`SELECT id FROM `+table+` WHERE booking_id=? AND trip_role=? LIMIT 1`, info.BookingID, tripRole).Scan(&existingID)
		} else {
			_ = db.QueryRow(`SELECT id FROM `+table+` WHERE booking_id=? LIMIT 1`, info.BookingID).Scan(&existingID)
		}
	}
	if existingID == 0 && strings.TrimSpace(info.TripNumber) != "" && intdb.HasColumn(db, table, "trip_number") {
		_ = db.QueryRow(`SELECT id FROM `+table+` WHERE trip_number=? LIMIT 1`, strings.TrimSpace(info.TripNumber)).Scan(&existingID)
	}

	now := time.Now()
	if existingID == 0 {
		cols := []string{"trip_number"}
		vals := []any{strings.TrimSpace(info.TripNumber)}
		add := func(col string, val any) {
		switch v := val.(type) {
		case string:
			if strings.TrimSpace(v) == "" {
				return
			}
		case int64:
			if v == 0 {
				return
			}
		}
		if intdb.HasColumn(db, table, col) {
			cols = append(cols, col)
			vals = append(vals, val)
		}
	}
		add("trip_details", info.TripDetails)
		add("departure_date", info.DepartureDate)
		add("departure_time", info.DepartureTime)
		add("driver_name", info.DriverName)
		add("vehicle_code", info.VehicleCode)
		add("license_plate", info.LicensePlate)
		add("e_surat_jalan", info.ESuratJalan)
		if info.BookingID > 0 {
			add("booking_id", info.BookingID)
		}
		if hasTripRole && tripRole != "" {
			add("trip_role", tripRole)
		}
		if intdb.HasColumn(db, table, "created_at") {
			cols = append(cols, "created_at")
			vals = append(vals, now)
		}
		if intdb.HasColumn(db, table, "updated_at") {
			cols = append(cols, "updated_at")
			vals = append(vals, now)
		}

		ph := make([]string, len(cols))
		for i := range ph {
			if cols[i] == "booking_id" {
				ph[i] = "NULLIF(?,0)"
			} else {
				ph[i] = "?"
			}
		}
		_, err := db.Exec(`INSERT INTO `+table+` (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(ph, ",")+`)`, vals...)
		return err
	}

	sets := []string{}
	args := []any{}
	addSet := func(col string, val any) {
		switch v := val.(type) {
		case string:
			if strings.TrimSpace(v) == "" {
				return
			}
		case int64:
			if v == 0 {
				return
			}
		}
		if intdb.HasColumn(db, table, col) {
			if col == "booking_id" {
				sets = append(sets, col+"=NULLIF(?,0)")
			} else {
				sets = append(sets, col+"=?")
			}
			args = append(args, val)
		}
	}
	addSet("trip_number", info.TripNumber)
	addSet("trip_details", info.TripDetails)
	addSet("departure_date", info.DepartureDate)
	addSet("departure_time", info.DepartureTime)
	addSet("driver_name", info.DriverName)
	addSet("vehicle_code", info.VehicleCode)
	addSet("license_plate", info.LicensePlate)
	addSet("e_surat_jalan", info.ESuratJalan)
	if info.BookingID > 0 {
		addSet("booking_id", info.BookingID)
	}
	if hasTripRole && tripRole != "" {
		addSet("trip_role", tripRole)
	}
	if intdb.HasColumn(db, table, "updated_at") {
		sets = append(sets, "updated_at=?")
		args = append(args, now)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, existingID)
	_, err := db.Exec(`UPDATE `+table+` SET `+strings.Join(sets, ",")+` WHERE id=?`, args...)
	return err
}
