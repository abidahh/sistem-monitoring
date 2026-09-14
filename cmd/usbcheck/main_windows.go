//go:build windows

// usbcheck adalah tool mandiri untuk mendeteksi perangkat USB yang terhubung
// ke laptop, dites manual lewat terminal SEBELUM diintegrasikan ke website.
//
// Cara pakai:
//
//	go run ./cmd/usbcheck               # tampilkan semua USB yang sedang terhubung
//	go run ./cmd/usbcheck -watch        # pantau USB yang masuk/keluar (live)
//	go run ./cmd/usbcheck -watch -interval 1
//	go run ./cmd/usbcheck -json         # output JSON (siap dipakai API website)
//
// Contoh baris USB yang keluar (setelah adapter Modbus dicolokkan):
//
//	[Ports] USB-SERIAL CH340 (COM5)  -> COM port yang harus dipakai Modbus RS485.
//
// Implementasi memakai SetupAPI Windows (golang.org/x/sys/windows) sehingga
// tanpa dependensi tambahan dan tanpa CGO.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// usbDevice mewakili satu perangkat USB yang terdeteksi PnP.
type usbDevice struct {
	InstanceID string `json:"instance_id"`
	Nama       string `json:"nama"`
	Kelas      string `json:"kelas"`
	Port       string `json:"com_port,omitempty"`
}

func main() {
	watch := flag.Bool("watch", false, "mode live: pantau USB yang masuk/keluar")
	interval := flag.Int("interval", 2, "selang polling (detik) saat mode -watch")
	asJSON := flag.Bool("json", false, "cetak dalam format JSON")
	flag.Parse()

	if *watch {
		runWatch(*interval, *asJSON)
		return
	}

	devices, err := listUSBDevices()
	if err != nil {
		fmt.Println("[ERROR]", err)
		return
	}

	if *asJSON {
		printJSON(devices)
		return
	}
	printTable(devices)
	printCOMPorts()
}

// listUSBDevices membaca semua perangkat PnP yang sedang terpasang ("present")
// dan menyaring yang berasal dari USB (bus USB, USBSTOR, atau host controller
// USB kelas PCI).
func listUSBDevices() ([]usbDevice, error) {
	devInfo, err := windows.SetupDiGetClassDevsEx(
		nil, "",
		0,
		windows.DIGCF_PRESENT|windows.DIGCF_ALLCLASSES,
		0, "",
	)
	if err != nil {
		return nil, fmt.Errorf("SetupDiGetClassDevsEx gagal: %w", err)
	}
	defer windows.SetupDiDestroyDeviceInfoList(devInfo)

	var out []usbDevice
	for i := 0; ; i++ {
		data, err := windows.SetupDiEnumDeviceInfo(devInfo, i)
		if err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
				break
			}
			continue
		}

		instID, err := windows.SetupDiGetDeviceInstanceId(devInfo, data)
		if err != nil {
			continue
		}

		var kelas string
		if v, err := devicePropString(devInfo, data, windows.SPDRP_CLASS); err == nil {
			kelas = v
		}
		if !isUSBRelated(instID, kelas) {
			continue
		}
		dev := usbDevice{InstanceID: instID, Kelas: kelas}

		if v, err := devicePropString(devInfo, data, windows.SPDRP_FRIENDLYNAME); err == nil {
			dev.Nama = v
		}
		if dev.Nama == "" {
			if v, err := devicePropString(devInfo, data, windows.SPDRP_DEVICEDESC); err == nil {
				dev.Nama = v
			}
		}
		if strings.EqualFold(dev.Kelas, "Ports") {
			if v, err := devicePropString(devInfo, data, windows.SPDRP_FRIENDLYNAME); err == nil {
				dev.Port = extractCOMPort(v)
			}
		}

		out = append(out, dev)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Kelas != out[j].Kelas {
			return out[i].Kelas < out[j].Kelas
		}
		return out[i].Nama < out[j].Nama
	})
	return out, nil
}

// isUSBRelated menentukan apakah sebuah perangkat berasal dari USB.
// Contoh instance ID:
//
//	USB\VID_1A86&PID_7523\123             -> adapter USB-serial / perangkat USB
//	USB\ROOT_HUB\4&1383B1A0&0             -> USB Root Hub (internal)
//	USBSTOR\Disk&Ven_SanDisk&Prod_...\..  -> flashdisk / USB storage
//	PCI\VEN_8086&DEV_....\..              -> host controller USB (kelas "USB")
//
// Host controller USB ber-instance PCI tetapi berkelas "USB"; tanpa pengecekan
// kelas, semua PCI (LAN, SATA, dsb) ikut terdeteksi, jadi di sini kelas menjadi
// syarat untuk instance PCI.
func isUSBRelated(instanceID, kelas string) bool {
	if strings.HasPrefix(instanceID, "USB\\") || strings.HasPrefix(instanceID, "USBSTOR\\") {
		return true
	}
	return strings.EqualFold(kelas, "USB")
}

// devicePropString mengambil properti registry perangkat sebagai string.
func devicePropString(devInfo windows.DevInfo, data *windows.DevInfoData, prop windows.SPDRP) (string, error) {
	value, err := devInfo.DeviceRegistryProperty(data, prop)
	if err != nil {
		return "", err
	}
	s, ok := value.(string)
	if !ok {
		return "", errors.New("properti bukan string")
	}
	return strings.TrimSpace(s), nil
}

// extractCOMPort mengambil "COMx" dari nama perangkat, misal
// "USB-SERIAL CH340 (COM5)" -> "COM5".
func extractCOMPort(friendlyName string) string {
	idx := strings.Index(friendlyName, "(COM")
	if idx < 0 {
		return ""
	}
	rest := friendlyName[idx+1:]
	end := strings.Index(rest, ")")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

// printTable mencetak daftar USB dalam format tabel terminal.
func printTable(devices []usbDevice) {
	if len(devices) == 0 {
		fmt.Println("Tidak ada perangkat USB terdeteksi.")
		return
	}

	fmt.Printf("Perangkat USB terhubung (%d):\n", len(devices))
	fmt.Println(strings.Repeat("-", 88))

	usbClassName := map[string]string{
		"USB":      "USB",
		"HIDClass": "HID",
		"Ports":    "Serial",
		"Camera":   "Camera",
		"Bluetooth": "Bluetooth",
	}

	for _, d := range devices {
		kelas := d.Kelas
		if label, ok := usbClassName[kelas]; ok {
			kelas = label
		}

		nama := d.Nama
		if d.Port != "" {
			nama = fmt.Sprintf("%s  [berada di %s]", nama, d.Port)
		}

		shortID := d.InstanceID
		parts := strings.Split(shortID, "\\")
		if len(parts) >= 3 {
			shortID = strings.Join(parts[1:], "\\")
		}

		fmt.Printf("  %-10s %-40s %s\n", kelas, truncate(nama, 40), shortID)
	}
	fmt.Println(strings.Repeat("-", 88))
}

// printCOMPorts membaca daftar port COM dari registry (agar kita tahu port mana
// yang tersedia untuk Modbus RS485).
func printCOMPorts() {
	portNames, err := listCOMPorts()
	if err != nil {
		return
	}
	if len(portNames) == 0 {
		return
	}
	fmt.Printf("\nPort COM tersedia: %s\n", strings.Join(portNames, ", "))
}

// listCOMPorts membaca key HARDWARE\DEVICEMAP\SERIALCOMM pada registry.
func listCOMPorts() ([]string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`HARDWARE\DEVICEMAP\SERIALCOMM`, registry.READ)
	if err != nil {
		return nil, err
	}
	defer key.Close()

	names, err := key.ReadValueNames(128)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	var ports []string
	for _, n := range names {
		if v, _, err := key.GetStringValue(n); err == nil {
			ports = append(ports, v)
		}
	}
	sort.Strings(ports)
	return ports, nil
}

// runWatch melakukan polling dan mencetak perangkat yang baru masuk/keluar.
func runWatch(interval int, asJSON bool) {
	if interval < 1 {
		interval = 1
	}

	prev := map[string]usbDevice{}
	first := true

	fmt.Printf("Memantau perangkat USB setiap %d detik. Colokkan/cabut USB untuk melihat perubahannya (Ctrl+C untuk berhenti).\n\n", interval)

	for {
		devices, err := listUSBDevices()
		if err != nil {
			fmt.Println("[ERROR]", err)
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		now := map[string]usbDevice{}
		for _, d := range devices {
			now[d.InstanceID] = d
		}

		if !first {
			nowTS := time.Now().Format("15:04:05")
			// Perangkat yang keluar
			for id, old := range prev {
				if _, ok := now[id]; !ok {
					if asJSON {
						fmt.Printf("{\"event\":\"keluar\",\"waktu\":\"%s\",\"device\":%s}\n",
							nowTS, marshal(old))
					} else {
						fmt.Printf("[%s] - %s / %s\n", nowTS, old.Nama, old.InstanceID)
					}
				}
			}
			// Perangkat yang masuk
			for id, d := range now {
				if _, ok := prev[id]; !ok {
					if asJSON {
						fmt.Printf("{\"event\":\"masuk\",\"waktu\":\"%s\",\"device\":%s}\n",
							nowTS, marshal(d))
					} else {
						kelas := d.Kelas
						if d.Port != "" {
							kelas = fmt.Sprintf("%s (%s)", d.Kelas, d.Port)
						}
						fmt.Printf("[%s] + %s  [%s]\n", nowTS, d.Nama, kelas)
					}
				}
			}
		}

		prev = now
		first = false
		time.Sleep(time.Duration(interval) * time.Second)
	}
}

// printJSON mencetak daftar USB sebagai array JSON.
func printJSON(devices []usbDevice) {
	b, err := json.MarshalIndent(devices, "", "  ")
	if err != nil {
		fmt.Println("[ERROR]", err)
		return
	}
	fmt.Println(string(b))
}

// marshal mengubah satu device menjadi JSON satu baris.
func marshal(d usbDevice) string {
	b, err := json.Marshal(d)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// truncate memotong string agar pas di kolom.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}