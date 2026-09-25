package main

import (
	"fmt"
	"os"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type SensorAverage struct {
	ID           uint
	SensorTypeID uint
	WindowStart  time.Time
	WindowSecs   int
	Average      float64
	Total        int64
}

func main() {
	db, err := gorm.Open(sqlite.Open("monitoring.db"), &gorm.Config{})
	if err != nil {
		fmt.Println("Gagal buka DB:", err)
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "clear" {
		db.Exec("DELETE FROM sensor_averages")
		fmt.Println("sensor_averages dikosongkan")
		return
	}
	var sd int64
	db.Table("sensor_data").Count(&sd)
	var a []SensorAverage
	db.Order("window_start desc").Limit(12).Find(&a)
	fmt.Printf("sensor_data=%d | sensor_averages=%d\n", sd, len(a))
	for _, r := range a {
		fmt.Printf("type=%d | %s (+%ds) | avg=%.2f | n=%d\n",
			r.SensorTypeID, r.WindowStart.Format("15:04"), r.WindowSecs, r.Average, r.Total)
	}
}