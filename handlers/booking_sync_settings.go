package handlers

import (
	"backend/config"
	"strings"
	"time"
)

// Sinkronisasi driver & kendaraan kepulangan ke trip_information + passengers
// menggunakan data yang sudah diisi di Pengaturan Kepulangan (return_settings).
func syncReturnDriverVehicleFromSettings(bookingID int64, tripNumber, driverName, vehicleCode, vehicleType string) {
	driverName = strings.TrimSpace(driverName)
	vehicleCode = strings.TrimSpace(vehicleCode)
	vehicleType = strings.TrimSpace(vehicleType)
	tripNumber = strings.TrimSpace(tripNumber)

	tx, err := config.DB.Begin()
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback() }()

	// cari trip_number jika kosong (ambil dari return_settings berdasarkan booking_id)
	if tripNumber == "" && bookingID > 0 && hasTable(tx, "return_settings") {
		_ = tx.QueryRow(`SELECT COALESCE(trip_number,'') FROM return_settings WHERE booking_id=? ORDER BY id DESC LIMIT 1`, bookingID).Scan(&tripNumber)
		tripNumber = strings.TrimSpace(tripNumber)
	}

	vehicleVal := vehicleCode
	if vehicleVal == "" {
		vehicleVal = vehicleType
	}

	// update trip_information (berdasarkan trip_number)
	if tripNumber != "" && hasTable(tx, "trip_information") {
		setParts := []string{}
		args := []any{}

		if hasColumn(tx, "trip_information", "driver_name") && driverName != "" {
			setParts = append(setParts, "driver_name=?")
			args = append(args, driverName)
		}
		if hasColumn(tx, "trip_information", "vehicle_code") && vehicleVal != "" {
			setParts = append(setParts, "vehicle_code=?")
			args = append(args, vehicleVal)
		}
		if hasColumn(tx, "trip_information", "vehicle_name") && vehicleType != "" {
			setParts = append(setParts, "vehicle_name=?")
			args = append(args, vehicleType)
		}
		if hasColumn(tx, "trip_information", "vehicle") && vehicleVal != "" {
			setParts = append(setParts, "vehicle=?")
			args = append(args, vehicleVal)
		}
		if hasColumn(tx, "trip_information", "vehicle_type") && vehicleType != "" {
			setParts = append(setParts, "vehicle_type=?")
			args = append(args, vehicleType)
		}
		if hasColumn(tx, "trip_information", "updated_at") {
			setParts = append(setParts, "updated_at=?")
			args = append(args, time.Now())
		}

		if len(setParts) > 0 {
			args = append(args, tripNumber)
			_, _ = tx.Exec(`UPDATE trip_information SET `+strings.Join(setParts, ",")+` WHERE trip_number=?`, args...)
		}
	}

	// update passengers (berdasarkan booking_id)
	if bookingID > 0 && hasTable(tx, "passengers") && hasColumn(tx, "passengers", "booking_id") {
		setParts := []string{}
		args := []any{}

		if hasColumn(tx, "passengers", "driver_name") && driverName != "" {
			setParts = append(setParts, "driver_name=?")
			args = append(args, driverName)
		}
		if hasColumn(tx, "passengers", "driver") && driverName != "" {
			setParts = append(setParts, "driver=?")
			args = append(args, driverName)
		}
		if hasColumn(tx, "passengers", "vehicle_name") && vehicleType != "" {
			setParts = append(setParts, "vehicle_name=?")
			args = append(args, vehicleType)
		}
		if hasColumn(tx, "passengers", "vehicle_type") && vehicleType != "" {
			setParts = append(setParts, "vehicle_type=?")
			args = append(args, vehicleType)
		}
		if hasColumn(tx, "passengers", "vehicle_code") && vehicleVal != "" {
			setParts = append(setParts, "vehicle_code=?")
			args = append(args, vehicleVal)
		}
		if hasColumn(tx, "passengers", "vehicle") && vehicleVal != "" {
			setParts = append(setParts, "vehicle=?")
			args = append(args, vehicleVal)
		}
		if hasColumn(tx, "passengers", "updated_at") {
			setParts = append(setParts, "updated_at=?")
			args = append(args, time.Now())
		}

		if len(setParts) > 0 {
			args = append(args, bookingID)
			_, _ = tx.Exec(`UPDATE passengers SET `+strings.Join(setParts, ", ")+` WHERE booking_id=?`, args...)
		}
	}

	_ = tx.Commit()
}
