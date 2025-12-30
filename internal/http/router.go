package api

import (
	"log"
	stdhttp "net/http"

	intconfig "backend/internal/config"
	h "backend/internal/http/handlers"
	"backend/internal/http/middleware"

	"github.com/gin-gonic/gin"
)

func NewRouter(env intconfig.Env) *gin.Engine {
	_ = env

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Logger(), gin.Recovery(), middleware.CORS())

	if err := r.SetTrustedProxies(nil); err != nil {
		log.Printf("warning: failed to set trusted proxies: %v", err)
	}

	r.OPTIONS("/*path", func(c *gin.Context) { c.AbortWithStatus(stdhttp.StatusNoContent) })

	r.NoRoute(func(c *gin.Context) {
		c.JSON(stdhttp.StatusNotFound, gin.H{
			"error":  "route tidak ditemukan",
			"path":   c.Request.URL.Path,
			"method": c.Request.Method,
		})
	})

	api := r.Group("/api")
	{
		api.GET("/health", h.Health)
		api.GET("/db-check", h.DBCheck)
		api.GET("/routes", h.Routes)

		// Bookings common
		bookings := api.Group("/bookings")
		bookings.POST("/:id/passengers", h.SaveBookingPassengers)
		bookings.GET("/:id/passengers", h.GetBookingPassengers)

		// Auth
		auth := api.Group("/auth")
		auth.POST("/login", h.Login)
		auth.POST("/register", h.Register)

		// Users
		users := api.Group("/users")
		users.GET("", h.GetUsers)
		users.GET("/:id", h.GetUserByID)
		users.POST("", h.CreateUser)
		users.PUT("/:id", h.UpdateUser)
		users.DELETE("/:id", h.DeleteUser)

		// Bookings (reguler)
		reguler := bookings.Group("/reguler")
		mountReguler(reguler)
		// legacy path
		legacyReguler := api.Group("/reguler")
		mountReguler(legacyReguler)

		// Payments
		payments := api.Group("/payments")
		mountPaymentValidations(payments)
		// legacy payment validations
		paymentValidations := api.Group("/payment-validations")
		mountPaymentValidations(paymentValidations)

		// Departures
		departures := api.Group("/departures")
		mountDepartureSettings(departures)
		legacyDepartures := api.Group("/departure-settings")
		legacyDepartures.GET("", h.GetDepartureSettings)
		legacyDepartures.GET("/:id", h.GetDepartureSettingByID)
		legacyDepartures.POST("", h.CreateDepartureSetting)
		legacyDepartures.PUT("/:id", h.UpdateDepartureSetting)
		legacyDepartures.DELETE("/:id", h.DeleteDepartureSetting)

		// Return settings
		returns := api.Group("/returns")
		returns.GET("/settings", h.GetReturnSettings)
		returns.GET("/settings/:id", h.GetReturnSettingByID)
		returns.POST("/settings", h.CreateReturnSetting)
		returns.PUT("/settings/:id", h.UpdateReturnSetting)
		returns.DELETE("/settings/:id", h.DeleteReturnSetting)
		legacyReturns := api.Group("/return-settings")
		legacyReturns.GET("", h.GetReturnSettings)
		legacyReturns.GET("/:id", h.GetReturnSettingByID)
		legacyReturns.POST("", h.CreateReturnSetting)
		legacyReturns.PUT("/:id", h.UpdateReturnSetting)
		legacyReturns.DELETE("/:id", h.DeleteReturnSetting)

		// Passengers
		passengers := api.Group("/passengers")
		passengers.GET("", h.GetPassengers)
		passengers.POST("", h.CreatePassenger)
		passengers.PUT("/:id", h.UpdatePassenger)
		passengers.DELETE("/:id", h.DeletePassenger)
		passengers.GET("/:id/e-ticket", h.GetPassengerETicketPDF)
		passengers.GET("/:id/invoice", h.GetPassengerInvoicePDF)

		// Trip Information
		tripInfo := api.Group("/trip-information")
		tripInfo.GET("", h.GetTripInformation)
		tripInfo.POST("", h.CreateTripInformation)
		tripInfo.PUT("/:id", h.UpdateTripInformation)
		tripInfo.DELETE("/:id", h.DeleteTripInformation)
		tripInfo.GET("/:id/surat-jalan", h.GetTripSuratJalan)

		// Trips (financial)
		trips := api.Group("/trips")
		trips.GET("", h.GetTrips)
		trips.POST("", h.CreateTrip)
		trips.PUT("/:id", h.UpdateTrip)
		trips.DELETE("/:id", h.DeleteTrip)

		// Reports
		reports := api.Group("/reports")
		reports.GET("/vehicle", h.ReportVehicle)
		reports.GET("/finance", h.GetFinanceReport)

		// Drivers & driver accounts
		drivers := api.Group("/drivers")
		drivers.GET("", h.GetDrivers)
		drivers.POST("", h.CreateDriver)
		drivers.PUT("/:id", h.UpdateDriver)
		drivers.DELETE("/:id", h.DeleteDriver)
		driverAccounts := api.Group("/driver-accounts")
		driverAccounts.GET("", h.GetDriverAccounts)
		driverAccounts.POST("", h.CreateDriverAccount)
		driverAccounts.PUT("/:id", h.UpdateDriverAccount)
		driverAccounts.DELETE("/:id", h.DeleteDriverAccount)

		// Vehicles
		vehicles := api.Group("/vehicles")
		vehicles.GET("", h.GetVehicles)
		vehicles.POST("", h.CreateVehicle)
		vehicles.PUT("/:id", h.UpdateVehicle)
		vehicles.DELETE("/:id", h.DeleteVehicle)

		// Costs & expenses
		api.GET("/vehicle-costs", h.ListVehicleCosts)
		api.POST("/vehicle-costs", h.UpsertVehicleCost)
		api.DELETE("/vehicle-costs/:id", h.DeleteVehicleCost)

		api.GET("/company-expenses", h.ListCompanyExpenses)
		api.POST("/company-expenses", h.UpsertCompanyExpense)
		api.DELETE("/company-expenses/:id", h.DeleteCompanyExpense)
	}

	return r
}

func mountReguler(g *gin.RouterGroup) {
	g.GET("/stops", h.GetRegulerStops)
	g.GET("/seats", h.GetRegulerSeats)
	g.POST("/quote", h.GetRegulerQuote)
	g.POST("/bookings", h.CreateRegulerBooking)
	g.GET("/bookings/:id/surat-jalan", h.GetRegulerSuratJalan)
	g.GET("/bookings/:id", h.GetRegulerBookingDetail)
	g.POST("/bookings/:id/submit-payment", h.SubmitRegulerPaymentProof)
	g.POST("/bookings/:id/confirm-cash", h.ConfirmRegulerCash)
}

func mountPaymentValidations(g *gin.RouterGroup) {
	g.GET("", h.GetPaymentValidations)
	g.POST("", h.CreatePaymentValidation)
	g.PUT("/:id", h.UpdatePaymentValidation)
	g.DELETE("/:id", h.DeletePaymentValidation)
	g.PUT("/:id/approve", h.ApprovePaymentValidation)
	g.PUT("/:id/reject", h.RejectPaymentValidation)
}

func mountDepartureSettings(g *gin.RouterGroup) {
	g.GET("/settings", h.GetDepartureSettings)
	g.GET("/settings/:id", h.GetDepartureSettingByID)
	g.POST("/settings", h.CreateDepartureSetting)
	g.PUT("/settings/:id", h.UpdateDepartureSetting)
	g.DELETE("/settings/:id", h.DeleteDepartureSetting)
}
