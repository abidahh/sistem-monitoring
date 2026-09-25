package models

import "gorm.io/gorm"

type ApiKey struct {
	gorm.Model
	// KeyHash menyimpan SHA-256 dari API key polos. Key polos tidak pernah
	// disimpan di database; hanya tampil sekali saat key dibuat.
	KeyHash string `gorm:"uniqueIndex;size:191" json:"-"`
	// Masked adalah bentuk disamarkan (mis. ab164a12••••10e3c) untuk tabel admin.
	Masked string `json:"masked"`
	Name   string `gorm:"not null" json:"name"`
	Active bool   `gorm:"default:true" json:"active"`
}
