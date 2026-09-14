package scanner

import (
	"fmt"
	"math"
	"testing"
)

func TestScoreFormatRejects(t *testing.T) {
	cases := []float64{
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
		1e8,
		-1e8,
		0.005, // > 0 tapi < 1e-2
	}
	for _, v := range cases {
		if s := scoreFormat("float.be", v); s > 0 {
			t.Errorf("scoreFormat(%v) = %d, want <= 0", v, s)
		}
	}
	// nol mutlak tetap diterima tapi diskor lebih rendah daripada nilai non-zero
	if s := scoreFormat("float.be", 0); s <= 0 {
		t.Errorf("scoreFormat(0) = %d, want > 0", s)
	}
	if s := scoreFormat("float.be", 0); s >= scoreFormat("float.be", 25.0) {
		t.Error("nilai 0 harus diskor lebih rendah daripada nilai non-zero")
	}
}

func TestScoreFormatAccepts(t *testing.T) {
	// nilai wajar untuk suhu / COD / pH
	for _, v := range []float64{25.3, 123.45, 6.8, 8.5, 2.0, 1e5, 0.011} {
		if s := scoreFormat("float.be", v); s <= 0 {
			t.Errorf("scoreFormat(%v) = %d, want > 0", v, s)
		}
	}
	if s := scoreFormat("float.be", 30); s <= scoreFormat("int16.be", 30) {
		t.Error("format float harus diskor lebih tinggi daripada int16 untuk nilai sama")
	}
}

func TestSelectTopCandidates(t *testing.T) {
	mk := func(reg uint16, format string, score int) Candidate {
		return Candidate{Register: reg, Format: format, Score: score}
	}
	all := []Candidate{
		mk(1, "float.be", 100),
		mk(1, "float.be", 150), // dupe (reg+format) → hanya skor tertinggi dipakai
		mk(1, "int16.le", 130),
		mk(2, "float.cdab", 140),
		mk(3, "float.bc", 90),
	}
	top := selectTopCandidates(all, 3)
	if len(top) != 3 {
		t.Fatalf("len(top) = %d, want 3", len(top))
	}
	if top[0].Register != 1 || top[0].Format != "float.be" || top[0].Score != 150 {
		t.Errorf("kandidat teratas salah: %+v", top[0])
	}
	// tidak boleh dupe register+format
	seen := map[string]bool{}
	for _, c := range top {
		key := fmt.Sprintf("%s|%d|%d|%s", c.Port, c.SlaveID, c.Register, c.Format)
		if seen[key] {
			t.Errorf("kandidat dupe: %s", key)
		}
		seen[key] = true
	}
}

// TestSelectTopCandidatesMultiPort memastikan kandidat yang identik (register +
// format sama) tapi berasal dari port/UID berbeda TIDAK dibuang — kasus satu alat
// fisik yang mengekspos beberapa COM port.
func TestSelectTopCandidatesMultiPort(t *testing.T) {
	all := []Candidate{
		{Port: "COM3", SlaveID: 1, Register: 1, Format: "float.be", Score: 150},
		{Port: "COM4", SlaveID: 1, Register: 1, Format: "float.be", Score: 150},
		{Port: "COM5", SlaveID: 1, Register: 1, Format: "float.be", Score: 150},
		{Port: "COM6", SlaveID: 1, Register: 1, Format: "float.be", Score: 150},
		{Port: "COM3", SlaveID: 2, Register: 1, Format: "float.be", Score: 140},
	}
	top := selectTopCandidates(all, 10)
	if len(top) != 5 {
		t.Fatalf("len(top) = %d, want 5 (semua identitas berbeda harus dipertahankan)", len(top))
	}
	seen := map[string]bool{}
	for _, c := range top {
		key := fmt.Sprintf("%s|%d|%d|%s", c.Port, c.SlaveID, c.Register, c.Format)
		if seen[key] {
			t.Errorf("kandidat dupe: %s", key)
		}
		seen[key] = true
	}
}