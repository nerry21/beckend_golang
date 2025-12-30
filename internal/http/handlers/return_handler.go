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
	GetReturnSettings    = legacy.GetReturnSettings
	GetReturnSettingByID = legacy.GetReturnSettingByID
	CreateReturnSetting  = legacy.CreateReturnSetting
	DeleteReturnSetting  = legacy.DeleteReturnSetting
)

// UpdateReturnSetting routes to ReturnService with key presence handling.
func UpdateReturnSetting(c *gin.Context) {
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

	svc := services.ReturnService{
		Repo:        repositories.ReturnRepository{},
		BookingRepo: repositories.BookingRepository{},
		SeatRepo:    repositories.BookingSeatRepository{},
		RequestID:   middleware.GetRequestID(c),
	}
	ret, err := svc.MarkPulang(id, raw)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "gagal memperbarui kepulangan", err)
		return
	}

	c.JSON(http.StatusOK, ret)
}
