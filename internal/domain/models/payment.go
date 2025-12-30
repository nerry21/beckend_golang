package models

// PaymentValidation represents a payment validation record.
type PaymentValidation struct {
	ID            int64  `json:"id"`
	BookingID     int64  `json:"booking_id"`
	CustomerName  string `json:"customer_name"`
	CustomerPhone string `json:"customer_phone"`
	PickupAddress string `json:"pickup_address"`
	BookingDate   string `json:"booking_date"`
	PaymentMethod string `json:"payment_method"`
	PaymentStatus string `json:"payment_status"`
	Notes         string `json:"notes"`
	ProofFile     string `json:"proof_file"`
	ProofFileName string `json:"proof_file_name"`
	Origin        string `json:"origin"`
	Destination   string `json:"destination"`
	TripRole      string `json:"trip_role"`
}
