package handlers

import (
	"database/sql"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	intconfig "backend/internal/config"

	"github.com/gin-gonic/gin"
)

// =======================
// DTO
// =======================

type TripDTO struct {
	ID    int64 `json:"id"`
	Day   int   `json:"day"`
	Month int   `json:"month"`
	Year  int   `json:"year"`

	CarCode     string `json:"carCode"`
	VehicleName string `json:"vehicleName"`
	DriverName  string `json:"driverName"`
	OrderNo     string `json:"orderNo"`

	DeptOrigin         string `json:"deptOrigin"`
	DeptDest           string `json:"deptDest"`
	DeptCategory       string `json:"deptCategory"`
	DeptPassengerCount int    `json:"deptPassengerCount"`
	DeptPassengerFare  int64  `json:"deptPassengerFare"`
	DeptPackageCount   int    `json:"deptPackageCount"`
	DeptPackageFare    int64  `json:"deptPackageFare"`

	RetOrigin         string `json:"retOrigin"`
	RetDest           string `json:"retDest"`
	RetCategory       string `json:"retCategory"`
	RetPassengerCount int    `json:"retPassengerCount"`
	RetPassengerFare  int64  `json:"retPassengerFare"`
	RetPackageCount   int    `json:"retPackageCount"`
	RetPackageFare    int64  `json:"retPackageFare"`

	OtherIncome   int64  `json:"otherIncome"`
	BBMFee        int64  `json:"bbmFee"`
	MealFee       int64  `json:"mealFee"`
	CourierFee    int64  `json:"courierFee"`
	TolParkirFee  int64  `json:"tolParkirFee"`
	PaymentStatus string `json:"paymentStatus"`

	DeptAdminPercentOverride *float64 `json:"deptAdminPercentOverride,omitempty"`
	RetAdminPercentOverride  *float64 `json:"retAdminPercentOverride,omitempty"`
}

type TripCalcDTO struct {
	DeptTotal int64 `json:"deptTotal"`
	RetTotal  int64 `json:"retTotal"`

	DeptAdminPercent float64 `json:"deptAdminPercent"`
	RetAdminPercent  float64 `json:"retAdminPercent"`

	DeptAdmin int64 `json:"deptAdmin"`
	RetAdmin  int64 `json:"retAdmin"`

	TotalNominalTrip int64 `json:"totalNominalTrip"`
	TotalNominal     int64 `json:"totalNominal"`
	TotalAdmin       int64 `json:"totalAdmin"`

	ResidualX   int64 `json:"residualX"`
	FeeSopir    int64 `json:"feeSopir"`
	ProfitNetto int64 `json:"profitNetto"`
}

type TripWithCalcDTO struct {
	Trip TripDTO     `json:"trip"`
	Calc TripCalcDTO `json:"calc"`
}

// =======================
// helpers
// =======================

func normalizeMonthToDB(m int) int {
	if m >= 0 && m <= 11 {
		return m + 1
	}
	if m >= 1 && m <= 12 {
		return m
	}
	return 1
}

func nullFloatPtr(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

func nullFloatValue(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

// =======================
// ROUTES
// =======================

// GET /api/trips
func GetTrips(c *gin.Context) {
	rows, err := intconfig.DB.Query(`
		SELECT id, day, month, year,
		       car_code, vehicle_name, driver_name, order_no,
		       dept_origin, dept_dest, dept_category, dept_passenger_count, dept_passenger_fare, dept_package_count, dept_package_fare,
		       ret_origin, ret_dest, ret_category, ret_passenger_count, ret_passenger_fare, ret_package_count, ret_package_fare,
		       COALESCE(other_income,0), COALESCE(bbm_fee,0), COALESCE(meal_fee,0), COALESCE(courier_fee,0), COALESCE(tol_parkir_fee,0),
		       COALESCE(payment_status,'Belum Lunas'),
		       dept_admin_percent_override, ret_admin_percent_override
		FROM trips
		ORDER BY year DESC, month DESC, day DESC, id DESC
	`)
	if err != nil {
		log.Println("GetTrips query error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var out []TripWithCalcDTO
	for rows.Next() {
		var t TripDTO
		var deptOv sql.NullFloat64
		var retOv sql.NullFloat64

		if err := rows.Scan(
			&t.ID, &t.Day, &t.Month, &t.Year,
			&t.CarCode, &t.VehicleName, &t.DriverName, &t.OrderNo,
			&t.DeptOrigin, &t.DeptDest, &t.DeptCategory, &t.DeptPassengerCount, &t.DeptPassengerFare, &t.DeptPackageCount, &t.DeptPackageFare,
			&t.RetOrigin, &t.RetDest, &t.RetCategory, &t.RetPassengerCount, &t.RetPassengerFare, &t.RetPackageCount, &t.RetPackageFare,
			&t.OtherIncome, &t.BBMFee, &t.MealFee, &t.CourierFee, &t.TolParkirFee,
			&t.PaymentStatus,
			&deptOv, &retOv,
		); err != nil {
			log.Println("GetTrips scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		t.CarCode = strings.TrimSpace(t.CarCode)
		t.OrderNo = strings.TrimSpace(t.OrderNo)
		t.DeptAdminPercentOverride = nullFloatPtr(deptOv)
		t.RetAdminPercentOverride = nullFloatPtr(retOv)

		calc := ComputeTripDTO(
			t.DeptCategory, t.DeptPassengerFare, t.DeptPackageFare, t.DeptAdminPercentOverride,
			t.RetCategory, t.RetPassengerFare, t.RetPackageFare, t.RetAdminPercentOverride,
			t.OtherIncome, t.BBMFee, t.MealFee, t.CourierFee, t.TolParkirFee,
		)

		out = append(out, TripWithCalcDTO{Trip: t, Calc: calc})
	}

	if err := rows.Err(); err != nil {
		log.Println("GetTrips rows err:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, out)
}

// POST /api/trips
func CreateTrip(c *gin.Context) {
	var t TripDTO
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
		return
	}

	t.OrderNo = strings.TrimSpace(t.OrderNo)
	t.CarCode = strings.TrimSpace(t.CarCode)
	if t.OrderNo == "" || t.CarCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "orderNo and carCode required"})
		return
	}

	if strings.TrimSpace(t.PaymentStatus) == "" {
		t.PaymentStatus = "Belum Lunas"
	}

	t.Month = normalizeMonthToDB(t.Month)

	res, err := intconfig.DB.Exec(`
		INSERT INTO trips (
		  day, month, year, car_code, vehicle_name, driver_name, order_no,
		  dept_origin, dept_dest, dept_category, dept_passenger_count, dept_passenger_fare, dept_package_count, dept_package_fare,
		  ret_origin, ret_dest, ret_category, ret_passenger_count, ret_passenger_fare, ret_package_count, ret_package_fare,
		  other_income, bbm_fee, meal_fee, courier_fee, tol_parkir_fee, payment_status,
		  dept_admin_percent_override, ret_admin_percent_override
		) VALUES (
		  ?, ?, ?, ?, ?, ?, ?,
		  ?, ?, ?, ?, ?, ?, ?,
		  ?, ?, ?, ?, ?, ?, ?,
		  ?, ?, ?, ?, ?, ?,
		  ?, ?
		)
	`,
		t.Day, t.Month, t.Year, t.CarCode, t.VehicleName, t.DriverName, t.OrderNo,
		t.DeptOrigin, t.DeptDest, t.DeptCategory, t.DeptPassengerCount, t.DeptPassengerFare, t.DeptPackageCount, t.DeptPackageFare,
		t.RetOrigin, t.RetDest, t.RetCategory, t.RetPassengerCount, t.RetPassengerFare, t.RetPackageCount, t.RetPackageFare,
		t.OtherIncome, t.BBMFee, t.MealFee, t.CourierFee, t.TolParkirFee, t.PaymentStatus,
		nullFloatValue(t.DeptAdminPercentOverride), nullFloatValue(t.RetAdminPercentOverride),
	)
	if err != nil {
		log.Println("CreateTrip insert error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	id, _ := res.LastInsertId()
	t.ID = id

	calc := ComputeTripDTO(
		t.DeptCategory, t.DeptPassengerFare, t.DeptPackageFare, t.DeptAdminPercentOverride,
		t.RetCategory, t.RetPassengerFare, t.RetPackageFare, t.RetAdminPercentOverride,
		t.OtherIncome, t.BBMFee, t.MealFee, t.CourierFee, t.TolParkirFee,
	)

	c.JSON(http.StatusCreated, TripWithCalcDTO{Trip: t, Calc: calc})
}

// PUT /api/trips/:id
func UpdateTrip(c *gin.Context) {
	idStr := c.Param("id")
	id64, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id64 <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	var t TripDTO
	if err := c.ShouldBindJSON(&t); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json: " + err.Error()})
		return
	}

	t.OrderNo = strings.TrimSpace(t.OrderNo)
	t.CarCode = strings.TrimSpace(t.CarCode)
	if t.OrderNo == "" || t.CarCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "orderNo and carCode required"})
		return
	}

	if strings.TrimSpace(t.PaymentStatus) == "" {
		t.PaymentStatus = "Belum Lunas"
	}

	t.Month = normalizeMonthToDB(t.Month)

	if _, err = intconfig.DB.Exec(`
		UPDATE trips SET
		  day=?, month=?, year=?, car_code=?, vehicle_name=?, driver_name=?, order_no=?,
		  dept_origin=?, dept_dest=?, dept_category=?, dept_passenger_count=?, dept_passenger_fare=?, dept_package_count=?, dept_package_fare=?,
		  ret_origin=?, ret_dest=?, ret_category=?, ret_passenger_count=?, ret_passenger_fare=?, ret_package_count=?, ret_package_fare=?,
		  other_income=?, bbm_fee=?, meal_fee=?, courier_fee=?, tol_parkir_fee=?, payment_status=?,
		  dept_admin_percent_override=?, ret_admin_percent_override=?
		WHERE id=?
	`,
		t.Day, t.Month, t.Year, t.CarCode, t.VehicleName, t.DriverName, t.OrderNo,
		t.DeptOrigin, t.DeptDest, t.DeptCategory, t.DeptPassengerCount, t.DeptPassengerFare, t.DeptPackageCount, t.DeptPackageFare,
		t.RetOrigin, t.RetDest, t.RetCategory, t.RetPassengerCount, t.RetPassengerFare, t.RetPackageCount, t.RetPackageFare,
		t.OtherIncome, t.BBMFee, t.MealFee, t.CourierFee, t.TolParkirFee, t.PaymentStatus,
		nullFloatValue(t.DeptAdminPercentOverride), nullFloatValue(t.RetAdminPercentOverride),
		id64,
	); err != nil {
		log.Println("UpdateTrip update error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	t.ID = id64
	calc := ComputeTripDTO(
		t.DeptCategory, t.DeptPassengerFare, t.DeptPackageFare, t.DeptAdminPercentOverride,
		t.RetCategory, t.RetPassengerFare, t.RetPackageFare, t.RetAdminPercentOverride,
		t.OtherIncome, t.BBMFee, t.MealFee, t.CourierFee, t.TolParkirFee,
	)

	c.JSON(http.StatusOK, TripWithCalcDTO{Trip: t, Calc: calc})
}

// DELETE /api/trips/:id
func DeleteTrip(c *gin.Context) {
	idStr := c.Param("id")
	id64, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id64 <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	if _, err := intconfig.DB.Exec(`DELETE FROM trips WHERE id=?`, id64); err != nil {
		log.Println("DeleteTrip delete error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// =======================
// CALC
// =======================

func ComputeTripDTO(
	deptCategory string, deptPassengerFare, deptPackageFare int64, deptOverride *float64,
	retCategory string, retPassengerFare, retPackageFare int64, retOverride *float64,
	otherIncome, bbmFee, mealFee, courierFee, tolParkirFee int64,
) TripCalcDTO {
	deptTotal := deptPassengerFare + deptPackageFare
	retTotal := retPassengerFare + retPackageFare

	deptAdmin, deptPct := calcAdminDTO(deptCategory, deptPassengerFare, deptPackageFare, deptOverride)
	retAdmin, retPct := calcAdminDTO(retCategory, retPassengerFare, retPackageFare, retOverride)

	deptNet := deptTotal - deptAdmin
	retNet := retTotal - retAdmin

	totalNominalTrip := deptTotal + retTotal
	totalNominal := totalNominalTrip + otherIncome
	totalAdmin := deptAdmin + retAdmin

	netPool := deptNet + retNet + otherIncome

	residualX := netPool - bbmFee - mealFee - courierFee - tolParkirFee
	if residualX < 0 {
		residualX = 0
	}

	feeSopir := int64(math.Round(float64(residualX) / 3.0))
	profitNetto := int64(math.Round((float64(residualX) / 3.0) * 2.0))

	return TripCalcDTO{
		DeptTotal:        deptTotal,
		RetTotal:         retTotal,
		DeptAdminPercent: deptPct,
		RetAdminPercent:  retPct,
		DeptAdmin:        deptAdmin,
		RetAdmin:         retAdmin,
		TotalNominalTrip: totalNominalTrip,
		TotalNominal:     totalNominal,
		TotalAdmin:       totalAdmin,
		ResidualX:        residualX,
		FeeSopir:         feeSopir,
		ProfitNetto:      profitNetto,
	}
}

func calcAdminDTO(category string, passengerFare, packageFare int64, override *float64) (admin int64, pct float64) {
	total := passengerFare + packageFare

	if override != nil {
		pct = *override
		if pct <= 0 {
			return 0, 0
		}
		return int64(math.Round(float64(total) * pct / 100.0)), pct
	}

	cat := strings.ToLower(strings.TrimSpace(category))
	threshold := int64(430000)

	if cat == "reguler" {
		if passengerFare > threshold || packageFare > threshold || total > threshold {
			pct = 15
			return int64(math.Round(float64(total) * 15.0 / 100.0)), pct
		}
		return 0, 0
	}

	pct = 10
	return int64(math.Round(float64(total) * 10.0 / 100.0)), pct
}
