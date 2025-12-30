// backend/handlers/passenger_docs.go
package handlers

import (
	"bytes"
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"backend/config"

	"github.com/gin-gonic/gin"
	"github.com/phpdave11/gofpdf"
)

type PassengerDocData struct {
	PassengerID    int64
	BookingID      int64
	PassengerName  string
	PassengerPhone string
	SeatCode       string

	RouteFrom string
	RouteTo   string

	TripDate string
	TripTime string

	Pickup  string
	Dropoff string

	ServiceType string
	VehicleCode string
	DriverName  string

	PricePerSeat int64
}

// GeneratePassengerETicket membuat PDF e-ticket (bytes, filename) untuk satu passenger.
func GeneratePassengerETicket(passengerID int64) ([]byte, string, error) {
	data, err := loadPassengerDocData(passengerID)
	if err != nil {
		return nil, "", err
	}
	return buildETicketPDF(data)
}

// GeneratePassengerInvoice membuat PDF invoice (bytes, filename) untuk satu passenger.
func GeneratePassengerInvoice(passengerID int64) ([]byte, string, error) {
	data, err := loadPassengerDocData(passengerID)
	if err != nil {
		return nil, "", err
	}
	return buildInvoicePDF(data)
}

func GetPassengerETicketPDF(c *gin.Context) {
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || pid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id passenger tidak valid"})
		return
	}

	data, err := loadPassengerDocData(pid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "data passenger tidak ditemukan"})
		return
	}

	pdfBytes, filename, err := buildETicketPDF(data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal membuat PDF e-ticket"})
		return
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

func GetPassengerInvoicePDF(c *gin.Context) {
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || pid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id passenger tidak valid"})
		return
	}

	data, err := loadPassengerDocData(pid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "data passenger tidak ditemukan"})
		return
	}

	pdfBytes, filename, err := buildInvoicePDF(data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "gagal membuat PDF invoice"})
		return
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

func loadPassengerDocData(passengerID int64) (PassengerDocData, error) {
	var out PassengerDocData

	if !hasTable(config.DB, "passengers") {
		return out, fmt.Errorf("table passengers tidak ada")
	}

	var (
		bookingID                        sql.NullInt64
		name, phone, seats               sql.NullString
		dateStr, timeStr                 sql.NullString
		pickup, dropoff                  sql.NullString
		serviceType, driverName, vehicle sql.NullString
		total                            sql.NullInt64
	)

	err := config.DB.QueryRow(`
		SELECT booking_id, passenger_name, passenger_phone, selected_seats, date, departure_time, pickup_address, dropoff_address, service_type, driver_name, vehicle_code, COALESCE(total_amount,0)
		FROM passengers
		WHERE id=?
		LIMIT 1
	`, passengerID).Scan(
		&bookingID, &name, &phone, &seats, &dateStr, &timeStr, &pickup, &dropoff, &serviceType, &driverName, &vehicle, &total,
	)
	if err != nil {
		return out, err
	}
	if !bookingID.Valid || bookingID.Int64 <= 0 {
		return out, fmt.Errorf("booking_id kosong")
	}

	out.PassengerID = passengerID
	out.BookingID = bookingID.Int64
	out.PassengerName = strings.TrimSpace(name.String)
	out.PassengerPhone = strings.TrimSpace(phone.String)
	out.SeatCode = strings.TrimSpace(firstSeatOnly(seats.String))
	out.TripDate = strings.TrimSpace(dateStr.String)
	out.TripTime = strings.TrimSpace(timeStr.String)
	out.Pickup = strings.TrimSpace(pickup.String)
	out.Dropoff = strings.TrimSpace(dropoff.String)
	out.ServiceType = strings.TrimSpace(serviceType.String)
	out.DriverName = strings.TrimSpace(driverName.String)
	out.VehicleCode = strings.TrimSpace(vehicle.String)
	if total.Valid && total.Int64 > 0 {
		out.PricePerSeat = total.Int64
	}

	// route & trip detail dari booking_seats (per seat)
	if hasTable(config.DB, "booking_seats") && hasColumn(config.DB, "booking_seats", "seat_code") {
		var rf, rt, td, tt sql.NullString
		_ = config.DB.QueryRow(`
			SELECT route_from, route_to, trip_date, trip_time
			FROM booking_seats
			WHERE booking_id=? AND seat_code=?
			ORDER BY id DESC
			LIMIT 1
		`, out.BookingID, out.SeatCode).Scan(&rf, &rt, &td, &tt)

		out.RouteFrom = strings.TrimSpace(rf.String)
		out.RouteTo = strings.TrimSpace(rt.String)

		if out.TripDate == "" {
			out.TripDate = strings.TrimSpace(td.String)
		}
		if out.TripTime == "" {
			out.TripTime = strings.TrimSpace(tt.String)
		}
	}

	// harga per seat dari bookings/reguler_bookings jika ada
	if out.PricePerSeat == 0 {
		out.PricePerSeat = loadPricePerSeat(out.BookingID)
	}
	return out, nil
}

func loadPricePerSeat(bookingID int64) int64 {
	if bookingID <= 0 {
		return 0
	}
	table := ""
	if hasTable(config.DB, "bookings") {
		table = "bookings"
	} else if hasTable(config.DB, "reguler_bookings") {
		table = "reguler_bookings"
	}
	if table == "" {
		return 0
	}

	// prioritas: price_per_seat, fallback bagi total/jumlah seat
	if hasColumn(config.DB, table, "price_per_seat") {
		var v sql.NullInt64
		if err := config.DB.QueryRow(`SELECT price_per_seat FROM `+table+` WHERE id=? LIMIT 1`, bookingID).Scan(&v); err == nil && v.Valid && v.Int64 > 0 {
			return v.Int64
		}
	}

	// fallback: total / jumlah seat di booking
	var total sql.NullInt64
	_ = config.DB.QueryRow(`SELECT COALESCE(total,0) FROM `+table+` WHERE id=? LIMIT 1`, bookingID).Scan(&total)

	seatCount := countBookingSeats(bookingID)
	if total.Valid && total.Int64 > 0 && seatCount > 0 {
		return total.Int64 / int64(seatCount)
	}
	if total.Valid {
		return total.Int64
	}
	return 0
}

func countBookingSeats(bookingID int64) int {
	if bookingID <= 0 {
		return 0
	}
	if hasTable(config.DB, "booking_seats") && hasColumn(config.DB, "booking_seats", "booking_id") {
		var c sql.NullInt64
		_ = config.DB.QueryRow(`SELECT COUNT(*) FROM booking_seats WHERE booking_id=?`, bookingID).Scan(&c)
		if c.Valid && c.Int64 > 0 {
			return int(c.Int64)
		}
	}
	return 0
}

func buildETicketPDF(d PassengerDocData) ([]byte, string, error) {
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

func buildInvoicePDF(d PassengerDocData) ([]byte, string, error) {
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

func formatRupiah(v int64) string {
	s := strconv.FormatInt(v, 10)
	if s == "" {
		return "Rp 0"
	}
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

func firstSeatOnly(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == '\n' || r == '\t'
	})
	if len(parts) == 0 {
		return s
	}
	return strings.TrimSpace(parts[0])
}

func safeFilenamePart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "NA"
	}
	re := regexp.MustCompile(`[^a-zA-Z0-9\-_]+`)
	s = re.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		return "NA"
	}
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
