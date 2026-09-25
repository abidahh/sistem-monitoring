# Alur Subscribe MQTT - Sistem Monitoring Sensor

## 📋 Overview
Sistem ini menggunakan protokol MQTT untuk menerima data sensor secara real-time dari hardware melalui broker MQTT.

## 🔄 Alur Lengkap Subscribe MQTT

```
┌─────────────┐
│  HARDWARE   │
│  (Sensor)   │
└──────┬──────┘
       │
       │ 1. Publish Data
       │    Topic: shelter/SHELTER-01/sensors
       │    Payload: JSON
       │
       ▼
┌─────────────────────┐
│   MQTT BROKER       │
│ shelter.cbinstrument.com:1883
│ (Tanpa TLS)         │
└──────┬──────────────┘
       │
       │ 2. Subscribe
       │    Topic: shelter/SHELTER-01/sensors/#
       │
       ▼
┌─────────────────────┐
│   BACKEND GO        │
│   (Subscriber)      │
└──────┬──────────────┘
       │
       │ 3. Process Message
       │    - Parse JSON
       │    - Mapping ExternalKey
       │    - Validate
       │
       ▼
┌─────────────────────┐
│   DATABASE          │
│   (SensorData)      │
└──────┬──────────────┘
       │
       │ 4. Query Data
       │
       ▼
┌─────────────────────┐
│   FRONTEND          │
│   (Dashboard)       │
└─────────────────────┘
```

## 🔧 Konfigurasi MQTT

### 1. Konfigurasi di Admin Panel
Lokasi: Admin → Kelola Sensor → Pengaturan MQTT

**Field Konfigurasi:**
- **Broker**: `shelter.cbinstrument.com:1883`
- **Protocol**: `TCP` (tanpa TLS)
- **Topik**: `shelter/SHELTER-01/sensors`
- **User**: (kosongkan jika tanpa autentikasi)
- **Password**: (kosongkan jika tanpa autentikasi)

### 2. Konfigurasi Sensor
Lokasi: Admin → Kelola Sensor → Tambah/Edit Sensor

**Field untuk sumber MQTT:**
- **Sumber**: Pilih `mqtt`
- **Topic MQTT (suffix)**: (opsional, untuk topik individual)
- **External Key**: Mapping ke API eksternal
  - `temperature` → Temperature
  - `humidity` → Humidity
  - `air_quality` → Air Quality
  - `light_level` → Light Level

## 📦 Format Payload MQTT

### Format 1: Array (Multi-sensor dalam satu pesan)
```json
{
  "sensors": [
    {"sensor_type": "temperature", "value": 28.5},
    {"sensor_type": "humidity", "value": 65},
    {"sensor_type": "air_quality", "value": 45},
    {"sensor_type": "light_level", "value": 1200}
  ]
}
```

### Format 2: Flat Object (Semua sensor dalam satu object)
```json
{
  "temperature": 28.5,
  "humidity": 65,
  "air_quality": 45,
  "light_level": 1200
}
```

## 🗂️ Mapping Sensor

Backend akan memetakan data MQTT ke sensor lokal menggunakan `ExternalKey`:

| External Key | Sensor Lokal |
|-------------|-------------|
| `temperature` | Suhu Air |
| `humidity` | Kelembaban |
| `air_quality` | Kualitas Udara |
| `light_level` | Cahaya |

## 🚀 Cara Kerja Subscribe

### 1. Hardware Publish Data
```python
# Contoh Python untuk hardware
import paho.mqtt.client as mqtt
import json

client = mqtt.Client()
client.connect("shelter.cbinstrument.com", 1883)

# Publish data sensor
payload = {
    "temperature": 28.5,
    "humidity": 65,
    "air_quality": 45,
    "light_level": 1200
}

client.publish("shelter/SHELTER-01/sensors", json.dumps(payload))
```

### 2. Backend Subscribe & Process
Backend Go akan:
1. Connect ke broker MQTT di `shelter.cbinstrument.com:1883`
2. Subscribe ke topik `shelter/SHELTER-01/sensors/#`
3. Parse JSON payload
4. Mapping berdasarkan `ExternalKey`
5. Simpan ke database dengan sumber `mqtt`

### 3. Frontend Display
Frontend JavaScript akan:
1. Fetch data dari API backend
2. Update dashboard real-time
3. Menampilkan grafik dan nilai sensor

## ⚙️ Konfigurasi Specific untuk shelter.cbinstrument.com

### MQTT Broker Configuration
- **Broker**: `shelter.cbinstrument.com:1883`
- **Protocol**: TCP (tanpa TLS)
- **Topic**: `shelter/SHELTER-01/sensors`
- **Authentication**: Tidak ada (API polos)

### API History Configuration
- **Base URL**: `https://shelter.cbinstrument.com`
- **History Endpoint**: `/sensor/history/{sensor_type}?limit=24`
- **Sensor Types**: `temperature`, `humidity`, `air_quality`, `light_level`

### Contoh Penggunaan API History:
```bash
# Temperature
https://shelter.cbinstrument.com/sensor/history/temperature?limit=24

# Humidity
https://shelter.cbinstrument.com/sensor/history/humidity?limit=24

# Air Quality
https://shelter.cbinstrument.com/sensor/history/air_quality?limit=24

# Light Level
https://shelter.cbinstrument.com/sensor/history/light_level?limit=24
```

### Proxy Backend:
Backend Go menyediakan proxy untuk mengakses API history ini:
- **Endpoint**: `GET /api/external/history/{sensor_type}?limit=24`
- **Sensor Types yang diizinkan**: `temperature`, `humidity`, `air_quality`, `light_level`

## 🔍 Troubleshooting

### Tidak ada data masuk ke database:
1. Cek koneksi MQTT broker di admin panel
2. Pastikan broker aktif dan bisa diakses
3. Cek topic yang benar: `shelter/SHELTER-01/sensors`
4. Verifikasi format payload JSON

### Data tidak muncul di dashboard:
1. Pastikan sensor aktif dan di-assign ke user
2. Cek mapping ExternalKey yang benar
3. Refresh dashboard untuk fetch data terbaru

### Mapping sensor tidak berfungsi:
1. Pastikan ExternalKey diisi dengan benar
2. Cek konsistensi antara payload MQTT dan ExternalKey
3. Verify case sensitivity (lowercase)

### API History tidak berfungsi:
1. Pastikan endpoint `https://shelter.cbinstrument.com` dapat diakses
2. Cek sensor_type yang valid: `temperature`, `humidity`, `air_quality`, `light_level`
3. Verifikasi parameter limit (default 24)

## 📝 Catatan Penting

1. **Tanpa TLS**: Koneksi MQTT menggunakan TCP tanpa TLS (port 1883)
2. **API Polos**: Tidak memerlukan API key atau token
3. **Topik Spesifik**: Topic MQTT adalah `shelter/SHELTER-01/sensors`
4. **Multi-sensor**: Satu payload berisi semua sensor sekaligus
5. **Mapping Penting**: ExternalKey harus sesuai dengan key di payload MQTT
6. **Dual Source**: Data bisa diterima via MQTT subscribe atau diambil via API history