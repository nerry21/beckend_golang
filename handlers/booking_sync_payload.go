package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func readBookingPayload(tx *sql.Tx, bookingID int64) (BookingSyncPayload, error) {
	table := "bookings"
	if !hasTable(tx, table) {
		if hasTable(tx, "reguler_bookings") {
			table = "reguler_bookings"
		}
	}

	cols := []string{"id"}

	// kategori
	if hasColumn(tx, table, "category") {
		cols = append(cols, "category")
	}
	if hasColumn(tx, table, "trip_role") {
		cols = append(cols, "trip_role")
	}

	// route/from/to (support beberapa schema)
	if hasColumn(tx, table, "from_city") {
		cols = append(cols, "from_city")
	} else if hasColumn(tx, table, "route_from") {
		cols = append(cols, "route_from")
	}
	if hasColumn(tx, table, "to_city") {
		cols = append(cols, "to_city")
	} else if hasColumn(tx, table, "route_to") {
		cols = append(cols, "route_to")
	}

	// tanggal/jam
	if hasColumn(tx, table, "trip_date") {
		cols = append(cols, "trip_date")
	}
	if hasColumn(tx, table, "trip_time") {
		cols = append(cols, "trip_time")
	}

	// pickup/dropoff
	if hasColumn(tx, table, "pickup_location") {
		cols = append(cols, "pickup_location")
	}
	if hasColumn(tx, table, "dropoff_location") {
		cols = append(cols, "dropoff_location")
	}

	// nama pemesan
	if hasColumn(tx, table, "booking_for") {
		cols = append(cols, "booking_for")
	}
	if hasColumn(tx, table, "passenger_name") {
		cols = append(cols, "passenger_name")
	}
	if hasColumn(tx, table, "passenger_phone") {
		cols = append(cols, "passenger_phone")
	}
	if hasColumn(tx, table, "customer_name") {
		cols = append(cols, "customer_name")
	}
	if hasColumn(tx, table, "customer_phone") {
		cols = append(cols, "customer_phone")
	}

	// dokumen e-ticket dari booking jika ada
	if hasColumn(tx, table, "eticket_photo") {
		cols = append(cols, "eticket_photo")
	} else if hasColumn(tx, table, "e_ticket_photo") {
		cols = append(cols, "e_ticket_photo")
	} else if hasColumn(tx, table, "eticket_url") {
		cols = append(cols, "eticket_url")
	} else if hasColumn(tx, table, "eticket") {
		cols = append(cols, "eticket")
	}

	// total
	switch {
	case hasColumn(tx, table, "total_amount"):
		cols = append(cols, "total_amount")
	case hasColumn(tx, table, "total"):
		cols = append(cols, "total")
	case hasColumn(tx, table, "grand_total"):
		cols = append(cols, "grand_total")
	case hasColumn(tx, table, "total_invoice"):
		cols = append(cols, "total_invoice")
	case hasColumn(tx, table, "invoice_total"):
		cols = append(cols, "invoice_total")
	case hasColumn(tx, table, "amount"):
		cols = append(cols, "amount")
	case hasColumn(tx, table, "nominal"):
		cols = append(cols, "nominal")
	}

	// passenger count (untuk hitung total jika total kosong)
	if hasColumn(tx, table, "passenger_count") {
		cols = append(cols, "passenger_count")
	}

	// payment
	if hasColumn(tx, table, "payment_method") {
		cols = append(cols, "payment_method")
	}
	if hasColumn(tx, table, "payment_status") {
		cols = append(cols, "payment_status")
	}

	// created_at
	if hasColumn(tx, table, "created_at") {
		cols = append(cols, "created_at")
	}

	// seats (json/string)
	if hasColumn(tx, table, "selected_seats") {
		cols = append(cols, "selected_seats")
	}
	if hasColumn(tx, table, "seats_json") {
		cols = append(cols, "seats_json")
	}
	if hasColumn(tx, table, "price_per_seat") {
		cols = append(cols, "price_per_seat")
	}

	q := fmt.Sprintf("SELECT %s FROM %s WHERE id=? LIMIT 1", strings.Join(cols, ","), table)

	// scan targets
	var (
		id int64

		category        sql.NullString
		fromCity        sql.NullString
		toCity          sql.NullString
		tripDate        sql.NullString
		tripTime        sql.NullString
		pickupLocation  sql.NullString
		dropoffLocation sql.NullString
		bookingFor      sql.NullString
		passengerName   sql.NullString
		passengerPhone  sql.NullString
		customerName    sql.NullString
		customerPhone   sql.NullString

		tripRole sql.NullString

		totalAmount  sql.NullInt64
		totalAmount2 sql.NullInt64
		payMethod    sql.NullString
		payStatus    sql.NullString
		createdAt    sql.NullTime

		selectedSeatsRaw sql.NullString
		seatsJSONRaw     sql.NullString
		eticketPhoto     sql.NullString

		pricePerSeat sql.NullInt64
		passengerCnt sql.NullInt64
	)

	dests := []any{&id}
	for _, c := range cols[1:] {
		switch c {
		case "category":
			dests = append(dests, &category)
		case "trip_role":
			dests = append(dests, &tripRole)
		case "from_city", "route_from":
			dests = append(dests, &fromCity)
		case "to_city", "route_to":
			dests = append(dests, &toCity)
		case "trip_date":
			dests = append(dests, &tripDate)
		case "trip_time":
			dests = append(dests, &tripTime)
		case "pickup_location":
			dests = append(dests, &pickupLocation)
		case "dropoff_location":
			dests = append(dests, &dropoffLocation)
		case "booking_for":
			dests = append(dests, &bookingFor)
		case "passenger_name":
			dests = append(dests, &passengerName)
		case "passenger_phone":
			dests = append(dests, &passengerPhone)
		case "customer_name":
			dests = append(dests, &customerName)
		case "customer_phone":
			dests = append(dests, &customerPhone)
		case "total_amount", "total":
			dests = append(dests, &totalAmount)
		case "grand_total", "total_invoice", "invoice_total", "amount", "nominal":
			dests = append(dests, &totalAmount2)
		case "payment_method":
			dests = append(dests, &payMethod)
		case "payment_status":
			dests = append(dests, &payStatus)
		case "created_at":
			dests = append(dests, &createdAt)
		case "passenger_count":
			dests = append(dests, &passengerCnt)
		case "selected_seats":
			dests = append(dests, &selectedSeatsRaw)
		case "seats_json":
			dests = append(dests, &seatsJSONRaw)
		case "eticket_photo", "e_ticket_photo", "eticket_url", "eticket":
			dests = append(dests, &eticketPhoto)
		case "price_per_seat":
			dests = append(dests, &pricePerSeat)
		default:
			// fallback
			var tmp any
			dests = append(dests, &tmp)
		}
	}

	if err := tx.QueryRow(q, bookingID).Scan(dests...); err != nil {
		return BookingSyncPayload{}, err
	}

	p := BookingSyncPayload{
		BookingID: id,
	}

	if category.Valid {
		p.Category = strings.TrimSpace(category.String)
	}
	if fromCity.Valid {
		p.From = strings.TrimSpace(fromCity.String)
	}
	if toCity.Valid {
		p.To = strings.TrimSpace(toCity.String)
	}
	if tripDate.Valid {
		p.Date = strings.TrimSpace(tripDate.String)
	}
	if tripTime.Valid {
		p.Time = strings.TrimSpace(tripTime.String)
	}
	if pickupLocation.Valid {
		p.PickupLocation = strings.TrimSpace(pickupLocation.String)
	}
	if dropoffLocation.Valid {
		p.DropoffLocation = strings.TrimSpace(dropoffLocation.String)
	}

	// nama pemesan fallback booking_for -> passenger_name
	if passengerName.Valid && strings.TrimSpace(passengerName.String) != "" {
		p.PassengerName = strings.TrimSpace(passengerName.String)
	} else if bookingFor.Valid {
		p.PassengerName = strings.TrimSpace(bookingFor.String)
	} else if customerName.Valid {
		p.PassengerName = strings.TrimSpace(customerName.String)
	}
	if customerName.Valid {
		p.CustomerName = strings.TrimSpace(customerName.String)
	}

	if passengerPhone.Valid && strings.TrimSpace(passengerPhone.String) != "" {
		p.PassengerPhone = strings.TrimSpace(passengerPhone.String)
	} else if customerPhone.Valid {
		p.PassengerPhone = strings.TrimSpace(customerPhone.String)
	}
	if customerPhone.Valid {
		p.CustomerPhone = strings.TrimSpace(customerPhone.String)
	}
	if passengerCnt.Valid && passengerCnt.Int64 > 0 {
		p.PassengerCount = int(passengerCnt.Int64)
	}

	if totalAmount.Valid {
		p.TotalAmount = totalAmount.Int64
	}
	if p.TotalAmount == 0 && totalAmount2.Valid {
		p.TotalAmount = totalAmount2.Int64
	}
	if payMethod.Valid {
		p.PaymentMethod = strings.TrimSpace(payMethod.String)
	}
	if payStatus.Valid {
		p.PaymentStatus = strings.TrimSpace(payStatus.String)
	}
	if tripRole.Valid {
		p.TripRole = strings.TrimSpace(tripRole.String)
	}
	if createdAt.Valid {
		p.CreatedAt = createdAt.Time
	}
	if pricePerSeat.Valid {
		p.PricePerSeat = pricePerSeat.Int64
	}

	// fallback trip_role dari payment_validations jika masih kosong
	if strings.TrimSpace(p.TripRole) == "" &&
		hasTable(tx, "payment_validations") &&
		hasColumn(tx, "payment_validations", "booking_id") &&
		hasColumn(tx, "payment_validations", "trip_role") {
		var pvRole sql.NullString
		_ = tx.QueryRow(
			`SELECT COALESCE(trip_role,'') FROM payment_validations WHERE booking_id=? ORDER BY id DESC LIMIT 1`,
			p.BookingID,
		).Scan(&pvRole)
		if pvRole.Valid && strings.TrimSpace(pvRole.String) != "" {
			p.TripRole = strings.TrimSpace(pvRole.String)
		}
	}

	// seats parsing
	var seats []string
	if selectedSeatsRaw.Valid && strings.TrimSpace(selectedSeatsRaw.String) != "" {
		seats = parseSeatsFlexible(selectedSeatsRaw.String)
	} else if seatsJSONRaw.Valid && strings.TrimSpace(seatsJSONRaw.String) != "" {
		seats = parseSeatsFlexible(seatsJSONRaw.String)
	}
	p.SelectedSeats = normalizeSeatsUnique(seats)

	// fallback price per seat dari rute jika kolom DB kosong
	if p.PricePerSeat == 0 {
		if fare := regulerFareFromBooking(p.From, p.To); fare > 0 {
			p.PricePerSeat = fare
		}
	}

	if eticketPhoto.Valid {
		p.ETicketPhoto = strings.TrimSpace(eticketPhoto.String)
	}

	// fallback nama/no hp dari payment_validations jika masih kosong
	if (strings.TrimSpace(p.PassengerName) == "" || strings.TrimSpace(p.PassengerPhone) == "") && hasTable(tx, "payment_validations") {
		n, ph := loadPaymentValidationContact(tx, p.BookingID)
		if strings.TrimSpace(p.PassengerName) == "" && n != "" {
			p.PassengerName = n
		}
		if strings.TrimSpace(p.PassengerPhone) == "" && ph != "" {
			p.PassengerPhone = ph
		}
	}

	// fallback TotalAmount dari jumlah kursi x harga kursi jika total masih kosong
	seatCount := len(p.SelectedSeats)
	if seatCount == 0 && p.PassengerCount > 0 {
		seatCount = p.PassengerCount
	}
	if seatCount == 0 {
		seatCount = 1
	}
	if p.TotalAmount == 0 && p.PricePerSeat > 0 {
		p.TotalAmount = p.PricePerSeat * int64(seatCount)
	}
	if p.TotalAmount == 0 {
		if inv := lookupInvoiceTotal(tx, p.BookingID); inv > 0 {
			p.TotalAmount = inv
		}
	}

	return p, nil
}

func normalizeSeatsUnique(seats []string) []string {
	m := map[string]bool{}
	out := []string{}
	for _, s := range seats {
		s = strings.ToUpper(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if !m[s] {
			m[s] = true
			out = append(out, s)
		}
	}
	return out
}

// parseSeatsFlexible handle input:
// - json array ["1A","1B"]
// - json string "\"1A,1B\""
// - string "1A, 1B"
// - string "1A 1B"
func parseSeatsFlexible(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	// coba parse json array
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return arr
	}

	// coba parse json string
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err == nil {
		raw = s
	}

	// split by comma/space
	raw = strings.ReplaceAll(raw, "\n", ",")
	raw = strings.ReplaceAll(raw, "\t", ",")
	raw = strings.ReplaceAll(raw, ";", ",")
	parts := strings.Split(raw, ",")
	out := []string{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// kalau masih ada spasi, pecah
		sub := strings.Fields(p)
		if len(sub) > 1 {
			for _, x := range sub {
				x = strings.TrimSpace(x)
				if x != "" {
					out = append(out, x)
				}
			}
		} else {
			out = append(out, p)
		}
	}
	return out
}

func normalizeDateOnly(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// kalau datetime "YYYY-MM-DD HH:MM:SS"
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func normalizeTripDate(s string) string {
	return normalizeDateOnly(s)
}

func normalizeTripTime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// normalisasi "08:00:00" -> "08:00"
	if len(s) >= 5 {
		return s[:5]
	}
	return s
}

func autoTripNumber(dateStr, timeStr, from, to string) string {
	dateOnly := normalizeDateOnly(dateStr)
	timeOnly := normalizeTripTime(timeStr)
	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	if dateOnly == "" {
		dateOnly = time.Now().Format("2006-01-02")
	}
	if timeOnly == "" {
		timeOnly = "00:00"
	}
	return fmt.Sprintf("%s-%s-%s-%s", dateOnly, timeOnly, from, to)
}

func buildSuratJalanAPI(bookingID int64) string {
	return fmt.Sprintf("http://localhost:8080/api/reguler/bookings/%d/surat-jalan", bookingID)
}

// loadPaymentValidationContact: ambil customer_name/phone dari payment_validations (relasi booking_id) jika tersedia.
func loadPaymentValidationContact(tx *sql.Tx, bookingID int64) (string, string) {
	if bookingID <= 0 {
		return "", ""
	}
	if !hasColumn(tx, "payment_validations", "booking_id") {
		return "", ""
	}

	cols := []string{}
	if hasColumn(tx, "payment_validations", "customer_name") {
		cols = append(cols, "COALESCE(customer_name,'')")
	}
	if hasColumn(tx, "payment_validations", "customer_phone") {
		cols = append(cols, "COALESCE(customer_phone,'')")
	}
	if len(cols) == 0 {
		return "", ""
	}

	q := fmt.Sprintf(`SELECT %s FROM payment_validations WHERE booking_id=? ORDER BY id DESC LIMIT 1`, strings.Join(cols, ","))

	var n sql.NullString
	var ph sql.NullString
	dests := []any{}
	switch len(cols) {
	case 2:
		dests = []any{&n, &ph}
	case 1:
		if strings.Contains(cols[0], "customer_name") {
			dests = []any{&n}
		} else {
			dests = []any{&ph}
		}
	default:
		dests = []any{&n, &ph}
	}

	if err := tx.QueryRow(q, bookingID).Scan(dests...); err != nil {
		return "", ""
	}

	outName := ""
	outPhone := ""
	if n.Valid {
		outName = strings.TrimSpace(n.String)
	}
	if ph.Valid {
		outPhone = strings.TrimSpace(ph.String)
	}
	return outName, outPhone
}
