package models

import "gorm.io/gorm"

type User struct {
	gorm.Model
	Nama        string `json:"nama"`
	Username    string `gorm:"unique;not null;size:191" json:"username"`
	Password    string `json:"-"`
	Role        string `gorm:"type:varchar(20);default:'operator'" json:"role"` // admin / operator
	IsApproved  bool   `gorm:"default:false" json:"is_approved"`                // false = butuh approval admin
	Status      string `gorm:"type:varchar(20);default:'pending'" json:"status"` // pending / approved / rejected
	CustomTitle string `gorm:"default:'Sistem Monitoring COD & Suhu Air'" json:"custom_title"` // judul custom per user
}