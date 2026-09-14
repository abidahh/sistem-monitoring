package controllers

import (
	"errors"
	"net/http"

	"sistem-monitoring-cod_golang/config"
	"sistem-monitoring-cod_golang/models"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// 1. LOGIN USER
func Login(c *gin.Context) {
	var input struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username dan Password wajib diisi!"})
		return
	}

	var user models.User
	if err := config.DB.Where("username = ?", input.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Username atau Password salah!"})
		return
	}

	// Cek Password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Username atau Password salah!"})
		return
	}

	// CEK APPROVAL ADMIN
	if !user.IsApproved || user.Status != "approved" {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Akun Anda belum disetujui oleh Admin! Silakan hubungi Administrator.",
		})
		return
	}

	// Simpan ke Session
	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Set("username", user.Username)
	session.Set("role", user.Role)
	session.Save()

	c.JSON(http.StatusOK, gin.H{
		"message": "Login berhasil!",
		"role":    user.Role,
		"nama":    user.Nama,
	})
}

// 3. GET DAFTAR SEMUA USER (Khusus Admin)
func GetUsers(c *gin.Context) {
	var users []models.User
	config.DB.Order("created_at desc").Find(&users)

	c.JSON(http.StatusOK, gin.H{
		"users": users,
	})
}

// 4. BUAT USER BARU (Khusus Admin) - Langsung Approved
func CreateUser(c *gin.Context) {
	var input struct {
		Nama     string `json:"nama" binding:"required"`
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required,min=6"`
		Role     string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Semua field harus diisi dan password minimal 6 karakter!"})
		return
	}

	if input.Role != "admin" && input.Role != "operator" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Role harus 'admin' atau 'operator'!"})
		return
	}

	var existingUser models.User
	err := config.DB.Where("username = ?", input.Username).First(&existingUser).Error
	if err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username sudah terdaftar!"})
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memeriksa ketersediaan username!"})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengenkripsi password!"})
		return
	}

	newUser := models.User{
		Nama:       input.Nama,
		Username:   input.Username,
		Password:   string(hashedPassword),
		Role:       input.Role,
		IsApproved: true,
		Status:     "approved",
	}

	if err := config.DB.Create(&newUser).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menyimpan user!"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Akun berhasil dibuat dan langsung aktif!",
		"user":    newUser,
	})
}

// 5. AKTIFKAN / NONAKTIFKAN USER (Khusus Admin)
func ToggleUserStatus(c *gin.Context) {
	id := c.Param("id")

	session := sessions.Default(c)
	currentUserID := session.Get("user_id")

	var user models.User
	if err := config.DB.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User tidak ditemukan!"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memuat data user!"})
		}
		return
	}

	if selfID, ok := currentUserID.(uint); ok && user.ID == selfID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tidak dapat menonaktifkan akun sendiri!"})
		return
	}

	user.IsApproved = !user.IsApproved
	if user.IsApproved {
		user.Status = "approved"
	} else {
		user.Status = "rejected"
	}
	if err := config.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menyimpan status akun, silakan coba lagi!"})
		return
	}

	status := "dinonaktifkan"
	if user.IsApproved {
		status = "diaktifkan"
	}
	c.JSON(http.StatusOK, gin.H{"message": "Akun berhasil " + status + "!", "user": user})
}

// 6. HAPUS USER (Khusus Admin)
func DeleteUser(c *gin.Context) {
	id := c.Param("id")

	session := sessions.Default(c)
	currentUserID := session.Get("user_id")

	var user models.User
	if err := config.DB.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User tidak ditemukan!"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memuat data user!"})
		}
		return
	}

	if selfID, ok := currentUserID.(uint); ok && user.ID == selfID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tidak dapat menghapus akun sendiri!"})
		return
	}

	if err := config.DB.Delete(&models.User{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menghapus user!"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Akun berhasil dihapus!"})
}

// 7. LOGOUT
func Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.JSON(http.StatusOK, gin.H{"message": "Berhasil logout."})
}
// 8. GET PROFILE USER LOGGED IN
func GetProfile(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id")

	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User tidak ditemukan!"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         user.ID,
		"nama":       user.Nama,
		"username":   user.Username,
		"role":       user.Role,
		"created_at": user.CreatedAt,
	})
}

// 9. UPDATE PROFIL (NAMA & GANTI PASSWORD)
func UpdateProfile(c *gin.Context) {
	session := sessions.Default(c)
	userID := session.Get("user_id")

	var user models.User
	if err := config.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User tidak ditemukan!"})
		return
	}

	var input struct {
		Nama        string `json:"nama" binding:"required"`
		CurrentPass string `json:"current_password"`
		NewPass     string `json:"new_password"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nama tidak boleh kosong!"})
		return
	}

	user.Nama = input.Nama

	// Ganti password hanya jika new_password diisi
	if input.NewPass != "" {
		if input.CurrentPass == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Password lama wajib diisi untuk mengganti password!"})
			return
		}
		if len(input.NewPass) < 6 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Password baru minimal 6 karakter!"})
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.CurrentPass)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Password lama salah!"})
			return
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(input.NewPass), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengenkripsi password!"})
			return
		}
		user.Password = string(hashed)
	}

	if err := config.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menyimpan profil!"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Profil berhasil diperbarui!",
		"id":         user.ID,
		"nama":       user.Nama,
		"username":   user.Username,
		"role":       user.Role,
		"created_at": user.CreatedAt,
	})
}