package mqtt

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"sistem-monitoring-cod_golang/config"
	"sistem-monitoring-cod_golang/controllers"
	"sistem-monitoring-cod_golang/models"

	"github.com/eclipse/paho.mqtt.golang"
	"gorm.io/gorm"
)

// Kunci penyimpanan pengaturan MQTT (tabel settings). Bisa diubah lewat panel
// admin tanpa restart; diinisialisasi dari default .env saat server pertama
// kali dijalankan.
const (
	settingEnabled     = "mqtt_enabled"
	settingBroker      = "mqtt_broker"
	settingProtocol    = "mqtt_protocol"
	settingWSPath      = "mqtt_ws_path"
	settingTopicPrefix = "mqtt_topic_prefix"
	settingUsername    = "mqtt_username"
	settingPassword    = "mqtt_password"
)

// Settings menyimpan konfigurasi MQTT aktif (dibaca dari database).
type Settings struct {
	Enabled      bool
	Broker       string // host:port saja (mis. "shelter.cbinstrument.com:1883")
	Protocol     string // "tcp" | "ws"
	WSPath       string // "/mqtt"
	TopicPrefix  string
	Username     string
	Password     string
}

var (
	mu            sync.Mutex
	client        mqtt.Client
	stopCh        chan struct{}
	settings      Settings
	connected     bool
	lastConnected time.Time
	lastError     string
)

// getSetting membaca satu nilai setting dari database dengan fallback default.
func getSetting(key, def string) string {
	var s models.Setting
	err := config.DB.Where("`key` = ?", key).First(&s).Error
	if err != nil || strings.TrimSpace(s.Value) == "" {
		return def
	}
	return s.Value
}

// LoadSettings membaca pengaturan MQTT dari database. Nilai yang belum pernah
// disimpan memakai default dari config (yang bersumber dari .env).
func LoadSettings() Settings {
	return Settings{
		Enabled:     getSetting(settingEnabled, "false") == "true",
		Broker:      getSetting(settingBroker, config.Cfg.MQTTBroker),
		Protocol:    getSetting(settingProtocol, config.Cfg.MQTTProtocol),
		WSPath:      getSetting(settingWSPath, config.Cfg.MQTTWSPath),
		TopicPrefix: strings.Trim(getSetting(settingTopicPrefix, config.Cfg.MQTTTopicPrefix), "/"),
		Username:    getSetting(settingUsername, config.Cfg.MQTTUsername),
		Password:    getSetting(settingPassword, config.Cfg.MQTTPassword),
	}
}

// buildBrokerURL membangun URL broker lengkap berdasarkan protocol.
// TCP    -> tcp://host:port
// WS     -> ws://host:port/path   (tanpa TLS)
// WSS    -> wss://host:port/path  (dengan TLS)
func buildBrokerURL(s Settings) string {
	host := s.Broker // host:port tanpa scheme
	switch s.Protocol {
	case "ws", "wss":
		path := s.WSPath
		if path == "" {
			path = "/mqtt"
		}
		return fmt.Sprintf("%s://%s%s", s.Protocol, host, path)
	default: // tcp
		return fmt.Sprintf("tcp://%s", host)
	}
}

// SaveSettings menyimpan pengaturan MQTT ke database (dipakai panel admin).
func SaveSettings(s Settings) error {
	vals := map[string]string{
		settingEnabled:     strconv.FormatBool(s.Enabled),
		settingBroker:      s.Broker,
		settingProtocol:    s.Protocol,
		settingWSPath:      s.WSPath,
		settingTopicPrefix: s.TopicPrefix,
		settingUsername:    s.Username,
		settingPassword:    s.Password,
	}
	for k, v := range vals {
		var rec models.Setting
		err := config.DB.Where("`key` = ?", k).First(&rec).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := config.DB.Create(&models.Setting{Key: k, Value: v}).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		rec.Value = v
		if err := config.DB.Save(&rec).Error; err != nil {
			return err
		}
	}
	return nil
}

// IsEnabled mengembalikan status aktif MQTT berdasarkan database.
func IsEnabled() bool {
	return LoadSettings().Enabled
}

// GetStatus mengembalikan status koneksi MQTT saat ini.
func GetStatus() map[string]interface{} {
	mu.Lock()
	defer mu.Unlock()

	status := map[string]interface{}{
		"enabled":       settings.Enabled,
		"connected":     connected,
		"broker":        settings.Broker,
		"last_connected": lastConnected,
		"last_error":    lastError,
	}

	if connected {
		status["status"] = "connected"
	} else if settings.Enabled {
		status["status"] = "disconnected"
	} else {
		status["status"] = "disabled"
	}

	return status
}

// Start meluncurkan subscriber MQTT di background. Aman dipanggil berulang
// kali. Subscriber otomatis berhenti bila pengaturan nonaktif.
func Start() {
	mu.Lock()
	defer mu.Unlock()
	if stopCh != nil {
		return
	}
	s := LoadSettings()
	settings = s
	stopCh = make(chan struct{})
	go runSubscriber(s, stopCh)
}

// Stop menghentikan subscriber MQTT dan memutus koneksi.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if stopCh != nil {
		close(stopCh)
		stopCh = nil
	}
	if client != nil {
		client.Disconnect(250)
		client = nil
	}
	connected = false
	lastError = "Subscriber dihentikan"
}

// Restart menerapkan ulang pengaturan MQTT (dipanggil setelah SaveSettings).
func Restart() {
	Stop()
	Start()
}

func runSubscriber(s Settings, stop <-chan struct{}) {
	if !s.Enabled || s.Broker == "" {
		log.Println("[MQTT] Subscriber nonaktif — sensor MQTT tidak diproses.")
		return
	}

	// Gunakan topic yang diisi user secara langsung, tambahkan wildcard jika belum ada
	topic := s.TopicPrefix
	if !strings.HasSuffix(topic, "#") {
		topic = topic + "/#"
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(buildBrokerURL(s))
	opts.SetClientID("cod-monitor-" + shortClientID())
	opts.SetCleanSession(false)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetConnectTimeout(10 * time.Second)  // Prevent hanging on connect
	opts.SetKeepAlive(30 * time.Second)       // Maintain connection health
	if s.Username != "" {
		opts.SetUsername(s.Username)
		opts.SetPassword(s.Password)
	}
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		mu.Lock()
		connected = true
		lastConnected = time.Now()
		lastError = ""
		mu.Unlock()
		if t := c.Subscribe(topic, 0, handleMessage); t.Wait() && t.Error() != nil {
			log.Printf("[MQTT] Gagal subscribe %q: %v", topic, t.Error())
			return
		}
	})

	c := mqtt.NewClient(opts)
	mu.Lock()
	client = c
	mu.Unlock()

	brokerURL := buildBrokerURL(s)
	log.Printf("[MQTT] Mencoba koneksi ke %s...", brokerURL)
	// Set connection timeout untuk WebSocket
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		mu.Lock()
		connected = false
		lastError = err.Error()
		mu.Unlock()
		log.Printf("[MQTT] Koneksi terputus ke %s: %v", brokerURL, err)
	})

	if t := c.Connect(); t.Wait() && t.Error() != nil {
		errMsg := t.Error().Error()
		mu.Lock()
		connected = false
		lastError = errMsg
		mu.Unlock()
		log.Printf("[MQTT] Gagal menghubungkan ke %s: %v", brokerURL, errMsg)
		return
	}
	mu.Lock()
	connected = true
	lastConnected = time.Now()
	lastError = ""
	mu.Unlock()
	log.Printf("[MQTT] Terhubung ke %s — subscribe %q", brokerURL, topic)

	<-stop
	c.Disconnect(250)
	log.Println("[MQTT] Subscriber dihentikan.")
}

// handleMessage menerima pesan MQTT dari topik shelter/SHELTER-01/sensors
// yang berisi SEMUA sensor dalam satu payload. Format yang didukung:
// 1. { "sensors": [ {"sensor_type": "temperature", "value": 28.5}, ... ] }
// 2. { "temperature": 28.5, "humidity": 65, "air_quality": 45, "light_level": 1200 }
func handleMessage(_ mqtt.Client, m mqtt.Message) {
	var payload map[string]any
	if err := json.Unmarshal(m.Payload(), &payload); err != nil {
		log.Printf("[MQTT] Payload bukan JSON valid: %s", string(m.Payload()))
		return
	}

	// Format 1: { "sensors": [ {...}, {...} ] }
	if sensorsArr, ok := payload["sensors"].([]any); ok {
		processSensorsArray(sensorsArr)
		return
	}

	// Format 2: flat object { "temperature": 28.5, "humidity": 65 }
	processFlatObject(payload)
}

func getKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func processSensorsArray(arr []any) {
	log.Printf("[MQTT] Menerima format array dengan %d item", len(arr))
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		sensorType, _ := obj["sensor_type"].(string)
		val, ok := toFloat64(obj["value"])
		if !ok || sensorType == "" {
			continue
		}
		log.Printf("[MQTT] Array item: sensor_type='%s', value=%.2f", sensorType, val)
		saveByExternalKey(sensorType, val)
	}
}

func processFlatObject(obj map[string]any) {
	// Debug log untuk melihat semua keys yang diterima
	keys := getKeys(obj)
	log.Printf("[MQTT] Payload flat object berisi keys: %v", keys)

	for key, val := range obj {
		f, ok := toFloat64(val)
		if !ok {
			continue
		}
		saveByExternalKey(key, f)
	}
}

func toFloat64(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func saveByExternalKey(externalKey string, value float64) {
	// Debug log untuk melihat key yang diterima
	log.Printf("[MQTT] Menerima key: '%s' dengan value: %.2f", externalKey, value)

	var st models.SensorType
	err := config.DB.Where("external_key = ? AND sumber = ? AND aktif = ?", externalKey, "mqtt", true).First(&st).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[MQTT] Gagal cari sensor external_key=%s: %v", externalKey, err)
		} else {
			log.Printf("[MQTT] Sensor dengan external_key='%s' tidak ditemukan (sumber=mqtt, aktif=true)", externalKey)
		}
		return
	}
	if err := controllers.SaveSensorReadingMQTT(st.ID, value); err != nil {
		log.Printf("[MQTT] Gagal simpan %s: %v", st.Nama, err)
		return
	}
	log.Printf("[MQTT] %s = %.2f %s (external_key=%s)", st.Nama, value, st.Unit, externalKey)
}

func shortClientID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "0000"
	}
	return hex.EncodeToString(b)
}