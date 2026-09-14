package controllers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"sistem-monitoring-cod_golang/config"
	"sistem-monitoring-cod_golang/models"

	"github.com/gin-gonic/gin"
)

func generateAPIKey() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func GetApiKeys(c *gin.Context) {
	var keys []models.ApiKey
	config.DB.Order("created_at desc").Find(&keys)
	c.JSON(http.StatusOK, gin.H{"keys": keys})
}

func CreateApiKey(c *gin.Context) {
	var input struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nama API Key harus diisi!"})
		return
	}

	newKey := models.ApiKey{
		Name:   input.Name,
		Key:    generateAPIKey(),
		Active: true,
	}
	config.DB.Create(&newKey)

	c.JSON(http.StatusCreated, gin.H{
		"message": "API Key berhasil dibuat!",
		"key":     newKey,
	})
}

func ToggleApiKey(c *gin.Context) {
	id := c.Param("id")
	var key models.ApiKey
	if err := config.DB.First(&key, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "API Key tidak ditemukan!"})
		return
	}

	key.Active = !key.Active
	config.DB.Save(&key)

	status := "dinonaktifkan"
	if key.Active {
		status = "diaktifkan"
	}
	c.JSON(http.StatusOK, gin.H{"message": "API Key berhasil " + status + "!", "key": key})
}

func DeleteApiKey(c *gin.Context) {
	id := c.Param("id")
	if err := config.DB.Delete(&models.ApiKey{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menghapus API Key!"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "API Key berhasil dihapus!"})
}
