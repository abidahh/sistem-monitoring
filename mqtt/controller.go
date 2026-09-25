package mqtt

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetMQTTSetting mengembalikan pengaturan MQTT untuk panel admin.
func GetMQTTSetting(c *gin.Context) {
	s := LoadSettings()
	c.JSON(http.StatusOK, gin.H{
		"enabled":       s.Enabled,
		"broker":        s.Broker,
		"protocol":      s.Protocol,
		"ws_path":       s.WSPath,
		"topic_prefix":  s.TopicPrefix,
		"username":      s.Username,
		"password":      s.Password,
	})
}

// GetMQTTStatus mengembalikan status koneksi MQTT saat ini.
func GetMQTTStatus(c *gin.Context) {
	status := GetStatus()
	c.JSON(http.StatusOK, status)
}

// UpdateMQTTSetting menyimpan pengaturan MQTT lalu menerapkan ulang subscriber
// tanpa perlu restart server.
func UpdateMQTTSetting(c *gin.Context) {
	var input struct {
		Enabled     *bool  `json:"enabled"`
		Broker      string `json:"broker"`
		Protocol    string `json:"protocol"`     // "tcp" | "ws"
		WSPath      string `json:"ws_path"`      // "/mqtt"
		TopicPrefix string `json:"topic_prefix"`
		Username    string `json:"username"`
		Password    string `json:"password"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Format input tidak valid!"})
		return
	}

	enabled := false
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	protocol := strings.TrimSpace(input.Protocol)
	if protocol == "" {
		protocol = "tcp"
	}
	wsPath := strings.TrimSpace(input.WSPath)
	if wsPath == "" {
		wsPath = "/mqtt"
	}

	s := Settings{
		Enabled:      enabled,
		Broker:       strings.TrimSpace(input.Broker),
		Protocol:     protocol,
		WSPath:       wsPath,
		TopicPrefix:  strings.Trim(strings.TrimSpace(input.TopicPrefix), "/"),
		Username:     strings.TrimSpace(input.Username),
		Password:     strings.TrimSpace(input.Password),
	}
	if s.Enabled && s.Broker == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Alamat broker MQTT wajib diisi saat MQTT aktif!"})
		return
	}

	if err := SaveSettings(s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menyimpan pengaturan MQTT!"})
		return
	}

	// Terapkan ulang subscriber dengan pengaturan yang baru saja disimpan.
	Restart()

	c.JSON(http.StatusOK, gin.H{
		"message":       "Pengaturan MQTT berhasil disimpan!",
		"enabled":       s.Enabled,
		"broker":        s.Broker,
		"protocol":      s.Protocol,
		"ws_path":       s.WSPath,
		"topic_prefix":  s.TopicPrefix,
	})
}