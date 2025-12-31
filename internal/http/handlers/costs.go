package handlers

import (
	"net/http"
	"strconv"

	intconfig "backend/internal/config"

	"github.com/gin-gonic/gin"
)

type VehicleCostMonthly struct {
	ID             int64  `json:"id"`
	CarCode        string `json:"carCode"`
	DriverName     string `json:"driverName"`
	Year           int    `json:"year"`
	Month          int    `json:"month"`
	MaintenanceFee int64  `json:"maintenanceFee"`
	InsuranceFee   int64  `json:"insuranceFee"`
	InstallmentFee int64  `json:"installmentFee"`
}

type CompanyExpenseMonthly struct {
	ID          int64 `json:"id"`
	Year        int   `json:"year"`
	Month       int   `json:"month"`
	StaffFee    int64 `json:"staffFee"`
	OfficeFee   int64 `json:"officeFee"`
	InternetFee int64 `json:"internetFee"`
	PromoFee    int64 `json:"promoFee"`
	FlyerFee    int64 `json:"flyerFee"`
	LegalFee    int64 `json:"legalFee"`
}

// GET /vehicle-costs?carCode=LK01&year=2025
func ListVehicleCosts(c *gin.Context) {
	car := c.Query("carCode")
	year := c.Query("year")
	if car == "" || year == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "carCode and year required"})
		return
	}

	rows, err := intconfig.DB.Query(`
		SELECT id, car_code, driver_name, year, month, maintenance_fee, insurance_fee, installment_fee
		FROM vehicle_costs_monthly
		WHERE car_code=? AND year=?
		ORDER BY month ASC
	`, car, year)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, out)
}

// POST /vehicle-costs (upsert by carCode+year+month)
func UpsertVehicleCost(c *gin.Context) {
	var x VehicleCostMonthly
	if err := c.ShouldBindJSON(&x); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
		return
	}
	if x.CarCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "carCode required"})
		return
	}
	if x.Year < 2000 || x.Year > 2100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "year invalid"})
		return
	}
	if x.Month < 1 || x.Month > 12 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "month must be 1..12"})
		return
	}

	res, err := intconfig.DB.Exec(`
		UPDATE vehicle_costs_monthly
		SET driver_name=?, maintenance_fee=?, insurance_fee=?, installment_fee=?
		WHERE car_code=? AND year=? AND month=?
	`, x.DriverName, x.MaintenanceFee, x.InsuranceFee, x.InstallmentFee, x.CarCode, x.Year, x.Month)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	aff, _ := res.RowsAffected()
	if aff == 0 {
		if _, err := intconfig.DB.Exec(`
			INSERT INTO vehicle_costs_monthly
			  (car_code, driver_name, year, month, maintenance_fee, insurance_fee, installment_fee)
			VALUES (?,?,?,?,?,?,?)
		`, x.CarCode, x.DriverName, x.Year, x.Month, x.MaintenanceFee, x.InsuranceFee, x.InstallmentFee); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GET /company-expenses?year=2025
func ListCompanyExpenses(c *gin.Context) {
	year := c.Query("year")
	if year == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "year required"})
		return
	}

	rows, err := intconfig.DB.Query(`
		SELECT id, year, month, staff_fee, office_fee, internet_fee, promo_fee, flyer_fee, legal_fee
		FROM company_expenses_monthly
		WHERE year=?
		ORDER BY month ASC
	`, year)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, out)
}

// POST /company-expenses (upsert by year+month)
func UpsertCompanyExpense(c *gin.Context) {
	var x CompanyExpenseMonthly
	if err := c.ShouldBindJSON(&x); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
		return
	}
	if x.Year < 2000 || x.Year > 2100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "year invalid"})
		return
	}
	if x.Month < 1 || x.Month > 12 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "month must be 1..12"})
		return
	}

	res, err := intconfig.DB.Exec(`
		UPDATE company_expenses_monthly
		SET staff_fee=?, office_fee=?, internet_fee=?, promo_fee=?, flyer_fee=?, legal_fee=?
		WHERE year=? AND month=?
	`, x.StaffFee, x.OfficeFee, x.InternetFee, x.PromoFee, x.FlyerFee, x.LegalFee, x.Year, x.Month)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	aff, _ := res.RowsAffected()
	if aff == 0 {
		if _, err := intconfig.DB.Exec(`
			INSERT INTO company_expenses_monthly
			  (year, month, staff_fee, office_fee, internet_fee, promo_fee, flyer_fee, legal_fee)
			VALUES (?,?,?,?,?,?,?,?)
		`, x.Year, x.Month, x.StaffFee, x.OfficeFee, x.InternetFee, x.PromoFee, x.FlyerFee, x.LegalFee); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func DeleteVehicleCost(c *gin.Context) {
	id64, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id64 <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}
	if _, err := intconfig.DB.Exec(`DELETE FROM vehicle_costs_monthly WHERE id=?`, id64); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func DeleteCompanyExpense(c *gin.Context) {
	id64, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id64 <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}
	if _, err := intconfig.DB.Exec(`DELETE FROM company_expenses_monthly WHERE id=?`, id64); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
