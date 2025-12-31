package models

// Booking captures minimal booking data used across services.
type Booking struct {
	ID             int64
	Category       string
	RouteFrom      string
	RouteTo        string
	TripDate       string
	TripTime       string
	PassengerName  string
	PassengerPhone string
	PassengerCount int
	PricePerSeat   int64
	Total          int64
	BookingFor     string
	PaymentMethod  string
	PaymentStatus  string
}

// BookingUpdate supports PATCH-style updates via key presence.
type BookingUpdate struct {
	PassengerName  *string
	PassengerPhone *string
}

// BookingSeat represents seat allocation for a booking.
type BookingSeat struct {
	SeatCode  string
	RouteFrom string
	RouteTo   string
	TripDate  string
	TripTime  string
}

// PassengerInput carries per-seat passenger info.
type PassengerInput struct {
	SeatCode string `json:"seat_code"`
	Name     string `json:"name"`
	Phone    string `json:"phone"`
}
