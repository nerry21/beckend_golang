package services

import (
	"database/sql"
	"strconv"
	"strings"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
	"backend/internal/domain/models"
	"backend/internal/repositories"
	"backend/internal/utils"
)

// ReturnService handles return_settings lifecycle.
type ReturnService struct {
	Repo        repositories.ReturnRepository
	BookingRepo repositories.BookingRepository
	SeatRepo    repositories.BookingSeatRepository
	RequestID   string
}

func (s ReturnService) db() *sql.DB {
	if s.Repo.DB != nil {
		return s.Repo.DB
	}
	if s.BookingRepo.DB != nil {
		return s.BookingRepo.DB
	}
	return intconfig.DB
}

type depMeta struct {
	DriverName    string
	VehicleCode   string
	VehicleType   string
	TripNumber    string
	DepartureTime string
	RouteFrom     string
	RouteTo       string
}

func (s ReturnService) loadLatestDepartureMeta(bookingID int64) depMeta {
	db := s.db()
	if db == nil {
		return depMeta{}
	}
	table := "departure_settings"
	if !intdb.HasTable(db, table) || !intdb.HasColumn(db, table, "booking_id") {
		return depMeta{}
	}

	sel := func(col string) string {
		if intdb.HasColumn(db, table, col) {
			return "COALESCE(" + col + ",'')"
		}
		return "''"
	}

	row := db.QueryRow(`
		SELECT
			`+sel("driver_name")+`,
			`+sel("vehicle_code")+`,
			`+sel("vehicle_type")+`,
			`+sel("trip_number")+`,
			`+sel("departure_time")+`,
			`+sel("route_from")+`,
			`+sel("route_to")+`
		FROM `+table+`
		WHERE booking_id=?
		ORDER BY id DESC
		LIMIT 1
	`, bookingID)

	var m depMeta
	_ = row.Scan(
		&m.DriverName,
		&m.VehicleCode,
		&m.VehicleType,
		&m.TripNumber,
		&m.DepartureTime,
		&m.RouteFrom,
		&m.RouteTo,
	)

	m.DriverName = strings.TrimSpace(m.DriverName)
	m.VehicleCode = strings.TrimSpace(m.VehicleCode)
	m.VehicleType = strings.TrimSpace(m.VehicleType)
	m.TripNumber = strings.TrimSpace(m.TripNumber)
	m.DepartureTime = strings.TrimSpace(m.DepartureTime)
	m.RouteFrom = strings.TrimSpace(m.RouteFrom)
	m.RouteTo = strings.TrimSpace(m.RouteTo)
	return m
}

func (s ReturnService) firstPassengerNameFromBookingPassengers(bookingID int64) string {
	db := s.db()
	if db == nil {
		return ""
	}
	table := "booking_passengers"
	if !intdb.HasTable(db, table) || !intdb.HasColumn(db, table, "booking_id") || !intdb.HasColumn(db, table, "passenger_name") {
		return ""
	}

	var name string
	err := db.QueryRow(`
		SELECT COALESCE(passenger_name,'')
		FROM booking_passengers
		WHERE booking_id=?
		ORDER BY id ASC
		LIMIT 1
	`, bookingID).Scan(&name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(name)
}

// CreateOrUpdateFromBooking ensures return_settings exists keyed by booking_id.
func (s ReturnService) CreateOrUpdateFromBooking(booking repositories.Booking, seats []repositories.BookingSeat) (models.ReturnSetting, error) {
	// seat codes unique + upper
	seen := map[string]bool{}
	seatCodes := []string{}
	for _, bs := range seats {
		code := strings.ToUpper(strings.TrimSpace(bs.SeatCode))
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		seatCodes = append(seatCodes, code)
	}
	seatJoined := strings.Join(seatCodes, ",")

	// bookingName: kalau "self/kosong" -> ambil dari booking_passengers
	name := firstNonEmpty(booking.BookingFor, booking.PassengerName)
	if strings.TrimSpace(name) == "" || strings.EqualFold(strings.TrimSpace(name), "self") {
		if nm := s.firstPassengerNameFromBookingPassengers(booking.ID); nm != "" {
			name = nm
		}
	}

	// fallback meta dari departure_settings (untuk driver/unit dll)
	meta := s.loadLatestDepartureMeta(booking.ID)

	ret := models.ReturnSetting{
		BookingName:    name,
		Phone:          booking.PassengerPhone,
		PickupAddress:  booking.PickupLocation,
		DepartureDate:  booking.TripDate,
		DepartureTime:  firstNonEmpty(booking.TripTime, meta.DepartureTime),
		SeatNumbers:    seatJoined,
		PassengerCount: strconv.Itoa(maxInt(booking.PassengerCount, len(seatCodes))),
		ServiceType:    booking.Category,
		RouteFrom:      firstNonEmpty(booking.RouteFrom, meta.RouteFrom),
		RouteTo:        firstNonEmpty(booking.RouteTo, meta.RouteTo),
		TripNumber:     meta.TripNumber,

		// ✅ otomatis ikut driver/vehicle dari departure_settings terbaru bila ada
		DriverName:   meta.DriverName,
		VehicleCode:  meta.VehicleCode,
		VehicleType:  meta.VehicleType,
		BookingID:    booking.ID,
		DepartureStatus: "",
	}

	return s.Repo.CreateFromBooking(ret)
}

// CreateOrUpdateFromBookingID fetches booking and seats before upsert.
func (s ReturnService) CreateOrUpdateFromBookingID(bookingID int64) (models.ReturnSetting, error) {
	booking, err := s.BookingRepo.GetByID(bookingID)
	if err != nil {
		return models.ReturnSetting{}, err
	}
	seats, _ := s.SeatRepo.GetSeats(bookingID)
	return s.CreateOrUpdateFromBooking(booking, seats)
}

// MarkPulang updates return_settings with key-presence semantics + enrich fallback.
func (s ReturnService) MarkPulang(id int, rawPayload []byte) (models.ReturnSetting, error) {
	if s.Repo.DB == nil {
		s.Repo.DB = s.db()
	}

	utils.LogEvent(s.RequestID, "return", "mark_pulang", "start id="+strconv.Itoa(id))
	updated, err := s.Repo.UpdatePartial(id, rawPayload)
	if err != nil {
		utils.LogEvent(s.RequestID, "return", "mark_pulang_error", err.Error())
		return updated, err
	}

	// ✅ ENRICH: kalau field masih kosong/self, isi dari booking_passengers & departure_settings
	if updated.BookingID > 0 {
		enrich := map[string]any{}

		// booking name
		if strings.TrimSpace(updated.BookingName) == "" || strings.EqualFold(strings.TrimSpace(updated.BookingName), "self") {
			if nm := s.firstPassengerNameFromBookingPassengers(updated.BookingID); nm != "" {
				updated.BookingName = nm
				enrich["booking_name"] = nm
			}
		}

		// driver/unit/meta fallback
		meta := s.loadLatestDepartureMeta(updated.BookingID)

		if strings.TrimSpace(updated.DriverName) == "" && meta.DriverName != "" {
			updated.DriverName = meta.DriverName
			enrich["driver_name"] = meta.DriverName
		}
		if strings.TrimSpace(updated.VehicleCode) == "" && meta.VehicleCode != "" {
			updated.VehicleCode = meta.VehicleCode
			enrich["vehicle_code"] = meta.VehicleCode
		}
		if strings.TrimSpace(updated.VehicleType) == "" && meta.VehicleType != "" {
			updated.VehicleType = meta.VehicleType
			enrich["vehicle_type"] = meta.VehicleType
		}
		if strings.TrimSpace(updated.TripNumber) == "" && meta.TripNumber != "" {
			updated.TripNumber = meta.TripNumber
			enrich["trip_number"] = meta.TripNumber
		}
		if strings.TrimSpace(updated.DepartureTime) == "" && meta.DepartureTime != "" {
			updated.DepartureTime = meta.DepartureTime
			enrich["departure_time"] = meta.DepartureTime
		}
		if strings.TrimSpace(updated.RouteFrom) == "" && meta.RouteFrom != "" {
			updated.RouteFrom = meta.RouteFrom
			enrich["route_from"] = meta.RouteFrom
		}
		if strings.TrimSpace(updated.RouteTo) == "" && meta.RouteTo != "" {
			updated.RouteTo = meta.RouteTo
			enrich["route_to"] = meta.RouteTo
		}

		// status default utk aksi mark_pulang
		if strings.TrimSpace(updated.DepartureStatus) == "" {
			updated.DepartureStatus = "Berangkat"
			enrich["departure_status"] = "Berangkat"
		}

		if len(enrich) > 0 {
			_ = s.Repo.UpdateEnrich(id, enrich)
			// reload biar output konsisten dari DB
			if fresh, e := s.Repo.GetByID(id); e == nil {
				updated = fresh
			}
		}

		// sync passenger seats & trip info
		passengerSvc := PassengerService{
			PassengerRepo:   repositories.PassengerRepository{},
			BookingRepo:     repositories.BookingRepository{},
			BookingSeatRepo: repositories.BookingSeatRepository{},
			RequestID:       s.RequestID,
		}
		if err := passengerSvc.SyncFromReturn(updated); err != nil {
			return updated, err
		}

		tripSvc := TripInfoService{
			Repo:      repositories.TripInformationRepository{},
			RequestID: s.RequestID,
		}
		if err := tripSvc.UpsertFromReturn(updated); err != nil {
			return updated, err
		}
	}

	utils.LogEvent(s.RequestID, "return", "mark_pulang_done", "id="+strconv.Itoa(id))
	return updated, nil
}
