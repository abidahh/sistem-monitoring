package controllers

import (
	"testing"
	"time"
)

// TestClosedWindowStart memverifikasi boundary bucket: bucket "sudah tutup"
// adalah kelipatan jendela terakhir di bawah now (sejajar epoch detik).
func TestClosedWindowStart(t *testing.T) {
	base := time.Unix(1700000000, 0) // 2023-11-14 22:13:20 UTC
	cases := []struct {
		name       string
		now        time.Time
		windowSecs int
		want        time.Time
	}{
		{"contoh lahir: sisa 200 detik", base, 300, time.Unix(1699999500, 0).UTC()},
		{"tepat di boundary", time.Unix(1699999800, 0), 300, time.Unix(1699999500, 0).UTC()},
		{"tengah bucket", time.Unix(1699999650, 0), 300, time.Unix(1699999200, 0).UTC()},
		{"detik 1 bucket baru", time.Unix(1699999501, 0), 300, time.Unix(1699999200, 0).UTC()},
		{"jendela 1 menit", base, 60, time.Unix(1699999920, 0).UTC()},
		{"jendela 1 jam", base, 3600, time.Unix(1699995600, 0).UTC()},
	}

	for _, tc := range cases {
		if got := closedWindowStart(tc.now, tc.windowSecs); !got.Equal(tc.want) {
			t.Errorf("%s: closedWindowStart(%v,%d) = %v, want %v",
				tc.name, tc.now, tc.windowSecs, got, tc.want)
		}
	}
}