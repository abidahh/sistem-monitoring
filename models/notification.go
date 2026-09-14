package models

import "time"

type NotificationLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Tipe      string    `gorm:"not null" json:"tipe"`
	Nilai     float64   `gorm:"not null" json:"nilai"`
	Batas     float64   `gorm:"not null" json:"batas"`
	Pesan     string    `gorm:"not null" json:"pesan"`
	CreatedAt time.Time `json:"created_at"`
}
