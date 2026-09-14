package models

import "gorm.io/gorm"

type ApiKey struct {
	gorm.Model
	Name   string `gorm:"not null" json:"name"`
	Key    string `gorm:"uniqueIndex;not null" json:"key"`
	Active bool   `gorm:"default:true" json:"active"`
}
