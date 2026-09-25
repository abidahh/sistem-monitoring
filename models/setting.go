package models

import "gorm.io/gorm"

type Setting struct {
	gorm.Model
	Key   string `gorm:"uniqueIndex;not null;size:191" json:"key"`
	Value string `gorm:"not null" json:"value"`
}