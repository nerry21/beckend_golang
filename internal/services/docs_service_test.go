package services

import (
	"testing"
	"time"
)

func TestDocsServiceGenerate(t *testing.T) {
	loader := func(id int64) (passengerDocData, error) {
		return passengerDocData{
			PassengerID:    id,
			BookingID:      10,
			PassengerName:  "Tester",
			PassengerPhone: "0800",
			SeatCode:       "A1",
			RouteFrom:      "CityA",
			RouteTo:        "CityB",
			TripDate:       time.Now().Format("2006-01-02"),
			TripTime:       "10:00",
			ServiceType:    "reguler",
			VehicleCode:    "B123",
			DriverName:     "Driver",
			PricePerSeat:   100000,
		}, nil
	}

	svc := DocsService{Loader: loader}

	pdf, filename, err := svc.GenerateETicket(1)
	if err != nil {
		t.Fatalf("GenerateETicket returned error: %v", err)
	}
	if len(pdf) == 0 || filename == "" {
		t.Fatalf("GenerateETicket returned empty data")
	}

	invoice, invName, err := svc.GenerateInvoice(1)
	if err != nil {
		t.Fatalf("GenerateInvoice returned error: %v", err)
	}
	if len(invoice) == 0 || invName == "" {
		t.Fatalf("GenerateInvoice returned empty data")
	}
}
