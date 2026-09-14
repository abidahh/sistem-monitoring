package controllers

import (
	"errors"
	"net/http"
	"strconv"

	"sistem-monitoring-cod_golang/config"
	"sistem-monitoring-cod_golang/models"
	"sistem-monitoring-cod_golang/modbusutil"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GET DAFTAR SEMUA TIPE SENSOR
func GetSensorTypes(c *gin.Context) {
	var types []models.SensorType
	if err := config.DB.Order("id asc").Find(&types).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memuat tipe sensor!"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sensor_types": types})
}

// BUAT TIPE SENSOR BARU (DENGAN KONFIGURASI MODBUS)
func CreateSensorType(c *gin.Context) {
	var input struct {
		Nama           string  `json:"nama" binding:"required"`
		Unit           string  `json:"unit" binding:"required"`
		NilaiMax       float64 `json:"nilai_max"`
		Aktif          *bool   `json:"aktif"`
		Warna          string  `json:"warna"`
		Sumber         string  `json:"sumber"`
		PortCom        string  `json:"port_com"`
		SlaveID        byte    `json:"slave_id"`
		RegisterAddr   uint16  `json:"register_addr"`
		BaudRate       int     `json:"baud_rate"`
		RegisterFormat string  `json:"register_format"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nama sensor dan unit wajib diisi!"})
		return
	}

	var existing models.SensorType
	err := config.DB.Where("nama = ?", input.Nama).First(&existing).Error
	if err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nama sensor sudah terdaftar!"})
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memeriksa ketersediaan nama sensor!"})
		return
	}

	aktif := true
	if input.Aktif != nil {
		aktif = *input.Aktif
	}
	warna := "#0d9488"
	if input.Warna != "" {
		warna = input.Warna
	}
	sumber := "simulasi"
	if input.Sumber == "real" {
		sumber = "real"
	}

	// Default nilai Modbus bila tidak dikirim dari frontend
	portCom := "COM3"
	if input.PortCom != "" {
		portCom = input.PortCom
	}
	var slaveID byte = 1
	if input.SlaveID != 0 {
		slaveID = input.SlaveID
	}
	baudRate := 9600
	if input.BaudRate != 0 {
		baudRate = input.BaudRate
	}
	registerFormat := "float.be"
	if modbusutil.IsValidFormat(input.RegisterFormat) {
		registerFormat = input.RegisterFormat
	}

	st := models.SensorType{
		Nama:           input.Nama,
		Unit:           input.Unit,
		NilaiMax:       input.NilaiMax,
		Aktif:          aktif,
		Warna:          warna,
		Sumber:         sumber,
		PortCom:        portCom,
		SlaveID:        slaveID,
		RegisterAddr:   input.RegisterAddr,
		BaudRate:       baudRate,
		RegisterFormat: registerFormat,
	}

	if err := config.DB.Create(&st).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menyimpan tipe sensor!"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Tipe sensor berhasil dibuat!", "sensor_type": st})
}

// UPDATE TIPE SENSOR (DENGAN KONFIGURASI MODBUS)
func UpdateSensorType(c *gin.Context) {
	id := c.Param("id")

	var st models.SensorType
	if err := config.DB.First(&st, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Tipe sensor tidak ditemukan!"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memuat tipe sensor!"})
		}
		return
	}

	var input struct {
		Nama           string  `json:"nama" binding:"required"`
		Unit           string  `json:"unit" binding:"required"`
		NilaiMax       float64 `json:"nilai_max"`
		Aktif          *bool   `json:"aktif"`
		Warna          string  `json:"warna"`
		Sumber         string  `json:"sumber"`
		PortCom        string  `json:"port_com"`
		SlaveID        byte    `json:"slave_id"`
		RegisterAddr   uint16  `json:"register_addr"`
		BaudRate       int     `json:"baud_rate"`
		RegisterFormat string  `json:"register_format"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nama sensor dan unit wajib diisi!"})
		return
	}

	var existing models.SensorType
	err := config.DB.Where("nama = ? AND id <> ?", input.Nama, st.ID).First(&existing).Error
	if err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nama sensor sudah terdaftar!"})
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memeriksa ketersediaan nama sensor!"})
		return
	}

	st.Nama = input.Nama
	st.Unit = input.Unit
	st.NilaiMax = input.NilaiMax
	if input.Aktif != nil {
		st.Aktif = *input.Aktif
	}
	if input.Warna != "" {
		st.Warna = input.Warna
	}
	if input.Sumber == "simulasi" || input.Sumber == "real" {
		st.Sumber = input.Sumber
	}

	// Update Nilai Modbus
	if input.PortCom != "" {
		st.PortCom = input.PortCom
	}
	st.SlaveID = input.SlaveID
	st.RegisterAddr = input.RegisterAddr
	if input.BaudRate != 0 {
		st.BaudRate = input.BaudRate
	}
	if modbusutil.IsValidFormat(input.RegisterFormat) {
		st.RegisterFormat = input.RegisterFormat
	}

	if err := config.DB.Save(&st).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menyimpan tipe sensor!"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tipe sensor berhasil diperbarui!", "sensor_type": st})
}

// HAPUS TIPE SENSOR
func DeleteSensorType(c *gin.Context) {
	id := c.Param("id")

	var st models.SensorType
	if err := config.DB.First(&st, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Tipe sensor tidak ditemukan!"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memuat tipe sensor!"})
		}
		return
	}

	config.DB.Where("sensor_type_id = ?", st.ID).Delete(&models.SensorData{})
	config.DB.Where("sensor_type_id = ?", st.ID).Delete(&models.UserSensor{})

	if err := config.DB.Delete(&st).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menghapus tipe sensor!"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tipe sensor berhasil dihapus!"})
}

// GET SENSOR YANG DIPILIH UNTUK SEBUAH USER
func GetUserSensors(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID user tidak valid!"})
		return
	}

	var assignments []models.UserSensor
	config.DB.Where("user_id = ?", uint(id)).Find(&assignments)

	ids := make([]uint, 0, len(assignments))
	for _, a := range assignments {
		ids = append(ids, a.SensorTypeID)
	}

	c.JSON(http.StatusOK, gin.H{"sensor_type_ids": ids})
}

// SIMPAN SENSOR YANG DITAMPILKAN UNTUK SEBUAH USER (REPLACE)
func SetUserSensors(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID user tidak valid!"})
		return
	}

	var input struct {
		SensorTypeIDs []uint `json:"sensor_type_ids"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Format input tidak valid!"})
		return
	}

	var user models.User
	if err := config.DB.First(&user, uint(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User tidak ditemukan!"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memuat user!"})
		}
		return
	}

	for _, sid := range input.SensorTypeIDs {
		var st models.SensorType
		if err := config.DB.First(&st, sid).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Ada tipe sensor yang tidak ditemukan!"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memverifikasi tipe sensor!"})
			return
		}
	}

	if err := config.DB.Where("user_id = ?", uint(id)).Delete(&models.UserSensor{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memperbarui sensor user!"})
		return
	}

	for _, sid := range input.SensorTypeIDs {
		config.DB.Create(&models.UserSensor{UserID: uint(id), SensorTypeID: sid})
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sensor user berhasil diperbarui!"})
}

// Helper: daftar SensorTypeID yang tampil untuk user yang sedang login.
func currentUserSensorTypeIDs(c *gin.Context) ([]uint, error) {
	session := sessions.Default(c)
	userID, ok := session.Get("user_id").(uint)
	if !ok {
		return nil, errors.New("user session tidak valid")
	}

	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		return nil, err
	}

	if user.Role == "admin" {
		var types []models.SensorType
		config.DB.Where("aktif = ?", true).Find(&types)
		ids := make([]uint, 0, len(types))
		for _, t := range types {
			ids = append(ids, t.ID)
		}
		return ids, nil
	}

	var assignments []models.UserSensor
	config.DB.Where("user_id = ?", userID).Find(&assignments)
	ids := make([]uint, 0, len(assignments))
	for _, a := range assignments {
		ids = append(ids, a.SensorTypeID)
	}
	return ids, nil
}