// Package testdb menyediakan utilitas pengujian database untuk cbt-go.
//
// Paket ini hanya boleh digunakan di lingkungan tes dan secara ketat
// menolak terhubung ke database yang tidak berakhiran "_test".
package testdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"

	"cbt-go/internal/config"
	"cbt-go/internal/db"
	"cbt-go/migrations"
)

const defaultTestDB = "cbt_sekolah_go_test"

func validateDBName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("nama database tidak boleh kosong")
	}
	if !strings.HasSuffix(trimmed, "_test") {
		return fmt.Errorf("nama database %q ditolak: harus berakhiran '_test'", trimmed)
	}
	return nil
}

// Open membuka koneksi ke database pengujian.
// Fungsi ini menolak jika nama database bukan database uji (*_test).
// Jika database tidak dapat dihubungi, tes akan dilewati (t.Skip).
func Open(t testing.TB) *sql.DB {
	t.Helper()

	_ = godotenv.Load() // abaikan jika .env tidak ada

	cfg, err := config.Load(os.Getenv)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		// Jika config gagal karena DB_NAME di .env bukan _test, periksa DB_NAME_TEST
		testDBName := os.Getenv("DB_NAME_TEST")
		if testDBName == "" {
			testDBName = defaultTestDB
		}
		os.Setenv("DB_NAME", testDBName)
		cfg, err = config.Load(os.Getenv)
		if err != nil {
			t.Fatalf("testdb: gagal memuat konfigurasi: %v", err)
		}
	}

	// Jika nama DB dari env bukan _test, coba ambil dari DB_NAME_TEST atau defaultTestDB
	if err := validateDBName(cfg.DB.Name); err != nil {
		if testName := os.Getenv("DB_NAME_TEST"); testName != "" {
			cfg.DB.Name = testName
		} else {
			cfg.DB.Name = defaultTestDB
		}
	}

	if err := validateDBName(cfg.DB.Name); err != nil {
		t.Fatalf("testdb: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, cfg.DB, cfg.Timezone)
	if err != nil {
		t.Skipf("testdb: database uji %s tidak dapat dihubungi: %v", cfg.DB.Name, err)
		return nil
	}

	t.Cleanup(func() {
		_ = pool.Close()
	})

	// Pastikan skema migrasi sudah terpasang di database uji
	goose.SetBaseFS(migrations.FS)
	_ = goose.SetDialect("mysql")
	if err := goose.UpContext(ctx, pool, "."); err != nil && !errors.Is(err, goose.ErrNoNextVersion) {
		t.Fatalf("testdb: gagal memastikan migrasi up: %v", err)
	}

	return pool
}

// Reset mengosongkan semua tabel pada database uji dengan aman-FK,
// tanpa menghapus tabel goose_db_version.
func Reset(t testing.TB, pool *sql.DB) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := pool.QueryContext(ctx, `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'
	`)
	if err != nil {
		t.Fatalf("testdb.Reset: gagal mengambil daftar tabel: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("testdb.Reset: gagal scan nama tabel: %v", err)
		}
		if name != "goose_db_version" && name != "schema_migrations" {
			tables = append(tables, name)
		}
	}

	if len(tables) == 0 {
		return
	}

	conn, err := pool.Conn(ctx)
	if err != nil {
		t.Fatalf("testdb.Reset: gagal memperoleh koneksi tunggal: %v", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		t.Fatalf("testdb.Reset: gagal menonaktifkan foreign key checks: %v", err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "SET FOREIGN_KEY_CHECKS = 1")
	}()

	for _, tbl := range tables {
		if _, err := conn.ExecContext(ctx, "TRUNCATE TABLE `"+tbl+"`"); err != nil {
			t.Fatalf("testdb.Reset: gagal truncate tabel %s: %v", tbl, err)
		}
	}
}
