package models

type Vehicle struct {
	ID          uint64 `json:"id"`
	VehicleCode string `json:"vehicleCode"`
	PlateNumber string `json:"plateNumber"`
	Color       string `json:"color,omitempty"`
	Kilometers  *int   `json:"kilometers,omitempty"`  // nullable
	LastService string `json:"lastService,omitempty"` // format: YYYY-MM-DD (atau "" jika null)
}

type VehiclePayload struct {
	VehicleCode string `json:"vehicleCode" binding:"required"`
	PlateNumber string `json:"plateNumber" binding:"required"`
	Color       string `json:"color"`
	Kilometers  *int   `json:"kilometers"`  // boleh null
	LastService string `json:"lastService"` // boleh kosong => NULL di DB
}
