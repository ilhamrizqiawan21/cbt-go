package httpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cbt-go/internal/config"
	"cbt-go/internal/httpserver"
)

func TestPublicRoutes(t *testing.T) {
	pool := setupTestDB(t)

	cfg := config.Config{Env: config.EnvDevelopment}
	sm := httpserver.NewSessionManager(pool, cfg)

	h := httpserver.New(httpserver.Deps{
		DB:             pool,
		Logger:         nil,
		Config:         cfg,
		SessionManager: sm,
	})

	t.Run("GET / anonim merender landing page dengan tautan login", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want %d", rec.Code, http.StatusOK)
		}
		body := rec.Body.String()
		for _, link := range []string{`href="/login"`, `href="/login-guru"`, `href="/login-admin"`} {
			if !strings.Contains(body, link) {
				t.Errorf("landing page harus memuat tautan %s", link)
			}
		}
	})

	t.Run("GET / dengan session admin diarahkan ke /admin/dashboard", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx, err := sm.Load(req.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		sm.Put(ctx, "adminId", 1)
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("status = %d; want 302 Found", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/admin/dashboard" {
			t.Errorf("Location = %q; want '/admin/dashboard'", loc)
		}
	})

	t.Run("GET / dengan session guru diarahkan ke /guru/dashboard", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx, err := sm.Load(req.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		sm.Put(ctx, "guruId", 1)
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("status = %d; want 302 Found", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/guru/dashboard" {
			t.Errorf("Location = %q; want '/guru/dashboard'", loc)
		}
	})

	t.Run("GET / dengan session siswa & ujian diarahkan ke /ujian", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx, err := sm.Load(req.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		sm.Put(ctx, "siswaId", 1)
		sm.Put(ctx, "ujianId", 1)
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("status = %d; want 302 Found", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/ujian" {
			t.Errorf("Location = %q; want '/ujian'", loc)
		}
	})

	t.Run("GET /login merender halaman login siswa", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/login", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want %d", rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), `action="/login-siswa"`) {
			t.Errorf("halaman login siswa harus memuat form /login-siswa")
		}
	})

	t.Run("GET /login-admin merender halaman login admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/login-admin", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want %d", rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), `action="/login-admin"`) {
			t.Errorf("halaman login admin harus memuat form /login-admin")
		}
	})

	t.Run("GET /login-guru merender halaman login guru", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/login-guru", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want %d", rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), `action="/login-guru"`) {
			t.Errorf("halaman login guru harus memuat form /login-guru")
		}
	})

	t.Run("GET /api/daftar-ujian mengembalikan array JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/daftar-ujian", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want %d", rec.Code, http.StatusOK)
		}
		var items []any
		if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
			t.Fatalf("respons bukan JSON array yang valid: %v", err)
		}
	})

	t.Run("GET font statis terlayani dengan mime font/woff2", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/fonts/amiri-v30-arabic-regular.woff2", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d; want %d", rec.Code, http.StatusOK)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "font/woff2") {
			t.Errorf("Content-Type = %q; want 'font/woff2'", ct)
		}
		if cc := rec.Header().Get("Cache-Control"); cc == "" {
			t.Errorf("Cache-Control harus di-set untuk font")
		}
	})

	t.Run("GET path tidak dikenal merender 404 dengan template error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/halaman-yang-pasti-tidak-ada-404", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d; want 404 Not Found", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "404") {
			t.Errorf("halaman error harus memuat kode status 404")
		}
	})
}
