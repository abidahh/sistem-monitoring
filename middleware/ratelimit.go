package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	loginMaxFails   = 5                // maksimum percobaan gagal
	loginWindow     = 15 * time.Minute // jendela pemantauan percobaan
	loginBlockFor   = 15 * time.Minute // lama penguncian setelah melebihi batas
	loginPruneEvery = 5 * time.Minute  // interval pembersihan memori
)

type loginAttemptRec struct {
	fails        []time.Time
	blockedUntil time.Time
}

var (
	loginMu     sync.Mutex
	loginBucket = map[string]*loginAttemptRec{}
)

func loginKey(ip, username string) string { return ip + "|" + username }

// LoginBlocked mengembalikan true bila IP+username sedang dikunci karena
// terlalu banyak percobaan login yang gagal.
func LoginBlocked(ip, username string) bool {
	loginMu.Lock()
	defer loginMu.Unlock()
	rec := loginBucket[loginKey(ip, username)]
	if rec == nil {
		return false
	}
	return time.Now().Before(rec.blockedUntil)
}

// RecordLoginFail mencatat percobaan login gagal. Bila jumlah kegagalan dalam
// jendela loginWindow mencapai loginMaxFails, akses login dikunci.
func RecordLoginFail(ip, username string) {
	now := time.Now()
	loginMu.Lock()
	defer loginMu.Unlock()

	key := loginKey(ip, username)
	rec := loginBucket[key]
	if rec == nil {
		rec = &loginAttemptRec{}
		loginBucket[key] = rec
	}
	rec.fails = append(rec.fails, now)

	// Buang percobaan yang berada di luar jendela.
	cut := now.Add(-loginWindow)
	i := 0
	for i < len(rec.fails) && rec.fails[i].Before(cut) {
		i++
	}
	rec.fails = rec.fails[i:]

	if len(rec.fails) >= loginMaxFails {
		rec.blockedUntil = now.Add(loginBlockFor)
	}
}

// ResetLoginAttempts menghapus catatan percobaan gagal saat login berhasil.
func ResetLoginAttempts(ip, username string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	delete(loginBucket, loginKey(ip, username))
}

// StartupApiRateLimitPruner membersihkan bucket rate limit API yang sudah
// tidak dipakai agar memori tidak membengkak. Panggil sekali dari main().
func StartApiRateLimitPruner() {
	go func() {
		t := time.NewTicker(loginPruneEvery)
		defer t.Stop()
		for range t.C {
			apiMu.Lock()
			cut := time.Now().Add(-2 * time.Minute)
			for k, arr := range apiBucket {
				if len(arr) == 0 || arr[len(arr)-1].Before(cut) {
					delete(apiBucket, k)
				}
			}
			apiMu.Unlock()
		}
	}()
}

// --- Rate limit endpoint publik (jendela geser per menit per IP) ---

var (
	apiMu     sync.Mutex
	apiBucket = map[string][]time.Time{}
)

// RateLimit membatasi jumlah permintaan per IP dalam jendela 1 menit
// (sliding window). Dipakai pada endpoint publik seperti POST /api/sensor.
func RateLimit(maxPerMinute int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxPerMinute <= 0 {
			c.Next()
			return
		}
		key := c.FullPath() + "|" + c.ClientIP()
		now := time.Now()

		apiMu.Lock()
		cut := now.Add(-time.Minute)
		arr := apiBucket[key]
		i := 0
		for i < len(arr) && arr[i].Before(cut) {
			i++
		}
		arr = arr[i:]

		if len(arr) >= maxPerMinute {
			apiMu.Unlock()
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Terlalu banyak permintaan. Coba lagi nanti."})
			c.Abort()
			return
		}

		apiBucket[key] = append(arr, now)
		apiMu.Unlock()
		c.Next()
	}
}

// StartLoginRateLimiterPruner membersihkan catatan lama secara berkala agar
// memori tidak membengkak. Panggil sekali dari main().
func StartLoginRateLimiterPruner() {
	go func() {
		t := time.NewTicker(loginPruneEvery)
		defer t.Stop()
		for range t.C {
			loginMu.Lock()
			now := time.Now()
			for k, rec := range loginBucket {
				blockedNow := rec.blockedUntil.After(now)
				tail := time.Time{}
				if n := len(rec.fails); n > 0 {
					tail = rec.fails[n-1]
				}
				expired := tail.Before(now.Add(-loginWindow))
				if !blockedNow && expired {
					delete(loginBucket, k)
				}
			}
			loginMu.Unlock()
		}
	}()
}
