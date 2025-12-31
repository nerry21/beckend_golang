package services

import (
	"testing"

	intconfig "backend/internal/config"
	"backend/internal/domain/models"
	"backend/internal/repositories"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPassengerSyncFromDepartureIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock init error: %v", err)
	}
	defer db.Close()
	intconfig.DB = db
	mock.MatchExpectationsInOrder(false)

	// booking_passengers table absence to skip passenger input lookup
	mock.ExpectQuery("information_schema\\.tables").WithArgs("booking_passengers").
		WillReturnRows(sqlmock.NewRows([]string{"table_name"}))
	mock.ExpectQuery("information_schema\\.tables").WithArgs("booking_passengers").
		WillReturnRows(sqlmock.NewRows([]string{"table_name"}))

	// First upsert (insert)
	expectPassengerTableQueries(mock)
	mock.ExpectQuery("SELECT id FROM passengers").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("INSERT INTO passengers").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Second upsert (update)
	expectPassengerTableQueries(mock)
	mock.ExpectQuery("SELECT id FROM passengers").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec("UPDATE passengers").
		WillReturnResult(sqlmock.NewResult(1, 1))

	svc := PassengerService{
		PassengerRepo:   repositories.PassengerRepository{DB: db},
		BookingRepo:     repositories.BookingRepository{},
		BookingSeatRepo: repositories.BookingSeatRepository{},
		DB:              db,
		FetchBooking: func(id int64) (repositories.Booking, []repositories.BookingSeat, error) {
			return repositories.Booking{
					ID:             id,
					BookingFor:     "Tester",
					PassengerPhone: "0800",
					TripDate:       "2025-01-01",
					TripTime:       "08:00",
					PickupLocation: "A",
					RouteTo:        "B",
					Category:       "reguler",
					PricePerSeat:   100000,
				}, []repositories.BookingSeat{
					{SeatCode: "A1", TripDate: "2025-01-01", TripTime: "08:00"},
				}, nil
		},
	}

	dep := repositories.Booking{
		ID:             1,
		PassengerName:  "Tester",
		PassengerPhone: "0800",
		RouteTo:        "B",
	}
	if err := svc.SyncFromDeparture(legacyDepartureFromBooking(dep, "A1")); err != nil {
		t.Fatalf("first sync error: %v", err)
	}
	if err := svc.SyncFromDeparture(legacyDepartureFromBooking(dep, "A1")); err != nil {
		t.Fatalf("second sync error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func expectPassengerTableQueries(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("information_schema\\.tables").WithArgs("passengers").
		WillReturnRows(sqlmock.NewRows([]string{"table_name"}).AddRow("passengers"))
	mock.ExpectQuery("information_schema\\.columns").WithArgs("passengers", "booking_id").
		WillReturnRows(sqlmock.NewRows([]string{"column_name"}).AddRow("booking_id"))
	mock.ExpectQuery("information_schema\\.columns").WithArgs("passengers", "selected_seats").
		WillReturnRows(sqlmock.NewRows([]string{"column_name"}).AddRow("selected_seats"))
	mock.ExpectQuery("information_schema\\.columns").WithArgs("passengers", "trip_role").
		WillReturnRows(sqlmock.NewRows([]string{"column_name"}).AddRow("trip_role"))
}

func legacyDepartureFromBooking(b repositories.Booking, seat string) models.DepartureSetting {
	return models.DepartureSetting{
		ID:             1,
		BookingID:      b.ID,
		BookingName:    b.BookingFor,
		Phone:          b.PassengerPhone,
		PickupAddress:  b.PickupLocation,
		DepartureDate:  b.TripDate,
		DepartureTime:  b.TripTime,
		SeatNumbers:    seat,
		RouteTo:        b.RouteTo,
		PassengerCount: "1",
	}
}
