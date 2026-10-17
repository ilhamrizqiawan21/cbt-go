package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/joho/godotenv"

	"cbt-go/internal/config"
	"cbt-go/internal/db"
	"cbt-go/internal/testdb"
)

func TestMigrateArgValidation(t *testing.T) {
	ctx := context.Background()

	// Tanpa argumen
	err := runCommand(ctx, nil, config.EnvDevelopment, []string{})
	if err == nil {
		t.Fatal("diharapkan error bila tanpa sub-perintah, dapat nil")
	}

	// Perintah tidak dikenal
	err = runCommand(ctx, nil, config.EnvDevelopment, []string{"unknown"})
	if err == nil {
		t.Fatal("diharapkan error bila sub-perintah tidak dikenal, dapat nil")
	}
}

func TestMigrateProductionDownRejected(t *testing.T) {
	ctx := context.Background()

	err := runCommand(ctx, nil, config.EnvProduction, []string{"down"})
	if err == nil {
		t.Fatal("diharapkan error bila menjalankan 'down' pada mode production, dapat nil")
	}
	if !errors.Is(err, errProductionDownBlocked) {
		t.Fatalf("diharapkan errProductionDownBlocked, dapat: %v", err)
	}
}

func TestMigrateIntegration(t *testing.T) {
	// Tes integrasi terhadap database uji.
	// Jika database uji tidak bisa dihubungi, lewati (t.Skip).
	if p := testdb.CariEnvRoot(); p != "" {
		_ = godotenv.Load(p)
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		testDBName := os.Getenv("DB_NAME_TEST")
		if testDBName == "" {
			testDBName = "cbt_sekolah_go_test"
		}
		os.Setenv("DB_NAME", testDBName)
		cfg, err = config.Load(os.Getenv)
		if err != nil {
			t.Skipf("lewati tes migrasi DB: konfigurasi tidak valid: %v", err)
		}
	}

	// Pastikan DB yang dipakai adalah DB test
	testDB := cfg.DB
	if testName := os.Getenv("DB_NAME_TEST"); testName != "" {
		testDB.Name = testName
	} else if testDB.Name != "cbt_sekolah_go_test" {
		testDB.Name = "cbt_sekolah_go_test"
	}

	ctx := context.Background()
	pool, err := db.Open(ctx, testDB, cfg.Timezone)
	if err != nil {
		t.Skipf("lewati tes migrasi DB: database uji %s tidak dapat dihubungi: %v", testDB.Name, err)
	}
	defer pool.Close()

	// Bersihkan schema_migrations / goose_db_version dan tabel jika ada sebelum mulai tes
	cleanupDB(t, pool)

	// 1. Up pertama kali
	if err := runCommand(ctx, pool, config.EnvTest, []string{"up"}); err != nil {
		t.Fatalf("migrasi up pertama gagal: %v", err)
	}

	// Verifikasi tabel siswa dan goose_db_version terbentuk
	var count int
	err = pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'siswa'").Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("tabel 'siswa' diharapkan ada setelah migrasi up, count=%d, err=%v", count, err)
	}

	// 2. Up kedua kali (harus idempoten, tidak error)
	if err := runCommand(ctx, pool, config.EnvTest, []string{"up"}); err != nil {
		t.Fatalf("migrasi up kedua kali (idempoten) gagal: %v", err)
	}

	// 3. Status
	if err := runCommand(ctx, pool, config.EnvTest, []string{"status"}); err != nil {
		t.Fatalf("migrasi status gagal: %v", err)
	}

	// 4. Down satu langkah: hanya migrasi terbaru (00002 izin_susulan) yang dicabut, data lain utuh.
	if err := runCommand(ctx, pool, config.EnvTest, []string{"down"}); err != nil {
		t.Fatalf("migrasi down gagal: %v", err)
	}
	tabelAda := func(nama string) int {
		var n int
		if err := pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", nama).Scan(&n); err != nil {
			t.Fatalf("cek tabel %s: %v", nama, err)
		}
		return n
	}
	if tabelAda("izin_susulan") != 0 || tabelAda("siswa") != 1 {
		t.Fatalf("setelah down satu langkah: izin_susulan=%d (want 0), siswa=%d (want 1)", tabelAda("izin_susulan"), tabelAda("siswa"))
	}

	// Down sekali lagi mencabut baseline sehingga tabel siswa ikut hilang.
	if err := runCommand(ctx, pool, config.EnvTest, []string{"down"}); err != nil {
		t.Fatalf("migrasi down baseline gagal: %v", err)
	}
	if tabelAda("siswa") != 0 {
		t.Fatalf("tabel 'siswa' diharapkan terhapus setelah down baseline")
	}

	// 5. Up kembali
	if err := runCommand(ctx, pool, config.EnvTest, []string{"up"}); err != nil {
		t.Fatalf("migrasi up kembali setelah down gagal: %v", err)
	}
}

func cleanupDB(t *testing.T, pool *sql.DB) {
	t.Helper()
	// Drop semua tabel di DB test agar kondisi bersih
	ctx := context.Background()
	rows, err := pool.QueryContext(ctx, `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = ? AND table_type = 'BASE TABLE'
	`, "cbt_sekolah_go_test")
	if err != nil {
		return
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			tables = append(tables, name)
		}
	}

	if len(tables) > 0 {
		conn, err := pool.Conn(ctx)
		if err != nil {
			return
		}
		defer conn.Close()

		_, _ = conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0")
		for _, tbl := range tables {
			_, _ = conn.ExecContext(ctx, "DROP TABLE IF EXISTS `"+tbl+"`")
		}
		_, _ = conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1")
	}
}
