package services

import (
	"strconv"
	"strings"

	legacy "backend/handlers"
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

// CreateOrUpdateFromBooking ensures return_settings exists keyed by booking_id.
func (s ReturnService) CreateOrUpdateFromBooking(booking repositories.Booking, seats []repositories.BookingSeat) (legacy.DepartureSetting, error) {
	seatCodes := []string{}
	for _, bs := range seats {
		if strings.TrimSpace(bs.SeatCode) != "" {
			seatCodes = append(seatCodes, strings.TrimSpace(bs.SeatCode))
		}
	}
	seatJoined := strings.Join(seatCodes, ",")

	ret := legacy.DepartureSetting{
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

	return s.Repo.CreateFromBooking(ret)
}

// CreateOrUpdateFromBookingID fetches booking and seats before upsert.
func (s ReturnService) CreateOrUpdateFromBookingID(bookingID int64) (legacy.DepartureSetting, error) {
	booking, err := s.BookingRepo.GetByID(bookingID)
	if err != nil {
		return legacy.DepartureSetting{}, err
	}
	seats, _ := s.SeatRepo.GetSeats(bookingID)
	return s.CreateOrUpdateFromBooking(booking, seats)
}

// MarkPulang updates return_settings with key-presence semantics.
func (s ReturnService) MarkPulang(id int, rawPayload []byte) (legacy.DepartureSetting, error) {
	if s.Repo.DB == nil {
		s.Repo.DB = s.BookingRepo.DB
	}
	utils.LogEvent(s.RequestID, "return", "mark_pulang", "start id="+strconv.Itoa(id))
	updated, err := s.Repo.UpdatePartial(id, rawPayload)
	if err != nil {
		utils.LogEvent(s.RequestID, "return", "mark_pulang_error", err.Error())
		return updated, err
	}

	if updated.BookingID > 0 {
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
