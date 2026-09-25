package realtime

import (
	"encoding/json"
	"sync"
	"time"
)

// Event adalah satu pesan yang disiarkan ke dashboard. Sesuaikan struktur
// JSON dengan apa yang dibutuhkan halaman dashboard.
type Event struct {
	Type               string    `json:"type"`
	SensorTypeID       uint      `json:"sensor_type_id"`
	Value              float64   `json:"value,omitempty"`
	CreatedAt          time.Time `json:"created_at,omitempty"`
	IsAnomaly          bool      `json:"is_anomaly,omitempty"`
	ThresholdExceeded  bool      `json:"threshold_exceeded,omitempty"`
	NotificationID     uint      `json:"notification_id,omitempty"`
	NotificationTipe   string    `json:"notification_tipe,omitempty"`
	NotificationPesan  string    `json:"notification_pesan,omitempty"`
	NotificationNilai  float64   `json:"notification_nilai,omitempty"`
}

// Client mewakili satu koneksi WebSocket. Hanya menerima event untuk
// sensor yang diizinkan (allowedIDs).
type Client struct {
	send       chan []byte
	allowedIDs []uint
	closeOnce  sync.Once
	closed     chan struct{}
}

// Hub menyiarkan event ke semua klien yang berhak menerimanya.
type Hub struct {
	mu        sync.RWMutex
	clients   map[*Client][]uint
	broadcast chan Event
}

var HubInstance = NewHub()

func NewHub() *Hub {
	return &Hub{
		clients:   make(map[*Client][]uint),
		broadcast: make(chan Event, 128),
	}
}

// Run memproses event broadcast dan mengirimkannya ke klien yang cocok.
// Jalankan sebagai goroutine saat startup.
func (h *Hub) Run() {
	for ev := range h.broadcast {
		payload, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		h.mu.RLock()
		for cl, ids := range h.clients {
			if !contains(ids, ev.SensorTypeID) {
				continue
			}
			select {
			case cl.send <- payload:
			default:
				// Saluran penuh: klien lambat. Tutup agar tidak menumpuk.
				go h.Unregister(cl)
			}
		}
		h.mu.RUnlock()
	}
}

func contains(ids []uint, id uint) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func (h *Hub) Register(cl *Client, allowedIDs []uint) {
	h.mu.Lock()
	h.clients[cl] = allowedIDs
	h.mu.Unlock()
}

func (h *Hub) Unregister(cl *Client) {
	h.mu.Lock()
	if _, ok := h.clients[cl]; ok {
		delete(h.clients, cl)
		cl.closeOnce.Do(func() { close(cl.closed) })
	}
	h.mu.Unlock()
}

// Broadcast mengirim event tanpa memblokir jika buffer penuh.
func (h *Hub) Broadcast(ev Event) {
	select {
	case h.broadcast <- ev:
	default:
	}
}

// PublishReading menyiarkan pembacaan sensor baru. Dipanggil dari
// controllers.Setelah data tersimpan ke database.
func PublishReading(sensorTypeID uint, value float64, createdAt time.Time, isAnomaly, thresholdExceeded bool) {
	HubInstance.Broadcast(Event{
		Type:              "reading",
		SensorTypeID:      sensorTypeID,
		Value:             value,
		CreatedAt:         createdAt,
		IsAnomaly:         isAnomaly,
		ThresholdExceeded: thresholdExceeded,
	})
}

// PublishNotification menyiarkan notifikasi ambang batas baru.
func PublishNotification(notificationID uint, sensorTypeID uint, tipe, pesan string, nilai float64, createdAt time.Time) {
	HubInstance.Broadcast(Event{
		Type:             "notification",
		SensorTypeID:     sensorTypeID,
		NotificationID:   notificationID,
		NotificationTipe: tipe,
		NotificationPesan: pesan,
		NotificationNilai: nilai,
		CreatedAt:        createdAt,
	})
}