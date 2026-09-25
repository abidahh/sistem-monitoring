package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"

	"sistem-monitoring-cod_golang/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	defaultAdminPassword = "admin123"
	defaultApiKey        = "cod-monitor-default-key-2024"
	defaultSessionSecret = "secret-key-monitoring-cod"
)

// loadDotEnv membaca file ".env" di direktori kerja bila ada lalu mengisi
// os.Getenv, sehingga konfigurasi rahasia bisa disimpan dalam file (tidak
// bergantung pada "set" manual di shell). Baris komentar "#" diabaikan,
// format: KEY=VALUE.
func loadDotEnv() {
	b, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = strings.Trim(val, `"'`)
		if key != "" {
			// Jangan menimpa variabel yang sudah di-set di environment, supaya
			// nilai eksternal (mis. SERVER_PORT saat deploy) tetap dihormati.
			if _, ok := os.LookupEnv(key); ok {
				continue
			}
			os.Setenv(key, val)
		}
	}
}

// ensureSessionSecret memastikan SESSION_SECRET selalu aman: memakai nilai dari
// env bila di-set, atau membangkitkan nilai acak dan menyimpannya di file di
// samping database agar sesi tetap valid antar restart.
func ensureSessionSecret(envVal, dbPath string) string {
	if v := strings.TrimSpace(envVal); v != "" {
		return v
	}
	file := dbPath + ".session_secret"
	if b, err := os.ReadFile(file); err == nil {
		if s := strings.TrimSpace(string(b)); len(s) >= 32 {
			return s
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		log.Fatalf("Gagal membangkitkan SESSION_SECRET acak: %v", err)
	}
	s := hex.EncodeToString(raw)
	if dir := filepath.Dir(file); dir != "" {
		_ = os.MkdirAll(dir, 0o700)
	}
	if err := os.WriteFile(file, []byte(s), 0o600); err != nil {
		log.Fatalf("Gagal menulis file SESSION_SECRET: %v", err)
	}
	return s
}

// HashAPIKey mengubah API key polos menjadi SHA-256 hex. Hanya hash ini yang
// disimpan di database; lookup dilakukan dengan menghash header yang masuk.
func HashAPIKey(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}

// MaskedKey menyamarkan API key polos untuk ditampilkan di panel admin,
// misalnya "ab164a12••••10e3c". Tidak pernah menampilkan key secara utuh.
func MaskedKey(plain string) string {
	if len(plain) <= 12 {
		return "••••"
	}
	return plain[:8] + "••••" + plain[len(plain)-4:]
}

// MigrateApiKeyHashing mengubah API key polos yang masih tersimpan di database
// menjadi hash SHA-256. Nilai key TIDAK diganti (tanpa rotasi), sehingga
// perangkat pengirim tetap memakai key yang sama seperti sebelumnya. Kolom "key"
// polos dihapus setelah selesai dipindahkan. Panggil setelah seeder.
func MigrateApiKeyHashing() {
	if err := DB.AutoMigrate(&models.ApiKey{}); err != nil {
		log.Printf("[SECURITY] Gagal menyesuaikan tabel API key: %v", err)
		return
	}

	type legacyRow struct {
		ID  uint
		Key string
	}
	var rows []legacyRow
	if !DB.Migrator().HasColumn(&models.ApiKey{}, "key") {
		rows = nil
	} else if err := DB.Raw("SELECT `id`, `key` FROM api_keys").Scan(&rows).Error; err != nil {
		log.Printf("[SECURITY] Gagal membaca API key lama: %v", err)
		return
	}

	migrated := 0
	for _, r := range rows {
		if r.Key == "" {
			continue
		}
		if err := DB.Exec("UPDATE api_keys SET `key_hash` = ?, `masked` = ? WHERE id = ?",
			HashAPIKey(r.Key), MaskedKey(r.Key), r.ID).Error; err != nil {
			log.Printf("[SECURITY] Gagal meng-hash API key id=%d: %v", r.ID, err)
			continue
		}
		migrated++
	}
	if migrated > 0 {
		log.Printf("[SECURITY] %d API key telah di-hash (SHA-256) tanpa mengubah nilainya.", migrated)
	}

	if DB.Migrator().HasColumn(&models.ApiKey{}, "key") {
		if err := DB.Migrator().DropColumn(&models.ApiKey{}, "key"); err != nil {
			log.Printf("[SECURITY] Gagal menghapus kolom API key polos: %v", err)
		} else {
			log.Println("[SECURITY] Kolom API key polos telah dihapus dari database.")
		}
	}
}

// EnforceSecurity menolak aplikasi berjalan bila masih memakai kredensial
// default bawaan / lemah.
func EnforceSecurity() {
	switch {
	case Cfg.SessionSecret == "" || Cfg.SessionSecret == defaultSessionSecret:
		log.Fatalf("[SECURITY] SESSION_SECRET masih memakai nilai default. " +
			"Isi SESSION_SECRET acak (min. 32 karakter) di file .env.")
	case Cfg.AdminPassword == "" || Cfg.AdminPassword == defaultAdminPassword || len(Cfg.AdminPassword) < 8:
		log.Fatalf("[SECURITY] ADMIN_PASSWORD lemah. Isi ADMIN_PASSWORD (min. 8 karakter) di file .env.")
	case Cfg.DefaultApiKey == "" || Cfg.DefaultApiKey == defaultApiKey || len(Cfg.DefaultApiKey) < 16:
		log.Fatalf("[SECURITY] DEFAULT_API_KEY lemah. Isi DEFAULT_API_KEY acak (min. 16 karakter) di file .env.")
	case (Cfg.TLSCertFile == "") != (Cfg.TLSKeyFile == ""):
		log.Fatalf("[SECURITY] SERVER_CERT dan SERVER_KEY harus diisi bersamaan untuk mengaktifkan HTTPS.")
	}
	log.Println("[SECURITY] Kredensial aman (non-default) — server siap berjalan.")
}

// UpgradeLegacyCredentials memperbaiki instalasi lama yang masih memakai
// password admin & API key default bawaan, memakai nilai yang diisi di .env
// (dipanggil setelah EnforceSecurity lolos, jadi nilainya sudah non-default).
func UpgradeLegacyCredentials() {
	upgradeAdminPassword()
	upgradeDefaultApiKey()
}

func upgradeAdminPassword() {
	var user models.User
	err := DB.Where("role = ?", "admin").Order("id asc").First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return
		}
		log.Printf("[SECURITY] Gagal memeriksa admin: %v", err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(defaultAdminPassword)) != nil {
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(Cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("[SECURITY] Gagal hash password admin baru: %v", err)
		return
	}
	user.Password = string(hash)
	if err := DB.Save(&user).Error; err == nil {
		log.Printf("[SECURITY] Password admin default telah diganti dari nilai .env (user: %s).", user.Username)
	}
}

func upgradeDefaultApiKey() {
	// Kolom "key" polos masih ada di tahap ini (belum di-drop oleh
	// MigrateApiKeyHashing), jadi upgrade memakai SQL mentah.
	if !DB.Migrator().HasColumn(&models.ApiKey{}, "key") {
		return
	}
	var row struct {
		ID   uint
		Name string
	}
	err := DB.Raw("SELECT `id`, `name` FROM api_keys WHERE `key` = ? AND deleted_at IS NULL ORDER BY id asc LIMIT 1",
		defaultApiKey).Scan(&row).Error
	if err != nil || row.ID == 0 {
		return
	}

	var already int64
	DB.Model(&models.ApiKey{}).Where("key_hash = ?", HashAPIKey(Cfg.DefaultApiKey)).Count(&already)
	if already > 0 {
		return
	}

	if err := DB.Exec("UPDATE api_keys SET `key` = ? WHERE id = ?", Cfg.DefaultApiKey, row.ID).Error; err == nil {
		log.Printf("[SECURITY] API key default telah diganti dari nilai .env (key: %s). Perbarui di perangkat pengirim.", row.Name)
	}
}
