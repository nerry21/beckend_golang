package services

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"backend/internal/repositories"
	"backend/internal/utils"

	"github.com/phpdave11/gofpdf"
)

// DocsService menghasilkan PDF e-ticket & invoice per penumpang.
type DocsService struct {
	PassengerRepo repositories.PassengerRepository
	SeatRepo      repositories.BookingSeatRepo
	BookingRepo   repositories.BookingRepository
	RequestID     string
	Loader        func(int64) (passengerDocData, error)
}

type passengerDocData struct {
	PassengerID    int64
	BookingID      int64
	PassengerName  string
	PassengerPhone string
	SeatCode       string
	RouteFrom      string
	RouteTo        string
	TripDate       string
	TripTime       string
	Pickup         string
	Dropoff        string
	ServiceType    string
	VehicleCode    string
	DriverName     string
	PricePerSeat   int64
}

func (s DocsService) GenerateETicket(passengerID int64) ([]byte, string, error) {
	data, err := s.loadPassengerDocData(passengerID)
	if err != nil {
		return nil, "", err
	}
	utils.LogEvent(s.RequestID, "docs", "generate_eticket", fmt.Sprintf("passenger_id=%d", passengerID))
	return buildETicketPDF(data)
}

func (s DocsService) GenerateInvoice(passengerID int64) ([]byte, string, error) {
	data, err := s.loadPassengerDocData(passengerID)
	if err != nil {
		return nil, "", err
	}
	utils.LogEvent(s.RequestID, "docs", "generate_invoice", fmt.Sprintf("passenger_id=%d", passengerID))
	return buildInvoicePDF(data)
}

func (s DocsService) loadPassengerDocData(passengerID int64) (passengerDocData, error) {
	if s.Loader != nil {
		return s.Loader(passengerID)
	}
	var out passengerDocData
	p, err := s.passengerRepo().GetByID(passengerID)
	if err != nil {
		return out, err
	}
	seatCount := 1
	if seats, err := s.seatRepo().ListByBookingID(p.BookingID); err == nil && len(seats) > 0 {
		seatCount = len(seats)
		if strings.TrimSpace(out.SeatCode) == "" {
			out.SeatCode = seats[0].SeatCode
		}
	}
	out.PassengerID = passengerID
	out.BookingID = p.BookingID
	out.PassengerName = p.PassengerName
	out.PassengerPhone = p.PassengerPhone
	out.SeatCode = p.SelectedSeat
	out.TripDate = p.Date
	out.TripTime = p.DepartureTime
	out.Pickup = p.PickupAddress
	out.Dropoff = p.DropoffAddress
	out.ServiceType = p.ServiceType
	out.VehicleCode = p.VehicleCode
	out.DriverName = p.DriverName
	if p.TotalAmount > 0 {
		out.PricePerSeat = p.TotalAmount
	}

	if seat, err := s.seatRepo().GetSeatByCode(p.BookingID, p.SelectedSeat); err == nil {
		if strings.TrimSpace(out.RouteFrom) == "" {
			out.RouteFrom = seat.RouteFrom
		}
		if strings.TrimSpace(out.RouteTo) == "" {
			out.RouteTo = seat.RouteTo
		}
		if strings.TrimSpace(out.TripDate) == "" {
			out.TripDate = seat.TripDate
		}
		if strings.TrimSpace(out.TripTime) == "" {
			out.TripTime = seat.TripTime
		}
	}

	if booking, err := s.bookingRepo().GetByID(p.BookingID); err == nil {
		if strings.TrimSpace(out.RouteFrom) == "" {
			out.RouteFrom = booking.RouteFrom
		}
		if strings.TrimSpace(out.RouteTo) == "" {
			out.RouteTo = booking.RouteTo
		}
		if strings.TrimSpace(out.TripDate) == "" {
			out.TripDate = booking.TripDate
		}
		if strings.TrimSpace(out.TripTime) == "" {
			out.TripTime = booking.TripTime
		}
		if out.PricePerSeat == 0 {
			out.PricePerSeat = booking.PricePerSeat
			if out.PricePerSeat == 0 {
				// fallback bagi total ke jumlah seat
				if seatCount > 0 && booking.Total > 0 {
					out.PricePerSeat = booking.Total / int64(seatCount)
				} else {
					out.PricePerSeat = booking.Total
				}
			}
		}
		if strings.TrimSpace(out.PassengerName) == "" {
			out.PassengerName = booking.PassengerName
		}
		if strings.TrimSpace(out.ServiceType) == "" {
			out.ServiceType = booking.Category
		}
	}

	// harga per seat by fare rules
	out.PricePerSeat = utils.ComputeFare(out.RouteFrom, out.RouteTo, out.PricePerSeat)

	return out, nil
}

func (s DocsService) passengerRepo() repositories.PassengerRepository {
	if s.PassengerRepo.DB != nil {
		return s.PassengerRepo
	}
	return repositories.PassengerRepository{}
}

func (s DocsService) seatRepo() repositories.BookingSeatRepo {
	if s.SeatRepo.DB != nil {
		return s.SeatRepo
	}
	return repositories.BookingSeatRepo{}
}

func (s DocsService) bookingRepo() repositories.BookingRepository {
	if s.BookingRepo.DB != nil {
		return s.BookingRepo
	}
	return repositories.BookingRepository{}
}

func buildETicketPDF(d passengerDocData) ([]byte, string, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetTitle("E-Ticket", false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 18)
	pdf.Cell(0, 10, "E-TICKET")
	pdf.Ln(12)

	pdf.SetFont("Helvetica", "", 12)
	lines := []string{
		fmt.Sprintf("Nama Penumpang : %s", safe(d.PassengerName, "-")),
		fmt.Sprintf("No HP          : %s", safe(d.PassengerPhone, "-")),
		fmt.Sprintf("Seat           : %s", safe(d.SeatCode, "-")),
		fmt.Sprintf("Layanan        : %s", safe(d.ServiceType, "-")),
		fmt.Sprintf("Rute           : %s -> %s", safe(d.RouteFrom, "-"), safe(d.RouteTo, "-")),
		fmt.Sprintf("Tanggal/Jam    : %s %s", safe(dateOnly(d.TripDate), "-"), safe(timeHM(d.TripTime), "-")),
		fmt.Sprintf("Pickup         : %s", safe(d.Pickup, "-")),
		fmt.Sprintf("Dropoff        : %s", safe(d.Dropoff, "-")),
		fmt.Sprintf("Driver         : %s", safe(d.DriverName, "-")),
		fmt.Sprintf("Kendaraan      : %s", safe(d.VehicleCode, "-")),
		fmt.Sprintf("Kode Booking   : #%d", d.BookingID),
		fmt.Sprintf("Kode Ticket    : TCK-%d-%s", d.BookingID, safeFilenamePart(d.SeatCode)),
	}
	for _, s := range lines {
		pdf.Cell(0, 7, s)
		pdf.Ln(7)
	}

	pdf.Ln(6)
	pdf.SetFont("Helvetica", "I", 10)
	pdf.MultiCell(0, 6, "Catatan: E-ticket ini berlaku untuk 1 penumpang (1 seat). Harap tunjukkan saat keberangkatan.", "", "", false)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, "", err
	}

	filename := fmt.Sprintf("ETICKET_%d_%s.pdf", d.BookingID, safeFilenamePart(d.PassengerName+"_"+d.SeatCode))
	return buf.Bytes(), filename, nil
}

func buildInvoicePDF(d passengerDocData) ([]byte, string, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetTitle("Invoice", false)
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 18)
	pdf.Cell(0, 10, "INVOICE")
	pdf.Ln(12)

	invNo := fmt.Sprintf("INV-%d-%s", d.BookingID, safeFilenamePart(d.SeatCode))
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(0, 7, "No Invoice   : "+invNo)
	pdf.Ln(7)
	pdf.Cell(0, 7, "Tanggal     : "+time.Now().Format("2006-01-02 15:04"))
	pdf.Ln(10)

	pdf.SetFont("Helvetica", "B", 12)
	pdf.Cell(0, 7, "Ditagihkan kepada:")
	pdf.Ln(7)

	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(0, 7, fmt.Sprintf("Nama   : %s", safe(d.PassengerName, "-")))
	pdf.Ln(7)
	pdf.Cell(0, 7, fmt.Sprintf("No HP  : %s", safe(d.PassengerPhone, "-")))
	pdf.Ln(10)

	desc := fmt.Sprintf("Tiket Travel %s -> %s (%s %s) Seat %s",
		safe(d.RouteFrom, "-"), safe(d.RouteTo, "-"),
		safe(dateOnly(d.TripDate), "-"), safe(timeHM(d.TripTime), "-"),
		safe(d.SeatCode, "-"),
	)

	price := d.PricePerSeat
	pdf.SetFont("Helvetica", "B", 12)
	pdf.Cell(0, 7, "Rincian:")
	pdf.Ln(8)

	pdf.SetFont("Helvetica", "", 11)
	pdf.MultiCell(0, 6, "1) "+desc, "", "", false)
	pdf.Ln(2)

	pdf.Cell(0, 6, "Harga (per penumpang): "+formatRupiah(price))
	pdf.Ln(8)

	pdf.SetFont("Helvetica", "B", 12)
	pdf.Cell(0, 8, "Total: "+formatRupiah(price))
	pdf.Ln(12)

	pdf.SetFont("Helvetica", "I", 10)
	pdf.MultiCell(0, 6, "Invoice ini berlaku untuk 1 penumpang (1 seat).", "", "", false)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, "", err
	}

	filename := fmt.Sprintf("INVOICE_%d_%s.pdf", d.BookingID, safeFilenamePart(d.PassengerName+"_"+d.SeatCode))
	return buf.Bytes(), filename, nil
}

func safe(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}

func dateOnly(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 10 {
		return v[:10]
	}
	return v
}

func timeHM(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 5 {
		return v[:5]
	}
	return v
}

func safeFilenamePart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "NA"
	}
	replacer := strings.NewReplacer(" ", "_", "/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	s = replacer.Replace(s)
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

func formatRupiah(v int64) string {
	if v <= 0 {
		return "Rp 0"
	}
	s := fmt.Sprintf("%d", v)
	var out []byte
	n := len(s)
	for i := 0; i < n; i++ {
		out = append(out, s[i])
		pos := n - i - 1
		if pos > 0 && pos%3 == 0 {
			out = append(out, '.')
		}
	}
	return "Rp " + string(out)
}
