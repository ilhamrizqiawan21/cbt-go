package config

import (
	"strings"
	"testing"
)

// env membuat fungsi getenv palsu dari sebuah map.
func env(m map[string]string) func(string) string {
	return func(key string) string { return m[key] }
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load() error = %v, ingin nil", err)
	}
	if cfg.Env != EnvDevelopment {
		t.Errorf("Env = %q, ingin %q", cfg.Env, EnvDevelopment)
	}
	if cfg.Port != 3000 {
		t.Errorf("Port = %d, ingin 3000", cfg.Port)
	}
	if cfg.DB.Name != "cbt_sekolah_go" {
		t.Errorf("DB.Name = %q, ingin cbt_sekolah_go", cfg.DB.Name)
	}
	if cfg.Timezone.String() != "Asia/Jakarta" {
		t.Errorf("Timezone = %q, ingin Asia/Jakarta", cfg.Timezone)
	}
	if cfg.TampilkanNilaiSiswa {
		t.Error("TampilkanNilaiSiswa harus false secara default (D-017)")
	}
	if cfg.IsProduction() {
		t.Error("IsProduction() harus false pada mode development")
	}
}

func TestLoad_Overrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"APP_ENV":     "production",
		"PORT":        "8080",
		"TRUST_PROXY": "true",
		"DB_NAME":     "cbt_sekolah_go_test",
		"DB_PASSWORD": " rahasia ", // spasi pada password harus dipertahankan
		"DB_SOCKET":   "/var/lib/mysql/mysql.sock",
		"LOG_LEVEL":   "debug",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v, ingin nil", err)
	}
	if !cfg.IsProduction() || cfg.Port != 8080 || !cfg.TrustProxy {
		t.Errorf("override tidak terbaca: %+v", cfg)
	}
	if cfg.DB.Password != " rahasia " {
		t.Errorf("DB.Password = %q, spasi tidak boleh di-trim", cfg.DB.Password)
	}
	if cfg.DB.Socket != "/var/lib/mysql/mysql.sock" {
		t.Errorf("DB.Socket = %q", cfg.DB.Socket)
	}
	if cfg.LogLevel.String() != "DEBUG" {
		t.Errorf("LogLevel = %s, ingin DEBUG", cfg.LogLevel)
	}
}

func TestLoad_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantMsg string
	}{
		{"port bukan angka", map[string]string{"PORT": "abc"}, "PORT harus berupa angka"},
		{"port di luar rentang", map[string]string{"PORT": "70000"}, "PORT di luar rentang"},
		{"env tidak dikenal", map[string]string{"APP_ENV": "staging"}, "APP_ENV tidak dikenal"},
		{"timezone salah", map[string]string{"APP_TIMEZONE": "Mars/Olympus"}, "APP_TIMEZONE"},
		{"bool salah", map[string]string{"TRUST_PROXY": "mungkin"}, "TRUST_PROXY harus true/false"},
		{"pool nol", map[string]string{"DB_MAX_OPEN_CONNS": "0"}, "DB_MAX_OPEN_CONNS minimal 1"},
		{"log level salah", map[string]string{"LOG_LEVEL": "berisik"}, "LOG_LEVEL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(env(tt.env))
			if err == nil {
				t.Fatal("Load() error = nil, ingin error")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, harus memuat %q", err, tt.wantMsg)
			}
		})
	}
}

func TestLoad_CollectsAllErrors(t *testing.T) {
	_, err := Load(env(map[string]string{"PORT": "abc", "APP_ENV": "staging"}))
	if err == nil {
		t.Fatal("Load() error = nil, ingin error")
	}
	for _, want := range []string{"PORT", "APP_ENV"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q harus memuat semua masalah, tidak ada %q", err, want)
		}
	}
}
