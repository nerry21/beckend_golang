package models

// DepartureSetting mirrors departure_settings / return_settings schema.
type DepartureSetting struct {
	ID                 int    `json:"id"`
	BookingID          int64  `json:"booking_id"`
	BookingName        string `json:"booking_name"`
	Phone              string `json:"phone"`
	PickupAddress      string `json:"pickup_address"`
	DepartureDate      string `json:"departure_date"`
	DepartureTime      string `json:"departure_time"`
	SeatNumbers        string `json:"seat_numbers"`
	PassengerCount     string `json:"passenger_count"`
	ServiceType        string `json:"service_type"`
	DriverName         string `json:"driver_name"`
	VehicleCode        string `json:"vehicle_code"`
	VehicleType        string `json:"vehicle_type"`
	SuratJalanFile     string `json:"surat_jalan_file"`
	SuratJalanFileName string `json:"surat_jalan_file_name"`
	DepartureStatus    string `json:"departure_status"`
	TripNumber         string `json:"trip_number"`
	RouteFrom          string `json:"route_from"`
	RouteTo            string `json:"route_to"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}
