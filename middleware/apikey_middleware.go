package middleware

import (
	"net/http"

	"sistem-monitoring-cod_golang/config"
	"sistem-monitoring-cod_golang/models"

	"github.com/gin-gonic/gin"
)

func ApiKeyRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "API Key diperlukan! Kirim header X-API-Key."})
			c.Abort()
			return
		}

		// DB hanya menyimpan hash SHA-256, jadi header di-hash dulu sebelum lookup.
		keyHash := config.HashAPIKey(apiKey)
		var key models.ApiKey
		if err := config.DB.Where("key_hash = ? AND active = ?", keyHash, true).First(&key).Error; err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "API Key tidak valid atau sudah tidak aktif!"})
			c.Abort()
			return
		}

		c.Next()
	}
}
