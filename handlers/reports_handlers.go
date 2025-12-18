// backend/handlers/reports_handlers.go
package handlers

import (
	"backend/config"
	"database/sql"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type VehicleMonthReport struct {
	Year  int `json:"year"`
	Month int `json:"month"` // UI 0..11 (Jan=0)

	CarCode    string `json:"carCode"`
	DriverName string `json:"driverName"`

	// Pendapatan mobil
	PendapatanTripKotor int64 `json:"pendapatanTripKotor"`
	PendapatanLain      int64 `json:"pendapatanLain"`
	TotalPendapatan     int64 `json:"totalPendapatan"`

	// Pengeluaran mobil
	AdminDept int64 `json:"adminDept"`
	AdminRet  int64 `json:"adminRet"`
	BBM       int64 `json:"bbm"`
	Makan     int64 `json:"makan"`
	Kurir     int64 `json:"kurir"`
	TolParkir int64 `json:"tolParkir"`
	FeeSopir  int64 `json:"feeSopir"`

	// biaya bulanan (dari vehicle_costs_monthly)
	Maintenance int64 `json:"maintenance"`
	Insurance   int64 `json:"insurance"`
	Installment int64 `json:"installment"`

	TotalPengeluaran int64 `json:"totalPengeluaran"`
	NettoMobil       int64 `json:"nettoMobil"`
}

type VehicleYearReport struct {
	CarCode string               `json:"carCode"`
	Year    int                  `json:"year"`
	Months  []VehicleMonthReport `json:"months"`

	TotalPendapatan  int64 `json:"totalPendapatan"`
	TotalPengeluaran int64 `json:"totalPengeluaran"`
	NettoMobil       int64 `json:"nettoMobil"`
}

func monthToIndex(dbMonth int) (int, bool) {
	// DB umumnya 1..12
	if dbMonth >= 1 && dbMonth <= 12 {
		return dbMonth - 1, true
	}
	// kalau ternyata sudah 0..11
	if dbMonth >= 0 && dbMonth <= 11 {
		return dbMonth, true
	}
	return 0, false
}

func nullFloatPtrFromNull(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

// GET /api/reports/vehicle?carCode=LK01&year=2025
func ReportVehicle(c *gin.Context) {
	car := strings.TrimSpace(c.Query("carCode"))
	yearStr := strings.TrimSpace(c.Query("year"))

	if car == "" || yearStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "carCode and year required"})
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 2000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "year invalid"})
		return
	}

	// init 12 bulan
	months := make([]VehicleMonthReport, 12)
	for m := 0; m < 12; m++ {
		months[m] = VehicleMonthReport{
			CarCode: car,
			Year:    year,
			Month:   m, // UI 0..11
		}
	}

	// 1) ambil trips untuk car+year
	rows, qerr := config.DB.Query(`
		SELECT
			month,
			dept_category, dept_passenger_fare, dept_package_fare, dept_admin_percent_override,
			ret_category,  ret_passenger_fare,  ret_package_fare,  ret_admin_percent_override,
			COALESCE(other_income, 0),
			COALESCE(bbm_fee, 0),
			COALESCE(meal_fee, 0),
			COALESCE(courier_fee, 0),
			COALESCE(tol_parkir_fee, 0),
			COALESCE(driver_name, '')
		FROM trips
		WHERE car_code = ? AND year = ?
		ORDER BY month ASC
	`, car, year)
	if qerr != nil {
		log.Println("ReportVehicle trips query error:", qerr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": qerr.Error()})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var monthDB int

		var deptCat, retCat string
		var dp, dpk, rp, rpk int64

		var deptOv sql.NullFloat64
		var retOv sql.NullFloat64

		var other, bbm, meal, kurir, tol int64
		var driverName string

		if err := rows.Scan(
			&monthDB,
			&deptCat, &dp, &dpk, &deptOv,
			&retCat, &rp, &rpk, &retOv,
			&other, &bbm, &meal, &kurir, &tol,
			&driverName,
		); err != nil {
			log.Println("ReportVehicle trips scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		idx, ok := monthToIndex(monthDB)
		if !ok {
			continue
		}

		deptPtr := nullFloatPtrFromNull(deptOv)
		retPtr := nullFloatPtrFromNull(retOv)

		calc := ComputeTripFinancials(
			deptCat, dp, dpk, deptPtr,
			retCat, rp, rpk, retPtr,
			other, bbm, meal, kurir, tol,
		)

		if months[idx].DriverName == "" && driverName != "" {
			months[idx].DriverName = driverName
		}

		months[idx].PendapatanTripKotor += calc.DeptTotal + calc.RetTotal
		months[idx].PendapatanLain += other

		months[idx].AdminDept += calc.DeptAdmin
		months[idx].AdminRet += calc.RetAdmin

		months[idx].BBM += bbm
		months[idx].Makan += meal
		months[idx].Kurir += kurir
		months[idx].TolParkir += tol

		months[idx].FeeSopir += calc.FeeSopir
	}

	if err := rows.Err(); err != nil {
		log.Println("ReportVehicle trips rows error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 2) ambil biaya bulanan mobil (maintenance/insurance/installment)
	crows, qerr2 := config.DB.Query(`
		SELECT
			month,
			COALESCE(driver_name, ''),
			COALESCE(maintenance_fee, 0),
			COALESCE(insurance_fee, 0),
			COALESCE(installment_fee, 0)
		FROM vehicle_costs_monthly
		WHERE car_code = ? AND year = ?
		ORDER BY month ASC
	`, car, year)
	if qerr2 != nil {
		log.Println("ReportVehicle costs query error:", qerr2)
		c.JSON(http.StatusInternalServerError, gin.H{"error": qerr2.Error()})
		return
	}
	defer crows.Close()

	for crows.Next() {
		var monthDB int
		var driver string
		var maint, ins, inst int64

		if err := crows.Scan(&monthDB, &driver, &maint, &ins, &inst); err != nil {
			log.Println("ReportVehicle costs scan error:", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		idx, ok := monthToIndex(monthDB)
		if !ok {
			continue
		}

		months[idx].Maintenance += maint
		months[idx].Insurance += ins
		months[idx].Installment += inst

		if months[idx].DriverName == "" && driver != "" {
			months[idx].DriverName = driver
		}
	}

	if err := crows.Err(); err != nil {
		log.Println("ReportVehicle costs rows error:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 3) finalize totals per bulan
	var yearTotalIn, yearTotalOut int64
	for i := range months {
		months[i].TotalPendapatan = months[i].PendapatanTripKotor + months[i].PendapatanLain

		months[i].TotalPengeluaran =
			months[i].AdminDept + months[i].AdminRet +
				months[i].BBM + months[i].Makan + months[i].Kurir + months[i].TolParkir +
				months[i].FeeSopir +
				months[i].Maintenance + months[i].Insurance + months[i].Installment

		months[i].NettoMobil = months[i].TotalPendapatan - months[i].TotalPengeluaran

		yearTotalIn += months[i].TotalPendapatan
		yearTotalOut += months[i].TotalPengeluaran
	}

	out := VehicleYearReport{
		CarCode:          car,
		Year:             year,
		Months:           months,
		TotalPendapatan:  yearTotalIn,
		TotalPengeluaran: yearTotalOut,
		NettoMobil:       yearTotalIn - yearTotalOut,
	}

	c.JSON(http.StatusOK, out)
}

// hasil compute per trip
type TripCalc struct {
	DeptTotal int64
	RetTotal  int64

	DeptAdmin int64
	RetAdmin  int64

	FeeSopir int64
}

func ComputeTripFinancials(
	deptCategory string, deptPassengerFare, deptPackageFare int64, deptOverride *float64,
	retCategory string, retPassengerFare, retPackageFare int64, retOverride *float64,
	otherIncome, bbmFee, mealFee, courierFee, tolParkirFee int64,
) TripCalc {
	deptTotal := deptPassengerFare + deptPackageFare
	retTotal := retPassengerFare + retPackageFare

	deptAdmin := calcAdminByRules(deptCategory, deptPassengerFare, deptPackageFare, deptOverride)
	retAdmin := calcAdminByRules(retCategory, retPassengerFare, retPackageFare, retOverride)

	deptNet := deptTotal - deptAdmin
	retNet := retTotal - retAdmin

	netPool := deptNet + retNet + otherIncome

	residual := netPool - bbmFee - mealFee - courierFee - tolParkirFee
	if residual < 0 {
		residual = 0
	}

	feeSopir := int64(math.Round(float64(residual) / 3.0))

	return TripCalc{
		DeptTotal: deptTotal,
		RetTotal:  retTotal,
		DeptAdmin: deptAdmin,
		RetAdmin:  retAdmin,
		FeeSopir:  feeSopir,
	}
}

func calcAdminByRules(category string, passengerFare, packageFare int64, override *float64) int64 {
	total := passengerFare + packageFare

	// override ada => pakai override
	if override != nil {
		pct := *override
		if pct <= 0 {
			return 0
		}
		return int64(math.Round(float64(total) * pct / 100.0))
	}

	cat := strings.ToLower(strings.TrimSpace(category))

	// REGULER: threshold-based 15%
	if cat == "reguler" {
		threshold := int64(430000)
		if passengerFare > threshold || packageFare > threshold || total > threshold {
			return int64(math.Round(float64(total) * 15.0 / 100.0))
		}
		return 0
	}

	// DROPPING / RENTAL (dan kategori lain): default 10%
	return int64(math.Round(float64(total) * 10.0 / 100.0))
}
