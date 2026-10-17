// Package config membaca konfigurasi aplikasi dari environment variable.
//
// Load menerima fungsi getenv (bukan memanggil os.Getenv langsung) supaya
// mudah diuji tanpa menyentuh environment proses.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Env adalah mode jalan aplikasi.
type Env string

const (
	EnvDevelopment Env = "development"
	EnvTest        Env = "test"
	EnvProduction  Env = "production"
)

const (
	defaultPort      = 3000
	defaultDBPort    = 3306
	defaultDBMaxOpen = 25
	defaultTimezone  = "Asia/Jakarta"
	defaultDBHost    = "127.0.0.1"
	defaultDBUser    = "root"
	defaultDBName    = "cbt_sekolah_go"
	maxTCPPort       = 65535
)

// DB menampung pengaturan koneksi MySQL/MariaDB.
type DB struct {
	Host         string
	Port         int
	Socket       string // jika diisi, dipakai menggantikan Host:Port
	User         string
	Password     string
	Name         string
	MaxOpenConns int
}

// Config adalah seluruh konfigurasi aplikasi.
type Config struct {
	Env                  Env
	Port                 int
	Timezone             *time.Location
	TrustProxy           bool // true bila di belakang reverse proxy/tunnel HTTPS
	CookieInsecure       bool // true bila COOKIE_SECURE=false (mis. HTTP LAN sekolah)
	CookieExplicitSecure bool // true bila COOKIE_SECURE=true eksplisit
	TampilkanNilaiSiswa  bool // D-017: default false
	LogLevel             slog.Level
	DB                   DB
}

// IsProduction memudahkan pengecekan mode produksi (mis. logging).
func (c Config) IsProduction() bool { return c.Env == EnvProduction }

// CookieSecure mengembalikan apakah cookie sesi harus memakai atribut Secure.
// Mengembalikan false bila COOKIE_SECURE=false (agar peramban siswa tidak menolak cookie saat akses HTTP IP LAN sekolah).
// Mengembalikan true bila COOKIE_SECURE=true, TRUST_PROXY=true, atau pada mode production default.
func (c Config) CookieSecure() bool {
	if c.CookieInsecure {
		return false
	}
	return c.CookieExplicitSecure || c.TrustProxy || c.IsProduction()
}

// Load membaca konfigurasi. Semua kesalahan validasi dikumpulkan dan
// dikembalikan sekaligus agar operator tidak perlu memperbaiki satu per satu.
func Load(getenv func(string) string) (Config, error) {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	str := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}
	integer := func(key string, def int) int {
		raw := strings.TrimSpace(getenv(key))
		if raw == "" {
			return def
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			fail("%s harus berupa angka bulat, dapat %q", key, raw)
			return def
		}
		return n
	}
	boolean := func(key string, def bool) bool {
		raw := strings.TrimSpace(getenv(key))
		if raw == "" {
			return def
		}
		b, err := strconv.ParseBool(raw)
		if err != nil {
			fail("%s harus true/false, dapat %q", key, raw)
			return def
		}
		return b
	}

	cfg := Config{
		Env:                 Env(str("APP_ENV", string(EnvDevelopment))),
		Port:                integer("PORT", defaultPort),
		TrustProxy:          boolean("TRUST_PROXY", false),
		TampilkanNilaiSiswa: boolean("TAMPILKAN_NILAI_SISWA", false),
		DB: DB{
			Host:         str("DB_HOST", defaultDBHost),
			Port:         integer("DB_PORT", defaultDBPort),
			Socket:       str("DB_SOCKET", ""),
			User:         str("DB_USER", defaultDBUser),
			Password:     getenv("DB_PASSWORD"), // tidak di-trim: spasi bisa bagian dari password
			Name:         str("DB_NAME", defaultDBName),
			MaxOpenConns: integer("DB_MAX_OPEN_CONNS", defaultDBMaxOpen),
		},
	}

	// Penentuan COOKIE_SECURE:
	// Jika disetel eksplisit, simpan nilai spesifiknya (true -> CookieExplicitSecure, false -> CookieInsecure).
	cookieSecureRaw := strings.TrimSpace(getenv("COOKIE_SECURE"))
	if cookieSecureRaw != "" {
		val := boolean("COOKIE_SECURE", false)
		if val {
			cfg.CookieExplicitSecure = true
		} else {
			cfg.CookieInsecure = true
		}
	}

	switch cfg.Env {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		fail("APP_ENV tidak dikenal: %q (pilihan: development, test, production)", cfg.Env)
	}

	if cfg.Port < 1 || cfg.Port > maxTCPPort {
		fail("PORT di luar rentang 1-%d: %d", maxTCPPort, cfg.Port)
	}
	if cfg.DB.Port < 1 || cfg.DB.Port > maxTCPPort {
		fail("DB_PORT di luar rentang 1-%d: %d", maxTCPPort, cfg.DB.Port)
	}
	if cfg.DB.MaxOpenConns < 1 {
		fail("DB_MAX_OPEN_CONNS minimal 1, dapat %d", cfg.DB.MaxOpenConns)
	}

	tzName := str("APP_TIMEZONE", defaultTimezone)
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		fail("APP_TIMEZONE %q tidak valid: %v", tzName, err)
	}
	cfg.Timezone = loc

	var level slog.Level
	if err := level.UnmarshalText([]byte(str("LOG_LEVEL", "info"))); err != nil {
		fail("LOG_LEVEL tidak valid: %v", err)
	}
	cfg.LogLevel = level

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("konfigurasi tidak valid: %w", errors.Join(errs...))
	}
	return cfg, nil
}
