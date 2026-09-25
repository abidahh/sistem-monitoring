package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config menyimpan nilai-nilai aplikasi yang sebelumnya di-hardcode.
// Semua nilai bisa ditimpa lewat environment variable (dengan default aman),
// mengikuti pola yang sudah dipakai serialreader (SERIAL_ENABLED, dll).
type Config struct {
	ServerPort    string
	SessionName   string
	SessionSecret string

	// Database: "sqlite" (default) atau "mysql".
	DBType string
	DBPath string
	DBDSN  string

	// Kredensial MySQL (dipakai saat DB_TYPE=mysql). Diisi dari env, tidak
	// pernah di-hardcode.
	MySQLHost string
	MySQLPort string
	MySQLUser string
	MySQLPass string
	MySQLName string

	// HTTPS opsional. Aktif bila SERVER_CERT dan SERVER_KEY diisi bersamaan.
	TLSCertFile string
	TLSKeyFile  string
	TLSEnabled  bool

	AdminUsername     string
	AdminPassword     string
	AdminNama         string
	DefaultApiKeyName string
	DefaultApiKey     string

	// Batas permintaan per menit per IP untuk endpoint publik (kirim data).
	ApiRateLimitPerMinute int

	// MQTT subscriber. Nilai ini hanya dipakai sebagai default; pengaturan
	// sebenarnya bisa diubah lewat panel admin (tersimpan di tabel settings).
	MQTTEnabled     bool
	MQTTBroker      string
	MQTTTopicPrefix string
	MQTTProtocol    string // "tcp" | "ws"
	MQTTWSPath      string // "/mqtt"
	MQTTUsername    string
	MQTTPassword    string

	DefaultSensorInterval       int
	DefaultAverageWindowMinutes int
	HardwareInactiveTimeout     time.Duration
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

// trimSlash menghapus garis miring di awal/akhir (misal topik MQTT).
func trimSlash(s string) string {
	return strings.Trim(s, "/")
}

// Load membaca seluruh konfigurasi dari environment variable (dengan default),
// termasuk file ".env" di direktori kerja bila ada. SESSION_SECRET otomatis
// dibuat acak dan disimpan bila tidak di-set. Panggil sekali di awal main().
func Load() {
	loadDotEnv()

	dbType := strings.ToLower(env("DB_TYPE", "sqlite"))
	dbPath := env("DB_PATH", "monitoring.db")
	cert := env("SERVER_CERT", "")
	key := env("SERVER_KEY", "")

	// Susun DSN sesuai tipe database: SQLite (file) atau MySQL (dsn tcp).
	var dsn string
	if dbType == "mysql" {
		dsn = buildMySQLDSN(
			env("DB_HOST", "127.0.0.1"),
			env("DB_PORT", "3306"),
			env("DB_USER", "monitoring"),
			env("DB_PASS", ""),
			env("DB_NAME", "monitoring"),
		)
	} else {
		dbType = "sqlite"
		dsn = buildDSN(dbPath)
	}

	Cfg = Config{
		ServerPort:    env("SERVER_PORT", ":8080"),
		SessionName:   env("SESSION_NAME", "mysession"),
		SessionSecret: ensureSessionSecret(env("SESSION_SECRET", ""), dbPath),

		DBType: dbType,
		DBPath: dbPath,
		DBDSN:  dsn,

		MySQLHost: env("DB_HOST", "127.0.0.1"),
		MySQLPort: env("DB_PORT", "3306"),
		MySQLUser: env("DB_USER", "monitoring"),
		MySQLPass: env("DB_PASS", ""),
		MySQLName: env("DB_NAME", "monitoring"),

		TLSCertFile: cert,
		TLSKeyFile:  key,
		TLSEnabled:  cert != "" && key != "",

		AdminUsername:     env("ADMIN_USERNAME", "admin"),
		AdminPassword:     env("ADMIN_PASSWORD", "admin123"),
		AdminNama:         env("ADMIN_NAMA", "Administrator COD"),
		DefaultApiKeyName: env("DEFAULT_API_KEY_NAME", "Default Sensor Key"),
		DefaultApiKey:     env("DEFAULT_API_KEY", "cod-monitor-default-key-2024"),

		ApiRateLimitPerMinute: envInt("API_RATE_LIMIT_PER_MINUTE", 60),

		MQTTEnabled:     env("MQTT_ENABLED", "false") == "true",
		MQTTBroker:      env("MQTT_BROKER", "shelter.cbinstrument.com:1883"),
		MQTTTopicPrefix: trimSlash(env("MQTT_TOPIC_PREFIX", "shelter/SHELTER-02/sensors")),
		MQTTProtocol:    env("MQTT_PROTOCOL", "tcp"),
		MQTTWSPath:      env("MQTT_WS_PATH", "/mqtt"),
		MQTTUsername:    env("MQTT_USERNAME", ""),
		MQTTPassword:    env("MQTT_PASSWORD", ""),

		DefaultSensorInterval:       envInt("DEFAULT_SENSOR_INTERVAL", 3),
		DefaultAverageWindowMinutes: envInt("AVERAGE_WINDOW_MINUTES", 5),
		HardwareInactiveTimeout:     time.Duration(envInt("HARDWARE_INACTIVE_TIMEOUT", 30)) * time.Second,
	}
}

// buildDSN menyusun DSN SQLite dengan pragma WAL, busy_timeout, dan foreign_keys.
func buildDSN(dbPath string) string {
	return "file:" + dbPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)"
}

// buildMySQLDSN menyusun DSN MySQL. parseTime=True & loc=Local agar kolom
// datetime terbaca sebagai time.Time dan dibuat sesuai zona waktu server.
func buildMySQLDSN(host, port, user, pass, name string) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		user, pass, host, port, name)
}

// DBIsMySQL helper dialek: apakah database aktif MySQL.
func DBIsMySQL() bool {
	return Cfg.DBType == "mysql"
}
