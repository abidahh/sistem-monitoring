package models

import "time"

type NotificationLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	// SensorTypeID menandai sensor mana yang memicu notifikasi, supaya riwayat
	// bisa di-scope per user (hanya sensor yang di-assign ke user).
	SensorTypeID uint      `gorm:"index" json:"sensor_type_id"`
	Tipe         string    `gorm:"not null" json:"tipe"`
	Nilai        float64   `gorm:"not null" json:"nilai"`
	Batas        float64   `gorm:"not null" json:"batas"`
	Pesan        string    `gorm:"not null" json:"pesan"`
	CreatedAt    time.Time `json:"created_at"`
}
