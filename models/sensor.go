package models

import "time"

type SensorData struct {
	ID           uint        `gorm:"primaryKey" json:"id"`
	SensorTypeID uint        `gorm:"index:idx_sd_type_time,priority:1" json:"sensor_type_id"`
	SensorType   SensorType  `gorm:"foreignKey:SensorTypeID" json:"sensor_type,omitempty"`
	Value        float64     `json:"value"`
	Sumber       string      `gorm:"default:real;index" json:"sumber"`
	IsAnomaly    bool        `gorm:"default:false;index" json:"is_anomaly"` // lonjakan: disimpan untuk riwayat, tapi dicoret dari rata-rata
	CreatedAt    time.Time   `gorm:"index:idx_sd_type_time,priority:2;precision:3" json:"created_at"`
}
