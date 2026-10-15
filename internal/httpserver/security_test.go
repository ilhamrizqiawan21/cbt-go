package httpserver_test

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexedwards/scs/mysqlstore"
	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/config"
	"cbt-go/internal/httpserver"
	"cbt-go/internal/testdb"
	"cbt-go/migrations"

	"github.com/pressly/goose/v3"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	pool := testdb.Open(t)
	if pool == nil {
		t.Skip("database uji tidak dapat dihubungi")
	}

	goose.SetBaseFS(migrations.FS)
	_ = goose.SetDialect("mysql")
	if err := goose.Up(pool, "."); err != nil {
		t.Fatalf("gagal migrasi DB test: %v", err)
	}
	testdb.Reset(t, pool)
	return pool
}

func TestSecurityHeaders(t *testing.T) {
	pool := setupTestDB(t)

	cfg := config.Config{
		Env: config.EnvDevelopment,
	}
	sm := scs.New()
	sm.Store = mysqlstore.New(pool)

	h := httpserver.New(httpserver.Deps{
		DB:             pool,
		Logger:         nil,
		Config:         cfg,
		SessionManager: sm,
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	res := rec.Result()
	if got := res.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q; want 'nosniff'", got)
	}
	if got := res.Header.Get("Referrer-Policy"); got == "" {
		t.Errorf("Referrer-Policy header tidak ada")
	}
	if got := res.Header.Get("Content-Security-Policy"); got == "" {
		t.Errorf("Content-Security-Policy header tidak ada")
	}
}

func TestCrossOriginProtection(t *testing.T) {
	pool := setupTestDB(t)

	cfg := config.Config{
		Env: config.EnvDevelopment,
	}
	sm := scs.New()
	sm.Store = mysqlstore.New(pool)

	h := httpserver.New(httpserver.Deps{
		DB:             pool,
		Logger:         nil,
		Config:         cfg,
		SessionManager: sm,
	})

	// 1. POST dengan Origin asing harus ditolak 403 Forbidden
	req := httptest.NewRequest(http.MethodPost, "/healthz", strings.NewReader(`{}`))
	req.Host = "localhost:3000"
	req.Header.Set("Origin", "https://attacker-site.com")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("POST lintas origin code = %d; want %d", rec.Code, http.StatusForbidden)
	}

	// 2. GET (metode safe) dengan Origin asing tetap diperbolehkan (tidak diblokir CSRF)
	reqGet := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	reqGet.Host = "localhost:3000"
	reqGet.Header.Set("Origin", "https://attacker-site.com")
	recGet := httptest.NewRecorder()

	h.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Errorf("GET lintas origin code = %d; want %d", recGet.Code, http.StatusOK)
	}
}

func TestSessionPersistenceAndCookies(t *testing.T) {
	pool := setupTestDB(t)

	cfgDev := config.Config{Env: config.EnvDevelopment}
	smDev := httpserver.NewSessionManager(pool, cfgDev)

	// Route uji untuk menulis dan membaca session
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login-dummy", func(w http.ResponseWriter, r *http.Request) {
		smDev.Put(r.Context(), "user_id", "siswa-123")
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /check-session", func(w http.ResponseWriter, r *http.Request) {
		userID := smDev.GetString(r.Context(), "user_id")
		w.Write([]byte(userID))
	})

	h1 := httpserver.New(httpserver.Deps{
		DB:             pool,
		Config:         cfgDev,
		SessionManager: smDev,
		CustomHandler:  mux,
	})

	// 1. Login dummy
	req1 := httptest.NewRequest(http.MethodPost, "/login-dummy", nil)
	req1.Host = "localhost:3000"
	rec1 := httptest.NewRecorder()
	h1.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("login-dummy gagal, status: %d", rec1.Code)
	}

	cookie := rec1.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookie {
		if c.Name == smDev.Cookie.Name {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("cookie session tidak ditemukan di response header: %v", cookie)
	}

	if !sessionCookie.HttpOnly {
		t.Errorf("session cookie harus HttpOnly")
	}
	if sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("session cookie SameSite = %v; want Lax (%v)", sessionCookie.SameSite, http.SameSiteLaxMode)
	}
	if sessionCookie.Secure {
		t.Errorf("session cookie di development mode tidak boleh Secure")
	}

	// 2. Akses dari instance server 2 yang berbeda tetapi berbagi DB pool (simulasi session di MySQL)
	smDev2 := httpserver.NewSessionManager(pool, cfgDev)
	mux2 := http.NewServeMux()
	mux2.HandleFunc("GET /check-session", func(w http.ResponseWriter, r *http.Request) {
		userID := smDev2.GetString(r.Context(), "user_id")
		w.Write([]byte(userID))
	})

	h2 := httpserver.New(httpserver.Deps{
		DB:             pool,
		Config:         cfgDev,
		SessionManager: smDev2,
		CustomHandler:  mux2,
	})

	req2 := httptest.NewRequest(http.MethodGet, "/check-session", nil)
	req2.AddCookie(sessionCookie)
	rec2 := httptest.NewRecorder()
	h2.ServeHTTP(rec2, req2)

	body, _ := io.ReadAll(rec2.Body)
	if string(body) != "siswa-123" {
		t.Fatalf("session tidak bertahan antar instance! got %q, want 'siswa-123'", string(body))
	}

	// 3. Mode Produksi harus memuat atribut Secure pada cookie
	cfgProd := config.Config{Env: config.EnvProduction}
	smProd := httpserver.NewSessionManager(pool, cfgProd)

	muxProd := http.NewServeMux()
	muxProd.HandleFunc("POST /login-dummy", func(w http.ResponseWriter, r *http.Request) {
		smProd.Put(r.Context(), "user_id", "siswa-prod")
		w.WriteHeader(http.StatusOK)
	})

	hProd := httpserver.New(httpserver.Deps{
		DB:             pool,
		Config:         cfgProd,
		SessionManager: smProd,
		CustomHandler:  muxProd,
	})

	reqProd := httptest.NewRequest(http.MethodPost, "/login-dummy", nil)
	reqProd.Host = "localhost:3000"
	recProd := httptest.NewRecorder()
	hProd.ServeHTTP(recProd, reqProd)

	var prodCookie *http.Cookie
	for _, c := range recProd.Result().Cookies() {
		if c.Name == smProd.Cookie.Name {
			prodCookie = c
			break
		}
	}
	if prodCookie == nil {
		t.Fatalf("cookie produksi tidak ditemukan")
	}
	if !prodCookie.Secure {
		t.Errorf("session cookie di mode production WAJIB Secure")
	}
}
