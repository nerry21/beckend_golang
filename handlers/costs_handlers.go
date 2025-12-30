package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

// API adalah wrapper dependency untuk handlers berbasis net/http.
type API struct {
	DB *sql.DB
}

type VehicleCostMonthly struct {
	ID             int64  `json:"id"`
	CarCode        string `json:"carCode"`
	DriverName     string `json:"driverName"`
	Year           int    `json:"year"`
	Month          int    `json:"month"` // 1-12 (DB)
	MaintenanceFee int64  `json:"maintenanceFee"`
	InsuranceFee   int64  `json:"insuranceFee"`
	InstallmentFee int64  `json:"installmentFee"`
}

type CompanyExpenseMonthly struct {
	ID          int64 `json:"id"`
	Year        int   `json:"year"`
	Month       int   `json:"month"` // 1-12 (DB)
	StaffFee    int64 `json:"staffFee"`
	OfficeFee   int64 `json:"officeFee"`
	InternetFee int64 `json:"internetFee"`
	PromoFee    int64 `json:"promoFee"`
	FlyerFee    int64 `json:"flyerFee"`
	LegalFee    int64 `json:"legalFee"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	// sengaja TIDAK DisallowUnknownFields supaya lebih fleksibel
	return dec.Decode(dst)
}

// GET /vehicle-costs?carCode=LK01&year=2025
func (a *API) ListVehicleCosts(w http.ResponseWriter, r *http.Request) {
	car := r.URL.Query().Get("carCode")
	year := r.URL.Query().Get("year")
	if car == "" || year == "" {
		badRequest(w, "carCode and year required")
		return
	}

	rows, err := a.DB.Query(`
		SELECT id, car_code, driver_name, year, month, maintenance_fee, insurance_fee, installment_fee
		FROM vehicle_costs_monthly
		WHERE car_code=? AND year=?
		ORDER BY month ASC
	`, car, year)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	defer rows.Close()

	out := make([]VehicleCostMonthly, 0, 12)
	for rows.Next() {
		var x VehicleCostMonthly
		if err := rows.Scan(
			&x.ID,
			&x.CarCode,
			&x.DriverName,
			&x.Year,
			&x.Month,
			&x.MaintenanceFee,
			&x.InsuranceFee,
			&x.InstallmentFee,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, out)
}

// POST /vehicle-costs (upsert by carCode+year+month)
func (a *API) UpsertVehicleCost(w http.ResponseWriter, r *http.Request) {
	var x VehicleCostMonthly
	if err := readJSON(r, &x); err != nil {
		badRequest(w, "invalid json: "+err.Error())
		return
	}
	if x.CarCode == "" {
		badRequest(w, "carCode required")
		return
	}
	if x.Year < 2000 || x.Year > 2100 {
		badRequest(w, "year invalid")
		return
	}
	if x.Month < 1 || x.Month > 12 {
		badRequest(w, "month must be 1..12")
		return
	}

	// UPDATE dulu
	res, err := a.DB.Exec(`
		UPDATE vehicle_costs_monthly
		SET driver_name=?, maintenance_fee=?, insurance_fee=?, installment_fee=?
		WHERE car_code=? AND year=? AND month=?
	`, x.DriverName, x.MaintenanceFee, x.InsuranceFee, x.InstallmentFee, x.CarCode, x.Year, x.Month)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	aff, _ := res.RowsAffected()
	if aff == 0 {
		// kalau belum ada row, INSERT
		_, err := a.DB.Exec(`
			INSERT INTO vehicle_costs_monthly
			  (car_code, driver_name, year, month, maintenance_fee, insurance_fee, installment_fee)
			VALUES (?,?,?,?,?,?,?)
		`, x.CarCode, x.DriverName, x.Year, x.Month, x.MaintenanceFee, x.InsuranceFee, x.InstallmentFee)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GET /company-expenses?year=2025
func (a *API) ListCompanyExpenses(w http.ResponseWriter, r *http.Request) {
	year := r.URL.Query().Get("year")
	if year == "" {
		badRequest(w, "year required")
		return
	}

	rows, err := a.DB.Query(`
		SELECT id, year, month, staff_fee, office_fee, internet_fee, promo_fee, flyer_fee, legal_fee
		FROM company_expenses_monthly
		WHERE year=?
		ORDER BY month ASC
	`, year)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	defer rows.Close()

	out := make([]CompanyExpenseMonthly, 0, 12)
	for rows.Next() {
		var x CompanyExpenseMonthly
		if err := rows.Scan(
			&x.ID,
			&x.Year,
			&x.Month,
			&x.StaffFee,
			&x.OfficeFee,
			&x.InternetFee,
			&x.PromoFee,
			&x.FlyerFee,
			&x.LegalFee,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, out)
}

// POST /company-expenses (upsert by year+month)
func (a *API) UpsertCompanyExpense(w http.ResponseWriter, r *http.Request) {
	var x CompanyExpenseMonthly
	if err := readJSON(r, &x); err != nil {
		badRequest(w, "invalid json: "+err.Error())
		return
	}
	if x.Year < 2000 || x.Year > 2100 {
		badRequest(w, "year invalid")
		return
	}
	if x.Month < 1 || x.Month > 12 {
		badRequest(w, "month must be 1..12")
		return
	}

	// UPDATE dulu
	res, err := a.DB.Exec(`
		UPDATE company_expenses_monthly
		SET staff_fee=?, office_fee=?, internet_fee=?, promo_fee=?, flyer_fee=?, legal_fee=?
		WHERE year=? AND month=?
	`, x.StaffFee, x.OfficeFee, x.InternetFee, x.PromoFee, x.FlyerFee, x.LegalFee, x.Year, x.Month)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	aff, _ := res.RowsAffected()
	if aff == 0 {
		_, err := a.DB.Exec(`
			INSERT INTO company_expenses_monthly
			  (year, month, staff_fee, office_fee, internet_fee, promo_fee, flyer_fee, legal_fee)
			VALUES (?,?,?,?,?,?,?,?)
		`, x.Year, x.Month, x.StaffFee, x.OfficeFee, x.InternetFee, x.PromoFee, x.FlyerFee, x.LegalFee)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
