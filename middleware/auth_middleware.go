package middleware

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// Memastikan User Sudah Login
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		userID := session.Get("user_id")

		if userID == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Silakan login terlebih dahulu!"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// Memastikan Hanya Admin yang Bisa Akses
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		role := session.Get("role")

		if role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Akses ditolak! Halaman ini khusus Admin."})
			c.Abort()
			return
		}
		c.Next()
	}
}