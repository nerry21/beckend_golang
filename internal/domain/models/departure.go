package models

// DepartureSetting mirrors departure_settings / return_settings schema.
type DepartureSetting struct {
	ID                 int
	BookingID          int64
	BookingName        string
	Phone              string
	PickupAddress      string
	DepartureDate      string
	DepartureTime      string
	SeatNumbers        string
	PassengerCount     string
	ServiceType        string
	DriverName         string
	VehicleCode        string
	VehicleType        string
	SuratJalanFile     string
	SuratJalanFileName string
	DepartureStatus    string
	TripNumber         string
	RouteFrom          string
	RouteTo            string
	CreatedAt          string
	UpdatedAt          string
}
