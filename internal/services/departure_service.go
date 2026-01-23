package services

import (
	"bytes"
	"errors"
	"log"
	"strconv"
	"strings"

	"backend/internal/domain/models"
	"backend/internal/repositories"
	"backend/internal/utils"
)

// DepartureService coordinates departure_settings updates and post-berangkat sync.
type DepartureService struct {
	Repo        repositories.DepartureRepository
	BookingRepo repositories.BookingRepository
	SeatRepo    repositories.BookingSeatRepository
	RequestID   string
}

// CreateOrUpdateFromBooking ensures departure_settings exists from booking data.
func (s DepartureService) CreateOrUpdateFromBooking(booking repositories.Booking, seats []repositories.BookingSeat) (models.DepartureSetting, error) {
	seatCodes := []string{}
	for _, bs := range seats {
		if strings.TrimSpace(bs.SeatCode) != "" {
			seatCodes = append(seatCodes, strings.TrimSpace(bs.SeatCode))
		}
	}
	seatJoined := strings.Join(seatCodes, ",")

	dep := models.DepartureSetting{
		BookingName:     firstNonEmpty(booking.BookingFor, booking.PassengerName),
		Phone:           booking.PassengerPhone,
		PickupAddress:   booking.PickupLocation,
		DepartureDate:   booking.TripDate,
		DepartureTime:   booking.TripTime,
		SeatNumbers:     seatJoined,
		PassengerCount:  strconv.Itoa(maxInt(booking.PassengerCount, len(seatCodes))),
		ServiceType:     booking.Category,
		RouteFrom:       booking.RouteFrom,
		RouteTo:         booking.RouteTo,
		BookingID:       booking.ID,
		DepartureStatus: "",
	}

	return s.Repo.UpsertFromBooking(dep)
}

// CreateOrUpdateFromBookingID fetches booking data before upsert.
func (s DepartureService) CreateOrUpdateFromBookingID(bookingID int64) (models.DepartureSetting, error) {
	booking, err := s.BookingRepo.GetByID(bookingID)
	if err != nil {
		return models.DepartureSetting{}, err
	}
	seats, _ := s.SeatRepo.GetSeats(bookingID)
	return s.CreateOrUpdateFromBooking(booking, seats)
}

// MarkBerangkat updates departure_settings with key-presence semantics and triggers sync when status Berangkat.
func (s DepartureService) MarkBerangkat(id int, rawPayload []byte) (models.DepartureSetting, error) {
	if len(bytes.TrimSpace(rawPayload)) == 0 {
		return models.DepartureSetting{}, errors.New("payload kosong")
	}
	utils.LogEvent(s.RequestID, "departure", "mark_berangkat", "start id="+strconv.Itoa(id))

	updated, err := s.Repo.UpdatePartial(id, rawPayload)
	if err != nil {
		utils.LogEvent(s.RequestID, "departure", "mark_berangkat_error", err.Error())
		return updated, err
	}

	reloaded, err := s.Repo.GetByID(id)
	if err != nil {
		return reloaded, err
	}

	log.Printf("[DEPARTURE] id=%d driverName=%s vehicleCode=%s vehicleType=%s", id, strings.TrimSpace(reloaded.DriverName), strings.TrimSpace(reloaded.VehicleCode), strings.TrimSpace(reloaded.VehicleType))

	if strings.EqualFold(strings.TrimSpace(reloaded.DepartureStatus), "Berangkat") {
		if err := s.syncAfterBerangkat(reloaded); err != nil {
			utils.LogEvent(s.RequestID, "departure", "sync_after_berangkat_error", err.Error())
			return reloaded, err
		}
	}

	utils.LogEvent(s.RequestID, "departure", "mark_berangkat_done", "id="+strconv.Itoa(id))
	return reloaded, nil
}

func (s DepartureService) syncAfterBerangkat(dep models.DepartureSetting) error {
	ref := dep
	if ref.BookingID <= 0 {
		if reloaded, err := s.Repo.GetByID(dep.ID); err == nil {
			ref = reloaded
		}
	}

	if ref.BookingID > 0 {
		passengerSvc := PassengerService{
			PassengerRepo:   repositories.PassengerRepository{},
			BookingRepo:     repositories.BookingRepository{},
			BookingSeatRepo: repositories.BookingSeatRepository{},
			RequestID:       s.RequestID,
		}
		if err := passengerSvc.SyncFromDeparture(ref); err != nil {
			log.Println("[BERANGKAT SYNC] warning passenger sync:", err)
		}

		tripSvc := TripInfoService{
			Repo:      repositories.TripInformationRepository{},
			RequestID: s.RequestID,
		}
		if err := tripSvc.UpsertFromDeparture(ref); err != nil {
			log.Println("[BERANGKAT SYNC] warning trip info sync:", err)
		}
	}

	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
