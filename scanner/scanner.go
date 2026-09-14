// Package scanner menyediakan auto-scan Modbus RTU: menemukan port COM yang
// tersedia, slave ID (UID) yang merespons, alamat register berisi nilai stabil,
// serta format decode yang paling wajar. Hasilnya berupa daftar kandidat yang
// ditampilkan di Panel Admin untuk dikonfirmasi user.
//
// Catatan: deteksi bersifat heuristik (skor kewajaran), bukan jaminan 100% —
// kandidat harus diverifikasi user dari preview nilai.
package scanner

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"sistem-monitoring-cod_golang/modbusutil"

	"github.com/goburrow/modbus"
)

// Candidate mewakili satu hasil deteksi perangkat yang layak dikonfirmasi.
type Candidate struct {
	Port     string  `json:"port"`
	SlaveID  byte    `json:"slave_id"`
	Register uint16  `json:"register"`
	Format   string  `json:"format"`
	Value    float64 `json:"value_preview"`
	Score    int     `json:"score"`
}

// Options mengatur batas-batas scan.
type Options struct {
	UIDMin   int           // slave ID pertama yang dicoba (default 1)
	UIDMax   int           // slave ID terakhir yang dicoba (default 31)
	Timeout  time.Duration // timeout per request (default 150ms)
	BaudRate int           // baud rate tunggal (dipakai bila BaudRates kosong)

	// BaudRates adalah daftar baud rate yang di-sweep. Bila kosong dan BaudRate=0,
	// dipakai defaultBaudRates (9600, 19200, 38400).
	BaudRates []int
	// Addrs adalah daftar alamat register yang diproba. Kosong = probeAddrPool.
	Addrs []uint16
	// Formats adalah daftar format decode yang diuji. Kosong = probeFormats.
	Formats []string
}

var activeFlag atomic.Bool

// SetActive menandai apakah scan sedang berjalan. Dipakai InitModbusScheduler
// agar pembacaan berkala dijeda selama scan (mencegah tabrakan frame RS485).
func SetActive(on bool) { activeFlag.Store(on) }

// IsActive mengembalikan status scan aktif.
func IsActive() bool { return activeFlag.Load() }

// ProgressFunc dipanggil selama scan berlangsung. portIndex 0-based, portTotal
// jumlah port yang dipindai, status berupa pesan singkat untuk UI.
type ProgressFunc func(portIndex, portTotal int, status string)

// probeAddrPool = alamat register yang dicoba (dalam bentuk satuan register).
// Mencakup area umum (0..xx), 0x1000, dan 0x1200.
var probeAddrPool = []uint16{
	0, 1, 2, 3, 4, 5, 6, 7, 8, 12, 16, 24, 32, 40, 48, 64, 96,
	4096, 4097, 4098, 4100, 4128,
	4608, 4609, 4610, 4611, 4616,
}

// probeFormats = seluruh format yang diuji untuk tiap register.
var probeFormats = []string{
	"float.be", "float.cdab", "float.badc", "float.dcba",
	"int32.be", "int32.le", "int16.be", "int16.le",
}

// defaultBaudRates = baud rate yang di-sweep bila Options.BaudRates kosong.
var defaultBaudRates = []int{9600, 19200, 38400}

// maxCandidatesPerSlave = batas kandidat per slave; probing register berhenti
// setelah tercapai agar port yang "hidup" tidak menghabiskan waktu.
const maxCandidatesPerSlave = 6

// ScanPorts memindai daftar port yang diberikan (di semua baud rate pada
// Options.BaudRates) dan mengembalikan kandidat terbaik (maksimal 15 di
// seluruh port). Kandidat lintas port/UID tidak saling menghapus karena dedup
// memperhitungkan identitas (port + slave).
func ScanPorts(ports []string, opts Options, progress ProgressFunc) []Candidate {
	if opts.UIDMax <= 0 {
		opts.UIDMax = 31
	}
	if opts.UIDMin < 1 {
		opts.UIDMin = 1
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 150 * time.Millisecond
	}

	bauds := opts.BaudRates
	if len(bauds) == 0 {
		if opts.BaudRate > 0 {
			bauds = []int{opts.BaudRate}
		} else {
			bauds = defaultBaudRates
		}
	}

	var all []Candidate
	total := len(ports)
	for pi, port := range ports {
		for _, baud := range bauds {
			localOpts := opts
			localOpts.BaudRate = baud
			if progress != nil {
				progress(pi, total, fmt.Sprintf("Memindai %s @ %d baud...", port, baud))
			}
			all = append(all, scanPort(port, localOpts, progress, pi, total)...)
		}
	}

	all = selectTopCandidates(all, 15)
	return all
}

// scanPort membuka koneksi serial ke satu port lalu mencoba seluruh rentang UID
// pada satu baud rate. Ada jeda stabilisasi untuk adapter USB serial
// multi-channel sekaligus progress per UID agar UI tidak terlihat macet.
func scanPort(port string, opts Options, progress ProgressFunc, pi, total int) []Candidate {
	handler := modbus.NewRTUClientHandler(port)
	handler.BaudRate = opts.BaudRate
	handler.DataBits = 8
	handler.Parity = "N"
	handler.StopBits = 1
	handler.Timeout = opts.Timeout

	if err := handler.Connect(); err != nil {
		log.Printf("SCAN: %s @ %d baud → gagal buka port: %v", port, opts.BaudRate, err)
		return nil
	}
	defer handler.Close()

	// Stabilisasi adapter USB-serial (terutama multi-channel) sebelum frame pertama.
	time.Sleep(150 * time.Millisecond)
	client := modbus.NewClient(handler)

	var out []Candidate
	for uid := opts.UIDMin; uid <= opts.UIDMax; uid++ {
		handler.SlaveId = byte(uid)
		if progress != nil {
			progress(pi, total, fmt.Sprintf("Memindai %s @ %d baud (UID %d-%d)...", port, opts.BaudRate, opts.UIDMin, opts.UIDMax))
		}
		out = append(out, inspectSlave(client, byte(uid), port, opts)...)
	}
	log.Printf("SCAN: %s @ %d baud selesai → %d kandidat", port, opts.BaudRate, len(out))
	return out
}

// presenceAddrPool = subset alamat yang cukup untuk mengetahui apakah ada
// perangkat merespons pada satu UID (mempercepat deteksi port yang kosong).
var presenceAddrPool = []uint16{0, 4096, 4608}

// inspectSlave memeriksa satu UID: apakah merespons, lalu mengevaluasi register
// untuk menemukan kandidat format/nilai yang wajar. Baca register diulang 2x
// dengan jeda anti-noise untuk mengurangi frame rusak di awal bus RS485.
func inspectSlave(client modbus.Client, uid byte, port string, opts Options) []Candidate {
	addrs := probeAddrPool
	if len(opts.Addrs) > 0 {
		addrs = opts.Addrs
	}
	formats := probeFormats
	if len(opts.Formats) > 0 {
		formats = opts.Formats
	}

	present := false
	for _, a := range presenceAddrPool {
		if presenceProbe(client, a) {
			present = true
			break
		}
	}
	if !present {
		return nil
	}

	var cands []Candidate
	for _, a := range addrs {
		b1, ok1 := readStable(client, a)
		if !ok1 {
			continue
		}
		time.Sleep(20 * time.Millisecond)
		b2, ok2 := readStable(client, a)
		if !ok2 {
			continue
		}
		if len(b1) < 4 || len(b2) < 4 ||
			b1[0] != b2[0] || b1[1] != b2[1] || b1[2] != b2[2] || b1[3] != b2[3] {
			continue // tidak stabil antar dua baca -> bukan nilai sensor yg valid
		}
		for _, f := range formats {
			v := modbusutil.DecodeValue(b1, f)
			s := scoreFormat(f, v)
			if s <= 0 {
				continue
			}
			cands = append(cands, Candidate{Port: port, SlaveID: uid, Register: a, Format: f, Value: v, Score: s})
		}

		// Cukup: perangkat sudah menghasilkan beberapa kandidat stabil.
		if len(cands) >= maxCandidatesPerSlave {
			break
		}
	}
	return selectTopCandidates(cands, maxCandidatesPerSlave)
}

// presenceProbe mengecek satu alamat dengan retry 1x (menangkal frame rusak
// akibat noise idle RS485). UID dianggap ada bila ada respons ATAU exception.
func presenceProbe(client modbus.Client, addr uint16) bool {
	if _, err := client.ReadHoldingRegisters(addr, 1); err == nil || isModbusException(err) {
		return true
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := client.ReadHoldingRegisters(addr, 1); err == nil || isModbusException(err) {
		return true
	}
	return false
}

// readStable membaca 2 register dengan retry 1x bila frame pertama gagal.
func readStable(client modbus.Client, addr uint16) ([]byte, bool) {
	b, err := client.ReadHoldingRegisters(addr, 2)
	if err == nil {
		return b, true
	}
	time.Sleep(30 * time.Millisecond)
	b2, err2 := client.ReadHoldingRegisters(addr, 2)
	if err2 == nil {
		return b2, true
	}
	return nil, false
}

// isModbusException membedakan error "device merespons tapi alamat tidak valid"
// dari timeout/error komunikasi. Substring khas dari goburrow/modbus.
func isModbusException(err error) bool {
	return err != nil && strings.Contains(err.Error(), "modbus: exception")
}

// scoreFormat menilai kewajaran sebuah nilai hasil decode.
// <=0 berarti nilai ditolak.
func scoreFormat(f string, v float64) int {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	av := math.Abs(v)
	if av > 1e7 || (av > 0 && av < 1e-2) {
		return 0 // nilai ekstrem / denormal → kemungkinan bypass salah
	}
	s := 100
	if av >= 1 && av <= 1e5 {
		s += 30 // rentang paling wajar untuk suhu/mg/L/dsb
	}
	if v != 0 {
		s += 20
	}
	if !strings.HasPrefix(f, "float") {
		s -= 20 // format integer jarang untuk sensor proses → prioritas lebih rendah
	}
	return s
}

// selectTopCandidates mengurutkan kandidat berdasarkan skor, mendedupe
// (per port + slave + register + format sehingga kandidat di COM/UID berbeda
// tidak saling membuang), dan membatasi jumlah hasil.
func selectTopCandidates(cands []Candidate, max int) []Candidate {
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Score > cands[j].Score })
	seen := make(map[string]bool)
	var top []Candidate
	for _, c := range cands {
		key := fmt.Sprintf("%s|%d|%d|%s", c.Port, c.SlaveID, c.Register, c.Format)
		if seen[key] {
			continue
		}
		seen[key] = true
		top = append(top, c)
		if len(top) >= max {
			break
		}
	}
	return top
}