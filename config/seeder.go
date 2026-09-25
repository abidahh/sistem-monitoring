package config

import (
	"log"

	"sistem-monitoring-cod_golang/models"

	"golang.org/x/crypto/bcrypt"
)

func SeedUsers() {
	var count int64
	DB.Model(&models.User{}).Count(&count)

	if count > 0 {
		log.Println("Seeder skipped: Data user sudah ada di database.")
		return
	}

	hashedPasswordAdmin, err := bcrypt.GenerateFromPassword([]byte(Cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal("Gagal hash password admin:", err)
	}

	// Buat Akun Admin Default yang Otomatis Approved
	admin := models.User{
		Nama:       Cfg.AdminNama,
		Username:   Cfg.AdminUsername,
		Password:   string(hashedPasswordAdmin),
		Role:       "admin",
		IsApproved: true,
		Status:     "approved",
	}

	err = DB.Create(&admin).Error
	if err != nil {
		log.Fatal("Gagal menjalankan seeder admin:", err)
	}

	log.Println("Seeder berhasil: Akun Admin default (Approved) berhasil dibuat!")
}

func SeedDefaultApiKey() {
	var count int64
	DB.Model(&models.ApiKey{}).Count(&count)

	if count > 0 {
		log.Println("Seeder skipped: API Key sudah ada di database.")
		return
	}

	defaultKey := models.ApiKey{
		Name:    Cfg.DefaultApiKeyName,
		KeyHash: HashAPIKey(Cfg.DefaultApiKey),
		Masked:  MaskedKey(Cfg.DefaultApiKey),
		Active:  true,
	}

	err := DB.Create(&defaultKey).Error
	if err != nil {
		log.Fatal("Gagal menjalankan seeder API Key:", err)
	}

	log.Println("Seeder berhasil: API Key default berhasil dibuat!", Cfg.DefaultApiKey)
}

func SeedSensorTypes() {
	var count int64
	DB.Model(&models.SensorType{}).Count(&count)

	if count > 0 {
		log.Println("Seeder skipped: Tipe sensor sudah ada di database.")
		return
	}

	defaultTypes := []models.SensorType{
		{Nama: "Suhu Air", Unit: "°C", NilaiMax: 30.0, Aktif: true, Warna: "#ef4444", Sumber: "simulasi"},
		{Nama: "Kelembaban", Unit: "%", NilaiMax: 100.0, Aktif: true, Warna: "#3b82f6", Sumber: "simulasi"},
		{Nama: "Kualitas Udara", Unit: "AQI", NilaiMax: 200.0, Aktif: true, Warna: "#f59e0b", Sumber: "simulasi"},
		{Nama: "Cahaya", Unit: "lux", NilaiMax: 50000.0, Aktif: true, Warna: "#22c55e", Sumber: "simulasi"},
		{Nama: "Kadar COD", Unit: "mg/L", NilaiMax: 100.0, Aktif: true, Warna: "#0d9488", Sumber: "simulasi"},
	}

	if err := DB.Create(&defaultTypes).Error; err != nil {
		log.Fatal("Gagal menjalankan seeder tipe sensor:", err)
	}

	log.Println("Seeder berhasil: Tipe sensor default berhasil dibuat!")
}
