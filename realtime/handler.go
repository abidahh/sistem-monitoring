package realtime

import (
	"log"
	"net/http"
	"time"

	"sistem-monitoring-cod_golang/config"
	"sistem-monitoring-cod_golang/models"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// upgrader mengizinkan semua origin: otorisasi tetap dijaga oleh session
// cookie yang dibawa pada handshake, bukan oleh Origin header.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 30 * time.Second
)

// allowedSensorTypeIDs menghitung ID sensor yang boleh dilihat user, sama
// seperti currentUserSensorTypeIDs di controllers (tanpa memperkenalkan
// siklus impor).
func allowedSensorTypeIDs(c *gin.Context) []uint {
	session := sessions.Default(c)
	userID, ok := session.Get("user_id").(uint)
	if !ok {
		return nil
	}

	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		return nil
	}

	if user.Role == "admin" {
		var types []models.SensorType
		config.DB.Where("aktif = ?", true).Find(&types)
		ids := make([]uint, 0, len(types))
		for _, t := range types {
			ids = append(ids, t.ID)
		}
		return ids
	}

	var assignments []models.UserSensor
	config.DB.Where("user_id = ?", userID).Find(&assignments)
	ids := make([]uint, 0, len(assignments))
	for _, a := range assignments {
		ids = append(ids, a.SensorTypeID)
	}
	return ids
}

// WSHandler menangani upgrade WebSocket dan melayani koneksi dashboard.
// Daftarkan di rute ber-AuthRequired.
func WSHandler(c *gin.Context) {
	ids := allowedSensorTypeIDs(c)
	if len(ids) == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Tidak ada sensor yang dapat diakses."})
		c.Abort()
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WS] Upgrade /ws gagal dari %s: %v", c.ClientIP(), err)
		return
	}
	log.Printf("[WS] Koneksi baru dari %s (akses %d sensor)", c.ClientIP(), len(ids))

	cl := &Client{
		send:       make(chan []byte, 16),
		allowedIDs: ids,
		closed:     make(chan struct{}),
	}
	HubInstance.Register(cl, ids)

	go cl.writePump(conn)
	cl.readPump(conn)
}

func (cl *Client) readPump(conn *websocket.Conn) {
	defer func() {
		conn.Close()
		HubInstance.Unregister(cl)
	}()
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error { conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		// Tidak ada pesan masuk yang berarti dari dashboard; baca untuk
		// menjaga keepalive dan mendeteksi koneksi yang tertutup.
		if _, _, err := conn.ReadMessage(); err != nil {
			log.Printf("[WS] Koneksi ditutup: %v", err)
			return
		}
	}
}

func (cl *Client) writePump(conn *websocket.Conn) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()
	for {
		select {
		case msg, ok := <-cl.send:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-cl.closed:
			return
		}
	}
}

