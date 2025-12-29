package handlers

import (
	"backend/config"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

type BookingSyncPayload struct {
	BookingID int64

	Category string
	TripRole string
	From     string
	To       string
	Date     string
	Time     string

	PickupLocation  string
	DropoffLocation string

	PassengerName  string
	PassengerPhone string
	CustomerName   string
	CustomerPhone  string
	SelectedSeats  []string
	PassengerCount int

	PricePerSeat  int64
	ETicketPhoto  string
	TotalAmount   int64
	PaymentMethod string
	PaymentStatus string
	CreatedAt     time.Time
}

// ================= helpers =================

func normalizeTripRoleLocal(s string) string {
	s = strings.TrimSpace(s)
	ls := strings.ToLower(s)
	switch ls {
	case "keberangkatan", "berangkat", "departure":
		return "Keberangkatan"
	case "kepulangan", "pulang", "return":
		return "Kepulangan"
	default:
		return strings.TrimSpace(s)
	}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func seatsToString(seats []string) string {
	if len(seats) == 0 {
		return ""
	}
	out := make([]string, 0, len(seats))
	seen := map[string]bool{}
	for _, s := range seats {
		ss := strings.TrimSpace(s)
		if ss == "" {
			continue
		}
		key := strings.ToUpper(ss)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, ss)
	}
	return strings.Join(out, ", ")
}

// untuk matching claim legacy row: coba p.Date dulu, lalu fallback createdAt
func dateCandidates(p BookingSyncPayload) []string {
	cands := []string{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		for _, e := range cands {
			if e == v {
				return
			}
		}
		cands = append(cands, v)
	}

	add(p.Date)
	if !p.CreatedAt.IsZero() {
		add(p.CreatedAt.Format("2006-01-02"))
	}
	return cands
}

// ambil kolom booking id yang ada di table
func getBookingIDCol(tx *sql.Tx, table string) string {
	if hasColumn(tx, table, "booking_id") {
		return "booking_id"
	}
	if hasColumn(tx, table, "reguler_booking_id") {
		return "reguler_booking_id"
	}
	return ""
}

// ================= resolve TripRole + customer fields from payment_validations =================

// TripRole user edit ada di payment_validations, bukan selalu di bookings.
// Jadi kita prioritas ambil dari payment_validations kalau tersedia.
func resolveTripRoleFromValidations(tx *sql.Tx, bookingID int64, current string) string {
	cur := normalizeTripRoleLocal(current)
	if cur == "Keberangkatan" || cur == "Kepulangan" {
		return cur
	}
	if tx == nil || bookingID <= 0 || !hasTable(tx, "payment_validations") {
		return cur
	}

	bcol := getBookingIDCol(tx, "payment_validations")
	if bcol == "" {
		// fallback paling umum
		if hasColumn(tx, "payment_validations", "booking_id") {
			bcol = "booking_id"
		}
	}
	if bcol == "" {
		return cur
	}

	roleCol := ""
	if hasColumn(tx, "payment_validations", "trip_role") {
		roleCol = "trip_role"
	} else if hasColumn(tx, "payment_validations", "role_trip") {
		roleCol = "role_trip"
	}
	if roleCol == "" {
		return cur
	}

	var r sql.NullString
	q := fmt.Sprintf(`SELECT %s FROM payment_validations WHERE %s=? ORDER BY id DESC LIMIT 1`, roleCol, bcol)
	if err := tx.QueryRow(q, bookingID).Scan(&r); err != nil {
		return cur
	}
	if !r.Valid {
		return cur
	}
	return normalizeTripRoleLocal(r.String)
}

func fillCustomerFromValidations(tx *sql.Tx, p *BookingSyncPayload) {
	// jangan ganggu kalau sudah ada
	if strings.TrimSpace(p.CustomerName) != "" && strings.TrimSpace(p.CustomerPhone) != "" && strings.TrimSpace(p.PickupLocation) != "" {
		return
	}
	if tx == nil || p == nil || p.BookingID <= 0 || !hasTable(tx, "payment_validations") {
		return
	}

	bcol := getBookingIDCol(tx, "payment_validations")
	if bcol == "" && hasColumn(tx, "payment_validations", "booking_id") {
		bcol = "booking_id"
	}
	if bcol == "" {
		return
	}

	// pilih kolom yang tersedia
	nameCol := ""
	if hasColumn(tx, "payment_validations", "customer_name") {
		nameCol = "customer_name"
	} else if hasColumn(tx, "payment_validations", "booking_name") {
		nameCol = "booking_name"
	}

	phoneCol := ""
	if hasColumn(tx, "payment_validations", "customer_phone") {
		phoneCol = "customer_phone"
	} else if hasColumn(tx, "payment_validations", "phone") {
		phoneCol = "phone"
	}

	pickCol := ""
	if hasColumn(tx, "payment_validations", "pickup_address") {
		pickCol = "pickup_address"
	} else if hasColumn(tx, "payment_validations", "pickup_location") {
		pickCol = "pickup_location"
	}

	// kalau tidak ada apa2, stop
	if nameCol == "" && phoneCol == "" && pickCol == "" {
		return
	}

	// build SELECT dinamis
	selectCols := []string{}
	dest := []any{}
	var name, phone, pickup sql.NullString

	if nameCol != "" {
		selectCols = append(selectCols, nameCol)
		dest = append(dest, &name)
	}
	if phoneCol != "" {
		selectCols = append(selectCols, phoneCol)
		dest = append(dest, &phone)
	}
	if pickCol != "" {
		selectCols = append(selectCols, pickCol)
		dest = append(dest, &pickup)
	}

	q := fmt.Sprintf(`SELECT %s FROM payment_validations WHERE %s=? ORDER BY id DESC LIMIT 1`, strings.Join(selectCols, ","), bcol)
	if err := tx.QueryRow(q, p.BookingID).Scan(dest...); err != nil {
		return
	}

	if strings.TrimSpace(p.CustomerName) == "" && name.Valid {
		p.CustomerName = strings.TrimSpace(name.String)
	}
	if strings.TrimSpace(p.CustomerPhone) == "" && phone.Valid {
		p.CustomerPhone = strings.TrimSpace(phone.String)
	}
	if strings.TrimSpace(p.PickupLocation) == "" && pickup.Valid {
		p.PickupLocation = strings.TrimSpace(pickup.String)
	}
}

// ================= claim legacy row (booking_id NULL/0) =================

// Cari row legacy (booking_id NULL/0) yang cocok phone + pickup_address + dateCol,
// lalu set booking_id supaya tidak bikin row baru (dan hapus legacy duplikat lainnya).
func claimLegacyRow(tx *sql.Tx, table string, bookingCol string, p BookingSyncPayload, dateCol string) error {
	if tx == nil || table == "" || bookingCol == "" {
		return nil
	}

	// minimal identifikasi
	if !hasColumn(tx, table, "phone") || !hasColumn(tx, table, "pickup_address") {
		return nil
	}

	phone := firstNonEmpty(p.CustomerPhone, p.PassengerPhone)
	pickup := strings.TrimSpace(p.PickupLocation)
	if phone == "" || pickup == "" {
		return nil
	}

	// ORDER BY: prioritaskan row yang sudah ada driver/vehicle (kalau kolom ada)
	orderExpr := "id DESC"
	hasDriver := hasColumn(tx, table, "driver_name")
	hasVType := hasColumn(tx, table, "vehicle_type")
	hasVCode := hasColumn(tx, table, "vehicle_code")
	if hasDriver || hasVType || hasVCode {
		parts := []string{}
		if hasDriver {
			parts = append(parts, "COALESCE(driver_name,'')<>''")
		}
		if hasVType {
			parts = append(parts, "COALESCE(vehicle_type,'')<>''")
		}
		if hasVCode {
			parts = append(parts, "COALESCE(vehicle_code,'')<>''")
		}
		score := "(" + strings.Join(parts, " OR ") + ")"
		orderExpr = "CASE WHEN " + score + " THEN 1 ELSE 0 END DESC, id DESC"
	}

	// kalau table punya dateCol, match pakai date candidates (p.Date lalu createdAt)
	cands := []string{""}
	if dateCol != "" && hasColumn(tx, table, dateCol) {
		cands = dateCandidates(p)
		if len(cands) == 0 {
			// tidak bisa match by date, tetap coba tanpa date
			cands = []string{""}
		}
	}

	for _, dt := range cands {
		var legacyID int64
		var qSel string
		var args []any

		if dateCol != "" && hasColumn(tx, table, dateCol) && strings.TrimSpace(dt) != "" {
			qSel = fmt.Sprintf(
				`SELECT id FROM %s
				 WHERE (%s IS NULL OR %s=0)
				   AND phone=?
				   AND pickup_address=?
				   AND %s=?
				 ORDER BY %s
				 LIMIT 1`,
				table, bookingCol, bookingCol, dateCol, orderExpr,
			)
			args = []any{phone, pickup, strings.TrimSpace(dt)}
		} else {
			qSel = fmt.Sprintf(
				`SELECT id FROM %s
				 WHERE (%s IS NULL OR %s=0)
				   AND phone=?
				   AND pickup_address=?
				 ORDER BY %s
				 LIMIT 1`,
				table, bookingCol, bookingCol, orderExpr,
			)
			args = []any{phone, pickup}
		}

		err := tx.QueryRow(qSel, args...).Scan(&legacyID)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}

		// set booking_id pada legacy row terpilih
		qUpd := fmt.Sprintf(`UPDATE %s SET %s=? WHERE id=?`, table, bookingCol)
		if _, err := tx.Exec(qUpd, p.BookingID, legacyID); err != nil {
			return err
		}

		// hapus legacy lain yang masih NULL/0 agar UI tidak dobel
		var qDel string
		var delArgs []any
		if dateCol != "" && hasColumn(tx, table, dateCol) && strings.TrimSpace(dt) != "" {
			qDel = fmt.Sprintf(
				`DELETE FROM %s
				 WHERE id<>?
				   AND (%s IS NULL OR %s=0)
				   AND phone=?
				   AND pickup_address=?
				   AND %s=?`,
				table, bookingCol, bookingCol, dateCol,
			)
			delArgs = []any{legacyID, phone, pickup, strings.TrimSpace(dt)}
		} else {
			qDel = fmt.Sprintf(
				`DELETE FROM %s
				 WHERE id<>?
				   AND (%s IS NULL OR %s=0)
				   AND phone=?
				   AND pickup_address=?`,
				table, bookingCol, bookingCol,
			)
			delArgs = []any{legacyID, phone, pickup}
		}
		_, _ = tx.Exec(qDel, delArgs...)

		// sukses claim -> stop
		return nil
	}

	return nil
}

// ================= UPSERT helpers (anti dobel permanen + no overwrite kosong) =================

func upsertDepartureSettingsFromPayload(tx *sql.Tx, p BookingSyncPayload) error {
	table := "departure_settings"
	if !hasTable(tx, table) {
		return nil
	}

	bookingCol := getBookingIDCol(tx, table)
	if bookingCol == "" {
		return fmt.Errorf("departure_settings tidak punya kolom booking_id/reguler_booking_id")
	}

	// claim legacy row booking_id NULL/0 biar tidak bikin row baru
	_ = claimLegacyRow(tx, table, bookingCol, p, "departure_date")

	seatStr := seatsToString(p.SelectedSeats)
	// untuk departure_settings, idealnya p.Date adalah tanggal berangkat.
	// Kalau kosong, fallback ke createdAt (tidak ideal, tapi mencegah blank).
	dt := ""
	cands := dateCandidates(p)
	if len(cands) > 0 {
		dt = cands[0]
	}

	// fallback nama/nohp agar tidak kosong
	name := firstNonEmpty(p.CustomerName, p.PassengerName)
	phone := firstNonEmpty(p.CustomerPhone, p.PassengerPhone)
	pickup := strings.TrimSpace(p.PickupLocation)
	category := strings.TrimSpace(p.Category)

	cols := []string{bookingCol}
	args := []any{p.BookingID}

	if hasColumn(tx, table, "booking_name") {
		cols = append(cols, "booking_name")
		args = append(args, name)
	}
	if hasColumn(tx, table, "phone") {
		cols = append(cols, "phone")
		args = append(args, phone)
	}
	if hasColumn(tx, table, "pickup_address") {
		cols = append(cols, "pickup_address")
		args = append(args, pickup)
	}
	if hasColumn(tx, table, "departure_date") {
		cols = append(cols, "departure_date")
		args = append(args, dt)
	}
	if hasColumn(tx, table, "seat_numbers") {
		cols = append(cols, "seat_numbers")
		args = append(args, seatStr)
	}
	if hasColumn(tx, table, "passenger_count") {
		cols = append(cols, "passenger_count")
		args = append(args, p.PassengerCount)
	}
	if hasColumn(tx, table, "service_type") {
		cols = append(cols, "service_type")
		args = append(args, category)
	}
	if hasColumn(tx, table, "updated_at") {
		cols = append(cols, "updated_at")
		args = append(args, time.Now())
	}
	if hasColumn(tx, table, "created_at") {
		cols = append(cols, "created_at")
		if !p.CreatedAt.IsZero() {
			args = append(args, p.CreatedAt)
		} else {
			args = append(args, time.Now())
		}
	}

	ph := make([]string, len(cols))
	for i := range ph {
		ph[i] = "?"
	}

	// UPDATE: jangan overwrite kalau VALUES(...) kosong/0
	updates := []string{}
	for _, c := range cols {
		if c == bookingCol || c == "created_at" {
			continue
		}
		switch c {
		case "booking_name", "phone", "pickup_address", "departure_date", "seat_numbers", "service_type":
			updates = append(updates, fmt.Sprintf("%s=COALESCE(NULLIF(VALUES(%s),''), %s)", c, c, c))
		case "passenger_count":
			updates = append(updates, "passenger_count=CASE WHEN VALUES(passenger_count)>0 THEN VALUES(passenger_count) ELSE passenger_count END")
		case "updated_at":
			updates = append(updates, "updated_at=VALUES(updated_at)")
		default:
			updates = append(updates, fmt.Sprintf("%s=VALUES(%s)", c, c))
		}
	}
	if len(updates) == 0 {
		updates = append(updates, fmt.Sprintf("%s=%s", bookingCol, bookingCol))
	}

	q := fmt.Sprintf(
		`INSERT INTO %s (%s) VALUES (%s)
		 ON DUPLICATE KEY UPDATE %s`,
		table,
		strings.Join(cols, ","),
		strings.Join(ph, ","),
		strings.Join(updates, ", "),
	)

	_, err := tx.Exec(q, args...)
	return err
}

func upsertReturnSettingsFromPayload(tx *sql.Tx, p BookingSyncPayload) error {
	table := "return_settings"
	if !hasTable(tx, table) {
		return nil
	}

	bookingCol := getBookingIDCol(tx, table)
	if bookingCol == "" {
		return fmt.Errorf("return_settings tidak punya kolom booking_id/reguler_booking_id")
	}

	// claim legacy row booking_id NULL/0
	// return_settings kamu kelihatannya pakai departure_date juga (sesuai screenshot struktur).
	dateCol := ""
	if hasColumn(tx, table, "return_date") {
		dateCol = "return_date"
	} else if hasColumn(tx, table, "departure_date") {
		dateCol = "departure_date"
	}
	_ = claimLegacyRow(tx, table, bookingCol, p, dateCol)

	seatStr := seatsToString(p.SelectedSeats)
	dt := ""
	cands := dateCandidates(p)
	if len(cands) > 0 {
		dt = cands[0]
	}

	name := firstNonEmpty(p.CustomerName, p.PassengerName)
	phone := firstNonEmpty(p.CustomerPhone, p.PassengerPhone)
	pickup := strings.TrimSpace(p.PickupLocation)
	category := strings.TrimSpace(p.Category)

	cols := []string{bookingCol}
	args := []any{p.BookingID}

	if hasColumn(tx, table, "booking_name") {
		cols = append(cols, "booking_name")
		args = append(args, name)
	}
	if hasColumn(tx, table, "phone") {
		cols = append(cols, "phone")
		args = append(args, phone)
	}
	if hasColumn(tx, table, "pickup_address") {
		cols = append(cols, "pickup_address")
		args = append(args, pickup)
	}

	if hasColumn(tx, table, "return_date") {
		cols = append(cols, "return_date")
		args = append(args, dt)
	} else if hasColumn(tx, table, "departure_date") {
		cols = append(cols, "departure_date")
		args = append(args, dt)
	}

	if hasColumn(tx, table, "seat_numbers") {
		cols = append(cols, "seat_numbers")
		args = append(args, seatStr)
	}
	if hasColumn(tx, table, "passenger_count") {
		cols = append(cols, "passenger_count")
		args = append(args, p.PassengerCount)
	}
	if hasColumn(tx, table, "service_type") {
		cols = append(cols, "service_type")
		args = append(args, category)
	}
	if hasColumn(tx, table, "updated_at") {
		cols = append(cols, "updated_at")
		args = append(args, time.Now())
	}
	if hasColumn(tx, table, "created_at") {
		cols = append(cols, "created_at")
		if !p.CreatedAt.IsZero() {
			args = append(args, p.CreatedAt)
		} else {
			args = append(args, time.Now())
		}
	}

	ph := make([]string, len(cols))
	for i := range ph {
		ph[i] = "?"
	}

	updates := []string{}
	for _, c := range cols {
		if c == bookingCol || c == "created_at" {
			continue
		}
		switch c {
		case "booking_name", "phone", "pickup_address", "return_date", "departure_date", "seat_numbers", "service_type":
			updates = append(updates, fmt.Sprintf("%s=COALESCE(NULLIF(VALUES(%s),''), %s)", c, c, c))
		case "passenger_count":
			updates = append(updates, "passenger_count=CASE WHEN VALUES(passenger_count)>0 THEN VALUES(passenger_count) ELSE passenger_count END")
		case "updated_at":
			updates = append(updates, "updated_at=VALUES(updated_at)")
		default:
			updates = append(updates, fmt.Sprintf("%s=VALUES(%s)", c, c))
		}
	}
	if len(updates) == 0 {
		updates = append(updates, fmt.Sprintf("%s=%s", bookingCol, bookingCol))
	}

	q := fmt.Sprintf(
		`INSERT INTO %s (%s) VALUES (%s)
		 ON DUPLICATE KEY UPDATE %s`,
		table,
		strings.Join(cols, ","),
		strings.Join(ph, ","),
		strings.Join(updates, ", "),
	)

	_, err := tx.Exec(q, args...)
	return err
}

// ===================== public sync functions =====================

// Kompatibel dengan call site lama: SyncConfirmedRegulerBooking(tx, bookingID)
func SyncConfirmedRegulerBooking(tx *sql.Tx, bookingID int64) error {
	if bookingID <= 0 {
		return fmt.Errorf("SyncConfirmedRegulerBooking: bookingID invalid")
	}

	ownTx := false
	committed := false

	if tx == nil {
		t, err := config.DB.Begin()
		if err != nil {
			return err
		}
		tx = t
		ownTx = true
	}

	if ownTx {
		defer func() {
			if !committed {
				_ = tx.Rollback()
			}
		}()
	}

	p, err := readBookingPayload(tx, bookingID)
	if err != nil {
		return err
	}
	// ✅ SAFETY: beberapa versi readBookingPayload tidak mengisi p.BookingID.
	// Kalau kosong, paksa isi dari argumen agar booking_id tidak menjadi 0 di table tujuan.
	if p.BookingID <= 0 {
		p.BookingID = bookingID
	}
	// ✅ SAFETY: beberapa versi readBookingPayload tidak mengisi p.BookingID.
	// Kalau kosong, paksa isi dari argumen agar booking_id tidak menjadi 0 di table tujuan.
	if p.BookingID <= 0 {
		p.BookingID = bookingID
	}

	// ✅ TripRole final: ambil dari payment_validations kalau ada
	p.TripRole = resolveTripRoleFromValidations(tx, p.BookingID, p.TripRole)

	// ✅ lengkapi customer dari payment_validations kalau masih kosong
	fillCustomerFromValidations(tx, &p)

	// ✅ HANYA keberangkatan
	if strings.TrimSpace(p.TripRole) != "Keberangkatan" {
		log.Println("[SYNC] skip booking", p.BookingID, "TripRole:", p.TripRole, "(harus Keberangkatan)")
		if ownTx {
			if err := tx.Commit(); err != nil {
				return err
			}
			committed = true
		}
		return nil
	}

	// ✅ hanya jika paid
	if !isPaidSuccess(p.PaymentStatus, p.PaymentMethod) {
		log.Println("[SYNC] skip booking", p.BookingID, "status:", p.PaymentStatus, "method:", p.PaymentMethod, "belum paid")
		if ownTx {
			if err := tx.Commit(); err != nil {
				return err
			}
			committed = true
		}
		return nil
	}

	// ✅ UPSERT anti dobel permanen + no overwrite kosong
	if err := upsertDepartureSettingsFromPayload(tx, p); err != nil {
		return err
	}

	if ownTx {
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
	}

	return nil
}

// SyncConfirmedRegulerBookingTx dipertahankan untuk kompatibilitas call site lama.
func SyncConfirmedRegulerBookingTx(tx *sql.Tx, bookingID int64) error {
	return SyncConfirmedRegulerBooking(tx, bookingID)
}

// SyncReturnBooking: versi khusus kepulangan -> hanya push ke return_settings.
func SyncReturnBooking(tx *sql.Tx, bookingID int64) error {
	if bookingID <= 0 {
		return fmt.Errorf("SyncReturnBooking: bookingID invalid")
	}

	ownTx := false
	committed := false

	if tx == nil {
		t, err := config.DB.Begin()
		if err != nil {
			return err
		}
		tx = t
		ownTx = true
	}

	if ownTx {
		defer func() {
			if !committed {
				_ = tx.Rollback()
			}
		}()
	}

	p, err := readBookingPayload(tx, bookingID)
	if err != nil {
		return err
	}

	// ✅ TripRole final: ambil dari payment_validations kalau ada
	p.TripRole = resolveTripRoleFromValidations(tx, p.BookingID, p.TripRole)

	// ✅ lengkapi customer dari payment_validations kalau masih kosong
	fillCustomerFromValidations(tx, &p)

	// ✅ HANYA kepulangan
	if strings.TrimSpace(p.TripRole) != "Kepulangan" {
		log.Println("[SYNC RETURN] skip booking", p.BookingID, "TripRole:", p.TripRole, "(harus Kepulangan)")
		if ownTx {
			if err := tx.Commit(); err != nil {
				return err
			}
			committed = true
		}
		return nil
	}

	// ✅ hanya jika paid
	if !isPaidSuccess(p.PaymentStatus, p.PaymentMethod) {
		log.Println("[SYNC RETURN] skip booking", p.BookingID, "status:", p.PaymentStatus, "method:", p.PaymentMethod, "belum paid")
		if ownTx {
			if err := tx.Commit(); err != nil {
				return err
			}
			committed = true
		}
		return nil
	}

	// ✅ UPSERT anti dobel permanen + no overwrite kosong
	if err := upsertReturnSettingsFromPayload(tx, p); err != nil {
		return err
	}

	if ownTx {
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
	}

	return nil
}

func isPaidSuccess(paymentStatus, paymentMethod string) bool {
	s := strings.ToLower(strings.TrimSpace(paymentStatus))
	m := strings.ToLower(strings.TrimSpace(paymentMethod))

	switch m {
	case "cash":
		return s == "" || s == "sukses" || s == "lunas" || s == "paid"
	default:
		switch s {
		case "sukses", "lunas", "paid":
			return true
		default:
			return false
		}
	}
}
