package middleware

import (
	"net/http"

	"sistem-monitoring-cod_golang/config"

	"github.com/gin-gonic/gin"
)

// SecurityHeaders memasang header keamanan HTTP dasar untuk seluruh respons
// aplikasi (halaman HTML maupun API JSON).
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// Saat HTTPS aktif, browser dipaksa terus memakai koneksi aman.
		if config.Cfg.TLSEnabled {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; "+
				"style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; "+
				"font-src 'self' data: https://cdn.jsdelivr.net; "+
				"img-src 'self' data:; "+
				"connect-src 'self' wss://shelter.cbinstrument.com; "+
				"base-uri 'self'; "+
				"form-action 'self'; "+
				"frame-ancestors 'none'; "+
				"object-src 'none'")
		// Data sensitif (akun, sensor, notifikasi) tidak boleh di-cache browser.
		h.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		h.Set("Pragma", "no-cache")
		h.Set("Expires", "0")
		c.Next()
	}
}

// BodyLimit membatasi ukuran body request pada endpoint tertentu (mencegah
// banjir payload). Request melebihi batas akan gagal saat pembacaan body.
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}
