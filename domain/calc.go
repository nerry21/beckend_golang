package domain

import "math"

const Threshold int64 = 430000

type CalcDetail struct {
	DeptTotal int64 `json:"deptTotal"`
	RetTotal  int64 `json:"retTotal"`

	DeptAdminPercent float64 `json:"deptAdminPercent"`
	RetAdminPercent  float64 `json:"retAdminPercent"`
	DeptAdmin        int64   `json:"deptAdmin"`
	RetAdmin         int64   `json:"retAdmin"`

	Pool        int64 `json:"pool"`
	Residual    int64 `json:"residual"`
	FeeSopir    int64 `json:"feeSopir"`
	ProfitNetto int64 `json:"profitNetto"`

	TotalNominal int64 `json:"totalNominal"` // dept+ret+other
}

func adminPercent(category string, override *float64, passenger, pkg int64) float64 {
	if override != nil {
		return *override
	}

	switch category {
	case "Reguler":
		if passenger > Threshold || pkg > Threshold || (passenger+pkg) > Threshold {
			return 0.15
		}
		return 0.0
	case "Dropping", "Rental":
		// default 10% (silakan ubah kalau mau tetap pakai threshold)
		return 0.10
	default:
		return 0.0
	}
}

func roundMoney(x float64) int64 {
	return int64(math.Round(x))
}

func ComputeTrip(
	deptCategory string, deptPassengerFare, deptPackageFare int64, deptOverride *float64,
	retCategory string, retPassengerFare, retPackageFare int64, retOverride *float64,
	otherIncome, bbm, meal, courier, tolParkir int64,
) CalcDetail {
	deptTotal := deptPassengerFare + deptPackageFare
	retTotal := retPassengerFare + retPackageFare

	deptPct := adminPercent(deptCategory, deptOverride, deptPassengerFare, deptPackageFare)
	retPct := adminPercent(retCategory, retOverride, retPassengerFare, retPackageFare)

	deptAdmin := roundMoney(float64(deptTotal) * deptPct)
	retAdmin := roundMoney(float64(retTotal) * retPct)

	pool := (deptTotal - deptAdmin) + (retTotal - retAdmin) + otherIncome
	residual := pool - bbm - meal - courier - tolParkir
	if residual < 0 {
		residual = 0
	}

	fee := roundMoney(float64(residual) / 3.0)
	profit := fee * 2

	return CalcDetail{
		DeptTotal: deptTotal,
		RetTotal:  retTotal,

		DeptAdminPercent: deptPct,
		RetAdminPercent:  retPct,
		DeptAdmin:        deptAdmin,
		RetAdmin:         retAdmin,

		Pool:        pool,
		Residual:    residual,
		FeeSopir:    fee,
		ProfitNetto: profit,

		TotalNominal: deptTotal + retTotal + otherIncome,
	}
}
