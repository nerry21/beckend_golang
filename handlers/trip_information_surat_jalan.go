package handlers

import (
	"backend/config"
	"database/sql"
	"encoding/base64"
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
//
// ✅ FIX tambahan:
// - Jika request datang dari <img src="..."> (Accept berisi image/*), maka server akan mengembalikan BYTES image
//   supaya preview tidak rusak di tabel Trip Information.
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

	accept := strings.ToLower(strings.TrimSpace(c.GetHeader("Accept")))

	// ============================
	// ✅ CASE 1: DB menyimpan data URL langsung
	// ============================
	if strings.HasPrefix(strings.ToLower(s), "data:") {
		if tryServeRawByAccept(c, accept, s) {
			return
		}

		// default JSON (kompatibilitas)
		if strings.HasPrefix(strings.ToLower(s), "data:image/") {
			c.JSON(http.StatusOK, gin.H{
				"__type": "image",
				"src":    s,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"__type": "file",
			"src":    s,
		})
		return
	}

	// ============================
	// ✅ CASE 2: DB menyimpan JSON string
	// ============================
	if json.Valid([]byte(s)) {
		var anyPayload any
		if err := json.Unmarshal([]byte(s), &anyPayload); err == nil {

			// ✅ kalau payload mengandung src data URL -> bisa dilayani sebagai raw image untuk preview
			if src := extractSrcFromAny(anyPayload); src != "" && strings.HasPrefix(strings.ToLower(src), "data:") {
				if tryServeRawByAccept(c, accept, src) {
					return
				}
			}

			c.JSON(http.StatusOK, anyPayload)
			return
		}
	}

	// ============================
	// ✅ CASE 3: kemungkinan base64 mentah tanpa prefix data:
	// (sering terjadi kalau frontend hanya simpan result FileReader tanpa "data:image/...;base64,")
	// ============================
	if looksLikeBase64(s) {
		mime := detectMimeFromBase64Prefix(s)
		if mime == "" {
			// fallback aman
			mime = "image/png"
		}
		dataURL := "data:" + mime + ";base64," + s

		// kalau request preview dari <img>, kirim bytes image
		if tryServeRawByAccept(c, accept, dataURL) {
			return
		}

		// default JSON (kompatibilitas)
		if strings.HasPrefix(mime, "image/") {
			c.JSON(http.StatusOK, gin.H{
				"__type": "image",
				"src":    dataURL,
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"__type": "file",
			"mime":   mime,
			"src":    dataURL,
		})
		return
	}

	// ============================
	// Fallback lama: kembalikan raw supaya frontend tetap bisa tampilkan teks jika perlu
	// ============================
	c.JSON(http.StatusOK, gin.H{
		"__type": "raw",
		"raw":    s,
	})
}

// ============================
// Helpers (✅ tambahan, tidak mengurangi logic lama)
// ============================

// Jika Accept header mengindikasikan browser ingin image/*,
// maka kita decode dataURL dan kirim bytes langsung.
func tryServeRawByAccept(c *gin.Context, accept string, dataURL string) bool {
	mime, b64, ok := parseDataURL(dataURL)
	if !ok {
		return false
	}

	// <img> biasanya kirim Accept ada "image/"
	// kita layani raw bytes agar preview tidak rusak
	wantImage := strings.Contains(accept, "image/")
	wantPDF := strings.Contains(accept, "application/pdf")

	if strings.HasPrefix(mime, "image/") && wantImage {
		if bs, err := base64.StdEncoding.DecodeString(b64); err == nil {
			c.Data(http.StatusOK, mime, bs)
			return true
		}
	}

	if mime == "application/pdf" && wantPDF {
		if bs, err := base64.StdEncoding.DecodeString(b64); err == nil {
			c.Data(http.StatusOK, mime, bs)
			return true
		}
	}

	return false
}

func parseDataURL(s string) (mime string, b64 string, ok bool) {
	ss := strings.TrimSpace(s)
	if !strings.HasPrefix(strings.ToLower(ss), "data:") {
		return "", "", false
	}
	parts := strings.SplitN(ss, ",", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	meta := strings.ToLower(strings.TrimSpace(parts[0])) // data:image/png;base64
	b64 = strings.TrimSpace(parts[1])

	// cari mime
	mime = strings.TrimPrefix(meta, "data:")
	mime = strings.TrimSpace(mime)
	mime = strings.SplitN(mime, ";", 2)[0]
	if mime == "" {
		return "", "", false
	}
	return mime, b64, true
}

// ekstrak src dari payload JSON yang mungkin bentuknya map / object
func extractSrcFromAny(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if s, ok := m["src"].(string); ok {
		return strings.TrimSpace(s)
	}
	// beberapa skema bisa pakai "eSuratJalan"
	if s, ok := m["eSuratJalan"].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func looksLikeBase64(s string) bool {
	ss := strings.TrimSpace(s)
	if ss == "" {
		return false
	}
	// base64 biasanya panjang dan tidak mengandung spasi
	if strings.ContainsAny(ss, " \n\r\t") {
		return false
	}
	// sedikit heuristik: harus ada karakter base64 umum
	// dan panjang minimal
	if len(ss) < 80 {
		return false
	}
	return true
}

// deteksi mime dari prefix base64 yang umum
func detectMimeFromBase64Prefix(b64 string) string {
	ss := strings.TrimSpace(b64)

	// PNG: iVBORw0KGgo
	if strings.HasPrefix(ss, "iVBORw0KGgo") {
		return "image/png"
	}
	// JPG: /9j/
	if strings.HasPrefix(ss, "/9j/") {
		return "image/jpeg"
	}
	// GIF: R0lGOD
	if strings.HasPrefix(ss, "R0lGOD") {
		return "image/gif"
	}
	// WEBP: UklGR
	if strings.HasPrefix(ss, "UklGR") {
		return "image/webp"
	}
	// PDF: JVBERi0
	if strings.HasPrefix(ss, "JVBERi0") {
		return "application/pdf"
	}

	return ""
}
