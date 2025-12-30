package handlers

import (
	"io"
	"net/http"
	"strconv"

	legacy "backend/handlers"
	"backend/internal/http/middleware"
	"backend/internal/repositories"
	"backend/internal/services"

	"github.com/gin-gonic/gin"
)

var (
	GetDepartureSettings    = legacy.GetDepartureSettings
	GetDepartureSettingByID = legacy.GetDepartureSettingByID
	CreateDepartureSetting  = legacy.CreateDepartureSetting
	DeleteDepartureSetting  = legacy.DeleteDepartureSetting
)

// UpdateDepartureSetting routes to DepartureService with key presence handling and sync.
func UpdateDepartureSetting(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil || id <= 0 {
		RespondError(c, http.StatusBadRequest, "id tidak valid", err)
		return
	}

	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		RespondError(c, http.StatusBadRequest, "gagal membaca payload", err)
		return
	}
	if len(raw) == 0 {
		RespondError(c, http.StatusBadRequest, "payload tidak boleh kosong", nil)
		return
	}

	svc := services.DepartureService{
		Repo:        repositories.DepartureRepository{DB: nil},
		BookingRepo: repositories.BookingRepository{},
		SeatRepo:    repositories.BookingSeatRepository{},
		RequestID:   middleware.GetRequestID(c),
	}
	dep, err := svc.MarkBerangkat(id, raw)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "gagal memperbarui keberangkatan", err)
		return
	}

	c.JSON(http.StatusOK, dep)
}
