package repositories

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"backend/config"
	legacy "backend/handlers"
	intdb "backend/internal/db"
)

// ReturnRepository wraps DB access for return_settings with key-presence PATCH semantics.
type ReturnRepository struct {
	DB *sql.DB
}

func (r ReturnRepository) db() *sql.DB {
	if r.DB != nil {
		return r.DB
	}
	return config.DB
}

// GetByID loads return_settings row.
func (r ReturnRepository) GetByID(id int) (legacy.DepartureSetting, error) {
	if id <= 0 {
		return legacy.DepartureSetting{}, sql.ErrNoRows
	}
	table := "return_settings"
	db := r.db()
	if db == nil || !intdb.HasTable(db, table) {
		return legacy.DepartureSetting{}, sql.ErrNoRows
	}
	var d legacy.DepartureSetting
	var count int
	var depTime, routeFrom, routeTo, vehicleType sql.NullString
	var createdAt sql.NullString

	err := db.QueryRow(`
		SELECT
			id,
			COALESCE(booking_name,''),
			COALESCE(phone,''),
			COALESCE(pickup_address,''),
			COALESCE(departure_date,''),
			COALESCE(seat_numbers,''),
			COALESCE(passenger_count,0),
			COALESCE(service_type,''),
			COALESCE(driver_name,''),
			COALESCE(vehicle_code,''),
			COALESCE(surat_jalan_file,''),
			COALESCE(surat_jalan_file_name,''),
			COALESCE(departure_status,''),
			COALESCE(trip_number,''),
			COALESCE(booking_id,0),
			COALESCE(departure_time,''), COALESCE(route_from,''), COALESCE(route_to,''), COALESCE(vehicle_type,''),
			COALESCE(created_at,'')
		FROM `+table+` WHERE id=? LIMIT 1`, id).Scan(
		&d.ID,
		&d.BookingName,
		&d.Phone,
		&d.PickupAddress,
		&d.DepartureDate,
		&d.SeatNumbers,
		&count,
		&d.ServiceType,
		&d.DriverName,
		&d.VehicleCode,
		&d.SuratJalanFile,
		&d.SuratJalanFileName,
		&d.DepartureStatus,
		&d.TripNumber,
		&d.BookingID,
		&depTime, &routeFrom, &routeTo, &vehicleType,
		&createdAt,
	)
	if err != nil {
		return legacy.DepartureSetting{}, err
	}
	d.PassengerCount = strconv.Itoa(count)
	d.DepartureTime = strings.TrimSpace(depTime.String)
	d.RouteFrom = strings.TrimSpace(routeFrom.String)
	d.RouteTo = strings.TrimSpace(routeTo.String)
	d.VehicleType = strings.TrimSpace(vehicleType.String)
	d.CreatedAt = strings.TrimSpace(createdAt.String)
	return d, nil
}

// GetByBookingID loads by booking_id if exists.
func (r ReturnRepository) GetByBookingID(bookingID int64) (legacy.DepartureSetting, error) {
	if bookingID <= 0 {
		return legacy.DepartureSetting{}, sql.ErrNoRows
	}
	table := "return_settings"
	db := r.db()
	if db == nil || !intdb.HasTable(db, table) || !intdb.HasColumn(db, table, "booking_id") {
		return legacy.DepartureSetting{}, sql.ErrNoRows
	}
	var id int
	if err := db.QueryRow(`SELECT id FROM `+table+` WHERE booking_id=? ORDER BY id DESC LIMIT 1`, bookingID).Scan(&id); err != nil {
		return legacy.DepartureSetting{}, err
	}

	var d legacy.DepartureSetting
	var count int
	var createdAt sql.NullString
	var depTime, routeFrom, routeTo, vehicleType sql.NullString
	_ = db.QueryRow(`
		SELECT
			id,
			COALESCE(booking_name,''),
			COALESCE(phone,''),
			COALESCE(pickup_address,''),
			COALESCE(departure_date,''),
			COALESCE(seat_numbers,''),
			COALESCE(passenger_count,0),
			COALESCE(service_type,''),
			COALESCE(driver_name,''),
			COALESCE(vehicle_code,''),
			COALESCE(surat_jalan_file,''),
			COALESCE(surat_jalan_file_name,''),
			COALESCE(departure_status,''),
			COALESCE(trip_number,''),
			COALESCE(booking_id,0),
			COALESCE(departure_time,''), COALESCE(route_from,''), COALESCE(route_to,''), COALESCE(vehicle_type,''),
			COALESCE(created_at,'')
		FROM `+table+` WHERE id=? LIMIT 1`, id).Scan(
		&d.ID,
		&d.BookingName,
		&d.Phone,
		&d.PickupAddress,
		&d.DepartureDate,
		&d.SeatNumbers,
		&count,
		&d.ServiceType,
		&d.DriverName,
		&d.VehicleCode,
		&d.SuratJalanFile,
		&d.SuratJalanFileName,
		&d.DepartureStatus,
		&d.TripNumber,
		&d.BookingID,
		&depTime, &routeFrom, &routeTo, &vehicleType,
		&createdAt,
	)
	d.PassengerCount = strconv.Itoa(count)
	d.DepartureTime = strings.TrimSpace(depTime.String)
	d.RouteFrom = strings.TrimSpace(routeFrom.String)
	d.RouteTo = strings.TrimSpace(routeTo.String)
	d.VehicleType = strings.TrimSpace(vehicleType.String)
	d.CreatedAt = strings.TrimSpace(createdAt.String)
	return d, nil
}

// CreateFromBooking upserts return_settings keyed by booking_id.
func (r ReturnRepository) CreateFromBooking(dep legacy.DepartureSetting) (legacy.DepartureSetting, error) {
	table := "return_settings"
	db := r.db()
	if db == nil || !intdb.HasTable(db, table) {
		return dep, fmt.Errorf("tabel return_settings tidak ditemukan")
	}

	var existingID int
	if intdb.HasColumn(db, table, "booking_id") {
		_ = db.QueryRow(`SELECT id FROM `+table+` WHERE booking_id=? LIMIT 1`, dep.BookingID).Scan(&existingID)
	}

	cols := []string{}
	vals := []any{}
	add := func(col string, val any) {
		if intdb.HasColumn(db, table, col) {
			cols = append(cols, col)
			vals = append(vals, val)
		}
	}

	add("booking_name", dep.BookingName)
	add("phone", dep.Phone)
	add("pickup_address", dep.PickupAddress)
	add("departure_date", dep.DepartureDate)
	add("seat_numbers", dep.SeatNumbers)
	if intdb.HasColumn(db, table, "passenger_count") {
		pc, _ := strconv.Atoi(strings.TrimSpace(dep.PassengerCount))
		cols = append(cols, "passenger_count")
		vals = append(vals, pc)
	}
	add("service_type", dep.ServiceType)
	add("driver_name", dep.DriverName)
	add("vehicle_code", dep.VehicleCode)
	add("vehicle_type", dep.VehicleType)
	add("surat_jalan_file", dep.SuratJalanFile)
	add("surat_jalan_file_name", dep.SuratJalanFileName)
	add("departure_status", dep.DepartureStatus)
	add("route_from", dep.RouteFrom)
	add("route_to", dep.RouteTo)
	add("trip_number", dep.TripNumber)
	add("departure_time", dep.DepartureTime)
	if intdb.HasColumn(db, table, "booking_id") {
		cols = append(cols, "booking_id")
		vals = append(vals, dep.BookingID)
	}

	now := time.Now()
	if existingID == 0 {
		if intdb.HasColumn(db, table, "created_at") {
			cols = append(cols, "created_at")
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
		if _, err := db.Exec(`INSERT INTO `+table+` (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(ph, ",")+`)`, vals...); err != nil {
			return dep, err
		}
		_ = db.QueryRow(`SELECT id FROM `+table+` WHERE booking_id=? ORDER BY id DESC LIMIT 1`, dep.BookingID).Scan(&existingID)
	} else {
		if intdb.HasColumn(db, table, "updated_at") {
			cols = append(cols, "updated_at")
			vals = append(vals, now)
		}
		setParts := make([]string, len(cols))
		for i, c := range cols {
			if c == "booking_id" {
				setParts[i] = c + "=NULLIF(?,0)"
			} else {
				setParts[i] = c + "=?"
			}
		}
		vals = append(vals, existingID)
		if _, err := db.Exec(`UPDATE `+table+` SET `+strings.Join(setParts, ",")+` WHERE id=?`, vals...); err != nil {
			return dep, err
		}
	}

	if existingID > 0 {
		return r.GetByBookingID(dep.BookingID)
	}
	return dep, nil
}

// UpdatePartial applies only fields present in raw JSON (key presence).
func (r ReturnRepository) UpdatePartial(id int, rawJSON []byte) (legacy.DepartureSetting, error) {
	if id <= 0 {
		return legacy.DepartureSetting{}, sql.ErrNoRows
	}
	table := "return_settings"
	db := r.db()
	if db == nil || !intdb.HasTable(db, table) {
		return legacy.DepartureSetting{}, fmt.Errorf("tabel return_settings tidak ditemukan")
	}

	existing, err := r.GetByBookingIDFromPayload(rawJSON)
	if err != nil || existing.ID == 0 {
		// fallback to load by id if booking_id not provided
		if existing, err = r.GetByID(id); err != nil {
			return legacy.DepartureSetting{}, err
		}
	}

	merged, presence, count, err := buildReturnPatch(existing, rawJSON)
	if err != nil {
		return merged, err
	}

	sets := []string{}
	args := []any{}
	add := func(cond bool, column string, val any) {
		if cond && intdb.HasColumn(db, table, column) {
			sets = append(sets, column+"=?")
			args = append(args, val)
		}
	}

	add(presence.BookingName, "booking_name", merged.BookingName)
	add(presence.Phone, "phone", merged.Phone)
	add(presence.PickupAddress, "pickup_address", merged.PickupAddress)
	if presence.DepartureDate && intdb.HasColumn(db, table, "departure_date") {
		sets = append(sets, "departure_date=?")
		args = append(args, nullIfEmptyString(merged.DepartureDate))
	}
	add(presence.SeatNumbers, "seat_numbers", merged.SeatNumbers)
	if presence.PassengerCount && intdb.HasColumn(db, table, "passenger_count") {
		sets = append(sets, "passenger_count=?")
		args = append(args, count)
	}
	add(presence.ServiceType, "service_type", merged.ServiceType)
	add(presence.DriverName, "driver_name", merged.DriverName)
	add(presence.VehicleCode, "vehicle_code", merged.VehicleCode)
	add(presence.VehicleType, "vehicle_type", merged.VehicleType)
	add(presence.SuratJalanFile, "surat_jalan_file", merged.SuratJalanFile)
	add(presence.SuratJalanFileName, "surat_jalan_file_name", merged.SuratJalanFileName)
	add(presence.DepartureStatus, "departure_status", merged.DepartureStatus)
	if presence.DepartureTime && intdb.HasColumn(db, table, "departure_time") {
		sets = append(sets, "departure_time=?")
		args = append(args, nullIfEmptyString(merged.DepartureTime))
	}
	add(presence.RouteFrom, "route_from", merged.RouteFrom)
	add(presence.RouteTo, "route_to", merged.RouteTo)
	add(presence.TripNumber, "trip_number", merged.TripNumber)
	if presence.BookingID && intdb.HasColumn(db, table, "booking_id") {
		sets = append(sets, "booking_id=NULLIF(?,0)")
		args = append(args, merged.BookingID)
	}

	if intdb.HasColumn(db, table, "updated_at") && len(sets) > 0 {
		sets = append(sets, "updated_at=?")
		args = append(args, time.Now())
	}
	if len(sets) == 0 {
		return merged, nil
	}
	args = append(args, id)
	if _, err := db.Exec(`UPDATE `+table+` SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
		return merged, err
	}

	merged.ID = id
	merged.PassengerCount = strconv.Itoa(count)
	return merged, nil
}

type returnFieldPresence struct {
	BookingName        bool
	Phone              bool
	PickupAddress      bool
	DepartureDate      bool
	SeatNumbers        bool
	PassengerCount     bool
	ServiceType        bool
	DriverName         bool
	VehicleCode        bool
	VehicleType        bool
	SuratJalanFile     bool
	SuratJalanFileName bool
	DepartureStatus    bool
	DepartureTime      bool
	RouteFrom          bool
	RouteTo            bool
	TripNumber         bool
	BookingID          bool
}

// GetByBookingIDFromPayload tries to parse booking_id from raw JSON before loading.
func (r ReturnRepository) GetByBookingIDFromPayload(rawJSON []byte) (legacy.DepartureSetting, error) {
	var input legacy.DepartureSetting
	if err := json.Unmarshal(rawJSON, &input); err != nil {
		return legacy.DepartureSetting{}, err
	}
	if input.BookingID > 0 {
		return r.GetByBookingID(input.BookingID)
	}
	return legacy.DepartureSetting{}, sql.ErrNoRows
}

// buildReturnPatch merges payload into existing row while respecting key presence semantics.
func buildReturnPatch(existing legacy.DepartureSetting, rawJSON []byte) (legacy.DepartureSetting, returnFieldPresence, int, error) {
	payloadKeys := map[string]bool{}
	var payloadMap map[string]any
	if err := json.Unmarshal(rawJSON, &payloadMap); err == nil {
		for k := range payloadMap {
			payloadKeys[strings.ToLower(k)] = true
		}
	}
	hasField := func(names ...string) bool {
		for _, n := range names {
			if payloadKeys[strings.ToLower(n)] {
				return true
			}
		}
		return false
	}

	var input legacy.DepartureSetting
	if err := json.Unmarshal(rawJSON, &input); err != nil {
		return existing, returnFieldPresence{}, 0, err
	}

	presence := returnFieldPresence{
		BookingName:        hasField("bookingname", "booking_name"),
		Phone:              hasField("phone"),
		PickupAddress:      hasField("pickupaddress", "pickup_address"),
		DepartureDate:      hasField("departuredate", "departure_date"),
		SeatNumbers:        hasField("seatnumbers", "seat_numbers"),
		PassengerCount:     hasField("passengercount", "passenger_count"),
		ServiceType:        hasField("servicetype", "service_type"),
		DriverName:         hasField("drivername", "driver_name"),
		VehicleCode:        hasField("vehiclecode", "vehicle_code"),
		VehicleType:        hasField("vehicletype", "vehicle_type"),
		SuratJalanFile:     hasField("suratjalanfile", "surat_jalan_file"),
		SuratJalanFileName: hasField("suratjalanfilename", "surat_jalan_file_name"),
		DepartureStatus:    hasField("departurestatus", "departure_status"),
		DepartureTime:      hasField("departuretime", "departure_time"),
		RouteFrom:          hasField("routefrom", "route_from"),
		RouteTo:            hasField("routeto", "route_to"),
		TripNumber:         hasField("tripnumber", "trip_number"),
		BookingID:          hasField("bookingid", "booking_id"),
	}

	// booking_id bisa hadir sebagai snake_case, isi manual jika belum terisi.
	if presence.BookingID && input.BookingID == 0 {
		if v, ok := payloadMap["booking_id"]; ok {
			switch val := v.(type) {
			case float64:
				input.BookingID = int64(val)
			case int64:
				input.BookingID = val
			case int:
				input.BookingID = int64(val)
			case string:
				if n, err := strconv.ParseInt(val, 10, 64); err == nil {
					input.BookingID = n
				}
			}
		}
	}

	merged := existing
	if presence.BookingName && strings.TrimSpace(input.BookingName) != "" {
		merged.BookingName = input.BookingName
	}
	if presence.Phone && strings.TrimSpace(input.Phone) != "" {
		merged.Phone = input.Phone
	}
	if presence.PickupAddress && strings.TrimSpace(input.PickupAddress) != "" {
		merged.PickupAddress = input.PickupAddress
	}
	if presence.DepartureDate && strings.TrimSpace(input.DepartureDate) != "" {
		merged.DepartureDate = input.DepartureDate
	}
	if presence.SeatNumbers && strings.TrimSpace(input.SeatNumbers) != "" {
		merged.SeatNumbers = input.SeatNumbers
	}
	if presence.PassengerCount && strings.TrimSpace(input.PassengerCount) != "" {
		merged.PassengerCount = input.PassengerCount
	}
	if presence.ServiceType && strings.TrimSpace(input.ServiceType) != "" {
		merged.ServiceType = input.ServiceType
	}
	if presence.DriverName && strings.TrimSpace(input.DriverName) != "" {
		merged.DriverName = input.DriverName
	}
	if presence.VehicleCode && strings.TrimSpace(input.VehicleCode) != "" {
		merged.VehicleCode = input.VehicleCode
	}
	if presence.VehicleType && strings.TrimSpace(input.VehicleType) != "" {
		merged.VehicleType = input.VehicleType
	}
	if presence.SuratJalanFile && strings.TrimSpace(input.SuratJalanFile) != "" {
		merged.SuratJalanFile = input.SuratJalanFile
	}
	if presence.SuratJalanFileName && strings.TrimSpace(input.SuratJalanFileName) != "" {
		merged.SuratJalanFileName = input.SuratJalanFileName
	}
	if presence.DepartureStatus && strings.TrimSpace(input.DepartureStatus) != "" {
		merged.DepartureStatus = input.DepartureStatus
	}
	if presence.DepartureTime && strings.TrimSpace(input.DepartureTime) != "" {
		merged.DepartureTime = input.DepartureTime
	}
	if presence.RouteFrom && strings.TrimSpace(input.RouteFrom) != "" {
		merged.RouteFrom = input.RouteFrom
	}
	if presence.RouteTo && strings.TrimSpace(input.RouteTo) != "" {
		merged.RouteTo = input.RouteTo
	}
	if presence.TripNumber && strings.TrimSpace(input.TripNumber) != "" {
		merged.TripNumber = input.TripNumber
	}
	if presence.BookingID && input.BookingID > 0 {
		merged.BookingID = input.BookingID
	} else if presence.BookingID && input.BookingID <= 0 {
		// jangan mengosongkan booking_id jika payload nol
		presence.BookingID = false
	}

	count, _ := strconv.Atoi(strings.TrimSpace(merged.PassengerCount))
	return merged, presence, count, nil
}
