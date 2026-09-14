package controllers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"sistem-monitoring-cod_golang/scanner"

	"github.com/gin-gonic/gin"
)

// ScanJob menyimpan status scan otomatis agar bisa di-poll oleh frontend.
type ScanJob struct {
	ID        string              `json:"id"`
	Status    string              `json:"status"` // running | done | error
	Message   string              `json:"message"`
	Progress  int                 `json:"progress"`
	Results   []scanner.Candidate `json:"results"`
	Error     string              `json:"error,omitempty"`
	CreatedAt time.Time           `json:"created_at"`
}

var (
	scanMu    sync.Mutex
	scanStore = map[string]*ScanJob{}
)

func newScanID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// StartSensorScan memulai scan otomatis secara async dan langsung mengembalikan
// scan_id untuk dipoll client. Hanya satu scan boleh berjalan pada satu waktu.
func StartSensorScan(c *gin.Context) {
	scanMu.Lock()
	for _, j := range scanStore {
		if j.Status == "running" {
			scanMu.Unlock()
			c.JSON(http.StatusConflict, gin.H{"error": "Scan otomatis sedang berjalan, tunggu sampai selesai."})
			return
		}
	}
	id := newScanID()
	job := &ScanJob{ID: id, Status: "running", Message: "Menunggu mulai...", CreatedAt: time.Now()}
	scanStore[id] = job

	// batasi penyimpanan: buang job selesai paling lama bila melebihi 10
	if len(scanStore) > 10 {
		for k, j := range scanStore {
			if j.Status != "running" {
				delete(scanStore, k)
				break
			}
		}
	}
	scanMu.Unlock()

	// Opsi scan opsional dari body: baud_rates, slave_min, slave_max.
	// Body kosong (tanpa body) berarti memakai default (sweep 9600/19200/38400).
	var opts scanner.Options
	var input struct {
		BaudRates []int `json:"baud_rates"`
		SlaveMin  int   `json:"slave_min"`
		SlaveMax  int   `json:"slave_max"`
	}
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		scanMu.Lock()
		delete(scanStore, id)
		scanMu.Unlock()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Format opsi scan tidak valid!"})
		return
	}
	if len(input.BaudRates) > 0 {
		for _, b := range input.BaudRates {
			if b >= 1200 && b <= 2000000 {
				opts.BaudRates = append(opts.BaudRates, b)
			}
		}
	}
	if input.SlaveMin > 1 {
		opts.UIDMin = input.SlaveMin
	}
	if input.SlaveMax >= input.SlaveMin && input.SlaveMax > 0 && input.SlaveMax <= 247 {
		opts.UIDMax = input.SlaveMax
	}

	scanner.SetActive(true)
	go func() {
		defer scanner.SetActive(false)

		ports, err := scanner.ListCOMPorts()
		if err != nil {
			finishScan(job, "error", "Gagal membaca daftar port: "+err.Error(), err.Error(), nil)
			return
		}
		if len(ports) == 0 {
			finishScan(job, "done", "Tidak ada port COM terdeteksi. Colokkan adaptor USB-RS485 dulu.", "", nil)
			return
		}

		results := scanner.ScanPorts(ports, opts, func(pi, total int, status string) {
			scanMu.Lock()
			job.Message = status
			if total > 0 {
				job.Progress = pi * 100 / total
			}
			scanMu.Unlock()
		})

		if len(results) == 0 {
			finishScan(job, "done", "Scan selesai: tidak ada kandidat ditemukan.", "", results)
			return
		}
		finishScan(job, "done", fmt.Sprintf("Scan selesai: %d kandidat ditemukan.", len(results)), "", results)
	}()

	c.JSON(http.StatusAccepted, gin.H{"scan_id": id})
}

// GetSensorScan mengembalikan status + hasil scan untuk polling frontend.
func GetSensorScan(c *gin.Context) {
	id := c.Param("id")
	scanMu.Lock()
	job := scanStore[id]
	scanMu.Unlock()
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Scan tidak ditemukan atau sudah kedaluwarsa."})
		return
	}
	c.JSON(http.StatusOK, job)
}

func finishScan(job *ScanJob, status, message, errMsg string, results []scanner.Candidate) {
	scanMu.Lock()
	defer scanMu.Unlock()
	job.Status = status
	job.Message = message
	job.Error = errMsg
	job.Progress = 100
	job.Results = results
}