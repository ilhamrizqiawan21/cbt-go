package config_test

import (
	"testing"

	"cbt-go/internal/config"
	"cbt-go/internal/httpserver"
)

// TestProductionEnvConfig memverifikasi perilaku konfigurasi aplikasi pada mode production
// sebagai padanan tests/production_env.test.js.
func TestProductionEnvConfig(t *testing.T) {
	t.Run("Mode Production Mengaktifkan IsProduction dan Cookie Secure", func(t *testing.T) {
		cfg, err := config.Load(func(k string) string {
			switch k {
			case "APP_ENV":
				return "production"
			case "PORT":
				return "3000"
			case "APP_TIMEZONE":
				return "Asia/Jakarta"
			default:
				return ""
			}
		})
		if err != nil {
			t.Fatalf("Load config production gagal: %v", err)
		}

		if !cfg.IsProduction() {
			t.Errorf("IsProduction harus bernilai true pada mode production")
		}

		// Verifikasi pembentukan session manager dengan cookie Secure aktif
		sm := httpserver.NewSessionManager(nil, cfg)
		if !sm.Cookie.Secure {
			t.Errorf("Cookie session harus memiliki Secure=true pada mode production")
		}
		if !sm.Cookie.HttpOnly {
			t.Errorf("Cookie session harus memiliki HttpOnly=true")
		}
	})

	t.Run("TrustProxy Mengaktifkan Cookie Secure Kendati Non-Production", func(t *testing.T) {
		cfg, err := config.Load(func(k string) string {
			switch k {
			case "APP_ENV":
				return "development"
			case "TRUST_PROXY":
				return "true"
			default:
				return ""
			}
		})
		if err != nil {
			t.Fatalf("Load config gagal: %v", err)
		}

		if cfg.IsProduction() {
			t.Errorf("IsProduction harus false pada mode development")
		}

		sm := httpserver.NewSessionManager(nil, cfg)
		if !sm.Cookie.Secure {
			t.Errorf("Cookie session harus memiliki Secure=true bila TRUST_PROXY=true")
		}
	})

	t.Run("Mode Development Default Menggunakan Secure=false", func(t *testing.T) {
		cfg, err := config.Load(func(string) string { return "" })
		if err != nil {
			t.Fatalf("Load default config gagal: %v", err)
		}

		sm := httpserver.NewSessionManager(nil, cfg)
		if sm.Cookie.Secure {
			t.Errorf("Cookie session harus memiliki Secure=false pada development tanpa proxy")
		}
	})
}

