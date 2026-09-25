package config

import (
	"log"

	"github.com/glebarez/sqlite"
	mysqldriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

// ConnectDatabase membuka koneksi sesuai Cfg.DBType:
//   - "sqlite": file monitoring.db (pure-Go, tanpa CGO — untuk deploy Pi).
//   - "mysql" : server MySQL/MariaDB lewat DSN di Cfg.DBDSN.
//
// Pool koneksi disesuaikan per dialek (SQLite serial 1 koneksi; MySQL paralel).
func ConnectDatabase() {
	var (
		database *gorm.DB
		err      error
	)

	switch Cfg.DBType {
	case "mysql":
		database, err = gorm.Open(mysqldriver.Open(Cfg.DBDSN), &gorm.Config{})
		if err != nil {
			log.Fatal("Gagal terhubung ke database MySQL:", err)
		}
	default:
		database, err = gorm.Open(sqlite.Open(Cfg.DBDSN), &gorm.Config{})
		if err != nil {
			log.Fatal("Gagal terhubung ke database SQLite:", err)
		}
	}

	sqlDB, err := database.DB()
	if err != nil {
		log.Fatal("Gagal menginisialisasi pool koneksi database:", err)
	}

	if Cfg.DBType == "mysql" {
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(4)
		sqlDB.SetConnMaxLifetime(0)
	} else {
		// Serialkan akses SQLite (1 koneksi) agar tidak ada bentrok tulis.
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	}

	DB = database
	log.Printf("Database %s berhasil terhubung.", Cfg.DBType)
}