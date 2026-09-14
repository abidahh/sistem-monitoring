package config

import (
	"log"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func ConnectDatabase() {
	// Membuka atau membuat file database 'monitoring.db' tanpa butuh CGO.
	// Mode WAL + busy_timeout untuk mencegah error "database is locked (SQLITE_BUSY)"
	// akibat tulis sensor (simulator) dan operasi admin yang berjalan bersamaan.
	dsn := Cfg.DBDSN
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("Gagal terhubung ke database SQLite:", err)
	}

	// Serialkan akses DB (1 koneksi) agar tidak ada bentrok tulis antar-goroutine.
	sqlDB, err := database.DB()
	if err != nil {
		log.Fatal("Gagal menginisialisasi pool koneksi SQLite:", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	DB = database
	log.Println("Database SQLite berhasil terhubung tanpa CGO!")
}