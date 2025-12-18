package domain

type Trip struct {
	ID    int64 `json:"id"`
	Day   int   `json:"day"`
	Month int   `json:"month"` // 0-11
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

	OtherIncome int64 `json:"otherIncome"`

	BBMFee      int64 `json:"bbmFee"`
	MealFee     int64 `json:"mealFee"`
	CourierFee  int64 `json:"courierFee"`
	TolParkirFee int64 `json:"tolParkirFee"`

	PaymentStatus string `json:"paymentStatus"`

	DeptAdminPercentOverride *float64 `json:"deptAdminPercentOverride"`
	RetAdminPercentOverride  *float64 `json:"retAdminPercentOverride"`
}

type TripWithCalc struct {
	Trip
	Calc CalcDetail `json:"calc"`
}

type VehicleCostMonthly struct {
	CarCode string `json:"carCode"`
	DriverName string `json:"driverName"`
	Year   int `json:"year"`
	Month  int `json:"month"`

	MaintenanceFee int64 `json:"maintenanceFee"`
	InsuranceFee   int64 `json:"insuranceFee"`
	InstallmentFee int64 `json:"installmentFee"`
}

type CompanyExpenseMonthly struct {
	Year  int `json:"year"`
	Month int `json:"month"`

	StaffFee   int64 `json:"staffFee"`
	OfficeFee  int64 `json:"officeFee"`
	InternetFee int64 `json:"internetFee"`
	PromoFee   int64 `json:"promoFee"`
	FlyerFee   int64 `json:"flyerFee"`
	LegalFee   int64 `json:"legalFee"`
}
