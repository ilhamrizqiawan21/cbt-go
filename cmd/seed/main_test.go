package main

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"cbt-go/internal/config"
	"cbt-go/internal/testdb"
	"cbt-go/migrations"

	"github.com/pressly/goose/v3"
)

func TestSeedProductionRejected(t *testing.T) {
	ctx := context.Background()
	err := runSeed(ctx, nil, config.EnvProduction)
	if err == nil {
		t.Fatal("diharapkan error bila seeding di mode production, dapat nil")
	}
	if !errors.Is(err, errProductionSeedBlocked) {
		t.Fatalf("diharapkan errProductionSeedBlocked, dapat: %v", err)
	}
}

func TestSeedIntegration(t *testing.T) {
	pool := testdb.Open(t)
	if pool == nil {
		t.Skip("database uji tidak dapat dihubungi")
	}

	ctx := context.Background()

	// Pastikan migrasi terbaru sudah diterapkan di DB test
	goose.SetBaseFS(migrations.FS)
	_ = goose.SetDialect("mysql")
	if err := goose.UpContext(ctx, pool, "."); err != nil {
		t.Fatalf("gagal migrasi DB test sebelum seed: %v", err)
	}

	// Bersihkan data lama
	testdb.Reset(t, pool)

	// 1. Jalankan seed pertama kali
	if err := runSeed(ctx, pool, config.EnvDevelopment); err != nil {
		t.Fatalf("seed pertama kali gagal: %v", err)
	}

	// Verifikasi hash bcrypt admin
	var adminPass string
	err := pool.QueryRowContext(ctx, "SELECT password FROM admin WHERE username = 'admin'").Scan(&adminPass)
	if err != nil {
		t.Fatalf("admin 'admin' tidak ditemukan: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(adminPass), []byte("admin123")); err != nil {
		t.Fatalf("hash password admin tidak cocok dengan 'admin123': %v", err)
	}

	// Verifikasi hash bcrypt guru
	var guruPass string
	err = pool.QueryRowContext(ctx, "SELECT password FROM guru WHERE username = 'guru'").Scan(&guruPass)
	if err != nil {
		t.Fatalf("guru 'guru' tidak ditemukan: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(guruPass), []byte("guru123")); err != nil {
		t.Fatalf("hash password guru tidak cocok dengan 'guru123': %v", err)
	}

	// Verifikasi hash bcrypt siswa
	var siswaPin string
	err = pool.QueryRowContext(ctx, "SELECT pin_ujian FROM siswa WHERE nis = '1001'").Scan(&siswaPin)
	if err != nil {
		t.Fatalf("siswa '1001' tidak ditemukan: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(siswaPin), []byte("1234")); err != nil {
		t.Fatalf("hash PIN siswa tidak cocok dengan '1234': %v", err)
	}

	// Hitung data awal
	var adminCount, guruCount, siswaCount, ujianCount, soalCount int
	_ = pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM admin").Scan(&adminCount)
	_ = pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM guru").Scan(&guruCount)
	_ = pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM siswa").Scan(&siswaCount)
	_ = pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM ujian").Scan(&ujianCount)
	_ = pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM soal").Scan(&soalCount)

	if siswaCount < 10 {
		t.Fatalf("diharapkan minimal 10 siswa sintetis, dapat %d", siswaCount)
	}
	if ujianCount < 1 {
		t.Fatalf("diharapkan minimal 1 ujian, dapat %d", ujianCount)
	}

	// 2. Jalankan seed kedua kali (harus idempoten)
	if err := runSeed(ctx, pool, config.EnvDevelopment); err != nil {
		t.Fatalf("seed kedua kali gagal: %v", err)
	}

	// Verifikasi data tidak menggandakan
	var adminCount2, guruCount2, siswaCount2, ujianCount2, soalCount2 int
	_ = pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM admin").Scan(&adminCount2)
	_ = pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM guru").Scan(&guruCount2)
	_ = pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM siswa").Scan(&siswaCount2)
	_ = pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM ujian").Scan(&ujianCount2)
	_ = pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM soal").Scan(&soalCount2)

	if adminCount != adminCount2 || guruCount != guruCount2 || siswaCount != siswaCount2 || ujianCount != ujianCount2 || soalCount != soalCount2 {
		t.Fatalf("seed tidak idempoten: sebelum=(admin:%d, guru:%d, siswa:%d, ujian:%d, soal:%d), sesudah=(admin:%d, guru:%d, siswa:%d, ujian:%d, soal:%d)",
			adminCount, guruCount, siswaCount, ujianCount, soalCount,
			adminCount2, guruCount2, siswaCount2, ujianCount2, soalCount2,
		)
	}
}
