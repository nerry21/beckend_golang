package services

import "backend/internal/repositories"

type FinanceReportFilter struct {
	TripRole  string
	StartDate string
	EndDate   string
}

type ReportsService struct {
	TripsRepo repositories.TripsRepository
}

// GetFinanceReport returns trips filtered by trip role and optional date range.
func (s ReportsService) GetFinanceReport(f FinanceReportFilter) ([]repositories.TripFinance, error) {
	role := f.TripRole
	if role == "" {
		role = "berangkat"
	}
	return s.TripsRepo.ListFinanceTrips(role, f.StartDate, f.EndDate)
}
