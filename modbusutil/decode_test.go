package modbusutil

import (
	"encoding/binary"
	"math"
	"testing"
)

func floatBytes(b []byte) float64 {
	return float64(math.Float32frombits(binary.BigEndian.Uint32(b)))
}

func TestDecodeValueFloatOrders(t *testing.T) {
	// raw = hasil dari bus: reg1 = 0x4194, reg2 = 0x0000.
	raw := []byte{0x41, 0x94, 0x00, 0x00}

	cases := []struct {
		format   string
		permuted []byte
	}{
		{"float.be", []byte{0x41, 0x94, 0x00, 0x00}},
		{"float.cdab", []byte{0x00, 0x00, 0x41, 0x94}},
		{"float.badc", []byte{0x94, 0x41, 0x00, 0x00}},
		{"float.dcba", []byte{0x00, 0x00, 0x94, 0x41}},
	}

	for _, tc := range cases {
		got := DecodeValue(raw, tc.format)
		want := floatBytes(tc.permuted)
		if got != want {
			t.Errorf("%s: got %v, want %v (permuted % x)", tc.format, got, want, tc.permuted)
		}
	}
}

func TestDecodeValueInts(t *testing.T) {
	cases := []struct {
		format string
		raw    []byte
		want   float64
	}{
		{"int32.be", []byte{0x00, 0x00, 0x00, 0xFF}, 255},
		{"int32.le", []byte{0xFF, 0x00, 0x00, 0x00}, 255},
		{"int32.be", []byte{0xFF, 0xFF, 0xFF, 0xFE}, -2},
		{"int32.le", []byte{0xFE, 0xFF, 0xFF, 0xFF}, -2},
		{"int16.be", []byte{0x00, 0xFF}, 255},
		{"int16.le", []byte{0xFF, 0x00}, 255},
		{"int16.be", []byte{0x80, 0x01}, -32767},
		{"int16.le", []byte{0x01, 0x80}, -32767},
	}

	for _, tc := range cases {
		got := DecodeValue(tc.raw, tc.format)
		if got != tc.want {
			t.Errorf("%s(% x): got %v, want %v", tc.format, tc.raw, got, tc.want)
		}
	}
}

func TestDecodeValueFallbackAndShort(t *testing.T) {
	// Format tak dikenal mengikuti perilaku lama (Big Endian float).
	raw := []byte{0x3F, 0x80, 0x00, 0x00} // 1.0
	if got := DecodeValue(raw, "float.xyz"); got != 1.0 {
		t.Errorf("unknown format: got %v, want 1.0", got)
	}
	// Raw terlalu pendek harus aman (0), tidak panic.
	for _, f := range ValidFormats {
		if got := DecodeValue([]byte{0x00}, f); got != 0 {
			t.Errorf("%s with short raw: got %v, want 0", f, got)
		}
	}
}

func TestIsValidFormat(t *testing.T) {
	for _, f := range ValidFormats {
		if !IsValidFormat(f) {
			t.Errorf("expected %q valid", f)
		}
	}
	for _, f := range []string{"", "float", "INT16.BE"} {
		if IsValidFormat(f) {
			t.Errorf("expected %q invalid", f)
		}
	}
	if len(ValidFormats) != 8 {
		t.Errorf("expected 8 formats, got %d", len(ValidFormats))
	}
}