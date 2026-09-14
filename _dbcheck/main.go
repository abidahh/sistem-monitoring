package main

import (
	"fmt"
	"os"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	ID         uint   `gorm:"primaryKey"`
	Nama       string
	Username   string
	Password   string
	Role       string
	IsApproved bool
	Status     string
}

func main() {
	db, err := gorm.Open(sqlite.Open("monitoring.db"), &gorm.Config{})
	if err != nil {
		fmt.Println("Gagal buka DB:", err)
		os.Exit(1)
	}
	var users []User
	db.Order("id asc").Find(&users)
	for _, u := range users {
		_, err := bcrypt.Cost([]byte(u.Password))
		isBcrypt := err == nil
		fmt.Printf("ID=%d | username=%s | role=%s | approved=%v | status=%s | bcrypt=%v\n", u.ID, u.Username, u.Role, u.IsApproved, u.Status, isBcrypt)
		if isBcrypt {
			for _, candidate := range []string{"admin123", "password", "123456", "admin"} {
				if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(candidate)) == nil {
					fmt.Printf("  -> password match: %s\n", candidate)
				}
			}
		}
	}
}