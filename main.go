package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"backend/config"
	"backend/handlers"

	"github.com/gin-gonic/gin"
)

func main() {
	// ===== Gin Mode =====
	if mode := strings.TrimSpace(os.Getenv("GIN_MODE")); mode != "" {
		gin.SetMode(mode) // "release" recommended
	}

	// ===== DB Connection =====
	config.ConnectDB()
	defer config.CloseDB()

	// ===== Router =====
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// Hilangkan warning "You trusted all proxies"
	if err := r.SetTrustedProxies(nil); err != nil {
		log.Fatalf("Gagal set trusted proxies: %v", err)
	}

	// CORS middleware (✅ sudah support localhost:3001)
	r.Use(corsMiddleware())

	// handle preflight global
	r.OPTIONS("/*path", func(c *gin.Context) {
		c.AbortWithStatus(http.StatusNoContent)
	})

	// 404 selalu JSON
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":  "route tidak ditemukan",
			"path":   c.Request.URL.Path,
			"method": c.Request.Method,
		})
	})

	// ===== Routes =====
	api := r.Group("/api")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "backend golang berjalan"})
		})

		api.GET("/db-check", func(c *gin.Context) {
			var count int
			err := config.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "gagal query ke database: " + err.Error(),
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"message":     "koneksi database OK",
				"users_in_db": count,
			})
		})

		api.GET("/routes", handlers.GetRoutes)

		// ============================
		// REGULER BOOKING
		// ============================
		reguler := api.Group("/reguler")
		{
			reguler.GET("/stops", handlers.GetRegulerStops)
			reguler.GET("/seats", handlers.GetRegulerSeats)
			reguler.POST("/quote", handlers.GetRegulerQuote)
			reguler.POST("/bookings", handlers.CreateRegulerBooking)

			// Surat jalan tetap boleh tampil walaupun belum bayar
			reguler.GET("/bookings/:id/surat-jalan", handlers.GetRegulerSuratJalan)

			// ============================
			// (NEW) PAYMENT FLOW REGULER
			// ============================

			// FE cek status pembayaran + method
			reguler.GET("/bookings/:id", handlers.GetRegulerBookingDetail)

			// Transfer/QRIS: upload bukti -> masuk validasi pembayaran (Menunggu Validasi)
			reguler.POST("/bookings/:id/submit-payment", handlers.SubmitRegulerPaymentProof)

			// Cash: langsung Lunas + auto-sync modul (Trip info, passengers, departure settings, trips)
			reguler.POST("/bookings/:id/confirm-cash", handlers.ConfirmRegulerCash)
		}

		// ============================
		// TRIP INFORMATION
		// ============================
		api.GET("/trip-information", handlers.GetTripInformation)
		api.POST("/trip-information", handlers.CreateTripInformation)
		api.PUT("/trip-information/:id", handlers.UpdateTripInformation)
		api.DELETE("/trip-information/:id", handlers.DeleteTripInformation)

		// ✅ FIX: endpoint preview surat jalan untuk Trip Information
		api.GET("/trip-information/:id/surat-jalan", handlers.GetTripSuratJalan)

		// ============================
		// FINANCIAL TRIPS
		// ============================
		api.GET("/trips", handlers.GetTrips)
		api.POST("/trips", handlers.CreateTrip)
		api.PUT("/trips/:id", handlers.UpdateTrip)
		api.DELETE("/trips/:id", handlers.DeleteTrip)

		// ============================
		// REPORTS
		// ============================
		api.GET("/reports/vehicle", handlers.ReportVehicle)

		// ============================
		// COSTS
		// ============================
		costAPI := &handlers.API{DB: config.DB}
		api.GET("/vehicle-costs", gin.WrapF(costAPI.ListVehicleCosts))
		api.POST("/vehicle-costs", gin.WrapF(costAPI.UpsertVehicleCost))

		api.DELETE("/vehicle-costs/:id", func(c *gin.Context) {
			id64, err := strconv.ParseInt(c.Param("id"), 10, 64)
			if err != nil || id64 <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
				return
			}
			if _, err := config.DB.Exec(`DELETE FROM vehicle_costs_monthly WHERE id=?`, id64); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})

		api.GET("/company-expenses", gin.WrapF(costAPI.ListCompanyExpenses))
		api.POST("/company-expenses", gin.WrapF(costAPI.UpsertCompanyExpense))

		api.DELETE("/company-expenses/:id", func(c *gin.Context) {
			id64, err := strconv.ParseInt(c.Param("id"), 10, 64)
			if err != nil || id64 <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
				return
			}
			if _, err := config.DB.Exec(`DELETE FROM company_expenses_monthly WHERE id=?`, id64); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})

		// ============================
		// PASSENGERS
		// ============================
		api.GET("/passengers", handlers.GetPassengers)
		api.POST("/passengers", handlers.CreatePassenger)
		api.PUT("/passengers/:id", handlers.UpdatePassenger)
		api.DELETE("/passengers/:id", handlers.DeletePassenger)

		// DRIVERS
		api.GET("/drivers", handlers.GetDrivers)
		api.POST("/drivers", handlers.CreateDriver)
		api.PUT("/drivers/:id", handlers.UpdateDriver)
		api.DELETE("/drivers/:id", handlers.DeleteDriver)

		// ============================
		// VALIDASI PEMBAYARAN
		// ============================
		api.GET("/payment-validations", handlers.GetPaymentValidations)
		api.POST("/payment-validations", handlers.CreatePaymentValidation)
		api.PUT("/payment-validations/:id", handlers.UpdatePaymentValidation)
		api.DELETE("/payment-validations/:id", handlers.DeletePaymentValidation)

		// (NEW) Approve / Reject untuk flow transfer/qris
		api.PUT("/payment-validations/:id/approve", handlers.ApprovePaymentValidation)
		api.PUT("/payment-validations/:id/reject", handlers.RejectPaymentValidation)

		// ============================
		// PENGATURAN KEBERANGKATAN
		// ============================
		api.GET("/departure-settings", handlers.GetDepartureSettings)
		api.POST("/departure-settings", handlers.CreateDepartureSetting)
		api.PUT("/departure-settings/:id", handlers.UpdateDepartureSetting)
		api.DELETE("/departure-settings/:id", handlers.DeleteDepartureSetting)

		// ============================
		// AKUN DRIVER
		// ============================
		api.GET("/driver-accounts", handlers.GetDriverAccounts)
		api.POST("/driver-accounts", handlers.CreateDriverAccount)
		api.PUT("/driver-accounts/:id", handlers.UpdateDriverAccount)
		api.DELETE("/driver-accounts/:id", handlers.DeleteDriverAccount)

		// ============================
		// VEHICLES
		// ============================
		api.GET("/vehicles", handlers.GetVehicles)
		api.POST("/vehicles", handlers.CreateVehicle)
		api.PUT("/vehicles/:id", handlers.UpdateVehicle)
		api.DELETE("/vehicles/:id", handlers.DeleteVehicle)

		// ============================
		// AUTH
		// ============================
		auth := api.Group("/auth")
		{
			auth.POST("/login", handlers.Login)
			auth.POST("/register", handlers.Register)
		}

		// ============================
		// USERS
		// ============================
		users := api.Group("/users")
		{
			users.GET("", handlers.GetUsers)
			users.GET("/:id", handlers.GetUserByID)
			users.POST("", handlers.CreateUser)
			users.PUT("/:id", handlers.UpdateUser)
			users.DELETE("/:id", handlers.DeleteUser)
		}
	}

	// ===== HTTP Server =====
	addr := strings.TrimSpace(os.Getenv("APP_ADDR"))
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("Server berjalan di http://localhost%s\n", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Gagal menjalankan server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Println("Mematikan server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Shutdown server gagal: %v", err)
	}

	log.Println("Server berhenti dengan aman.")
}

// ===== CORS Middleware =====
func corsMiddleware() gin.HandlerFunc {
	allowedOrigins := map[string]bool{
		"http://localhost:3000": true,
		"http://127.0.0.1:3000": true,
		"http://localhost:3001": true,
		"http://127.0.0.1:3001": true,
		"http://localhost:5173": true,
		"http://127.0.0.1:5173": true,
	}

	// override dari ENV kalau ada
	if env := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS")); env != "" {
		allowedOrigins = map[string]bool{}
		for _, o := range strings.Split(env, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				allowedOrigins[o] = true
			}
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// ✅ selalu set jika origin masuk allowlist
		if origin != "" && allowedOrigins[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept, Origin")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
