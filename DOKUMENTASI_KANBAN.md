# Dokumentasi Kanban Pekerjaan - Sistem Monitoring COD & Suhu Air

## 📅 Tanggal: 17 September 2026

---

## ✅ **SELESAI - Fitur yang Berhasil Diimplementasikan**

### **1. Integrasi MQTT dengan shelter.cbinstrument.com**
- **Status**: ✅ Selesai
- **Deskripsi**: Integrasi sistem dengan broker MQTT shelter.cbinstrument.com untuk real-time data
- **Detail Implementasi**:
  - Default broker: `shelter.cbinstrument.com:1883`
  - Default topic: `shelter/SHELTER-01/sensors`
  - Protocol: TCP (tanpa TLS)
  - Mapping sensor via External Key (temperature, humidity, air_quality, light_level)
- **File yang Diubah**:
  - `config/config.go` - update default MQTT config
  - `mqtt/subscriber.go` - topic handling & wildcard subscribe
  - `ALUR_MQTT.md` - dokumentasi update
- **Hasil**: Sistem berhasil subscribe ke broker MQTT dengan konfigurasi yang sesuai

### **2. Status Koneksi MQTT Real-time**
- **Status**: ✅ Selesai
- **Deskripsi**: Indikator visual status koneksi MQTT di admin panel
- **Detail Implementasi**:
  - Endpoint: `/api/setting/mqtt/status`
  - Status badge di admin panel (Terhubung/Terputus/Nonaktif)
  - Auto-refresh setiap 5 detik saat tab sensor aktif
  - Tracking: connected, lastConnected, lastError
- **File yang Diubah**:
  - `mqtt/subscriber.go` - tambah state tracking
  - `mqtt/controller.go` - tambah GetMQTTStatus()
  - `main.go` - tambah route status endpoint
  - `views/admin.html` - tambah UI badge & polling
- **Hasil**: Admin bisa melihat status koneksi MQTT secara real-time

### **3. Perbaikan CRUD Sensor**
- **Status**: ✅ Selesai
- **Deskripsi**: Memperbaiki bug case sensitivity pada validation External Key
- **Detail Implementasi**:
  - Restore ke validation normal (bukan case-insensitive) karena SQLite issue
  - Backend validation sudah berfungsi normal
- **File yang Diubah**:
  - `controllers/sensor_type_controller.go` - restore validation normal
- **Hasil**: CRUD sensor sudah berfungsi normal kembali

### **4. Custom Title Dashboard per User**
- **Status**: ✅ Selesai
- **Deskripsi**: Setiap user bisa custom judul dashboard sesuai keinginan
- **Detail Implementasi**:
  - Tambah kolom `custom_title` di tabel users
  - Field input di menu Profil untuk custom title
  - Update judul topbar secara real-time
  - Default: "Sistem Monitoring COD & Suhu Air"
- **File yang Diubah**:
  - `models/user.go` - tambah CustomTitle field
  - `controllers/auth_controller.go` - update profile API
  - `main.go` - manual migration untuk kolom custom_title
  - `views/dashboard.html` - UI custom title & update logic
  - `views/admin.html` - UI admin panel custom title
- **Hasil**: User bisa custom judul dashboard tanpa mengganggu fungsi lain

### **5. Unit Sensor Baru**
- **Status**: ✅ Selesai
- **Deskripsi**: Menambahkan unit yang sesuai untuk MQTT sensor
- **Detail Implementasi**:
  - Tambah unit: `%` (kelembaban), `lux` (cahaya), `AQI` (air quality), `μg/m³`
- **File yang Diubah**:
  - `views/admin.html` - update dropdown unit
- **Hasil**: Unit dropdown sudah lengkap untuk semua jenis sensor

### **6. API History Debugging**
- **Status**: ✅ Selesai
- **Deskripsi**: Endpoint untuk debugging data sensor via terminal
- **Detail Implementasi**:
  - Endpoint: `/api/debug/sensor-data`
  - Menampilkan 20 data sensor terbaru dengan info lengkap
  - Bisa diakses via curl atau browser
- **File yang Diubah**:
  - `controllers/sensor_controller.go` - tambah GetSensorDataDebug()
  - `main.go` - tambah route debugging
- **Hasil**: Memudahkan testing dan debugging data sensor

---

## 🔧 **PERUBAHAN USER (Dilakukan oleh User)**

### **1. Simplifikasi MQTT Configuration**
- **Status**: ✅ Selesai
- **Deskripsi**: Menghapus opsi WebSocket dan menyederhanakan MQTT config
- **Perubahan User**:
  - Hapus opsi WebSocket protocol
  - Hapus WS Path field
  - Ubah label "TOPIK" menjadi "PREFIX TOPIC"
  - Default topic: `shelter/SHELTER-01`
- **File yang Diubah**:
  - `views/admin.html` - simplifikasi MQTT form
- **Hasil**: MQTT config lebih sederhana dan fokus pada TCP

### **2. Hapus Topic MQTT Individual**
- **Status**: ✅ Selesai
- **Deskripsi**: Menghapus field topic MQTT di level sensor individual
- **Perubahan User**:
  - Hapus field "TOPIC MQTT" dari form sensor
  - Gunakan global topic config untuk semua sensor
  - Fokus hanya pada External Key untuk mapping
- **File yang Diubah**:
  - `views/admin.html` - hapus field topic individual
  - `controllers/sensor_type_controller.go` - hapus mqtt_topic handling
- **Hasil**: MQTT konfigurasi lebih terpusat dan mudah dikelola

### **3. Perbaiki Timezone Issue**
- **Status**: ✅ Selesai
- **Deskripsi**: Memperbaiki timezone yang menunjukkan jam salah
- **Perubahan User**:
  - Tambah "Z" suffix ke string ISO date untuk parsing UTC yang benar
  - `const d = new Date(k + 'Z');`
- **File yang Diubah**:
  - `views/dashboard.html` - perbaiki timezone parsing
- **Hasil**: Waktu di riwayat data sekarang menunjukkan waktu yang benar

---

## 📋 **PENDING - Fitur yang Tidak Diimplementasikan**

### **1. Date Filter Preset**
- **Status**: ❌ Dibatalkan
- **Alasan**: User meminta untuk tidak menambah fitur tambahan
- **Rencana Original**: Quick filter buttons (Hari Ini, 7 Hari, 30 Hari, dll)
- **Keputusan**: Keep existing manual date filter

### **2. Automatic MQTT Sensor Discovery**
- **Status**: ❌ Tidak Diterima
- **Alasan**: User ingin tetap menggunakan CRUD manual untuk sensor
- **Rencana Original**: Auto-detect dan auto-create sensor dari broker
- **Keputusan**: Gunakan CRUD manual dengan External Key mapping

### **3. Konversi REST API ke WebSocket**
- **Status**: 📋 Rencana
- **Deskripsi**: Mengubah sistem polling REST API menjadi WebSocket untuk real-time data yang lebih efisien
- **Detail Implementasi**:
  - Setup WebSocket server menggunakan gorilla/websocket
  - Endpoint: `/ws` untuk koneksi WebSocket klien
  - Koneksi real-time untuk data sensor terbaru
  - Broadcast otomatis saat data baru masuk (MQTT/Modbus/HTTP)
  - Auto-reconnection handling di sisi klien
  - Fallback ke REST API jika WebSocket gagal
- **File yang Akan Diubah**:
  - `main.go` - tambah WebSocket endpoint dan handler
  - `controllers/sensor_controller.go` - tambah broadcast function
  - `mqtt/subscriber.go` - broadcast saat data MQTT masuk
  - `views/dashboard.html` - ganti polling dengan WebSocket client
  - `config/config.go` - tambah WebSocket config
- **Keuntungan**:
  - Mengurangi load server (tidak perlu polling berkala)
  - Update data lebih cepat (real-time)
  - Menghemat bandwidth
  - Pengalaman user lebih responsif
- **Tantangan**:
  - Manajemen koneksi WebSocket yang robust
  - Handle disconnected/reconnected klien
  - Kompatibilitas dengan existing REST API
- **Timeline Estimasi**: 2-3 hari implementasi + testing

---

## 🎯 **HASIL AKHIR SISTEM**

### **Konfigurasi MQTT Final:**
```
Broker: shelter.cbinstrument.com:1883
Protocol: TCP
Topic: shelter/SHELTER-01/sensors
Authentication: Tidak ada
```

### **Mapping Sensor Final:**
| External Key | Nama Sensor | Unit |
|-------------|-------------|------|
| temperature | Temperature | °C |
| humidity | Humidity | % |
| air_quality | Air Quality | AQI |
| light_level | Light Level | lux |

### **Fitur yang Aktif:**
✅ MQTT Subscribe real-time  
✅ Status koneksi MQTT visual  
✅ CRUD sensor manual  
✅ Custom title per user  
✅ Unit sensor lengkap  
✅ API history debugging  
✅ Timezone yang benar  

### **Alur Data:**
```
Hardware → shelter.cbinstrument.com:1883
         → Topic: shelter/SHELTER-01/sensors
         → Backend Subscribe
         → External Key Mapping
         → Database
         → Dashboard (WIB timezone)
```

---

## 📝 **Catatan Penting**

1. **User Preferences**: User lebih menyukai sistem yang sederhana dan praktis
2. **Manual CRUD**: User ingin tetap kontrol penuh pada sensor management
3. **No Auto-Discovery**: Tidak ada fitur auto-detect/auto-create sensor
4. **Minimal Changes**: Hindari perubahan yang tidak perlu
5. **Stability**: Prioritas stabilitas sistem daripada fitur tambahan

---

## 🚀 **Rekomendasi Masa Depan (Opsional)**

Jika ingin pengembangan lebih lanjut:
1. Export laporan dalam format lain (CSV, PDF)
2. Alert via email/SMS untuk threshold violations
3. Multi-location monitoring
4. Mobile app untuk monitoring on-the-go
5. Advanced analytics dan trend analysis

Tapi semua ini opsional dan perlu persetujuan user terlebih dahulu.