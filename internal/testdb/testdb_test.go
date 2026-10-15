package testdb

import (
	"context"
	"strings"
	"testing"
)

func TestValidateTestDBName(t *testing.T) {
	tests := []struct {
		name    string
		dbName  string
		wantErr bool
	}{
		{"valid test db", "cbt_sekolah_go_test", false},
		{"valid suffix _test", "my_app_test", false},
		{"invalid dev db", "cbt_sekolah_go", true},
		{"invalid node db", "cbt_sekolah", true},
		{"empty name", "", true},
		{"test not suffix", "test_cbt", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDBName(tc.dbName)
			if tc.wantErr && err == nil {
				t.Fatalf("diharapkan error untuk nama DB %q, dapat nil", tc.dbName)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("tidak diharapkan error untuk nama DB %q, dapat: %v", tc.dbName, err)
			}
		})
	}
}

func TestOpenAndReset(t *testing.T) {
	pool := Open(t)
	if pool == nil {
		t.Fatal("pool tidak boleh nil jika tidak di-skip")
	}

	// Masukkan data dummy jika tabel ada
	ctx := context.Background()
	_, err := pool.ExecContext(ctx, "INSERT IGNORE INTO kelas (nama_kelas) VALUES ('TEST-A')")
	if err != nil {
		if strings.Contains(err.Error(), "doesn't exist") {
			t.Skip("tabel kelas belum ada, jalankan migrasi dulu")
		}
		t.Fatalf("gagal insert data dummy: %v", err)
	}

	// Reset harus mengosongkan tabel kelas tanpa menyentuh goose_db_version
	Reset(t, pool)

	var count int
	err = pool.QueryRowContext(ctx, "SELECT COUNT(*) FROM kelas").Scan(&count)
	if err != nil {
		t.Fatalf("gagal query tabel kelas setelah reset: %v", err)
	}
	if count != 0 {
		t.Fatalf("tabel kelas diharapkan kosong setelah Reset, dapat count = %d", count)
	}
}
