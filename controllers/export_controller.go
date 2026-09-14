package controllers

import (
	"fmt"
	"net/http"
	"time"

	"sistem-monitoring-cod_golang/config"
	"sistem-monitoring-cod_golang/models"

	"github.com/gin-gonic/gin"
	"github.com/jung-kurt/gofpdf"
	"github.com/xuri/excelize/v2"
)

// Helper: ambil tipe sensor yang tampil untuk user yang sedang login
func exportSensorTypes(c *gin.Context) []models.SensorType {
	ids, err := currentUserSensorTypeIDs(c)
	if err != nil || len(ids) == 0 {
		return []models.SensorType{}
	}
	var types []models.SensorType
	config.DB.Where("id IN ?", ids).Find(&types)
	return types
}

// sumberLabel mengubah nilai sumber ('real'/'simulasi') menjadi label terbaca.
func sumberLabel(s string) string {
	if s == "simulasi" {
		return "Simulasi"
	}
	return "Real-time"
}

// 1. EXPORT TO EXCEL (.xlsx) — KOLOM DINAMIS PER SENSOR
func ExportExcel(c *gin.Context) {
	sensorTypes := exportSensorTypes(c)

	ids := make([]uint, 0, len(sensorTypes))
	for _, t := range sensorTypes {
		ids = append(ids, t.ID)
	}

	query := config.DB.Preload("SensorType")
	if len(ids) > 0 {
		query = query.Where("sensor_type_id IN ?", ids)
	}
	query = query.Order("created_at asc")

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	if startDate != "" && endDate != "" {
		start, errStart := time.Parse("2006-01-02", startDate)
		end, errEnd := time.Parse("2006-01-02", endDate)
		if errStart == nil && errEnd == nil {
			query = query.Where("created_at >= ? AND created_at < ?", start, end.AddDate(0, 0, 1))
		}
	}

	var sensorData []models.SensorData
	query.Find(&sensorData)

	f := excelize.NewFile()
	defer f.Close()

	sheet := "Data"
	f.SetSheetName("Sheet1", sheet)

	// Header: No, Waktu, Sumber, lalu satu kolom per tipe sensor
	f.SetCellValue(sheet, "A1", "No")
	f.SetCellValue(sheet, "B1", "Waktu")
	f.SetCellValue(sheet, "C1", "Sumber Data")
	colIdx := 4
	typeColumn := make(map[uint]int) // sensor_type_id -> excel column
	for _, t := range sensorTypes {
		colName, _ := excelize.ColumnNumberToName(colIdx)
		f.SetCellValue(sheet, fmt.Sprintf("%s1", colName), fmt.Sprintf("%s (%s)", t.Nama, t.Unit))
		typeColumn[t.ID] = colIdx
		colIdx++
	}

	// Isi Data: satu baris per pembacaan, nilai di kolom sensor terkait
	for i, data := range sensorData {
		row := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), i+1)
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), data.CreatedAt.Format("02-01-2006 15:04:05"))
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), sumberLabel(data.Sumber))
		if col, ok := typeColumn[data.SensorTypeID]; ok {
			colName, _ := excelize.ColumnNumberToName(col)
			f.SetCellValue(sheet, fmt.Sprintf("%s%d", colName, row), data.Value)
		}
	}

	if len(sensorTypes) > 0 && len(sensorData) > 0 {
		lastRow := len(sensorData) + 1

		series := make([]excelize.ChartSeries, 0, len(sensorTypes))
		for _, t := range sensorTypes {
			col, ok := typeColumn[t.ID]
			if !ok {
				continue
			}
			colName, _ := excelize.ColumnNumberToName(col)
			series = append(series, excelize.ChartSeries{
				Name:       fmt.Sprintf("%s (%s)", t.Nama, t.Unit),
				Categories: fmt.Sprintf("%s!$B$2:$B$%d", sheet, lastRow),
				Values:     fmt.Sprintf("%s!$%s$2:$%s$%d", sheet, colName, colName, lastRow),
			})
		}

		chartCol, _ := excelize.ColumnNumberToName(colIdx)
		err := f.AddChart(sheet, fmt.Sprintf("%s2", chartCol), &excelize.Chart{
			Type: excelize.Line,
			Series: series,
			Dimension: excelize.ChartDimension{Width: 760, Height: 420},
			Title: excelize.ChartTitle{
				Paragraph: []excelize.RichTextRun{{Text: "Grafik Pemantauan Sensor"}},
			},
			Legend: excelize.ChartLegend{Position: "bottom"},
			XAxis: excelize.ChartAxis{
				Title: excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: "Waktu"}}},
			},
			YAxis: excelize.ChartAxis{
				Title:          excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: "Nilai"}}},
				MajorGridLines: true,
			},
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membuat grafik Excel!"})
			return
		}
	}

	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", "attachment; filename=Laporan_Monitoring_Sensor.xlsx")
	c.Header("File-Transfer-Encoding", "binary")

	_ = f.Write(c.Writer)
}

// 2. EXPORT TO PDF (.pdf) — KOLOM DINAMIS PER SENSOR
func ExportPDF(c *gin.Context) {
	sensorTypes := exportSensorTypes(c)

	ids := make([]uint, 0, len(sensorTypes))
	for _, t := range sensorTypes {
		ids = append(ids, t.ID)
	}

	query := config.DB.Preload("SensorType")
	if len(ids) > 0 {
		query = query.Where("sensor_type_id IN ?", ids)
	}
	query = query.Order("created_at desc")

	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	if startDate != "" && endDate != "" {
		start, errStart := time.Parse("2006-01-02", startDate)
		end, errEnd := time.Parse("2006-01-02", endDate)
		if errStart == nil && errEnd == nil {
			query = query.Where("created_at >= ? AND created_at < ?", start, end.AddDate(0, 0, 1))
		}
	}

	var sensorData []models.SensorData
	query.Find(&sensorData)

	pdf := gofpdf.New("L", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)

	pdf.CellFormat(277, 10, "LAPORAN MONITORING SENSOR", "0", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(277, 6, "Sistem Monitoring Real-Time", "0", 1, "C", false, 0, "")
	pdf.Ln(5)

	// Header Tabel
	noW := 15.0
	timeW := 60.0
	sumberW := 24.0
	dataW := 10.0
	if len(sensorTypes) > 0 {
		dataW = (277 - noW - timeW - sumberW) / float64(len(sensorTypes))
	}

	pdf.SetFont("Arial", "B", 8)
	pdf.CellFormat(noW, 8, "No", "1", 0, "C", false, 0, "")
	pdf.CellFormat(timeW, 8, "Waktu", "1", 0, "C", false, 0, "")
	pdf.CellFormat(sumberW, 8, "Sumber Data", "1", 0, "C", false, 0, "")
	for _, t := range sensorTypes {
		pdf.CellFormat(dataW, 8, fmt.Sprintf("%s (%s)", t.Nama, t.Unit), "1", 0, "C", false, 0, "")
	}
	pdf.Ln(-1)

	// Isi Data
	pdf.SetFont("Arial", "", 8)
	for i, data := range sensorData {
		if i > 0 && i%28 == 0 {
			pdf.AddPage()
			pdf.SetFont("Arial", "B", 8)
			pdf.CellFormat(noW, 8, "No", "1", 0, "C", false, 0, "")
			pdf.CellFormat(timeW, 8, "Waktu", "1", 0, "C", false, 0, "")
			pdf.CellFormat(sumberW, 8, "Sumber Data", "1", 0, "C", false, 0, "")
			for _, t := range sensorTypes {
				pdf.CellFormat(dataW, 8, fmt.Sprintf("%s (%s)", t.Nama, t.Unit), "1", 0, "C", false, 0, "")
			}
			pdf.Ln(-1)
			pdf.SetFont("Arial", "", 8)
		}
		pdf.CellFormat(noW, 7, fmt.Sprintf("%d", i+1), "1", 0, "C", false, 0, "")
		pdf.CellFormat(timeW, 7, data.CreatedAt.Format("02-01-2006 15:04:05"), "1", 0, "C", false, 0, "")
		pdf.CellFormat(sumberW, 7, sumberLabel(data.Sumber), "1", 0, "C", false, 0, "")
		for _, t := range sensorTypes {
			cell := ""
			if data.SensorTypeID == t.ID {
				cell = fmt.Sprintf("%.1f", data.Value)
			}
			pdf.CellFormat(dataW, 7, cell, "1", 0, "C", false, 0, "")
		}
		pdf.Ln(-1)
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", "attachment; filename=Laporan_Monitoring_Sensor.pdf")

	_ = pdf.Output(c.Writer)
}
