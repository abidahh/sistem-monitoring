package models

import "gorm.io/gorm"

type SensorType struct {
	gorm.Model
	Nama     string  `gorm:"uniqueIndex;not null" json:"nama"`
	Unit     string  `gorm:"not null" json:"unit"`
	NilaiMax float64 `json:"nilai_max"`
	Aktif    bool    `gorm:"default:true" json:"aktif"`
	Warna    string  `gorm:"default:'#0d9488'" json:"warna"`
	// Sumber data: "simulasi" (data dibangkitkan otomatis) atau "real" (data dari hardware/serial/HTTP).
	Sumber string `gorm:"default:'simulasi'" json:"sumber"`
	
	// KOLOM DYNAMICAL MODBUS CONFIG
	SlaveID      byte   `json:"slave_id" gorm:"default:1"`
	RegisterAddr uint16 `json:"register_addr" gorm:"default:0"`
	BaudRate     int    `json:"baud_rate" gorm:"default:9600"`
	PortCom      string `json:"port_com" gorm:"default:'COM3'"`
	// Format decode nilai register Modbus (lihat modbusutil): default "float.be".
	RegisterFormat string `json:"register_format" gorm:"default:'float.be'"`
}