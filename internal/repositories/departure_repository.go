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
	"backend/utils"
)

// DepartureRepository wraps DB access for departure_settings with key-presence PATCH semantics.
type DepartureRepository struct {
	DB *sql.DB
}

// GetByID loads the latest departure_settings row including optional columns.
func (r DepartureRepository) GetByID(id int) (legacy.DepartureSetting, error) {
	return legacy.GetDepartureSettingForSyncByID(id)
}

// GetByBookingID loads a departure setting by booking_id if exists.
func (r DepartureRepository) GetByBookingID(bookingID int64) (legacy.DepartureSetting, error) {
	if bookingID <= 0 {
		return legacy.DepartureSetting{}, sql.ErrNoRows
	}
	table := "departure_settings"
	if !utils.HasTable(config.DB, table) || !utils.HasColumn(config.DB, table, "booking_id") {
		return legacy.DepartureSetting{}, sql.ErrNoRows
	}
	var id int
	if err := config.DB.QueryRow(`SELECT id FROM `+table+` WHERE booking_id=? ORDER BY id DESC LIMIT 1`, bookingID).Scan(&id); err != nil {
		return legacy.DepartureSetting{}, err
	}
	return r.GetByID(id)
}

// CreateFromBooking upserts departure_settings keyed by booking_id.
func (r DepartureRepository) CreateFromBooking(dep legacy.DepartureSetting) (legacy.DepartureSetting, error) {
	return r.UpsertFromBooking(dep)
}

// UpsertFromBooking membuat/memperbarui departure_settings berdasarkan booking_id.
func (r DepartureRepository) UpsertFromBooking(dep legacy.DepartureSetting) (legacy.DepartureSetting, error) {
	table := "departure_settings"
	if !utils.HasTable(config.DB, table) {
		return dep, fmt.Errorf("tabel departure_settings tidak ditemukan")
	}

	// cek existing by booking_id
	var existingID int
	if utils.HasColumn(config.DB, table, "booking_id") {
		_ = config.DB.QueryRow(`SELECT id FROM `+table+` WHERE booking_id=? LIMIT 1`, dep.BookingID).Scan(&existingID)
	}

	cols := []string{}
	vals := []any{}

	add := func(col string, val any) {
		if utils.HasColumn(config.DB, table, col) {
			cols = append(cols, col)
			vals = append(vals, val)
		}
	}

	add("booking_name", dep.BookingName)
	add("phone", dep.Phone)
	add("pickup_address", dep.PickupAddress)
	add("departure_date", dep.DepartureDate)
	add("seat_numbers", dep.SeatNumbers)
	if utils.HasColumn(config.DB, table, "passenger_count") {
		pc, _ := strconv.Atoi(strings.TrimSpace(dep.PassengerCount))
		cols = append(cols, "passenger_count")
		vals = append(vals, pc)
	}
	add("service_type", dep.ServiceType)
	add("driver_name", dep.DriverName)
	add("vehicle_type", dep.VehicleType)
	add("vehicle_code", dep.VehicleCode)
	add("surat_jalan_file", dep.SuratJalanFile)
	add("surat_jalan_file_name", dep.SuratJalanFileName)
	add("departure_status", dep.DepartureStatus)
	add("route_from", dep.RouteFrom)
	add("route_to", dep.RouteTo)
	add("trip_number", dep.TripNumber)
	add("departure_time", dep.DepartureTime)
	if utils.HasColumn(config.DB, table, "booking_id") {
		cols = append(cols, "booking_id")
		vals = append(vals, dep.BookingID)
	}

	now := time.Now()

	if existingID == 0 {
		if utils.HasColumn(config.DB, table, "created_at") {
			cols = append(cols, "created_at")
			vals = append(vals, now)
		}
		placeholders := make([]string, len(cols))
		for i := range placeholders {
			if cols[i] == "booking_id" {
				placeholders[i] = "NULLIF(?,0)"
			} else {
				placeholders[i] = "?"
			}
		}
		_, err := config.DB.Exec(`INSERT INTO `+table+` (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(placeholders, ",")+`)`, vals...)
		if err != nil {
			return dep, err
		}
		// fetch inserted row id
		_ = config.DB.QueryRow(`SELECT id FROM `+table+` WHERE booking_id=? ORDER BY id DESC LIMIT 1`, dep.BookingID).Scan(&existingID)
	} else {
		if utils.HasColumn(config.DB, table, "updated_at") {
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
		if _, err := config.DB.Exec(`UPDATE `+table+` SET `+strings.Join(setParts, ",")+` WHERE id=?`, vals...); err != nil {
			return dep, err
		}
	}

	if existingID > 0 {
		return legacy.GetDepartureSettingForSyncByID(existingID)
	}
	return dep, nil
}

// UpdatePartial applies only fields present in raw JSON (key presence), keeping existing data intact.
func (r DepartureRepository) UpdatePartial(id int, rawJSON []byte) (legacy.DepartureSetting, error) {
	if id <= 0 {
		return legacy.DepartureSetting{}, sql.ErrNoRows
	}

	existing, err := r.GetByID(id)
	if err != nil {
		return legacy.DepartureSetting{}, err
	}

	merged, presence, count, err := buildDeparturePatch(existing, rawJSON)
	if err != nil {
		return merged, err
	}

	table := "departure_settings"
	if !utils.HasTable(config.DB, table) {
		return merged, fmt.Errorf("tabel departure_settings tidak ditemukan")
	}

	sets := []string{}
	args := []any{}

	add := func(cond bool, column string, val any) {
		if cond && utils.HasColumn(config.DB, table, column) {
			sets = append(sets, column+"=?")
			args = append(args, val)
		}
	}

	add(presence.BookingName, "booking_name", merged.BookingName)
	add(presence.Phone, "phone", merged.Phone)
	add(presence.PickupAddress, "pickup_address", merged.PickupAddress)
	if presence.DepartureDate && utils.HasColumn(config.DB, table, "departure_date") {
		sets = append(sets, "departure_date=?")
		args = append(args, nullIfEmptyString(merged.DepartureDate))
	}
	add(presence.SeatNumbers, "seat_numbers", merged.SeatNumbers)
	if presence.PassengerCount && utils.HasColumn(config.DB, table, "passenger_count") {
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

	if presence.DepartureTime && utils.HasColumn(config.DB, table, "departure_time") {
		sets = append(sets, "departure_time=?")
		args = append(args, nullIfEmptyString(merged.DepartureTime))
	}
	add(presence.RouteFrom, "route_from", merged.RouteFrom)
	add(presence.RouteTo, "route_to", merged.RouteTo)
	add(presence.TripNumber, "trip_number", merged.TripNumber)

	if presence.BookingID && utils.HasColumn(config.DB, table, "booking_id") {
		sets = append(sets, "booking_id=NULLIF(?,0)")
		args = append(args, merged.BookingID)
	}

	if utils.HasColumn(config.DB, table, "updated_at") && len(sets) > 0 {
		sets = append(sets, "updated_at=?")
		args = append(args, time.Now())
	}

	if len(sets) == 0 {
		return merged, nil
	}

	args = append(args, id)
	if _, err := config.DB.Exec(`UPDATE `+table+` SET `+strings.Join(sets, ", ")+` WHERE id=?`, args...); err != nil {
		return merged, err
	}

	merged.ID = id
	merged.PassengerCount = strconv.Itoa(count)
	return merged, nil
}

type departureFieldPresence struct {
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

// buildDeparturePatch merges payload into existing row respecting key presence.
func buildDeparturePatch(existing legacy.DepartureSetting, rawJSON []byte) (legacy.DepartureSetting, departureFieldPresence, int, error) {
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
		return existing, departureFieldPresence{}, 0, err
	}

	presence := departureFieldPresence{
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

	// booking_id bisa datang sebagai snake_case, isi manual jika belum terisi.
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
		// treat as not updating booking_id to avoid nulling existing value
		presence.BookingID = false
	}

	count, _ := strconv.Atoi(strings.TrimSpace(merged.PassengerCount))
	merged.PassengerCount = strconv.Itoa(count)

	return merged, presence, count, nil
}

func nullIfEmptyString(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
