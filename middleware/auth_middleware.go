package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// wantsHTML menebak apakah permintaan berasal dari navigasi browser ke
// halaman (Accept: text/html) atau dari pemanggilan API/fetch (Accept: */*).
// Dipakai untuk memilih: redirect halaman web vs respons JSON untuk API.
func wantsHTML(c *gin.Context) bool {
	return strings.Contains(c.Request.Header.Get("Accept"), "text/html")
}

// Memastikan User Sudah Login. Permintaan halaman web (navigasi browser)
// diarahkan ke halaman login; permintaan API ditolak dengan status 401 JSON.
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		userID := session.Get("user_id")

		if userID == nil {
			if wantsHTML(c) {
				c.Redirect(http.StatusFound, "/")
				c.Abort()
				return
			}
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Silakan login terlebih dahulu!"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// Memastikan Hanya Admin yang Bisa Akses. Halaman /admin diarahkan kembali ke
// dashboard untuk non-admin; permintaan API ditolak dengan status 403 JSON.
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		role := session.Get("role")

		if role != "admin" {
			if wantsHTML(c) {
				c.Redirect(http.StatusFound, "/dashboard")
				c.Abort()
				return
			}
			c.JSON(http.StatusForbidden, gin.H{"error": "Akses ditolak! Endpoint ini khusus Admin."})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireAdmin adalah cek otorisasi berlapis (defense in depth) yang dipanggil
// langsung dari dalam controller admin. Data tetap aman walau sebuah rute
// keliru didaftarkan di luar grup AdminOnly.
func RequireAdmin(c *gin.Context) bool {
	session := sessions.Default(c)
	if role, ok := session.Get("role").(string); ok && role == "admin" {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "Akses ditolak! Endpoint ini khusus Admin."})
	c.Abort()
	return false
}