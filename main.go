package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sistem-monitoring-cod_golang/config"
	"sistem-monitoring-cod_golang/controllers"
	"sistem-monitoring-cod_golang/middleware"
	"sistem-monitoring-cod_golang/modbusutil"
	"sistem-monitoring-cod_golang/models"
	"sistem-monitoring-cod_golang/mqtt"
	"sistem-monitoring-cod_golang/realtime"
	"sistem-monitoring-cod_golang/scanner"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/goburrow/modbus"
)

// migrateLegacySensorData memindahkan data sensor lama ke skema baru (sensor_type_id + value)
func migrateLegacySensorData() {
	// Deteksi kolom legacy TERLEBIH DAHULU. Bila tidak ada (skema baru di
	// SQLite maupun MySQL), seluruh blok migrasi di-skip — tidak ada query
	// "SELECT suhu" yang akan error (Error 1054/1) saat startup.
	if !config.DB.Migrator().HasColumn(&models.SensorData{}, "suhu") {
		return
	}

	log.Println("Migrasi data sensor lama (suhu & cod) ke skema baru...")

	ensureDefaultSensorTypes()

	var suhuType, codType models.SensorType
	config.DB.Where("nama = ?", "Suhu Air").First(&suhuType)
	config.DB.Where("nama = ?", "Kadar COD").First(&codType)

	// Salin data suhu
	var suhuRows []struct {
		ID        uint
		Suhu      float64
		CreatedAt time.Time
	}
	config.DB.Raw("SELECT id, suhu, created_at FROM sensor_data WHERE suhu IS NOT NULL").Scan(&suhuRows)
	for _, r := range suhuRows {
		config.DB.Create(&models.SensorData{
			SensorTypeID: suhuType.ID,
			Value:        r.Suhu,
			CreatedAt:    r.CreatedAt,
		})
	}

	// Salin data cod
	var codRows []struct {
		ID        uint
		COD       float64
		CreatedAt time.Time
	}
	config.DB.Raw("SELECT id, cod, created_at FROM sensor_data WHERE cod IS NOT NULL").Scan(&codRows)
	for _, r := range codRows {
		config.DB.Create(&models.SensorData{
			SensorTypeID: codType.ID,
			Value:        r.COD,
			CreatedAt:    r.CreatedAt,
		})
	}

	// Hapus data lama & kolom lama
	config.DB.Exec("DELETE FROM sensor_data WHERE sensor_type_id IS NULL")
	config.DB.Migrator().DropColumn(&models.SensorData{}, "suhu")
	config.DB.Migrator().DropColumn(&models.SensorData{}, "cod")

	log.Println("Migrasi data sensor lama selesai.")
}

// migrateLegacyNotificationData mengisi sensor_type_id pada notifikasi lama
// yang tersimpan sebelum kolom tersebut ada, dengan mencocokkan nama sensor
// (kolom tipe) ke tabel sensor_types.
func migrateLegacyNotificationData() {
	var logs []models.NotificationLog
	config.DB.Where("sensor_type_id = 0").Find(&logs)
	if len(logs) == 0 {
		return
	}

	updated := 0
	for _, l := range logs {
		var st models.SensorType
		if err := config.DB.Where("nama = ?", l.Tipe).First(&st).Error; err == nil {
			l.SensorTypeID = st.ID
			if config.DB.Save(&l).Error == nil {
				updated++
			}
		}
	}
	log.Printf("Backfill selesai: %d notifikasi lama dipetakan ke sensor.", updated)
}

// ensureDefaultSensorTypes membuat 2 tipe sensor default bila belum ada.
func ensureDefaultSensorTypes() {
	var count int64
	config.DB.Model(&models.SensorType{}).Count(&count)
	if count == 0 {
		config.DB.Create(&[]models.SensorType{
			{
				Nama:         "Suhu Air",
				Unit:         "°C",
				NilaiMax:     30.0,
				Aktif:        true,
				Warna:        "#ef4444",
				PortCom:      "COM3",
				SlaveID:      1,
				RegisterAddr: 0,
				BaudRate:     9600,
			},
			{
				Nama:         "Kadar COD",
				Unit:         "mg/L",
				NilaiMax:     100.0,
				Aktif:        true,
				Warna:        "#0d9488",
				PortCom:      "COM3",
				SlaveID:      15,
				RegisterAddr: 4608,
				BaudRate:     9600,
			},
		})
	}
}

// ReadSensorModbus membaca register Modbus RTU berdasarkan konfigurasi dinamis dari DB.
// Nilai register di-decode sesuai format yang dipilih (kolom register_format) lewat
// modbusutil.DecodeValue, lalu disimpan ke database berlabel sumber "real".
func ReadSensorModbus(portName string, slaveID byte, registerAddr uint16, baudRate int, sensorTypeID uint, format string) (float64, error) {
	handler := modbus.NewRTUClientHandler(portName)
	handler.BaudRate = baudRate // Baud Rate dinamis dari DB
	handler.DataBits = 8
	handler.Parity = "N"
	handler.StopBits = 1
	handler.SlaveId = slaveID // Slave ID dinamis dari DB
	handler.Timeout = 1 * time.Second

	err := handler.Connect()
	if err != nil {
		return 0, err
	}
	defer handler.Close()

	client := modbus.NewClient(handler)
	results, err := client.ReadHoldingRegisters(registerAddr, 2)
	if err != nil {
		return 0, err
	}

	// Decode sesuai format yang dipilih di konfigurasi tipe sensor
	sensorValue := modbusutil.DecodeValue(results, format)

	// Simpan hasil ke database via controller agar deteksi lonjakan ikut
	// diterapkan (lonjakan tetap tersimpan, tapi dicoret dari rata-rata).
	if _, err := controllers.SaveSensorReadingPublic(sensorTypeID, sensorValue); err != nil {
		return 0, err
	}

	return sensorValue, nil
}

// InitModbusScheduler membaca tipe sensor bersumber "real" dari database dan
// mencoba membacanya via Modbus RS485 secara berkala. Konfigurasi (port, slave
// ID, alamat register, baud rate) diambil per sensor dari kolom Modbus.
// Aman: setiap kegagalan koneksi/baca hanya dicatat lewat log, tidak menghentikan server.
func InitModbusScheduler() {
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			// Jeda pembacaan selama scan otomatis berlangsung agar frame RS485
			// tidak bertabrakan dengan probe scanner.
			if scanner.IsActive() {
				continue
			}

			// Hanya tipe sensor aktif bersumber "real" yang dibaca via Modbus
			var realSensorTypes []models.SensorType
			if err := config.DB.Where("aktif = ? AND sumber = ?", true, "real").Find(&realSensorTypes).Error; err != nil {
				continue
			}

			for _, st := range realSensorTypes {
				port := st.PortCom
				if port == "" {
					port = "COM3"
				}
				slaveID := st.SlaveID
				if slaveID == 0 {
					slaveID = 1
				}
				baudRate := st.BaudRate
				if baudRate <= 0 {
					baudRate = 9600
				}

				if _, err := ReadSensorModbus(port, slaveID, st.RegisterAddr, baudRate, st.ID, st.RegisterFormat); err != nil {
					log.Printf("Gagal membaca sensor %s (%s) via Modbus: %v", st.Nama, port, err)
				}
			}
		}
	}()
}

func main() {
	config.Load()
	// Tolak aktif bila masih memakai kredensial default bawaan.
	config.EnforceSecurity()

	r := gin.Default()
	// Jangan percaya header X-Forwarded-For tanpa proxy tepercaya (anti-spoofing
	// IP, penting untuk pembatasan IP rate limit).
	_ = r.SetTrustedProxies(nil)
	r.LoadHTMLGlob("views/*")
	r.Use(middleware.SecurityHeaders())

	store := cookie.NewStore([]byte(config.Cfg.SessionSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		Secure:   config.Cfg.TLSEnabled,
		SameSite: http.SameSiteLaxMode,
	})
	r.Use(sessions.Sessions(config.Cfg.SessionName, store))

	config.ConnectDatabase()
	config.DB.AutoMigrate(
		&models.User{},
		&models.SensorData{},
		&models.Setting{},
		&models.ApiKey{},
		&models.NotificationLog{},
		&models.SensorType{},
		&models.UserSensor{},
		&models.SensorAverage{},
	)
	migrateLegacySensorData()
	migrateLegacyNotificationData()

	// Manual migration untuk kolom custom_title yang baru ditambahkan
	if !config.DB.Migrator().HasColumn(&models.User{}, "custom_title") {
		config.DB.Migrator().AddColumn(&models.User{}, "CustomTitle")
		log.Println("Kolom custom_title ditambahkan ke tabel users")
	}

	config.SeedUsers()
	config.SeedDefaultApiKey()
	config.SeedSensorTypes()

	// Perbaiki instalasi lama yang masih memakai password/API key default.
	config.UpgradeLegacyCredentials()

	// Ubah API key polos di database menjadi hash SHA-256 (nilai key tidak
	// berubah, sehingga perangkat pengirim tetap memakai key yang sama).
	config.MigrateApiKeyHashing()

	// Muat pengaturan interval dari database (agar tetap tersimpan antar restart)
	controllers.InitIntervalSetting()
	controllers.InitAverageSetting()
	controllers.InitSpikeSetting()

	// Simpan rata-rata per bucket ke database (sensor_averages) setiap menit.
	controllers.StartAverageScheduler()

	// Simulator dinonaktifkan agar tidak membuat data palsu saat perangkat offline
	// controllers.StartSimulator()

	// Jalankan pembacaan Modbus RS485 Native di background
	InitModbusScheduler()

	// Jembatan real-time: hub WebSocket menyiarkan pembacaan & notifikasi baru
	go realtime.HubInstance.Run()

	// Subscriber MQTT (aktif bila diaktifkan lewat pengaturan admin).
	mqtt.Start()
	// Note: signal.Notify(os.Interrupt) membajak sinyal dan menahan perilaku
	// default (terminate). Karena itu setelah membersihkan MQTT kita harus
	// keluar secara eksplisit agar Ctrl+C benar-benar menutup aplikasi.
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		<-ch
		log.Println("Sinyal berhenti diterima — mematikan aplikasi.")
		mqtt.Stop()
		os.Exit(0)
	}()

	// Pembatasan percobaan login yang gagal (anti brute force)
	middleware.StartLoginRateLimiterPruner()
	// Pembersih bucket rate limit endpoint publik
	middleware.StartApiRateLimitPruner()

	// --- 1. PUBLIC ROUTES ---
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "login.html", nil)
	})
	r.POST("/api/login", controllers.Login)

	r.POST("/api/sensor", middleware.BodyLimit(64*1024),
		middleware.RateLimit(config.Cfg.ApiRateLimitPerMinute),
		middleware.ApiKeyRequired(),
		controllers.StoreSensorData)

	// --- 2. PROTECTED ROUTES ---
	authorized := r.Group("/")
	authorized.Use(middleware.AuthRequired())
	{
		authorized.GET("/dashboard", func(c *gin.Context) {
			c.HTML(http.StatusOK, "dashboard.html", nil)
		})
		authorized.POST("/api/logout", controllers.Logout)
		authorized.GET("/api/sensor/latest", controllers.GetLatestSensorData)
		authorized.GET("/api/sensor/history", controllers.GetSensorHistory)
		authorized.GET("/api/sensor/stats", controllers.GetSensorStats)
		authorized.GET("/api/sensor/average/latest", controllers.GetAverage5Latest)
		authorized.GET("/api/sensor/average/history", controllers.GetAverage5History)
		authorized.POST("/api/sensor/simulate", controllers.StoreSimulatedData)
		authorized.GET("/api/notifications", controllers.GetNotificationLogs)

		// WebSocket real-time: push pembacaan & notifikasi baru ke dashboard.
		authorized.GET("/ws", realtime.WSHandler)

		// ROUTE PROFIL
		authorized.GET("/api/profile", controllers.GetProfile)
		authorized.POST("/api/profile/update", controllers.UpdateProfile)

		// ROUTE PENGATURAN INTERVAL SENSOR
		authorized.GET("/api/setting/interval", controllers.GetIntervalSetting)
		authorized.POST("/api/setting/interval", controllers.UpdateIntervalSetting)

		// ROUTE EXPORT LAPORAN
		authorized.GET("/api/export/excel", controllers.ExportExcel)
		authorized.GET("/api/export/pdf", controllers.ExportPDF)

		// ROUTE DEBUGGING (untuk cek data yang masuk via terminal)
		authorized.GET("/api/debug/sensor-data", controllers.GetSensorDataDebug)

		// ROUTE HISTORY EKSTERNAL (proxy ke shelter.cbinstrument.com)
		authorized.GET("/api/external/history/:sensor_type", controllers.GetExternalHistory)

		// --- 3. KHUSUS ADMIN ---
		adminGroup := authorized.Group("/")
		adminGroup.Use(middleware.AdminOnly())
		{
			adminGroup.GET("/admin", func(c *gin.Context) {
				c.HTML(http.StatusOK, "admin.html", nil)
			})
			adminGroup.GET("/api/admin/users", controllers.GetUsers)
			adminGroup.POST("/api/admin/users", controllers.CreateUser)
			adminGroup.POST("/api/admin/users/:id/toggle", controllers.ToggleUserStatus)
			adminGroup.DELETE("/api/admin/users/:id", controllers.DeleteUser)

			// ATUR SENSOR YANG DITAMPILKAN PER USER
			adminGroup.GET("/api/admin/users/:id/sensors", controllers.GetUserSensors)
			adminGroup.POST("/api/admin/users/:id/sensors", controllers.SetUserSensors)

			// KELOLA TIPE SENSOR
			adminGroup.GET("/api/admin/sensor-types", controllers.GetSensorTypes)
			adminGroup.POST("/api/admin/sensor-types", controllers.CreateSensorType)
			adminGroup.PUT("/api/admin/sensor-types/:id", controllers.UpdateSensorType)
			adminGroup.DELETE("/api/admin/sensor-types/:id", controllers.DeleteSensorType)

			// SCAN OTOMATIS MODBUS (deteksi port/UID/register/format)
			adminGroup.POST("/api/admin/scan", controllers.StartSensorScan)
			adminGroup.GET("/api/admin/scan/:id", controllers.GetSensorScan)

			// KELOLA API KEY
			adminGroup.GET("/api/admin/api-keys", controllers.GetApiKeys)
			adminGroup.POST("/api/admin/api-keys", controllers.CreateApiKey)
			adminGroup.POST("/api/admin/api-keys/:id/toggle", controllers.ToggleApiKey)
			adminGroup.DELETE("/api/admin/api-keys/:id", controllers.DeleteApiKey)

			// PENGATURAN JENDELA RATA-RATA (khusus admin)
			adminGroup.GET("/api/setting/average", controllers.GetAverageSetting)
			adminGroup.POST("/api/setting/average", controllers.UpdateAverageSetting)

			// PENGATURAN DETEKSI LONJAKAN (khusus admin)
			adminGroup.GET("/api/setting/spike", controllers.GetSpikeSetting)
			adminGroup.POST("/api/setting/spike", controllers.UpdateSpikeSetting)

			// PENGATURAN MQTT (khusus admin)
			adminGroup.GET("/api/setting/mqtt", mqtt.GetMQTTSetting)
			adminGroup.POST("/api/setting/mqtt", mqtt.UpdateMQTTSetting)
			adminGroup.GET("/api/setting/mqtt/status", mqtt.GetMQTTStatus)
		}
	}

	if config.Cfg.TLSEnabled {
		log.Printf("Server HTTPS berjalan di https://localhost%s", config.Cfg.ServerPort)
		r.RunTLS(config.Cfg.ServerPort, config.Cfg.TLSCertFile, config.Cfg.TLSKeyFile)
		return
	}
	log.Printf("Server berjalan di http://localhost%s", config.Cfg.ServerPort)
	r.Run(config.Cfg.ServerPort)
}
