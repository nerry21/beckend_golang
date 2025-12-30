package repositories

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	intconfig "backend/internal/config"
	intdb "backend/internal/db"
	"backend/internal/domain/models"
)

type PaymentRepository struct {
	DB *sql.DB
}

func (r PaymentRepository) db() *sql.DB {
	if r.DB != nil {
		return r.DB
	}
	return intconfig.DB
}

func (r PaymentRepository) table() string {
	return "payment_validations"
}

// GetValidationByID fetches payment validation by primary key.
func (r PaymentRepository) GetValidationByID(id int64) (models.PaymentValidation, error) {
	if id <= 0 {
		return models.PaymentValidation{}, fmt.Errorf("id tidak valid")
	}
	db := r.db()
	table := r.table()
	if db == nil || !intdb.HasTable(db, table) {
		return models.PaymentValidation{}, fmt.Errorf("tabel payment_validations tidak ditemukan")
	}

	query := `
		SELECT id,
		       COALESCE(booking_id,0),
		       COALESCE(customer_name,''),
		       COALESCE(customer_phone,''),
		       COALESCE(pickup_address,''),
		       COALESCE(booking_date,''),
		       COALESCE(payment_method,''),
		       COALESCE(payment_status,''),
		       COALESCE(notes,''),
		       COALESCE(proof_file,''),
		       COALESCE(proof_file_name,''),
		       COALESCE(origin,''),
		       COALESCE(destination,''),
		       COALESCE(trip_role,'')
		FROM ` + table + `
		WHERE id=? LIMIT 1`

	var p models.PaymentValidation
	if err := db.QueryRow(query, id).Scan(
		&p.ID,
		&p.BookingID,
		&p.CustomerName,
		&p.CustomerPhone,
		&p.PickupAddress,
		&p.BookingDate,
		&p.PaymentMethod,
		&p.PaymentStatus,
		&p.Notes,
		&p.ProofFile,
		&p.ProofFileName,
		&p.Origin,
		&p.Destination,
		&p.TripRole,
	); err != nil {
		return models.PaymentValidation{}, err
	}
	return p, nil
}

// GetByBookingID returns validation by booking_id if exists.
func (r PaymentRepository) GetByBookingID(bookingID int64) (models.PaymentValidation, error) {
	if bookingID <= 0 {
		return models.PaymentValidation{}, fmt.Errorf("booking_id tidak valid")
	}
	db := r.db()
	table := r.table()
	if db == nil || !intdb.HasTable(db, table) || !intdb.HasColumn(db, table, "booking_id") {
		return models.PaymentValidation{}, fmt.Errorf("validation tidak tersedia")
	}

	query := `
		SELECT id,
		       COALESCE(booking_id,0),
		       COALESCE(customer_name,''),
		       COALESCE(customer_phone,''),
		       COALESCE(pickup_address,''),
		       COALESCE(booking_date,''),
		       COALESCE(payment_method,''),
		       COALESCE(payment_status,''),
		       COALESCE(notes,''),
		       COALESCE(proof_file,''),
		       COALESCE(proof_file_name,''),
		       COALESCE(origin,''),
		       COALESCE(destination,''),
		       COALESCE(trip_role,'')
		FROM ` + table + `
		WHERE booking_id=? LIMIT 1`

	var p models.PaymentValidation
	if err := db.QueryRow(query, bookingID).Scan(
		&p.ID,
		&p.BookingID,
		&p.CustomerName,
		&p.CustomerPhone,
		&p.PickupAddress,
		&p.BookingDate,
		&p.PaymentMethod,
		&p.PaymentStatus,
		&p.Notes,
		&p.ProofFile,
		&p.ProofFileName,
		&p.Origin,
		&p.Destination,
		&p.TripRole,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.PaymentValidation{}, nil
		}
		return models.PaymentValidation{}, err
	}
	return p, nil
}

// CreateOrUpdateValidation inserts/updates payment_validations by booking_id using key presence.
func (r PaymentRepository) CreateOrUpdateValidation(bookingID int64, raw json.RawMessage) error {
	if bookingID <= 0 {
		return fmt.Errorf("booking_id tidak valid")
	}
	db := r.db()
	table := r.table()
	if db == nil || !intdb.HasTable(db, table) {
		return fmt.Errorf("tabel payment_validations tidak ditemukan")
	}

	var payload map[string]any
	_ = json.Unmarshal(raw, &payload)
	hasField := func(names ...string) bool {
		for _, n := range names {
			if payload[strings.ToLower(n)] != nil {
				return true
			}
			if payload[n] != nil {
				return true
			}
		}
		return false
	}

	fieldString := func(key string) string {
		if v, ok := payload[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		if v, ok := payload[strings.ToLower(key)]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}

	columns := []string{}
	values := []any{}

	set := func(present bool, col string, val any) {
		if present && intdb.HasColumn(db, table, col) {
			columns = append(columns, col+"=?")
			values = append(values, val)
		}
	}

	set(hasField("customer_name"), "customer_name", fieldString("customer_name"))
	set(hasField("customer_phone"), "customer_phone", fieldString("customer_phone"))
	set(hasField("pickup_address"), "pickup_address", fieldString("pickup_address"))
	set(hasField("booking_date"), "booking_date", fieldString("booking_date"))
	set(hasField("payment_method"), "payment_method", fieldString("payment_method"))
	set(hasField("payment_status"), "payment_status", fieldString("payment_status"))
	set(hasField("notes"), "notes", fieldString("notes"))
	set(hasField("proof_file"), "proof_file", fieldString("proof_file"))
	set(hasField("proof_file_name"), "proof_file_name", fieldString("proof_file_name"))
	set(hasField("origin"), "origin", fieldString("origin"))
	set(hasField("destination"), "destination", fieldString("destination"))
	set(hasField("trip_role"), "trip_role", fieldString("trip_role"))

	// cek apakah sudah ada row by booking_id
	var existingID int64
	if intdb.HasColumn(db, table, "booking_id") {
		_ = db.QueryRow(`SELECT id FROM `+table+` WHERE booking_id=? LIMIT 1`, bookingID).Scan(&existingID)
	}

	if existingID == 0 {
		cols := []string{"booking_id"}
		vals := []any{bookingID}
		for _, part := range columns {
			cols = append(cols, strings.Split(part, "=")[0])
		}
		vals = append(vals, values...)
		if len(cols) == 1 {
			_, err := db.Exec(`INSERT INTO `+table+` (booking_id) VALUES (?)`, bookingID)
			return err
		}
		placeholders := make([]string, len(cols))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		_, err := db.Exec(`INSERT INTO `+table+` (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(placeholders, ",")+`)`, vals...)
		return err
	}

	if len(columns) == 0 {
		return nil
	}
	values = append(values, existingID)
	_, err := db.Exec(`UPDATE `+table+` SET `+strings.Join(columns, ",")+` WHERE id=?`, values...)
	return err
}

// UpsertValidationByBooking kept for backward compatibility.
func (r PaymentRepository) UpsertValidationByBooking(bookingID int64, raw json.RawMessage) error {
	return r.CreateOrUpdateValidation(bookingID, raw)
}
