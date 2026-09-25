package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/glebarez/go-sqlite"
	_ "github.com/go-sql-driver/mysql"
)

// cmd/migratemysql — migrasi data SQLite (monitoring.db) → MySQL, READ-ONLY
// terhadap file SQLite. Mempertahankan ID & timestamp asli, batch insert,
// progress, aman di-interupsi (resume memakai INSERT IGNORE).
// Tidak menghapus/mengubah monitoring.db.

var (
	tablesOrder = []string{
		"users",
		"api_keys",
		"settings",
		"sensor_types",
		"user_sensors",
		"notification_logs",
		"sensor_data",
		"sensor_averages",
	}
	timeCols = map[string]bool{
		"created_at":   true,
		"updated_at":   true,
		"deleted_at":   true,
		"window_start": true,
	}
)

func main() {
	checkOnly := flag.Bool("check", false, "hanya tampilkan jumlah record source vs destination")
	srcPath := flag.String("src", "monitoring.db", "lokasi file SQLite sumber (read-only)")
	batchSize := flag.Int("batch", 500, "ukuran batch insert")
	only := flag.String("only", "", "migrasi hanya tabel tertentu")
	flag.Parse()

	src, err := sql.Open("sqlite", "file:"+*srcPath+"?mode=ro&_pragma=busy_timeout(10000)")
	if err != nil {
		fatal("buka SQLite:", err)
	}
	defer src.Close()

	dsn := mysqlDSN()
	dst, err := sql.Open("mysql", dsn)
	if err != nil {
		fatal("buka MySQL:", err)
	}
	defer dst.Close()

	if *checkOnly {
		verifyAll(src, dst)
		return
	}

	for _, t := range tablesOrder {
		if *only != "" && t != *only {
			continue
		}
		migrateTable(src, dst, t, *batchSize)
	}
	fmt.Println("\nSelesai. Jalankan dengan --check untuk verifikasi jumlah.")
}

func mysqlDSN() string {
	host := envOr("DB_HOST", "127.0.0.1")
	port := envOr("DB_PORT", "3306")
	user := envOr("DB_USER", "monitoring")
	pass := envOr("DB_PASS", "")
	name := envOr("DB_NAME", "monitoring")
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		user, pass, host, port, name)
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func fatal(msg string, err error) {
	fmt.Fprintln(os.Stderr, msg, err)
	os.Exit(1)
}

// tableColumns mengembalikan daftar kolom tabel (dari sqlite PRAGMA).
func tableColumns(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query("PRAGMA table_info(" + quote(table) + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid, pk, nn int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &nn, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, nil
}

// verifyAll membandingkan jumlah record source vs destination per tabel.
func verifyAll(src, dst *sql.DB) {
	fmt.Printf("%-22s %12s %12s %10s\n", "TABEL", "SOURCE", "MYSQL", "SELISIH")
	for _, t := range tablesOrder {
		s, serr := countRows(src, t)
		m, merr := countRows(dst, t)
		if serr != nil {
			fmt.Printf("%-22s ERROR source: %v\n", t, serr)
			continue
		}
		if merr != nil {
			fmt.Printf("%-22s %12d %12s %10s  (tabel MySQL belum ada)\n", t, s, "-", "-")
			continue
		}
		diff := m - s
		extra := ""
		if diff < 0 {
			extra = "  <-- KURANG"
		} else if diff > 0 {
			extra = "  <-- LEBIH"
		}
		fmt.Printf("%-22s %12d %12d %10d%s\n", t, s, m, diff, extra)
	}
}

func countRows(db *sql.DB, table string) (int64, error) {
	var n int64
	err := db.QueryRow("SELECT COUNT(*) FROM " + quote(table)).Scan(&n)
	return n, err
}

// migrateTable memindahkan seluruh baris satu tabel dari sqlite ke MySQL.
func migrateTable(src, dst *sql.DB, table string, batch int) {
	srcCols, err := tableColumns(src, table)
	if err != nil {
		fmt.Printf("skip %s (baca kolom source): %v\n", table, err)
		return
	}
	dstCols, err := tableColumns(dst, table)
	if err != nil {
		fmt.Printf("skip %s (baca kolom MySQL): %v\n", table, err)
		return
	}
	// Kolom yang ada di kedua sisi (menangani kolom legacy semisal mqtt_topic).
	useCols := intersect(srcCols, dstCols)
	if len(useCols) == 0 {
		fmt.Printf("skip %s (tanpa kolom yang cocok)\n", table)
		return
	}

	total := rowCountOr(src, table)
	if total == 0 && table != "sensor_data" {
		fmt.Printf("%s: kosong (0 baris), dilewati.\n", table)
		return
	}

	// Menentukan sifat resume: bila MySQL sudah punya data pada tabel ini,
	// pakai INSERT IGNORE agar baris duluan tidak error PK.
	existing := rowCountOr(dst, table)
	insertMode := "INSERT"
	if existing > 0 {
		insertMode = "INSERT IGNORE"
		fmt.Printf("%s: destination sudah ada %d baris → memakai %s (resume).\n", table, existing, insertMode)
	}

	cols := strings.Join(quotedAll(useCols), ",")
	placeholders := "(" + strings.TrimRight(strings.Repeat("?,", len(useCols)), ",") + ")"

	stmt := fmt.Sprintf("SELECT %s FROM %s ORDER BY id ASC", cols, quote(table))

	rows, err := src.Query(stmt)
	if err != nil {
		fmt.Printf("skip %s (baca data): %v\n", table, err)
		return
	}
	defer rows.Close()

	selCOLS := useCols
	bufRows := make([][]any, 0, batch)
	copied := int64(0)
	var lastErr error

	for rows.Next() {
		vals := make([]any, len(selCOLS))
		dest := make([]any, len(selCOLS))
		for i := range vals {
			dest[i] = &vals[i]
		}
		if err := rows.Scan(dest...); err != nil {
			lastErr = err
			break
		}
		bufRows = append(bufRows, normalizeRow(selCOLS, vals))
		if len(bufRows) >= batch {
			if err := insertBatch(dst, table, insertMode, cols, placeholders, bufRows); err != nil {
				lastErr = err
				break
			}
			copied += int64(len(bufRows))
			progress(table, copied, total)
			bufRows = bufRows[:0]
		}
	}
	if lastErr == nil {
		lastErr = rows.Err()
	}
	if len(bufRows) > 0 && lastErr == nil {
		if err := insertBatch(dst, table, insertMode, cols, placeholders, bufRows); err != nil {
			lastErr = err
		}
		copied += int64(len(bufRows))
		progress(table, copied, total)
	}
	progressDone(table, copied, total)

	if lastErr != nil {
		fmt.Printf("\n%s → GAGAL di tengah (%v). Baris sudah di-commit per batch.\n  Lanjut ulangi perintah; mode resume akan melewati baris yang sudah ada.\n", table, lastErr)
		return
	}
	if copied > 0 {
		verifyOne(src, dst, table)
	}
}

func insertBatch(dst *sql.DB, table, mode, cols, placeholders string, rows [][]any) error {
	tx, err := dst.Begin()
	if err != nil {
		return err
	}
	q := fmt.Sprintf("%s INTO %s (%s) VALUES %s", mode, quote(table), cols,
		strings.Repeat(placeholders+",", len(rows))[:len(rows)*(len(placeholders)+1)-1])
	args := make([]any, 0, len(rows)*len(rows[0]))
	for _, r := range rows {
		args = append(args, r...)
	}
	if _, err := tx.Exec(q, args...); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// normalizeRow mengonversi nilai baca sqlite agar kompatibel dengan MySQL.
func normalizeRow(cols []string, vals []any) []any {
	out := make([]any, len(vals))
	for i, v := range vals {
		out[i] = normalizeValue(cols[i], v)
	}
	return out
}

func normalizeValue(col string, v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []byte:
		s := string(x)
		if timeCols[col] {
			if t, ok := parseTime(s); ok {
				return t
			}
		}
		return s
	case string:
		if timeCols[col] {
			if t, ok := parseTime(x); ok {
				return t
			}
		}
		return x
	default:
		return v
	}
}

func parseTime(s string) (time.Time, bool) {
	layouts := []string{
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func rowCountOr(db *sql.DB, table string) int64 {
	n, err := countRows(db, table)
	if err != nil {
		fmt.Printf("(!) gagal hitung %s: %v\n", table, err)
		return 0
	}
	return n
}

func verifyOne(src, dst *sql.DB, table string) {
	s, _ := countRows(src, table)
	m, _ := countRows(dst, table)
	if s != m {
		fmt.Printf("  [VERIFIKASI] %s: source=%d mysql=%d (%d belum sinkron)\n", table, s, m, m-s)
	}
}

func progress(table string, done, total int64) {
	pct := int64(0)
	if total > 0 {
		pct = done * 100 / total
	}
	fmt.Printf("\r%s: %d/%d (%d%%)      ", table, done, total, pct)
}

func progressDone(table string, done, total int64) {
	fmt.Printf("\r%s: %d/%d selesai.          \n", table, done, total)
}

func quote(s string) string {
	return "`" + s + "`"
}

func quotedAll(cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = quote(c)
	}
	return out
}

func intersect(a, b []string) []string {
	set := map[string]bool{}
	for _, x := range b {
		set[x] = true
	}
	var out []string
	for _, x := range a {
		if set[x] {
			out = append(out, x)
		}
	}
	return out
}