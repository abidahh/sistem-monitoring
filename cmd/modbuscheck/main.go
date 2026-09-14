// modbuscheck adalah tool mandiri untuk membuka koneksi Modbus RTU RS485 dan
// menampilkan nilai yang terbaca langsung di terminal, PERSIS seperti cara app
// utama (main.go -> ReadSensorModbus) membaca sensor fisik.
//
// Fungsinya murni "baca & tampilkan": tidak menyimpan hasil ke database.
//
// Cara pakai:
//
//	go run ./cmd/modbuscheck              # baca berulang tiap 3 detik (default: sensor Kadar COD)
//	go run ./cmd/modbuscheck -count 3     # baca 3x lalu keluar
//	go run ./cmd/modbuscheck -count 1 -json  # baca sekali, output JSON
//	go run ./cmd/modbuscheck -list        # daftar port COM tersedia
//
// Contoh konfigurasi sensor lain tanpa mengubah kode:
//
//	go run ./cmd/modbuscheck -port COM5 -slave 1 -register 0 -baud 9600
//
// Format decode nilai bisa dipilih sesuai sensor (default float.be / ABCD):
//
//	go run ./cmd/modbuscheck -format float.cdab -count 1
//	go run ./cmd/modbuscheck -format int16.be -count 1
//
// Daftar format sama dengan dropdown "Format Data" di Panel Admin:
// float.be | float.cdab | float.badc | float.dcba | int32.be | int32.le | int16.be | int16.le
//
// Untuk memastikan port COM yang benar, jalankan:
//
//	go run ./cmd/usbcheck        # cari baris "[Ports] USB-SERIAL CH340 (COMx)"
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"strings"
	"time"

	"sistem-monitoring-cod_golang/modbusutil"

	"github.com/goburrow/modbus"
)

func main() {
	port := flag.String("port", "COM3", "port serial untuk Modbus RTU (mis. COM3)")
	slave := flag.Int("slave", 15, "Slave ID Modbus")
	register := flag.Int("register", 4608, "alamat register holding (desimal)")
	baud := flag.Int("baud", 9600, "baud rate serial")
	format := flag.String("format", "float.be", "format decode nilai (float.be/cdab/badc/dcba, int32.be/le, int16.be/le)")
	interval := flag.Int("interval", 3, "selang pembacaan (detik)")
	count := flag.Int("count", 0, "berapa kali baca lalu berhenti (0 = terus-menerus)")
	asJSON := flag.Bool("json", false, "cetak dalam format JSON")
	listPorts := flag.Bool("list", false, "tampilkan port COM tersedia lalu keluar")
	flag.Parse()

	if *listPorts {
		names, err := listCOMPorts()
		if err != nil {
			fmt.Println("[ERROR]", err)
			return
		}
		if len(names) == 0 {
			fmt.Println("Tidak ada port COM terdeteksi.")
			return
		}
		fmt.Printf("Port COM tersedia: %s\n", strings.Join(names, ", "))
		return
	}

	if *interval < 1 {
		*interval = 1
	}
	if *slave < 0 || *slave > 255 {
		fmt.Println("[ERROR] slave ID harus bernilai 0-255.")
		return
	}
	if *register < 0 || *register > 65535 {
		fmt.Println("[ERROR] alamat register harus bernilai 0-65535.")
		return
	}
	if !modbusutil.IsValidFormat(*format) {
		fmt.Printf("[ERROR] format %q tidak dikenal. Pilih salah satu: %s\n",
			*format, strings.Join(modbusutil.ValidFormats, ", "))
		return
	}

	regHex := fmt.Sprintf("0x%04X", uint16(*register))
	fmt.Printf("Membaca Modbus RTU: port=%s slave=%d register=%s (%d) baud=%d\n",
		*port, *slave, regHex, *register, *baud)
	fmt.Printf("Decode format: %s\n", *format)
	if *count > 0 {
		fmt.Printf("Akan berhenti setelah %d kali pembacaan.\n", *count)
	}
	fmt.Println("Tekan Ctrl+C untuk berhenti.")

	n := 0
	for {
		n++
		value, raw, err := readModbus(*port, byte(*slave), uint16(*register), *baud, *format)
		waktu := time.Now().Format("15:04:05")

		if err != nil {
			if *asJSON {
				fmt.Printf("{\"waktu\":\"%s\",\"error\":\"%s\"}\n", waktu, err)
			} else {
				fmt.Printf("[%s] [ERROR] port=%s slave=%d reg=%s: %s\n", waktu, *port, *slave, regHex, err)
				if n == 1 {
					fmt.Println("   Petunjuk: cek kabel RS485 & port COM (go run ./cmd/usbcheck).")
				}
			}
		} else {
			if *asJSON {
				fmt.Printf("{\"waktu\":\"%s\",\"port\":\"%s\",\"slave\":%d,\"register\":%d,\"raw\":%d,\"nilai\":%f}\n",
					waktu, *port, *slave, *register, raw, value)
			} else {
				fmt.Printf("[%s] nilai=%f (raw=0x%08X) | %s slave=%d reg=%s\n",
					waktu, value, raw, *port, *slave, regHex)
			}
		}

		if *count > 0 && n >= *count {
			break
		}
		time.Sleep(time.Duration(*interval) * time.Second)
	}
}

// readModbus membuka koneksi RTU, membaca 2 register holding di alamat register,
// lalu mendecode nilai sesuai format yang dipilih (modbusutil.DecodeValue). Mirip
// ReadSensorModbus pada app utama. Mengembalikan nilai, nilai mentah (uint32),
// serta error bila ada.
func readModbus(port string, slave byte, register uint16, baud int, format string) (float64, uint32, error) {
	handler := modbus.NewRTUClientHandler(port)
	handler.BaudRate = baud
	handler.DataBits = 8
	handler.Parity = "N"
	handler.StopBits = 1
	handler.SlaveId = slave
	handler.Timeout = 1 * time.Second

	if err := handler.Connect(); err != nil {
		return 0, 0, fmt.Errorf("koneksi gagal: %w", err)
	}
	defer handler.Close()

	results, err := modbus.NewClient(handler).ReadHoldingRegisters(register, 2)
	if err != nil {
		return 0, 0, fmt.Errorf("gagal membaca register %d: %w", register, err)
	}

	bits := binary.BigEndian.Uint32(results)
	return modbusutil.DecodeValue(results, format), bits, nil
}