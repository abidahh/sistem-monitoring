package controllers

import "testing"

func TestIsSpikeReading(t *testing.T) {
	cases := []struct {
		name   string
		prev   float64
		value  float64
		ratio  float64
		absMin float64
		want   bool
	}{
		{"kenaikan drastis 100 -> 900", 100, 900, 0.5, 5, true},
		{"penurunan drastis 100 -> 5", 100, 5, 0.5, 5, true},
		{"fluktuasi normal 100 -> 105", 100, 105, 0.5, 5, false},
		{"fluktuasi naik 100 -> 120 (<50%)", 100, 120, 0.5, 5, false},
		{"tepat di ambang 100 -> 150 (delta == threshold)", 100, 150, 0.5, 5, false},
		{"lonjakan dari baseline 0 -> 100", 0, 100, 0.5, 5, true},
		{"delta kecil di bawah absMin", 10, 13, 0.5, 5, false},
		{"delta persis absMin bukan lonjakan", 10, 15, 0.5, 5, false},
		{"nilai negatif lonjakan tajam", 100, -10, 0.5, 5, true},
	}

	for _, tc := range cases {
		if got := isSpikeReading(tc.prev, tc.value, tc.ratio, tc.absMin); got != tc.want {
			t.Errorf("%s: isSpikeReading(%v,%v,%.2f,%.2f) = %v, want %v",
				tc.name, tc.prev, tc.value, tc.ratio, tc.absMin, got, tc.want)
		}
	}
}

func TestSpikeDetectionConfigDefaults(t *testing.T) {
	// Default harus cukup peka: contoh kasus dari user (100 -> 5 / 900) tertolak,
	// sedangkan fluktuasi normal (100 -> 105) tetap lolos.
	if !isSpikeReading(100, 900, spikeJumpRatio, spikeAbsMin) {
		t.Error("default harus mendeteksi 100 -> 900 sebagai lonjakan")
	}
	if !isSpikeReading(100, 5, spikeJumpRatio, spikeAbsMin) {
		t.Error("default harus mendeteksi 100 -> 5 sebagai lonjakan")
	}
	if isSpikeReading(100, 105, spikeJumpRatio, spikeAbsMin) {
		t.Error("default tidak boleh menandai 100 -> 105 sebagai lonjakan")
	}
}