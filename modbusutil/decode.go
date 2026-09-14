// Package modbusutil menyediakan utilitas untuk membaca & menginterpretasikan
// nilai register Modbus RTU. Decode dipakai bersama oleh app utama (main.go) dan
// tool uji terminal (cmd/modbuscheck).
//
// Format yang didukung (kolom register_format pada sensor_types):
//
//	"float.be"   Float 32-bit ABCD  (Big Endian)           -> byte: 0 1 2 3
//	"float.cdab" Float 32-bit CDAB  (tukar word/register)  -> byte: 2 3 0 1
//	"float.badc" Float 32-bit BADC  (tukar byte dalam word)-> byte: 1 0 3 2
//	"float.dcba" Float 32-bit DCBA  (Little Endian)        -> byte: 3 2 1 0
//	"int32.be"   Integer 32-bit Big Endian
//	"int32.le"   Integer 32-bit Little Endian
//	"int16.be"   Integer 16-bit Big Endian (register pertama)
//	"int16.le"   Integer 16-bit Little Endian (register pertama)
package modbusutil

import (
	"encoding/binary"
	"math"
)

// ValidFormats memuat seluruh kode format yang dapat dipilih.
// Dipakai untuk validasi input pada API admin.
var ValidFormats = []string{
	"float.be",
	"float.cdab",
	"float.badc",
	"float.dcba",
	"int32.be",
	"int32.le",
	"int16.be",
	"int16.le",
}

// IsValidFormat memastikan format berada dalam daftar ValidFormats.
func IsValidFormat(f string) bool {
	for _, v := range ValidFormats {
		if v == f {
			return true
		}
	}
	return false
}

// DecodeValue mengubah hasil pembacaan register (biasanya 2 register = 4 byte)
// menjadi nilai float64 sesuai format yang dipilih. Bila raw lebih pendek dari
// yang dibutuhkan, mengembalikan 0 (dan data dianggap tidak terbaca).
func DecodeValue(raw []byte, format string) float64 {
	switch format {
	case "float.be", "float.cdab", "float.badc", "float.dcba":
		if len(raw) < 4 {
			return 0
		}
		b := orderBytes(raw, format)
		return float64(math.Float32frombits(binary.BigEndian.Uint32(b)))
	case "int32.be":
		if len(raw) < 4 {
			return 0
		}
		return float64(int32(binary.BigEndian.Uint32(raw[0:4])))
	case "int32.le":
		if len(raw) < 4 {
			return 0
		}
		return float64(int32(binary.LittleEndian.Uint32(raw[0:4])))
	case "int16.be":
		if len(raw) < 2 {
			return 0
		}
		return float64(int16(binary.BigEndian.Uint16(raw[0:2])))
	case "int16.le":
		if len(raw) < 2 {
			return 0
		}
		return float64(int16(binary.LittleEndian.Uint16(raw[0:2])))
	default:
		// Format tak dikenal = perilaku lama (Big Endian float).
		if len(raw) < 4 {
			return 0
		}
		return float64(math.Float32frombits(binary.BigEndian.Uint32(raw[0:4])))
	}
}

// orderBytes menyusun ulang 4 byte sesuai varian urutan float32.
func orderBytes(raw []byte, format string) []byte {
	b := make([]byte, 4)
	copy(b, raw[0:4])
	switch format {
	case "float.cdab":
		return []byte{b[2], b[3], b[0], b[1]}
	case "float.badc":
		return []byte{b[1], b[0], b[3], b[2]}
	case "float.dcba":
		return []byte{b[3], b[2], b[1], b[0]}
	default:
		return b
	}
}