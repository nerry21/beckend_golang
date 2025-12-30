package handlers

import (
	"net/http"
	"strings"

	"backend/internal/repositories"
	"backend/internal/services"

	"github.com/gin-gonic/gin"
)

// GetFinanceReport handles finance report per trip_role with optional date range.
func GetFinanceReport(c *gin.Context) {
	role := strings.ToLower(strings.TrimSpace(c.DefaultQuery("role", "berangkat")))
	start := strings.TrimSpace(c.Query("start_date"))
	end := strings.TrimSpace(c.Query("end_date"))

	svc := services.ReportsService{
		TripsRepo: repositories.TripsRepository{},
	}
	report, err := svc.GetFinanceReport(services.FinanceReportFilter{
		TripRole:  role,
		StartDate: start,
		EndDate:   end,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}
