package handlers

import (
	"net/http"
	"strconv"

	legacyconfig "backend/config"
	legacy "backend/handlers"

	"github.com/gin-gonic/gin"
)

func ListVehicleCosts(c *gin.Context) {
	api := legacy.API{DB: legacyconfig.DB}
	gin.WrapF(api.ListVehicleCosts)(c)
}

func UpsertVehicleCost(c *gin.Context) {
	api := legacy.API{DB: legacyconfig.DB}
	gin.WrapF(api.UpsertVehicleCost)(c)
}

func ListCompanyExpenses(c *gin.Context) {
	api := legacy.API{DB: legacyconfig.DB}
	gin.WrapF(api.ListCompanyExpenses)(c)
}

func UpsertCompanyExpense(c *gin.Context) {
	api := legacy.API{DB: legacyconfig.DB}
	gin.WrapF(api.UpsertCompanyExpense)(c)
}

func DeleteVehicleCost(c *gin.Context) {
	id64, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id64 <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}
	if _, err := legacyconfig.DB.Exec(`DELETE FROM vehicle_costs_monthly WHERE id=?`, id64); err != nil {
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
	if _, err := legacyconfig.DB.Exec(`DELETE FROM company_expenses_monthly WHERE id=?`, id64); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
