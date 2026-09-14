"""
sensor_writer.py
================
Skrip bantu untuk menguji jalur serial TANPA alat hardware.

Fungsinya MENIRU pembacaan sensor: membuka satu port serial (COM) dan mengirim
baris JSON, persis seperti yang nanti dikirim alat asli (ESP32/Arduino).

Cara pakai (loopback 2 kabel USB-serial):
  1. Sambungkan 2 kabel USB-serial (misal COM3 & COM4) secara loopback:
        TX adp-1 -> RX adp-2
        RX adp-1 -> TX adp-2
        GND adp-1 -> GND adp-2
  2. Jalankan skrip ini di SATU port (misal COM3):
        python sensor_writer.py COM3
  3. Jalankan aplikasi Go membaca port PASANGANNYA (misal COM4):
        set SERIAL_PORT=COM4
        set SERIAL_BAUD=9600
        go run .
  4. Cek log server muncul: "Serial reader: port COM4 dibuka @ 9600 baud."
     lalu ada baris "1 -> 28.5 (threshold:false)".

Pilih bentuk JSON yang dikirim dengan argumen --format (lihat serialreader/serial.go):

    --format single  (default)  satu baris per sensor:
        {"sensor_type_id":1,"value":28.5}
        {"nama":"Suhu Air","value":29.1}

    --format array    banyak sensor dalam satu baris (array "sensor"):
        {"sensor":[{"sensor_type_id":1,"value":28.5},{"sensor_type_id":2,"value":95.0}]}

    --format named    satu baris objek ber-kunci nama tipe sensor:
        {"Suhu Air":28.5,"Kadar COD":95.0}

Contoh:  python sensor_writer.py COM3 --format array

Ganti nilai di daftar SAMPLES dan INTERVAL sesuai kebutuhan.
"""

import sys
import time
import json

import serial

# Kumpulan contoh pembacaan. Dikirim berulang (dipakai sebagai rangkaian acak).
SAMPLES = [
    {"sensor_type_id": 1, "value": 28.5},
    {"nama": "Suhu Air", "value": 29.1},
    {"sensor_type_id": 1, "value": 30.2},
    {"sensor_type_id": 2, "value": 95.0},
    {"nama": "Kadar COD", "value": 110.0},
    {"sensor_type_id": 2, "value": 98.5},
]

# Interval antar kiriman (detik). Default 3 agar sinkron dgn interval aplikasi.
INTERVAL = 3.0

# Format JSON yang didukung oleh aplikasi Go.
FORMATS = ("single", "array", "named")


def resolve_args() -> tuple:
    """Ambil port & format dari argumen CLI."""
    port = None
    fmt = "single"
    args = sys.argv[1:]
    i = 0
    while i < len(args):
        if args[i] == "--format":
            i += 1
            if i >= len(args):
                print("--format butuh argumen (single | array | named)")
                sys.exit(1)
            fmt = args[i]
        elif port is None:
            port = args[i]
        else:
            print(f"Argumen tak dikenal: {args[i]}")
            sys.exit(1)
        i += 1

    if port is None:
        print("Gunakan: python sensor_writer.py <PORT> [--format single|array|named]")
        sys.exit(1)
    if fmt not in FORMATS:
        print(f"--format harus salah satu dari: {', '.join(FORMATS)}")
        sys.exit(1)
    return port, fmt


def list_ports() -> None:
    """Tampilkan daftar port serial yang tersedia di komputer ini."""
    from serial.tools import list_ports
    ports = list_ports.comports()
    if not ports:
        print("Tidak ada port serial terdeteksi.")
        return
    print("Port serial tersedia:")
    for p in ports:
        print(f"  - {p.device}  {p.description or ''}")


def build_line(fmt: str, sample: dict) -> str:
    """Susun satu baris JSON sesuai format yang diminta."""
    if fmt == "array":
        # Satu baris multi-sensor: pakai beberapa sample sekaligus.
        items = [{"sensor_type_id": s["sensor_type_id"], "value": s["value"]}
                 for s in (SAMPLES[0], SAMPLES[3])]  # tipe 1 & tipe 2
        payload = {"sensor": items}
    elif fmt == "named":
        # Satu baris objek ber-kunci nama tipe sensor.
        payload = {"Suhu Air": 28.5, "Kadar COD": 95.0}
    else:  # single
        # Baris per sensor, persis seperti sample.
        if "nama" in sample:
            payload = {"nama": sample["nama"], "value": sample["value"]}
        else:
            payload = {"sensor_type_id": sample["sensor_type_id"], "value": sample["value"]}
    return json.dumps(payload) + "\n"


def main() -> None:
    port, fmt = resolve_args()

    try:
        sr = serial.Serial(port, 9600, timeout=1)
    except serial.SerialException as exc:
        print(f"[ERROR] Tidak bisa membuka {port}: {exc}")
        print("Mungkin port salah / dipakai proses lain. Daftar port di bawah:")
        list_ports()
        sys.exit(1)

    print(f"Mengirim JSON (format={fmt}) ke {port} @ 9600 baud, tiap {INTERVAL} detik.")
    print("Tekan Ctrl+C untuk berhenti.\n")

    try:
        idx = 0
        while True:
            sample = SAMPLES[idx % len(SAMPLES)]
            idx += 1

            line = build_line(fmt, sample)
            sr.write(line.encode("utf-8"))
            print(">>", line.strip())

            time.sleep(INTERVAL)
    except KeyboardInterrupt:
        print("\nDihentikan oleh user.")
    finally:
        sr.close()


if __name__ == "__main__":
    main()
