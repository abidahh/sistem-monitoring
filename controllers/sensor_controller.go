package controllers

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sistem-monitoring-cod_golang/config"
	"sistem-monitoring-cod_golang/models"
	"sistem-monitoring-cod_golang/realtime"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	readInterval  = 3
	settingsMutex sync.RWMutex
	// externalKeyRe memvalidasi nama sensor / shelter_id dari URL agar aman
	// diteruskan ke API eksternal (mencegah injeksi path/query).
	externalKeyRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

	// Deteksi lonjakan: nilai dianggap lonjakan bila selisih dari bacaan
	// sebelumnya melebihi max(absMin, ratio x nilai sebelumnya).
	spikeEnabled   = true
	spikeJumpRatio = 0.5
	spikeAbsMin    = 5.0
)

var (
	simulatorStopCh  chan struct{}
	simulatorRunning bool
	simulatorMutex   sync.Mutex
	lastHardwareRead atomic.Int64
	hardwareEverSeen atomic.Bool
)

// NotifyHardwareActivity dipanggil oleh serial reader setiap kali berhasil
// membaca satu baris data hardware. Menandai bahwa hardware masih aktif,
// sehingga simulator menunda pembangkitan data selama waktu tertentu.
func NotifyHardwareActivity() {
	hardwareEverSeen.Store(true)
	lastHardwareRead.Store(time.Now().Unix())
}

// hardwareIdleFor memeriksa apakah sudah melewati HardwareInactiveTimeout
// sejak pembacaan hardware terakhir.
func hardwareIdleFor() bool {
	if !hardwareEverSeen.Load() {
		return false
	}
	last := lastHardwareRead.Load()
	if last == 0 {
		return false
	}
	return time.Since(time.Unix(last, 0)) > config.Cfg.HardwareInactiveTimeout
}

// StopSimulator menghentikan goroutine simulator secara permanen.
// Dipakai untuk menonaktifkan simulator sepenuhnya (misal saat shutdown).
func StopSimulator() {
	simulatorMutex.Lock()
	simulatorRunning = false
	simulatorMutex.Unlock()

	if simulatorStopCh != nil {
		select {
		case <-simulatorStopCh:
		default:
			close(simulatorStopCh)
		}
	}
}

func IsSimulatorRunning() bool {
	simulatorMutex.Lock()
	defer simulatorMutex.Unlock()
	return simulatorRunning
}

// Inisialisasi interval dari Database saat server dinyalakan
func InitIntervalSetting() {
	var settingInterval models.Setting
	if err := config.DB.Where("`key` = ?", "sensor_interval").First(&settingInterval).Error; err == nil {
		if val, err := strconv.Atoi(settingInterval.Value); err == nil && val > 0 {
			readInterval = val
		}
	} else {
		readInterval = config.Cfg.DefaultSensorInterval
		config.DB.Create(&models.Setting{Key: "sensor_interval", Value: strconv.Itoa(config.Cfg.DefaultSensorInterval)})
	}
}

// getSpikeDetectionConfig mengembalikan konfigurasi deteksi lonjakan saat ini.
func getSpikeDetectionConfig() (bool, float64, float64) {
	settingsMutex.RLock()
	defer settingsMutex.RUnlock()
	return spikeEnabled, spikeJumpRatio, spikeAbsMin
}

// InitSpikeSetting memuat pengaturan deteksi lonjakan dari DB saat start.
func InitSpikeSetting() {
	var s models.Setting
	if err := config.DB.Where("`key` = ?", "spike_detection_enabled").First(&s).Error; err == nil {
		settingsMutex.Lock()
		spikeEnabled = s.Value == "true"
		settingsMutex.Unlock()
	} else {
		saveOrUpdateSetting("spike_detection_enabled", "true")
	}

	if err := config.DB.Where("`key` = ?", "spike_jump_ratio").First(&s).Error; err == nil {
		if v, convErr := strconv.ParseFloat(s.Value, 64); convErr == nil && v > 0 {
			settingsMutex.Lock()
			spikeJumpRatio = v
			settingsMutex.Unlock()
		}
	} else {
		saveOrUpdateSetting("spike_jump_ratio", "0.5")
	}

	if err := config.DB.Where("`key` = ?", "spike_abs_min").First(&s).Error; err == nil {
		if v, convErr := strconv.ParseFloat(s.Value, 64); convErr == nil && v >= 0 {
			settingsMutex.Lock()
			spikeAbsMin = v
			settingsMutex.Unlock()
		}
	} else {
		saveOrUpdateSetting("spike_abs_min", "5")
	}
}

// isSpikeReading menilai apakah kenaikan/penurunan dari bacaan sebelumnya
// terlalu besar sehingga dianggap lonjakan (bukan perubahan normal).
func isSpikeReading(prev, value, ratio, absMin float64) bool {
	ref := absMin
	if v := prev * ratio; v > ref {
		ref = v
	}
	return math.Abs(value-prev) > ref
}

// detectAnomaly mencari bacaan terakhir tipe sensor lalu menilai apakah nilai
// baru merupakan lonjakan. Tanpa bacaan sebelumnya, nilai tidak pernah dinilai
// lonjakan (bacaan pertama selalu diterima).
func detectAnomaly(sensorTypeID uint, value float64) bool {
	enabled, ratio, absMin := getSpikeDetectionConfig()
	if !enabled {
		return false
	}

	var prev models.SensorData
	err := config.DB.Where("sensor_type_id = ?", sensorTypeID).Order("created_at desc, id desc").First(&prev).Error
	if err != nil {
		return false
	}
	return isSpikeReading(prev.Value, value, ratio, absMin)
}

// 1. RECEIVE DATA DARI SENSOR / RASPBERRY PI (payload per-sensor via HTTP)
func StoreSensorData(c *gin.Context) {
	var input struct {
		SensorTypeID uint    `json:"sensor_type_id" binding:"required"`
		Value        float64 `json:"value" binding:"required"`
		Sumber       string  `json:"sumber"` // "real" (default) atau "simulasi"
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Payload data sensor terlalu besar!"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "Format data sensor tidak valid! Kirim sensor_type_id dan value."})
		return
	}

	sumber := "real"
	if input.Sumber == "simulasi" {
		sumber = "simulasi"
	}

	thresholdExceeded, isAnomaly, err := recordSensorReading(input.SensorTypeID, input.Value, sumber)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":            "Data sensor berhasil disimpan!",
		"threshold_exceeded": thresholdExceeded,
		"is_anomaly":         isAnomaly,
	})
}

// StoreSimulatedData menyimpan pembacaan simulasi dari dashboard (sumber
// "simulasi"). Endpoint ini memakai verifikasi sesi login, bukan API key,
// sehingga key polos tidak perlu dibocorkan ke browser.
func StoreSimulatedData(c *gin.Context) {
	var input struct {
		SensorTypeID uint    `json:"sensor_type_id" binding:"required"`
		Value        float64 `json:"value" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Format data simulasi tidak valid! Kirim sensor_type_id dan value."})
		return
	}

	thresholdExceeded, isAnomaly, err := recordSensorReading(input.SensorTypeID, input.Value, "simulasi")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":            "Data simulasi berhasil disimpan!",
		"threshold_exceeded": thresholdExceeded,
		"is_anomaly":         isAnomaly,
	})
}

// SaveSensorReadingPublic adalah pembungkus ekspor untuk saveSensorReading,
// dipakai oleh pembaca hardware (Modbus/serial). Data diasumsikan real-time.
func SaveSensorReadingPublic(sensorTypeID uint, value float64) (bool, error) {
	return saveSensorReading(sensorTypeID, value, "real")
}

// SaveSensorReadingMQTT menyimpan bacaan yang datang dari subscriber MQTT
// (sumber "mqtt"). Perilaku sama dengan jalur lain: validasi, simpan, dan
// cek ambang batas.
func SaveSensorReadingMQTT(sensorTypeID uint, value float64) error {
	_, _, err := recordSensorReading(sensorTypeID, value, "mqtt")
	return err
}

// saveSensorReading memvalidasi tipe sensor, menyimpan pembacaan, dan mencatat
// notifikasi bila nilai melewati ambang batas. Diajak oleh SaveSensorReadingPublic.
func saveSensorReading(sensorTypeID uint, value float64, sumber string) (bool, error) {
	thresholdExceeded, _, err := recordSensorReading(sensorTypeID, value, sumber)
	return thresholdExceeded, err
}

// recordSensorReading memvalidasi tipe sensor, mendeteksi lonjakan, menyimpan
// pembacaan (tetap disimpan walau lonjakan), dan mencatat notifikasi bila nilai
// melewati ambang batas. Mengembalikan status threshold dan status lonjakan.
func recordSensorReading(sensorTypeID uint, value float64, sumber string) (bool, bool, error) {
	var st models.SensorType
	if err := config.DB.First(&st, sensorTypeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, false, fmt.Errorf("Tipe sensor tidak ditemukan!")
		}
		return false, false, fmt.Errorf("Gagal memuat tipe sensor!")
	}

	if !st.Aktif {
		return false, false, fmt.Errorf("Tipe sensor sedang nonaktif!")
	}

	isAnomaly := detectAnomaly(st.ID, value)

	data := models.SensorData{
		SensorTypeID: st.ID,
		Value:        value,
		Sumber:       sumber,
		IsAnomaly:    isAnomaly,
	}

	if err := config.DB.Create(&data).Error; err != nil {
		return false, false, fmt.Errorf("Gagal menyimpan data sensor ke database!")
	}

	// Cek threshold dan log notifikasi
	thresholdExceeded := false
	if st.NilaiMax > 0 && value > st.NilaiMax {
		thresholdExceeded = true
		msg := fmt.Sprintf("%s %.1f %s melebihi batas aman %.1f %s", st.Nama, value, st.Unit, st.NilaiMax, st.Unit)
		logNotification(st.Nama, st.ID, value, st.NilaiMax, msg)
	}

	// Siarkan pembacaan baru ke dashboard real-time (WebSocket).
	realtime.PublishReading(st.ID, value, data.CreatedAt, isAnomaly, thresholdExceeded)

	return thresholdExceeded, isAnomaly, nil
}

// 2. GET LATEST 20 DATA (SESUAI SENSOR YANG DITAMPILKAN KE USER)
func GetLatestSensorData(c *gin.Context) {
	ids, err := currentUserSensorTypeIDs(c)
	if err != nil || len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"data": []models.SensorData{}, "interval": GetCurrentInterval(), "sensor_types": []models.SensorType{}, "server_time": time.Now()})
		return
	}

	var sensorData []models.SensorData
	config.DB.Preload("SensorType").Where("sensor_type_id IN ?", ids).Order("created_at desc").Limit(20).Find(&sensorData)

	var types []models.SensorType
	config.DB.Where("id IN ? AND aktif = ?", ids, true).Find(&types)

	c.JSON(http.StatusOK, gin.H{
		"data":         sensorData,
		"interval":     GetCurrentInterval(),
		"sensor_types": types,
		"server_time":  time.Now(),
	})
}

// 3. GET CURRENT SETTINGS (INTERVAL SAJA; THRESHOLD PER SENSOR DIDAPAT DARI latest)
func GetIntervalSetting(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"interval": GetCurrentInterval(),
	})
}

// 4. UPDATE SETTINGS (INTERVAL)
func UpdateIntervalSetting(c *gin.Context) {
	var input struct {
		Interval int `json:"interval" binding:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Input pengaturan tidak valid!"})
		return
	}

	settingsMutex.Lock()
	readInterval = input.Interval
	settingsMutex.Unlock()

	saveOrUpdateSetting("sensor_interval", strconv.Itoa(input.Interval))

	c.JSON(http.StatusOK, gin.H{
		"message":  "Pengaturan berhasil diperbarui!",
		"interval": input.Interval,
	})
}

func saveOrUpdateSetting(key string, val string) {
	var setting models.Setting
	if err := config.DB.Where("`key` = ?", key).First(&setting).Error; err == nil {
		setting.Value = val
		config.DB.Save(&setting)
	} else {
		config.DB.Create(&models.Setting{Key: key, Value: val})
	}
}

func logNotification(tipe string, sensorTypeID uint, nilai, batas float64, pesan string) {
	logEntry := models.NotificationLog{
		SensorTypeID: sensorTypeID,
		Tipe:         tipe,
		Nilai:        nilai,
		Batas:        batas,
		Pesan:        pesan,
	}
	if err := config.DB.Create(&logEntry).Error; err != nil {
		log.Println("Gagal menyimpan notifikasi:", err)
		return
	}
	realtime.PublishNotification(logEntry.ID, sensorTypeID, tipe, pesan, nilai, logEntry.CreatedAt)
}

func GetNotificationLogs(c *gin.Context) {
	// Notifikasi dibatasi ke sensor yang tampil untuk user yang login, sama
	// seperti data sensor/history, agar riwayat tidak bocor antar user.
	ids, err := currentUserSensorTypeIDs(c)
	if err != nil || len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"logs": []models.NotificationLog{}})
		return
	}

	var logs []models.NotificationLog
	config.DB.Where("sensor_type_id IN ?", ids).Order("created_at desc").Limit(50).Find(&logs)
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// GET RIWAYAT SENSOR (FILTER TANGGAL OPSIONAL + PAGINATION)
func GetSensorHistory(c *gin.Context) {
	ids, err := currentUserSensorTypeIDs(c)
	if err != nil || len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"data": []models.SensorData{}, "total": 0, "page": 1, "limit": 50, "total_pages": 1})
		return
	}

	var sensorData []models.SensorData
	var total int64

	query := config.DB.Preload("SensorType").Model(&models.SensorData{}).
		Where("sensor_type_id IN ?", ids).Order("created_at desc")

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	if startDate != "" && endDate != "" {
		start, errStart := time.Parse("2006-01-02", startDate)
		end, errEnd := time.Parse("2006-01-02", endDate)
		if errStart == nil && errEnd == nil {
			query = query.Where("created_at >= ? AND created_at < ?", start, end.AddDate(0, 0, 1))
		}
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}

	query.Count(&total)
	query.Offset((page - 1) * limit).Limit(limit).Find(&sensorData)

	totalPages := (total + int64(limit) - 1) / int64(limit)

	c.JSON(http.StatusOK, gin.H{
		"data":        sensorData,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
	})
}

// GET STATISTIK PER TIPE SENSOR (MIN, MAX, AVG) SESUAI USER
type sensorStats struct {
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Avg   float64 `json:"avg"`
	Total int64   `json:"count"`
}

// GetSensorDataDebug endpoint untuk debugging data sensor via terminal
func GetSensorDataDebug(c *gin.Context) {
	var sensorData []models.SensorData
	config.DB.Preload("SensorType").Order("created_at desc").Limit(20).Find(&sensorData)

	debugData := make([]map[string]interface{}, 0, len(sensorData))
	for _, data := range sensorData {
		debugData = append(debugData, map[string]interface{}{
			"id":           data.ID,
			"sensor_name":  data.SensorType.Nama,
			"sensor_unit":  data.SensorType.Unit,
			"sensor_source": data.SensorType.Sumber,
			"external_key": data.SensorType.ExternalKey,
			"value":        data.Value,
			"data_source":  data.Sumber,
			"is_anomaly":   data.IsAnomaly,
			"created_at":   data.CreatedAt,
			"time_ago":     time.Since(data.CreatedAt).String(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"total_count": len(debugData),
		"data":        debugData,
		"server_time": time.Now(),
	})
}

func GetSensorStats(c *gin.Context) {
	ids, err := currentUserSensorTypeIDs(c)
	if err != nil || len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"stats": []gin.H{}})
		return
	}

	var types []models.SensorType
	config.DB.Where("id IN ?", ids).Find(&types)

	stats := make([]gin.H, 0, len(types))
	for _, t := range types {
		var s sensorStats
		config.DB.Raw(
			"SELECT COALESCE(MIN(value),0) as min, COALESCE(MAX(value),0) as max, COALESCE(AVG(value),0) as avg, COUNT(*) as total FROM sensor_data WHERE sensor_type_id = ?",
			t.ID).Scan(&s)
		stats = append(stats, gin.H{
			"sensor_type": t,
			"min":         s.Min,
			"max":         s.Max,
			"avg":         s.Avg,
			"count":       s.Total,
		})
	}

	c.JSON(http.StatusOK, gin.H{"stats": stats})
}

func GetCurrentInterval() int {
	settingsMutex.RLock()
	defer settingsMutex.RUnlock()
	return readInterval
}

// SIMULATOR OTOMATIS
func nextSimValue(current, maxStep, min, max float64) float64 {
	delta := (rand.Float64()*2 - 1) * maxStep
	next := current + delta
	if next < min {
		next = min
	}
	if next > max {
		next = max
	}
	return next
}

func StartSimulator() {
	InitIntervalSetting()

	simulatorMutex.Lock()
	simulatorStopCh = make(chan struct{})
	simulatorRunning = true
	simulatorMutex.Unlock()

	go func() {
		for {
			select {
			case <-simulatorStopCh:
				log.Println("Simulator otomatis dihentikan.")
				return
			default:
			}

			// Jika hardware baru saja mengirim data, simulator "menyerah" dan
			// tidak membangkitkan data agar tidak dobel. Simulator akan
			// otomatis kembali aktif bila hardware diam > HardwareInactiveTimeout.
			if !hardwareIdleFor() && hardwareEverSeen.Load() {
				log.Println("Simulator dijeda: data hardware aktif.")
				time.Sleep(config.Cfg.HardwareInactiveTimeout)
				continue
			}

			interval := GetCurrentInterval()
			time.Sleep(time.Duration(interval) * time.Second)

			// Simulasi semua tipe sensor aktif yang bersumber 'simulasi'
			var types []models.SensorType
			config.DB.Where("aktif = ? AND sumber = ?", true, "simulasi").Find(&types)

			if len(types) == 0 {
				continue
			}

			for _, st := range types {
				// Nilai dasar simulasi: sekitar nilai_max, atau range umum
				base := st.NilaiMax
				if base <= 0 {
					base = 50.0
				}
				min := base * 0.5
				max := base * 1.3
				step := base * 0.03
				if step < 0.1 {
					step = 0.1
				}

				simVal := nextSimValue(base, step, min, max)
				data := models.SensorData{
					SensorTypeID: st.ID,
					Value:        float64(int(simVal*10)) / 10,
					Sumber:       "simulasi",
					IsAnomaly:    detectAnomaly(st.ID, float64(int(simVal*10))/10),
				}

				if err := config.DB.Create(&data).Error; err != nil {
					log.Println("Simulator gagal menyimpan data sensor:", err)
					continue
				}

				// Cek threshold di simulator juga
				if st.NilaiMax > 0 && data.Value > st.NilaiMax {
					logNotification(st.Nama, st.ID, data.Value, st.NilaiMax,
						fmt.Sprintf("%s %.1f %s melebihi batas aman %.1f %s", st.Nama, data.Value, st.Unit, st.NilaiMax, st.Unit))
				}
			}
		}
	}()
}

// RATA-RATA (JENDELA DINAMIS)
// ================================================================

// averageWindowMinutes adalah ukuran jendela rata-rata dalam menit.
// Nilai di-load dari DB saat start dan bisa diubah admin (1-120).
var averageWindowMinutes = 5

const averageMaxMinutes = 120

// GetCurrentAverageWindow mengembalikan jendela rata-rata saat ini (menit).
func GetCurrentAverageWindow() int {
	settingsMutex.RLock()
	defer settingsMutex.RUnlock()
	return averageWindowMinutes
}

// InitAverageSetting memuat pengaturan jendela rata-rata dari DB saat start.
func InitAverageSetting() {
	var s models.Setting
	if err := config.DB.Where("`key` = ?", "average_window_minutes").First(&s).Error; err == nil {
		if val, err := strconv.Atoi(s.Value); err == nil && val >= 1 && val <= averageMaxMinutes {
			settingsMutex.Lock()
			averageWindowMinutes = val
			settingsMutex.Unlock()
			return
		}
	}
	settingsMutex.Lock()
	averageWindowMinutes = config.Cfg.DefaultAverageWindowMinutes
	settingsMutex.Unlock()
	saveOrUpdateSetting("average_window_minutes", strconv.Itoa(averageWindowMinutes))
}

// GetAverageSetting mengembalikan jendela rata-rata untuk UI Pengaturan.
func GetAverageSetting(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"window_minutes": GetCurrentAverageWindow()})
}

// UpdateAverageSetting menyimpan jendela rata-rata baru (khusus admin).
func UpdateAverageSetting(c *gin.Context) {
	var input struct {
		WindowMinutes int `json:"window_minutes" binding:"required,min=1,max=120"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.WindowMinutes < 1 || input.WindowMinutes > averageMaxMinutes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Interval rata-rata harus antara 1 sampai 120 menit!"})
		return
	}

	settingsMutex.Lock()
	averageWindowMinutes = input.WindowMinutes
	settingsMutex.Unlock()
	saveOrUpdateSetting("average_window_minutes", strconv.Itoa(input.WindowMinutes))

	c.JSON(http.StatusOK, gin.H{
		"message":        "Jendela rata-rata berhasil diperbarui!",
		"window_minutes": input.WindowMinutes,
	})
}

// ================================================================
// PERSISTENSI RATA-RATA (TABEL sensor_averages)
// ================================================================

const (
	avgSchedulerTick    = 60 * time.Second
	avgBackfillBuckets  = 12 // jumlah bucket terakhir yang diisi saat start
)

// closedWindowStart mengembalikan awal bucket jendela penuh yang tepat sudah
// lewat (tutup), sejajar ke epoch detik — boundary identik dengan
// bucket5SQLExpr agar konsisten dengan riwayat live.
func closedWindowStart(now time.Time, windowSecs int) time.Time {
	trunc := now.Unix() - (now.Unix() % int64(windowSecs))
	return time.Unix(trunc-int64(windowSecs), 0).UTC()
}

// persistAverageBucket menghitung rata-rata sensor_data pada satu bucket
// (is_anomaly=false) lalu menyimpannya ke sensor_averages (upsert).
func persistAverageBucket(sensorTypeID uint, windowStart time.Time, windowSecs int) error {
	windowEnd := windowStart.Add(time.Duration(windowSecs) * time.Second)
	var avg float64
	var cnt int64
	if err := config.DB.Model(&models.SensorData{}).
		Where("sensor_type_id = ? AND created_at >= ? AND created_at < ? AND is_anomaly = ?",
			sensorTypeID, windowStart, windowEnd, false).
		Select("COALESCE(AVG(value),0), COUNT(*)").Row().Scan(&avg, &cnt); err != nil {
		return err
	}
	if cnt == 0 {
		return nil // bucket tanpa data tidak perlu disimpan
	}

	rec := models.SensorAverage{
		SensorTypeID: sensorTypeID,
		WindowStart:  windowStart,
		WindowSecs:   windowSecs,
		Average:      math.Round(avg*10) / 10,
		Total:        cnt,
	}
	return config.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "sensor_type_id"}, {Name: "window_start"}, {Name: "window_secs"}},
		DoUpdates: clause.AssignmentColumns([]string{"average", "total", "updated_at"}),
	}).Create(&rec).Error
}

// persistLatestClosedBuckets menulis rata-rata untuk sejumlah bucket penuh
// terakhir (dihitung mundur dari waktu sekarang) untuk semua tipe sensor
// yang pernah punya data.
func persistLatestClosedBuckets(buckets int) {
	windowSecs := GetCurrentAverageWindow() * 60
	var ids []uint
	if err := config.DB.Model(&models.SensorData{}).Distinct().Pluck("sensor_type_id", &ids).Error; err != nil || len(ids) == 0 {
		return
	}

	now := time.Now()
	persisted := 0
	for _, id := range ids {
		for i := 0; i < buckets; i++ {
			start := closedWindowStart(now.Add(-time.Duration(i)*time.Duration(windowSecs)*time.Second), windowSecs)
			if err := persistAverageBucket(id, start, windowSecs); err != nil {
				log.Printf("[AVG] Gagal persist sensor #%d bucket %s: %v", id, start.Format(time.RFC3339), err)
				continue
			}
			persisted++
		}
	}
	log.Printf("[AVG] Persist %d bucket rata-rata selesai (jendela %d menit, %d sensor)",
		persisted, windowSecs/60, len(ids))
}

// StartAverageScheduler menulis rata-rata bucket tersimpan setiap menit dari
// sensor_data, tanpa mengubah jalur tampilan (kartu & riwayat tetap live).
func StartAverageScheduler() {
	go func() {
		time.Sleep(3 * time.Second) // tunggu seed & koneksi DB stabil
		persistLatestClosedBuckets(avgBackfillBuckets)
		ticker := time.NewTicker(avgSchedulerTick)
		defer ticker.Stop()
		for range ticker.C {
			persistLatestClosedBuckets(1)
		}
	}()
}

// GetSpikeSetting mengembalikan konfigurasi deteksi lonjakan untuk UI admin.
func GetSpikeSetting(c *gin.Context) {
	enabled, ratio, absMin := getSpikeDetectionConfig()
	c.JSON(http.StatusOK, gin.H{
		"enabled":    enabled,
		"jump_ratio": ratio,
		"abs_min":    absMin,
	})
}

// UpdateSpikeSetting menyimpan konfigurasi deteksi lonjakan (khusus admin).
func UpdateSpikeSetting(c *gin.Context) {
	var input struct {
		Enabled   *bool    `json:"enabled"`
		JumpRatio *float64 `json:"jump_ratio"`
		AbsMin    *float64 `json:"abs_min"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Format input tidak valid!"})
		return
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	ratio := spikeJumpRatio
	if input.JumpRatio != nil && *input.JumpRatio > 0 {
		ratio = *input.JumpRatio
	}
	absMin := spikeAbsMin
	if input.AbsMin != nil && *input.AbsMin >= 0 {
		absMin = *input.AbsMin
	}

	settingsMutex.Lock()
	spikeEnabled = enabled
	spikeJumpRatio = ratio
	spikeAbsMin = absMin
	settingsMutex.Unlock()

	saveOrUpdateSetting("spike_detection_enabled", strconv.FormatBool(enabled))
	saveOrUpdateSetting("spike_jump_ratio", strconv.FormatFloat(ratio, 'f', -1, 64))
	saveOrUpdateSetting("spike_abs_min", strconv.FormatFloat(absMin, 'f', -1, 64))

	c.JSON(http.StatusOK, gin.H{
		"message":    "Pengaturan deteksi lonjakan berhasil diperbarui!",
		"enabled":    enabled,
		"jump_ratio": ratio,
		"abs_min":    absMin,
	})
}

// bucket5SQLExpr menghasilkan ekspresi SQL yang membulatkan created_at ke awal
// kotak jendela rata-rata (windowSecs detik). Dipakai untuk agregasi riwayat.
// Dialek-aware: SQLite pakai strftime, MySQL pakai UNIX_TIMESTAMP, agar kedua
// database memberikan hasil yang setara (bucket sejajar epoch detik UTC).
func bucket5SQLExpr(windowSecs int) string {
	if config.DBIsMySQL() {
		return "FROM_UNIXTIME(UNIX_TIMESTAMP(created_at) - (UNIX_TIMESTAMP(created_at) % " + strconv.Itoa(windowSecs) + "))"
	}
	return "datetime((strftime('%s', created_at) - (strftime('%s', created_at) % " + strconv.Itoa(windowSecs) + ")), 'unixepoch')"
}

// GetAverage5Latest menghitung rata-rata jendela terakhir per tipe sensor
// untuk kartu "Rata-rata" di halaman utama.
func GetAverage5Latest(c *gin.Context) {
	windowMin := GetCurrentAverageWindow()
	ids, err := currentUserSensorTypeIDs(c)
	if err != nil || len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"averages": []gin.H{}, "window_minutes": windowMin, "sensor_types": []models.SensorType{}})
		return
	}

	var types []models.SensorType
	config.DB.Where("id IN ? AND aktif = ?", ids, true).Find(&types)
	since := time.Now().Add(-time.Duration(windowMin) * time.Minute)

	averages := make([]gin.H, 0, len(types))
	for _, t := range types {
		var avg float64
		var cnt int64
		config.DB.Model(&models.SensorData{}).
			Where("sensor_type_id = ? AND created_at >= ? AND is_anomaly = ?", t.ID, since, false).
			Select("COALESCE(AVG(value),0), COUNT(*)").
			Row().Scan(&avg, &cnt)

		averages = append(averages, gin.H{
			"sensor_type_id": t.ID,
			"nama":           t.Nama,
			"unit":           t.Unit,
			"nilai_max":      t.NilaiMax,
			"warna":          t.Warna,
			"average":        math.Round(avg*10) / 10,
			"count":          cnt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"window_minutes": windowMin,
		"start":          since,
		"sensor_types":   types,
		"averages":       averages,
	})
}

type averageItem struct {
	SensorTypeID uint    `json:"sensor_type_id"`
	Average      float64 `json:"average"`
	Total        int64   `json:"total"`
}

type averageBucket struct {
	BucketTime time.Time     `json:"bucket_time"`
	Averages   []averageItem `json:"averages"`
}

// GetAverage5History mengembalikan riwayat rata-rata per kotak jendela
// dinamis (menit), dipaginasi (dan bisa difilter tanggal), sesuai sensor user.
func GetAverage5History(c *gin.Context) {
	windowMin := GetCurrentAverageWindow()
	windowSecs := windowMin * 60
	bucketSQL := bucket5SQLExpr(windowSecs)

	ids, err := currentUserSensorTypeIDs(c)
	if err != nil || len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"data": []averageBucket{}, "sensor_types": []models.SensorType{}, "window_minutes": windowMin, "total": 0, "page": 1, "limit": 50, "total_pages": 1})
		return
	}

	var types []models.SensorType
	config.DB.Where("id IN ? AND aktif = ?", ids, true).Find(&types)

	ph := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	// Lonjakan (is_anomaly = 1) tetap disimpan untuk riwayat, tapi tidak
	// dihitung ke rata-rata.
	where := "sensor_type_id IN (" + ph + ") AND is_anomaly = 0"
	args := make([]any, 0, len(ids)+2)
	for _, id := range ids {
		args = append(args, id)
	}

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	if startDate != "" && endDate != "" {
		start, errS := time.Parse("2006-01-02", startDate)
		end, errE := time.Parse("2006-01-02", endDate)
		if errS == nil && errE == nil {
			where += " AND created_at >= ? AND created_at < ?"
			args = append(args, start, end.AddDate(0, 0, 1))
		}
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	offset := (page - 1) * limit

	// Jumlah total kotak waktu (untuk pagination)
	countArgs := append([]any{}, args...)
	var total int64
	config.DB.Raw(
		"SELECT COUNT(*) FROM (SELECT "+bucketSQL+" AS bucket_time FROM sensor_data WHERE "+where+" GROUP BY bucket_time) AS t",
		countArgs...,
	).Scan(&total)

	// Baris agregat per (kotak, tipe sensor) dengan pagination
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, limit, offset)
	rows := []struct {
		SensorTypeID uint
		AvgValue     float64
		Total        int64
		BucketTime   string
	}{}
	config.DB.Raw(
		"SELECT sensor_type_id, AVG(value) AS avg_value, COUNT(*) AS total, "+bucketSQL+" AS bucket_time"+
			" FROM sensor_data WHERE "+where+
			" GROUP BY sensor_type_id, bucket_time ORDER BY bucket_time DESC, sensor_type_id ASC LIMIT ? OFFSET ?",
		queryArgs...,
	).Scan(&rows)

	// Kelompokkan baris per kotak waktu
	bucketMap := make(map[string]*averageBucket)
	var order []string
	for _, r := range rows {
		key := r.BucketTime
		b, ok := bucketMap[key]
		if !ok {
			t, _ := time.Parse("2006-01-02 15:04:05", key)
			b = &averageBucket{BucketTime: t, Averages: []averageItem{}}
			bucketMap[key] = b
			order = append(order, key)
		}
		b.Averages = append(b.Averages, averageItem{
			SensorTypeID: r.SensorTypeID,
			Average:      math.Round(r.AvgValue*10) / 10,
			Total:        r.Total,
		})
	}

	data := make([]averageBucket, 0, len(order))
	for _, key := range order {
		data = append(data, *bucketMap[key])
	}

	totalPages := int64(0)
	if total > 0 {
		totalPages = (total + int64(limit) - 1) / int64(limit)
	}

	c.JSON(http.StatusOK, gin.H{
		"data":           data,
		"sensor_types":   types,
		"window_minutes": windowMin,
		"total":          total,
		"page":           page,
		"limit":          limit,
		"total_pages":    totalPages,
	})
}

// GetExternalHistory proxy ke API eksternal shelter.cbinstrument.com
// GET /api/external/history/:sensor_type?shelter_id=SHELTER-01&limit=24
// sensor_type: kunci sensor eksternal (temperature, humidity, air_quality, light_level, dll)
// limit: maks 500 titik (default 24), shelter_id default SHELTER-01
func GetExternalHistory(c *gin.Context) {
	sensorType := c.Param("sensor_type")
	limit := c.DefaultQuery("limit", "24")
	shelterID := c.DefaultQuery("shelter_id", "SHELTER-01")

	// Validasi ringan agar tidak bisa menyusupkan path/query ke URL eksternal.
	if !externalKeyRe.MatchString(sensorType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nama sensor tidak valid"})
		return
	}
	if !externalKeyRe.MatchString(shelterID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "shelter_id tidak valid"})
		return
	}
	limitNum, err := strconv.Atoi(limit)
	if err != nil || limitNum < 1 {
		limitNum = 24
	}
	if limitNum > 500 {
		limitNum = 500
	}

	url := fmt.Sprintf("https://shelter.cbinstrument.com/sensor/history/%s?shelter_id=%s&limit=%d",
		sensorType, shelterID, limitNum)

	req, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, url, nil)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Gagal menghubungi API eksternal: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, "application/json", body)
}
