# Dokumentasi untuk Tim Hardware - Pengiriman Data Sensor

## 📋 Overview
Dokumentasi ini untuk tim hardware mengenai cara mengirim data sensor ke sistem monitoring via MQTT.

## 🔧 Spesifikasi Koneksi MQTT

### Konfigurasi Broker
- **Broker Host**: `shelter.cbinstrument.com`
- **Port**: `1883`
- **Protocol**: TCP (tanpa TLS)
- **Topic**: `/sensors`
- **Authentication**: Tidak ada (API polos, tanpa username/password)

### Format Koneksi
```
mqtt://shelter.cbinstrument.com:1883
Topic: /sensors
QoS: 0 (at most once)
```

## 📦 Format Payload Data

### Format 1: Array (Recommended)
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

### Format 2: Flat Object
```json
{
  "temperature": 28.5,
  "humidity": 65,
  "air_quality": 45,
  "light_level": 1200
}
```

## 🗂️ Mapping Sensor Type

| Key di Payload | Sensor di Sistem | Unit yang Diharapkan |
|----------------|------------------|---------------------|
| `temperature` | Suhu Air | °C |
| `humidity` | Kelembaban | % |
| `air_quality` | Kualitas Udara | AQI/Unit |
| `light_level` | Cahaya | Lux |

## 💻 Contoh Implementasi

### Python (paho-mqtt)
```python
import paho.mqtt.client as mqtt
import json
import time

# Konfigurasi MQTT
BROKER = "shelter.cbinstrument.com"
PORT = 1883
TOPIC = "/sensors"

# Callback saat terhubung
def on_connect(client, userdata, flags, rc):
    print(f"Terhubung ke broker dengan kode: {rc}")

# Callback saat publish berhasil
def on_publish(client, userdata, mid):
    print(f"Data berhasil dipublish dengan message ID: {mid}")

# Setup client MQTT
client = mqtt.Client()
client.on_connect = on_connect
client.on_publish = on_publish

# Connect ke broker
client.connect(BROKER, PORT, 60)

# Fungsi untuk kirim data sensor
def send_sensor_data(temp, humidity, air_quality, light):
    payload = {
        "sensors": [
            {"sensor_type": "temperature", "value": temp},
            {"sensor_type": "humidity", "value": humidity},
            {"sensor_type": "air_quality", "value": air_quality},
            {"sensor_type": "light_level", "value": light}
        ]
    }
    
    # Publish ke topic /sensors
    client.publish(TOPIC, json.dumps(payload))
    print(f"Data dikirim: {payload}")

# Contoh penggunaan - kirim data setiap 5 detik
try:
    client.loop_start()
    
    while True:
        # Baca data dari sensor fisik (ganti dengan fungsi baca sensor sebenarnya)
        temperature = read_temperature_sensor()  # Ganti dengan fungsi baca sensor
        humidity = read_humidity_sensor()      # Ganti dengan fungsi baca sensor
        air_quality = read_air_quality_sensor() # Ganti dengan fungsi baca sensor
        light_level = read_light_sensor()      # Ganti dengan fungsi baca sensor
        
        # Kirim data via MQTT
        send_sensor_data(temperature, humidity, air_quality, light_level)
        
        # Tunggu 5 detik sebelum kirim berikutnya
        time.sleep(5)
        
except KeyboardInterrupt:
    print("Program dihentikan")
    client.loop_stop()
    client.disconnect()
```

### Arduino (PubSubClient)
```cpp
#include <ESP8266WiFi.h>
#include <PubSubClient.h>

// Konfigurasi WiFi
const char* ssid = "NAMA_WIFI";
const char* password = "PASSWORD_WIFI";

// Konfigurasi MQTT
const char* mqtt_server = "shelter.cbinstrument.com";
const int mqtt_port = 1883;
const char* mqtt_topic = "/sensors";

WiFiClient espClient;
PubSubClient client(espClient);

// Fungsi setup WiFi
void setup_wifi() {
  delay(10);
  Serial.println("Connecting to WiFi...");
  WiFi.begin(ssid, password);
  
  while (WiFi.status() != WL_CONNECTED) {
    delay(500);
    Serial.print(".");
  }
  
  Serial.println("WiFi connected");
}

// Fungsi callback MQTT
void callback(char* topic, byte* payload, unsigned int length) {
  // Tidak perlu handle callback untuk publisher only
}

// Fungsi reconnect MQTT
void reconnect() {
  while (!client.connected()) {
    Serial.print("Attempting MQTT connection...");
    
    if (client.connect("ESP8266_Sensor")) {
      Serial.println("connected");
    } else {
      Serial.print("failed, rc=");
      Serial.print(client.state());
      Serial.println(" try again in 5 seconds");
      delay(5000);
    }
  }
}

// Fungsi kirim data sensor
void sendSensorData(float temp, float humidity, int airQuality, int light) {
  String payload = "{\"sensors\":[";
  payload += "{\"sensor_type\":\"temperature\",\"value\":" + String(temp) + "},";
  payload += "{\"sensor_type\":\"humidity\",\"value\":" + String(humidity) + "},";
  payload += "{\"sensor_type\":\"air_quality\",\"value\":" + String(airQuality) + "},";
  payload += "{\"sensor_type\":\"light_level\",\"value\":" + String(light) + "}";
  payload += "]}";
  
  client.publish(mqtt_topic, payload.c_str());
  Serial.println("Data sent: " + payload);
}

void setup() {
  Serial.begin(115200);
  setup_wifi();
  
  client.setServer(mqtt_server, mqtt_port);
  client.setCallback(callback);
}

void loop() {
  if (!client.connected()) {
    reconnect();
  }
  client.loop();
  
  // Baca data dari sensor fisik
  float temperature = readTemperature(); // Ganti dengan fungsi baca sensor
  float humidity = readHumidity();     // Ganti dengan fungsi baca sensor
  int airQuality = readAirQuality();   // Ganti dengan fungsi baca sensor
  int lightLevel = readLightLevel();   // Ganti dengan fungsi baca sensor
  
  // Kirim data setiap 5 detik
  sendSensorData(temperature, humidity, airQuality, lightLevel);
  delay(5000);
}
```

### Node.js (mqtt.js)
```javascript
const mqtt = require('mqtt');

// Konfigurasi MQTT
const BROKER = 'mqtt://shelter.cbinstrument.com:1883';
const TOPIC = '/sensors';

// Connect ke broker
const client = mqtt.connect(BROKER);

client.on('connect', () => {
  console.log('Terhubung ke MQTT broker');
});

client.on('error', (err) => {
  console.error('MQTT connection error:', err);
});

// Fungsi kirim data sensor
function sendSensorData(temp, humidity, airQuality, light) {
  const payload = {
    sensors: [
      { sensor_type: 'temperature', value: temp },
      { sensor_type: 'humidity', value: humidity },
      { sensor_type: 'air_quality', value: airQuality },
      { sensor_type: 'light_level', value: light }
    ]
  };
  
  client.publish(TOPIC, JSON.stringify(payload), (err) => {
    if (err) {
      console.error('Gagal publish:', err);
    } else {
      console.log('Data berhasil dikirim:', payload);
    }
  });
}

// Contoh penggunaan - kirim data setiap 5 detik
setInterval(() => {
  // Baca data dari sensor fisik (ganti dengan fungsi baca sensor sebenarnya)
  const temperature = readTemperatureSensor();  // Ganti dengan fungsi baca sensor
  const humidity = readHumiditySensor();      // Ganti dengan fungsi baca sensor
  const airQuality = readAirQualitySensor(); // Ganti dengan fungsi baca sensor
  const lightLevel = readLightSensor();      // Ganti dengan fungsi baca sensor
  
  sendSensorData(temperature, humidity, airQuality, lightLevel);
}, 5000);
```

## ⚙️ Pengaturan di Sistem Monitoring

### 1. Konfigurasi MQTT (Admin Panel)
Setelah sistem dijalankan, admin perlu mengkonfigurasi:
- **Broker**: `shelter.cbinstrument.com:1883`
- **Protocol**: `TCP`
- **Topic**: `/sensors`
- **Aktif**: Centang "Aktif"

### 2. Konfigurasi Sensor (Admin Panel)
Untuk setiap sensor yang akan menerima data:
- **Sumber**: Pilih `mqtt`
- **External Key**: 
  - `temperature` untuk Suhu Air
  - `humidity` untuk Kelembaban
  - `air_quality` untuk Kualitas Udara
  - `light_level` untuk Cahaya

## 🔍 Testing

### Test Connection
Sebelum integrasi penuh, tim hardware bisa test koneksi:

**Python Test Script:**
```python
import paho.mqtt.client as mqtt
import json

def on_connect(client, userdata, flags, rc):
    print(f"Connected with result code {rc}")
    # Test publish
    test_payload = {
        "sensors": [
            {"sensor_type": "temperature", "value": 25.0},
            {"sensor_type": "humidity", "value": 60}
        ]
    }
    client.publish("/sensors", json.dumps(test_payload))
    print("Test payload sent")

client = mqtt.Client()
client.on_connect = on_connect
client.connect("shelter.cbinstrument.com", 1883, 60)
client.loop_forever()
```

### Verifikasi Data
Setelah mengirim data, verifikasi di:
1. **Dashboard**: Data harus muncul real-time
2. **Database**: Data tersimpan di tabel sensor_data
3. **Logs**: Cek log backend untuk pesan MQTT

## 📞 Kontak untuk Support
Jika ada masalah dengan koneksi atau format data, hubungi tim backend/frontend untuk:
- Verifikasi konfigurasi MQTT
- Cek format payload
- Debug mapping sensor

## 📝 Catatan Penting
1. **Interval Pengiriman**: Disarankan 5-10 detik sekali
2. **Format JSON**: Pastikan JSON valid dan sesuai format
3. **Sensor Type**: Key harus sesuai dengan mapping yang sudah disepakati
4. **Error Handling**: Implement error handling untuk koneksi MQTT
5. **Reconnection**: Pastikan auto-reconnect jika koneksi putus