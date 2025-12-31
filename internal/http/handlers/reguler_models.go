package handlers

// Max jumlah penumpang untuk reguler (sesuaikan jika kursi berubah)
const RegulerMaxPax = 6

// PassengerItem dipakai untuk surat jalan / list penumpang.
// バ. Ada Phone agar tidak error p.Phone.
type PassengerItem struct {
	Name  string `json:"name"`
	Seat  string `json:"seat"`
	Phone string `json:"phone"`
}

// RegulerBookingRequest: payload dari frontend ketika create booking.
// Catatan:
// - Field baru dibuat OPTIONAL agar kompatibel dengan request lama.
// - Date/time tetap string (frontend mengirim YYYY-MM-DD dan HH:mm).
type RegulerBookingRequest struct {
	Category string `json:"category"`

	From string `json:"from"`
	To   string `json:"to"`

	Date string `json:"date"` // YYYY-MM-DD
	Time string `json:"time"` // HH:mm

	SelectedSeats []string `json:"selectedSeats"`

	// バ. Tambahan agar cocok dengan reguler_handler.go (tidak error lagi)
	BookingFor     string          `json:"bookingFor,omitempty"`     // "self" / "other" (opsional)
	PassengerCount int             `json:"passengerCount,omitempty"` // opsional; default = len(selectedSeats)
	Passengers     []PassengerItem `json:"passengers,omitempty"`     // opsional; detail per-seat

	PassengerName  string `json:"passengerName"`
	PassengerPhone string `json:"passengerPhone"`

	PickupLocation  string `json:"pickupLocation"`
	DropoffLocation string `json:"dropoffLocation"`

	// bisa diisi frontend, tapi backend boleh hitung ulang (lebih aman)
	TotalAmount int64 `json:"totalAmount"`

	// Payment flow baru
	PaymentMethod string `json:"paymentMethod"`           // cash/transfer/qris (optional saat create)
	PaymentStatus string `json:"paymentStatus,omitempty"` // optional: "Menunggu Validasi" / "Lunas" / dst

	// OPTIONAL: upload bukti (biasanya lewat endpoint submit-payment)
	ProofFile     string `json:"proofFile,omitempty"`     // base64/dataURL
	ProofFileName string `json:"proofFileName,omitempty"` // nama file
}

// RegulerBookingResponse: respon backend setelah booking dibuat/diambil.
type RegulerBookingResponse struct {
	BookingID int64 `json:"bookingId"`

	Category string `json:"category"`

	From string `json:"from"`
	To   string `json:"to"`

	Date string `json:"date"`
	Time string `json:"time"`

	SelectedSeats []string `json:"selectedSeats"`

	PassengerName  string `json:"passengerName"`
	PassengerPhone string `json:"passengerPhone"`

	PickupLocation  string `json:"pickupLocation"`
	DropoffLocation string `json:"dropoffLocation"`

	// backend bisa isi hasil hitung/quote
	PricePerSeat int64 `json:"pricePerSeat,omitempty"`
	TotalAmount  int64 `json:"totalAmount"`

	PaymentMethod string `json:"paymentMethod"`
	PaymentStatus string `json:"paymentStatus"`

	// OPTIONAL: relasi ke tabel payment_validations jika disimpan di booking
	PaymentValidationID *int64 `json:"paymentValidationId,omitempty"`
}
