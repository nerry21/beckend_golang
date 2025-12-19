package handlers

import (
	"backend/config"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// GET /api/trip-information/:id/surat-jalan
// Mengembalikan:
// - JSON payload surat jalan (kalau tersimpan JSON string di DB)
// - atau { "__type":"image", "src":"data:image/..." } kalau tersimpan base64 image
func GetTripSuratJalan(c *gin.Context) {
	id64, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id64 <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id tidak valid"})
		return
	}

	// Pastikan tabel ada
	table := "trip_information"
	if !hasTable(config.DB, table) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tabel trip_information tidak ditemukan"})
		return
	}

	// Cari kolom yang menyimpan surat jalan (buat kompatibel berbagai skema)
	// Kamu bisa tambah nama kolom lain jika di DB-mu berbeda.
	candidates := []string{
		"e_surat_jalan",
		"eSuratJalan",
		"surat_jalan",
		"suratJalan",
		"e_surat_jalan_json",
		"surat_jalan_json",
	}

	col := ""
	for _, cc := range candidates {
		if hasColumn(config.DB, table, cc) {
			col = cc
			break
		}
	}
	if col == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "kolom surat jalan tidak ditemukan di trip_information"})
		return
	}

	var raw sql.NullString
	q := "SELECT COALESCE(" + col + ",'') FROM " + table + " WHERE id=? LIMIT 1"
	if err := config.DB.QueryRow(q, id64).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "trip tidak ditemukan"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s := strings.TrimSpace(raw.String)
	if s == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "surat jalan belum tersedia"})
		return
	}

	// Kalau base64 image
	if strings.HasPrefix(strings.ToLower(s), "data:image/") {
		c.JSON(http.StatusOK, gin.H{
			"__type": "image",
			"src":    s,
		})
		return
	}

	// ✅ (FIX) Kalau di DB disimpan URL/PATH surat jalan kalau booking sudah selesai
	// Contoh hasil sync dari booking_sync.go:
	// "/api/reguler/bookings/123/surat-jalan"
	// Maka endpoint ini akan redirect ke sana.
	// (redirect relative path aman, dan fetch/axios umumnya mengikuti redirect)
	{
		low := strings.ToLower(s)

		// normalize jika tersimpan tanpa "/" depan
		if strings.HasPrefix(s, "api/") {
			s = "/" + s
			low = "/" + low
		}

		if strings.HasPrefix(s, "/api/") || strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
			c.Redirect(http.StatusTemporaryRedirect, s)
			return
		}
	}

	// Kalau JSON string (yang kamu tunjukkan di screenshot “Pretty-print”)
	if json.Valid([]byte(s)) {
		var anyPayload any
		if err := json.Unmarshal([]byte(s), &anyPayload); err == nil {
			c.JSON(http.StatusOK, anyPayload)
			return
		}
	}

	// Fallback: kembalikan raw supaya frontend tetap bisa tampilkan teks jika perlu
	c.JSON(http.StatusOK, gin.H{
		"__type": "raw",
		"raw":    s,
	})
}
