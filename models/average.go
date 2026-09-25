package models

import "time"

// SensorAverage menyimpan rata-rata per bucket jendela (mis. per 5 menit)
// per tipe sensor. Ditulis oleh scheduler latar belakang dari sensor_data;
// data mentah tidak pernah diubah di sini.
type SensorAverage struct {
	ID           uint        `gorm:"primaryKey" json:"id"`
	SensorTypeID uint        `gorm:"uniqueIndex:idx_sa_bucket,priority:1" json:"sensor_type_id"`
	WindowStart  time.Time   `gorm:"uniqueIndex:idx_sa_bucket,priority:2" json:"window_start"`
	WindowSecs   int         `gorm:"uniqueIndex:idx_sa_bucket,priority:3" json:"window_secs"`
	Average      float64     `json:"average"`
	Total        int64       `json:"total"`
	UpdatedAt    time.Time   `json:"updated_at"`
}