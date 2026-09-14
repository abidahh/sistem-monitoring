package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config menyimpan nilai-nilai aplikasi yang sebelumnya di-hardcode.
// Semua nilai bisa ditimpa lewat environment variable (dengan default aman),
// mengikuti pola yang sudah dipakai serialreader (SERIAL_ENABLED, dll).
type Config struct {
	ServerPort string
	SessionName string
	SessionSecret string

	DBPath string
	DBDSN string

	AdminUsername string
	AdminPassword string
	AdminNama string
	DefaultApiKeyName string
	DefaultApiKey string

	DefaultSensorInterval int
	DefaultAverageWindowMinutes int
	HardwareInactiveTimeout time.Duration
}

var Cfg Config

// env mengambil nilai env, dengan fallback default jika kosong.
func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// envInt mengambil nilai env sebagai int, dengan fallback default.
func envInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// Load membaca seluruh konfigurasi dari environment variable (dengan default).
// Panggil sekali di awal main() sebelum memakai nilai lainnya.
func Load() {
	Cfg = Config{
		ServerPort:   env("SERVER_PORT", ":8080"),
		SessionName:  env("SESSION_NAME", "mysession"),
		SessionSecret: env("SESSION_SECRET", "secret-key-monitoring-cod"),

		DBPath: env("DB_PATH", "monitoring.db"),
		DBDSN:  buildDSN(env("DB_PATH", "monitoring.db")),

		AdminUsername:     env("ADMIN_USERNAME", "admin"),
		AdminPassword:     env("ADMIN_PASSWORD", "admin123"),
		AdminNama:         env("ADMIN_NAMA", "Administrator COD"),
		DefaultApiKeyName: env("DEFAULT_API_KEY_NAME", "Default Sensor Key"),
		DefaultApiKey:     env("DEFAULT_API_KEY", "cod-monitor-default-key-2024"),

		DefaultSensorInterval:  envInt("DEFAULT_SENSOR_INTERVAL", 3),
		DefaultAverageWindowMinutes: envInt("AVERAGE_WINDOW_MINUTES", 5),
		HardwareInactiveTimeout: time.Duration(envInt("HARDWARE_INACTIVE_TIMEOUT", 30)) * time.Second,
	}
}

// buildDSN menyusun DSN SQLite dengan pragma WAL, busy_timeout, dan foreign_keys.
func buildDSN(dbPath string) string {
	return "file:" + dbPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)"
}
