package models

type UserSensor struct {
	ID           uint `gorm:"primaryKey"`
	UserID       uint `gorm:"uniqueIndex:idx_user_sensor"`
	SensorTypeID uint `gorm:"uniqueIndex:idx_user_sensor"`
}
