package services

import (
	"fmt"
	"strings"

	legacy "backend/handlers"
	"backend/internal/repositories"
	"backend/internal/utils"
)

// TripInfoService mengelola upsert trip_information dari departure/return.
type TripInfoService struct {
	Repo      repositories.TripInformationRepository
	RequestID string
}

func (s TripInfoService) UpsertFromDeparture(dep legacy.DepartureSetting) error {
	return s.upsertFromSetting(dep, "berangkat")
}

func (s TripInfoService) UpsertFromReturn(ret legacy.DepartureSetting) error {
	return s.upsertFromSetting(ret, "pulang")
}

func (s TripInfoService) upsertFromSetting(dep legacy.DepartureSetting, tripRole string) error {
	tripNumber := strings.TrimSpace(dep.TripNumber)
	if tripNumber == "" && dep.BookingID > 0 {
		tripNumber = fmt.Sprintf("TRIP-%s-%d", strings.ToUpper(tripRole), dep.BookingID)
	}
	if tripNumber == "" {
		tripNumber = fmt.Sprintf("TRIP-%s-%d", strings.ToUpper(tripRole), dep.ID)
	}

	tripDetails := ""
	if strings.TrimSpace(dep.RouteFrom) != "" || strings.TrimSpace(dep.RouteTo) != "" {
		tripDetails = strings.TrimSpace(strings.TrimSpace(dep.RouteFrom) + " - " + strings.TrimSpace(dep.RouteTo))
	} else {
		tripDetails = tripNumber
	}

	eSurat := strings.TrimSpace(dep.SuratJalanFile)
	if eSurat == "" && dep.BookingID > 0 {
		eSurat = buildSuratJalanAPI(dep.BookingID)
	}

	info := repositories.TripInformation{
		TripNumber:    tripNumber,
		TripDetails:   tripDetails,
		DepartureDate: dep.DepartureDate,
		DepartureTime: dep.DepartureTime,
		DriverName:    dep.DriverName,
		VehicleCode:   dep.VehicleCode,
		LicensePlate:  dep.VehicleType,
		ESuratJalan:   eSurat,
		BookingID:     dep.BookingID,
		TripRole:      tripRole,
	}

	utils.LogEvent(s.RequestID, "trip_info", "upsert", fmt.Sprintf("booking_id=%d trip_role=%s", dep.BookingID, tripRole))
	return s.Repo.UpsertTripInfo(info)
}

func buildSuratJalanAPI(bookingID int64) string {
	return fmt.Sprintf("http://localhost:8080/api/reguler/bookings/%d/surat-jalan", bookingID)
}
