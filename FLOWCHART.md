# FLOWCHART SISTEM MONITORING SENSOR

## 📊 Gambaran Umum

```
┌────────────────────────────────────────────────────────────────────────┐
│                        SUMBER DATA SENSOR                              │
│                                                                        │
│  ┌────────────┐   ┌──────────────┐   ┌─────────────────────────────┐  │
│  │  HARDWARE  │   │ MODBUS RS485 │   │  API EKSTERNAL / HTTP POST  │  │
│  │ (MQTT Pub) │   │ (Serial COM) │   │  /api/sensor (API Key)      │  │
│  └─────┬──────┘   └──────┬───────┘   └──────────────┬──────────────┘  │
└────────┼─────────────────┼──────────────────────────┼─────────────────┘
         ▼                 ▼                          ▼
┌────────────────────────────────────────────────────────────────────────┐
│                         BACKEND GO (Gin)                               │
│                                                                        │
│  ┌─────────────┐   ┌──────────────────┐   ┌──────────────────────┐    │
│  │ MQTT Suber  │   │ Modbus Scheduler │   │ HTTP API & Web      │    │
│  │ subscriber  │   │ (5 detik)        │   │ (Gin Router)        │    │
│  │ → parser    │   │ → baca register  │   │                     │    │
│  │ → mapping   │   │ → decode format  │   │  - Login / Session  │    │
│  │ ExternalKey │   └────────┬─────────┘   │  - Auth (JWT/Sess)  │    │
│  └──────┬──────┘            │             │  - API Key Mildewr  │    │
│         │                   │             │  - Rate Limit       │    │
│         └─────────┬─────────┘             │  - Admin Only       │    │
│                   ▼                       │  - Security Headers │    │
│      ┌────────────────────┐               └──────────┬──────────┘    │
│      │  Deteksi Lonjakan  │                          │                │
│      │  (Spike Detection) │◄─────── admin setting ────┤                │
│      └─────────┬──────────┘                          │                │
│                ▼                                     │                │
│      ┌────────────────────┐                          │                │
│      │   SensorData DB    │                          │                │
│      │   NotificationLog  │                          │                │
│      └─────────┬──────────┘                          │                │
└────────────────┼─────────────────────────────────────┼────────────────┘
                 ▼                                     ▼
┌────────────────────────────────────────┐   ┌─────────────────────────┐
│              DATABASE SQLite           │   │       FRONTEND          │
│                                        │   │                         │
│  - SensorData       - SensorType       │   │  /login  (Login form)   │
│  - NotificationLog  - User             │   │  /dashboard  (Realtime) │
│  - Setting          - ApiKey           │   │  /admin  (Kelola)       │
│  - UserSensor                          │   │                         │
└────────────────────────────────────────┘   └─────────────────────────┘
```

## 🔄 Alur Server Startup (main.go)

```
                START
                  │
                  ▼
         ┌─────────────────┐
         │  config.Load()  │
         │  config.Enforce │
         │  Security()     │
         └────────┬────────┘
                  ▼
         ┌─────────────────┐
         │  Gin Router     │
         │  + Sessions     │
         │  + TrustedProxy │
         └────────┬────────┘
                  ▼
         ┌─────────────────┐
         │  ConnectDB +    │
         │  AutoMigrate    │
         │  Migrasi Legacy │
         └────────┬────────┘
                  ▼
         ┌─────────────────┐
         │ Seed Users,     │
         │ API Key, Sensor │
         │Types + Upgrade  │
         │ Credentials     │
         └────────┬────────┘
                  ▼
   ┌──────────────┼──────────────┐
   │              │              │
   ▼              ▼              ▼
┌──────────┐ ┌─────────┐ ┌──────────────┐
│ Modbus   │ │ MQTT    │ │ Rate limiter │
│Scheduler │ │ Start() │ │ + Pruners   │
│(5 detik) │ │         │ │ (background) │
└──────────┘ └─────────┘ └──────────────┘
                  │
                  ▼
         ┌─────────────────┐
         │  Daftarkan      │
         │  Semua Routes   │
         │  (Public/Admin) │
         └────────┬────────┘
                  ▼
         ┌─────────────────┐
         │  r.Run(Port)    │
         │  Server berjalan│
         └─────────────────┘
```

## 🌐 Alur Permintaan HTTP (Gin Router)

```
                    ┌──────────┐
                    │ CLIENT   │
                    └─────┬────┘
                          │ HTTP Request
                          ▼
                ┌─────────────────┐
                │ SecurityHeaders │
                │ Sessions        │
                └────────┬────────┘
                         │
              ┌──────────┼────────────────┐
              ▼          ▼                ▼
     ┌──────────────┐ ┌─────────────┐ ┌─────────────────┐
     │ PUBLIC       │ │ AUTH        │ │ ADMIN (auth+   │
     │ /api/login   │ │ /dashboard  │ │ admin-only)    │
     │ /api/sensor  │ │ /api/sensor │ │ /admin + /api/ │
     │ (API Key +   │ │ /latest,    │ │ admin/*        │
     │  RateLimit)  │ │ /history... │ └────────┬────────┘
     └────────┬─────┘ └────────┬────┘          │
              │                │               │
              └───────┬────────┼───────────────┘
                      ▼        ▼
              ┌───────────────────────┐
              │    CONTROLLERS        │
              │  - StoreSensorData    │
              │  - GetLatest/History  │
              │  - Export Excel/PDF   │
              │  - Kelola User/Sensor │
              │  - MQTT Setting       │
              └──────────┬────────────┘
                         ▼
               ┌─────────────────┐
               │     DATABASE    │
               └─────────────────┘
```

## 📡 Alur Data MQTT (Sumber: mqtt)

```
  HARDWARE PUBLISH (Topic: shelter/SHELTER-01/sensors)
            │  Payload JSON
            ▼
  ┌─────────────────────┐
  │   MQTT BROKER       │
  │ tcp://broker:1883   │
  └────────┬────────────┘
           │ Subscribe (topic/#)
           ▼
  ┌─────────────────────┐
  │ handleMessage()     │
  │ Parse JSON payload  │
  └────────┬────────────┘
           │
  ┌────────┴──────────────────┐
  │ Format?                   │
  │  "sensors": [...]  array  │
  │  atau flat object      │
  └────────┬──────────────────┘
           ▼
  ┌─────────────────────┐
  │ processSensorsArray │ / processFlatObject
  │ extract (key,value) │
  └────────┬────────────┘
           ▼
  ┌─────────────────────┐
  │ saveByExternalKey() │
  │ cari SensorType     │
  │ external_key +      │
  │ sumber=mqtt+ aktif  │
  └────────┬────────────┘
           │ ditemukan?
           │  Ya ──► SaveSensorReadingMQTT ──► DB + Deteksi Lonjakan
           │  Tidak ──► log, pesan diabaikan
```

## 🔌 Alur Data Modbus RS485 (Sumber: real)

```
  MODBUS SCHEDULER (Loop tiap 5 detik)
            │
            │ scanning aktif?
            │  Ya ──► lewati (hindari tabrakan RS485)
            │  Tidak
            ▼
  ┌─────────────────────────┐
  │ Query SensorType:       │
  │ aktif + sumber = "real" │
  └────────┬────────────────┘
           ▼
  ┌─────────────────────────┐
  │ ReadSensorModbus()      │
  │ - Connect RTU serial    │
  │ - ReadHoldingRegisters │
  │   (addr, 2 register)    │
  │ - DecodeValue (format)  │
  └────────┬────────────────┘
           ▼
  ┌─────────────────────────┐
  │ SaveSensorReadingPublic │
  │ (tersimpan + cek        │
  │   lonjakan)             │
  └────────┬────────────────┘
           ▼
        DATABASE
```

## 📈 Alur Penyimpanan & Deteksi Lonjakan

```
  Nilai sensor masuk (MQTT / Modbus / HTTP)
              │
              ▼
  ┌─────────────────────────┐
  │ SaveSensorReading       │
  └────────┬────────────────┘
           │
  ┌────────┴───────────────┐
  │ Deteksi lonjakan?      │
  │ │value - prev│ >       │
  │ max(abs, ratio×prev)   │
  └────────┬────────────────┘
        Ya │          │ Tidak
           ▼          ▼
  ┌────────────┐  ┌─────────────┐
  │ Tandai     │  │ Simpan      │
  │ lonjakan + │  │ data normal │
  │ catat ke   │  │ masuk       │
  │            │  │ rata-rata   │
  │Notifikasi  │  └─────────────┘
  └─────┬──────┘
        ▼
  ┌────────────┐
  │ NotificationLog (DB) │
  └────────────┘

  Rata-rata 5 terakhir: nilai lonjakan DIBUANG dari perhitungan.
```

## 👤 Alur Login & Dashboard

```
  GET / ──► Login Page
            │
            │ POST /api/login + rate limit
            ▼
  ┌──────────────────┐
  │ AuthController   │
  │ Login            │
  │ - Validasi user  │
  │ - Cek status     │
  │ - Hash password  │
  └────────┬─────────┘
           │ sukses?
           │  Ya ──► Set Session ──► /dashboard
           │  Tidak ──► Error + cek brute force
           ▼
  ┌─────────────────────────┐
  │ DASHBOARD (dashboard.html)│
  │ - Fetch /api/sensor/latest│
  │ - /history /stats        │
  │ - /average/latest        │
  │ - /notifications         │
  └─────────────────────────┘
```

## 🛠️ Alur Admin & Scan Otomatis

```
  /admin (login admin)
     │
     ├── Kelola User        ──► CRUD users + assign sensor
     ├── Kelola Tipe Sensor ──► CRUD sensor_types
     │                           (port, slaveID, addr, baud,
     │                            format, sumber, external_key)
     ├── Scan Otomatis      ──► StartSensorScan
     │                           │
     │                           ├── probe port COM / USB
     │                           ├── deteksi slave ID
     │                           ├── scan register + format
     │                           └── simpan hasil ──► GetSensorScan(id)
     ├── Kelola API Key     ──► CRUD + toggle (hash SHA-256)
     ├── Pengaturan Interval/Average/Spike ──► Setting DB
     └── Pengaturan MQTT    ──► SaveSettings + Restart() subscriber
```