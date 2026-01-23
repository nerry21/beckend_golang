package repositories

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
	"backend/internal/domain/models"
)

// DepartureRepository wraps DB access for departure_settings with key-presence PATCH semantics.
type DepartureRepository struct {
	DB *sql.DB
}

func (r DepartureRepository) db() *sql.DB {
	if r.DB != nil {
		return r.DB
	}
	return intconfig.DB
}

// GetByID loads the latest departure_settings row including optional columns.
func (r DepartureRepository) GetByID(id int) (models.DepartureSetting, error) {
	if id <= 0 {
		return models.DepartureSetting{}, sql.ErrNoRows
	}
	table := "departure_settings"
	db := r.db()
	if db == nil || !intdb.HasTable(db, table) {
		return models.DepartureSetting{}, sql.ErrNoRows
	}

	var d models.DepartureSetting
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
		return models.DepartureSetting{}, err
	}
	d.PassengerCount = strconv.Itoa(count)
	d.DepartureTime = strings.TrimSpace(depTime.String)
	d.RouteFrom = strings.TrimSpace(routeFrom.String)
	d.RouteTo = strings.TrimSpace(routeTo.String)
	d.VehicleType = strings.TrimSpace(vehicleType.String)
	d.CreatedAt = strings.TrimSpace(createdAt.String)
	return d, nil
}

// GetByBookingID loads a departure setting by booking_id if exists.
func (r DepartureRepository) GetByBookingID(bookingID int64) (models.DepartureSetting, error) {
	if bookingID <= 0 {
		return models.DepartureSetting{}, sql.ErrNoRows
	}
	table := "departure_settings"
	db := r.db()
	if db == nil || !intdb.HasTable(db, table) || !intdb.HasColumn(db, table, "booking_id") {
		return models.DepartureSetting{}, sql.ErrNoRows
	}
	var id int
	if err := db.QueryRow(`SELECT id FROM `+table+` WHERE booking_id=? ORDER BY id DESC LIMIT 1`, bookingID).Scan(&id); err != nil {
		return models.DepartureSetting{}, err
	}
	return r.GetByID(id)
}

// CreateFromBooking upserts departure_settings keyed by booking_id.
func (r DepartureRepository) CreateFromBooking(dep models.DepartureSetting) (models.DepartureSetting, error) {
	return r.UpsertFromBooking(dep)
}

// UpsertFromBooking membuat/memperbarui departure_settings berdasarkan booking_id.
func (r DepartureRepository) UpsertFromBooking(dep models.DepartureSetting) (models.DepartureSetting, error) {
	table := "departure_settings"
	db := r.db()
	if db == nil || !intdb.HasTable(db, table) {
		return dep, fmt.Errorf("tabel departure_settings tidak ditemukan")
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
	add("vehicle_type", dep.VehicleType)
	add("vehicle_code", dep.VehicleCode)
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
		placeholders := make([]string, len(cols))
		for i := range placeholders {
			if cols[i] == "booking_id" {
				placeholders[i] = "NULLIF(?,0)"
			} else {
				placeholders[i] = "?"
			}
		}
		_, err := db.Exec(`INSERT INTO `+table+` (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(placeholders, ",")+`)`, vals...)
		if err != nil {
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
		return r.GetByID(existingID)
	}
	return dep, nil
}

// UpdatePartial applies only fields present in raw JSON (key presence), keeping existing data intact.
func (r DepartureRepository) UpdatePartial(id int, rawJSON []byte) (models.DepartureSetting, error) {
	if id <= 0 {
		return models.DepartureSetting{}, sql.ErrNoRows
	}

	existing, err := r.GetByID(id)
	if err != nil {
		return models.DepartureSetting{}, err
	}

	merged, presence, count, err := buildDeparturePatch(existing, rawJSON)
	if err != nil {
		return merged, err
	}

	table := "departure_settings"
	db := r.db()
	if db == nil || !intdb.HasTable(db, table) {
		return merged, fmt.Errorf("tabel departure_settings tidak ditemukan")
	}

	if strings.TrimSpace(merged.VehicleType) == "" {
		if vt := lookupVehicleTypeByDriver(db, merged.DriverName); vt != "" {
			merged.VehicleType = vt
			presence.VehicleType = true
		}
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
func buildDeparturePatch(existing models.DepartureSetting, rawJSON []byte) (models.DepartureSetting, departureFieldPresence, int, error) {
	payloadMap := map[string]any{}
	if err := json.Unmarshal(rawJSON, &payloadMap); err != nil {
		return existing, departureFieldPresence{}, 0, err
	}

	payloadKeys := map[string]bool{}
	for k := range payloadMap {
		payloadKeys[strings.ToLower(k)] = true
	}

	hasField := func(names ...string) bool {
		for _, n := range names {
			if payloadKeys[strings.ToLower(n)] {
				return true
			}
		}
		return false
	}

	getVal := func(names ...string) (any, bool) {
		for key, val := range payloadMap {
			for _, name := range names {
				if strings.EqualFold(key, name) {
					return val, true
				}
			}
		}
		return nil, false
	}

	getString := func(names ...string) string {
		if val, ok := getVal(names...); ok {
			return strings.TrimSpace(fmt.Sprint(val))
		}
		return ""
	}

	getInt64 := func(names ...string) int64 {
		if val, ok := getVal(names...); ok {
			switch v := val.(type) {
			case float64:
				return int64(v)
			case float32:
				return int64(v)
			case int:
				return int64(v)
			case int64:
				return v
			case json.Number:
				if n, err := v.Int64(); err == nil {
					return n
				}
			case string:
				if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
					return n
				}
			}
		}
		return 0
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

	merged := existing

	if presence.BookingName {
		if v := getString("bookingName", "booking_name"); v != "" {
			merged.BookingName = v
		}
	}
	if presence.Phone {
		if v := getString("phone"); v != "" {
			merged.Phone = v
		}
	}
	if presence.PickupAddress {
		if v := getString("pickupAddress", "pickup_address"); v != "" {
			merged.PickupAddress = v
		}
	}
	if presence.DepartureDate {
		if v := getString("departureDate", "departure_date"); v != "" {
			merged.DepartureDate = v
		}
	}
	if presence.SeatNumbers {
		if v := getString("seatNumbers", "seat_numbers"); v != "" {
			merged.SeatNumbers = v
		}
	}
	if presence.PassengerCount {
		if v := getString("passengerCount", "passenger_count"); v != "" {
			merged.PassengerCount = v
		}
	}
	if presence.ServiceType {
		if v := getString("serviceType", "service_type"); v != "" {
			merged.ServiceType = v
		}
	}
	if presence.DriverName {
		if v := getString("driverName", "driver_name"); v != "" {
			merged.DriverName = v
		}
	}
	if presence.VehicleCode {
		if v := getString("vehicleCode", "vehicle_code"); v != "" {
			merged.VehicleCode = v
		}
	}
	if presence.VehicleType {
		if v := getString("vehicleType", "vehicle_type"); v != "" {
			merged.VehicleType = v
		}
	}
	if presence.SuratJalanFile {
		if v := getString("suratJalanFile", "surat_jalan_file"); v != "" {
			merged.SuratJalanFile = v
		}
	}
	if presence.SuratJalanFileName {
		if v := getString("suratJalanFileName", "surat_jalan_file_name"); v != "" {
			merged.SuratJalanFileName = v
		}
	}
	if presence.DepartureStatus {
		if v := getString("departureStatus", "departure_status"); v != "" {
			merged.DepartureStatus = v
		}
	}

	if presence.DepartureTime {
		if v := getString("departureTime", "departure_time"); v != "" {
			merged.DepartureTime = v
		}
	}
	if presence.RouteFrom {
		if v := getString("routeFrom", "route_from"); v != "" {
			merged.RouteFrom = v
		}
	}
	if presence.RouteTo {
		if v := getString("routeTo", "route_to"); v != "" {
			merged.RouteTo = v
		}
	}
	if presence.TripNumber {
		if v := getString("tripNumber", "trip_number"); v != "" {
			merged.TripNumber = v
		}
	}

	if presence.BookingID {
		if bid := getInt64("bookingId", "booking_id"); bid > 0 {
			merged.BookingID = bid
		} else {
			presence.BookingID = false
		}
	}

	if presence.BookingID && merged.BookingID <= 0 {
		presence.BookingID = false
	}

	count, _ := strconv.Atoi(strings.TrimSpace(merged.PassengerCount))
	merged.PassengerCount = strconv.Itoa(count)

	return merged, presence, count, nil
}

func lookupVehicleTypeByDriver(db *sql.DB, driverName string) string {
	if db == nil {
		return ""
	}
	name := strings.TrimSpace(driverName)
	if name == "" {
		return ""
	}
	lowerName := strings.ToLower(name)

	sources := []struct {
		table   string
		nameCol string
		typeCol string
	}{
		{table: "drivers", nameCol: "name", typeCol: "vehicle_type"},
		{table: "driver_accounts", nameCol: "driver_name", typeCol: "vehicle_type"},
	}

	for _, src := range sources {
		if !intdb.HasTable(db, src.table) ||
			!intdb.HasColumn(db, src.table, src.nameCol) ||
			!intdb.HasColumn(db, src.table, src.typeCol) {
			continue
		}

		var vt sql.NullString
		err := db.QueryRow(
			`SELECT COALESCE(`+src.typeCol+`,'') FROM `+src.table+` WHERE LOWER(`+src.nameCol+`)=? OR `+src.nameCol+`=? ORDER BY id DESC LIMIT 1`,
			lowerName,
			name,
		).Scan(&vt)
		if err == nil && strings.TrimSpace(vt.String) != "" {
			return strings.TrimSpace(vt.String)
		}
	}

	return ""
}

func nullIfEmptyString(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
