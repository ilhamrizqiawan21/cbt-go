package httpserver_test

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/config"
	"cbt-go/internal/httpserver"
	"cbt-go/internal/ratelimit"
)

type auditMockRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r *auditMockRows) Columns() []string { return r.columns }
func (r *auditMockRows) Close() error      { return nil }
func (r *auditMockRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	for i, v := range r.rows[r.idx] {
		dest[i] = v
	}
	r.idx++
	return nil
}

type auditMockDriver struct{}

func (d *auditMockDriver) Open(name string) (driver.Conn, error) {
	return &auditMockConn{}, nil
}

type auditMockConn struct{}

func (c *auditMockConn) Prepare(query string) (driver.Stmt, error) {
	return &auditMockStmt{conn: c, query: query}, nil
}
func (c *auditMockConn) Close() error              { return nil }
func (c *auditMockConn) Begin() (driver.Tx, error) { return &auditMockTx{}, nil }

type auditMockTx struct{}

func (tx *auditMockTx) Commit() error   { return nil }
func (tx *auditMockTx) Rollback() error { return nil }

type auditMockStmt struct {
	conn  *auditMockConn
	query string
}

func (s *auditMockStmt) Close() error  { return nil }
func (s *auditMockStmt) NumInput() int { return -1 }
func (s *auditMockStmt) Exec(args []driver.Value) (driver.Result, error) {
	return auditMockResult{}, nil
}
func (s *auditMockStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.QueryContext(context.Background(), s.query, named)
}

type auditMockResult struct{}

func (r auditMockResult) LastInsertId() (int64, error) { return 1, nil }
func (r auditMockResult) RowsAffected() (int64, error) { return 1, nil }

func (c *auditMockConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return auditMockResult{}, nil
}

func (c *auditMockConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	// 0. Query validasi sesi di RequireSiswaAPI (SELECT device_token, status FROM sesi_ujian ...)
	if strings.Contains(query, "device_token, status") {
		return &auditMockRows{
			columns: []string{"device_token", "status"},
			rows: [][]driver.Value{
				{"token-device-valid", "sedang_ujian"},
			},
		}, nil
	}

	// 1. Cek waktu ujian (SELECT (tanggal_mulai <= NOW() ...))
	if strings.Contains(query, "tanggal_mulai <= NOW()") {
		return &auditMockRows{
			columns: []string{"valid"},
			rows: [][]driver.Value{
				{true},
			},
		}, nil
	}

	// 2. Query konfigurasi acak ujian
	if strings.Contains(query, "acak_soal") {
		return &auditMockRows{
			columns: []string{"acak_soal", "acak_pilihan"},
			rows: [][]driver.Value{
				{1, 1},
			},
		}, nil
	}

	// 3. Query seed sesi
	if strings.Contains(query, "SELECT seed") {
		return &auditMockRows{
			columns: []string{"seed"},
			rows: [][]driver.Value{
				{12345},
			},
		}, nil
	}

	// 4. Query butir soal (tanpa jawaban_benar!)
	if strings.Contains(query, "FROM soal") {
		return &auditMockRows{
			columns: []string{"id", "tipe_soal", "teks_soal", "gambar", "poin", "pilihan_a", "pilihan_b", "pilihan_c", "pilihan_d", "opsi_tambahan"},
			rows: [][]driver.Value{
				{
					int64(101), "pg", "Ibu kota Indonesia?", nil, int64(2),
					"Jakarta", "Bandung", "Surabaya", "Medan", nil,
				},
				{
					int64(102), "essay", "Jelaskan proses fotosintesis!", nil, int64(4),
					nil, nil, nil, nil, nil,
				},
			},
		}, nil
	}

	// 4. Query jawaban_siswa
	if strings.Contains(query, "FROM jawaban_siswa") {
		return &auditMockRows{
			columns: []string{"soal_id", "jawaban_dipilih"},
			rows:    [][]driver.Value{},
		}, nil
	}

	return &auditMockRows{columns: []string{"id"}, rows: [][]driver.Value{}}, nil
}

// TestAuditKeamanan_KunciJawabanTidakPernahBocor memverifikasi bahwa kunci jawaban
// atau kata kunci jawaban_benar / is_benar tidak pernah dikirim ke respon klien siswa.
func TestAuditKeamanan_KunciJawabanTidakPernahBocor(t *testing.T) {
	driverName := namaDriverMock("mock_audit")
	sql.Register(driverName, &auditMockDriver{})
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open mock db: %v", err)
	}
	defer db.Close()

	cfg := config.Config{Env: config.EnvTest, Timezone: time.UTC}
	sm := scs.New()
	deps := httpserver.Deps{
		DB:             db,
		Config:         cfg,
		SessionManager: sm,
	}
	h := httpserver.New(deps)

	// Buat cookie sesi siswa yang terotentikasi
	reqInit := httptest.NewRequest("GET", "/", nil)
	ctx, err := sm.Load(reqInit.Context(), "")
	if err != nil {
		t.Fatalf("session load: %v", err)
	}
	sm.Put(ctx, "peran", "siswa")
	sm.Put(ctx, "siswaId", 101)
	sm.Put(ctx, "ujianId", 10)
	sm.Put(ctx, "deviceToken", "token-device-valid")
	token, _, _ := sm.Commit(ctx)

	cookie := &http.Cookie{Name: sm.Cookie.Name, Value: token}

	// Panggil GET /api/soal/10
	req := httptest.NewRequest("GET", "/api/soal/10", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/soal/10 status = %d, ingin 200. Body: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if strings.Contains(body, "jawaban_benar") {
		t.Errorf("KEBOCORAN: respon soal memuat kata 'jawaban_benar': %s", body)
	}
	if strings.Contains(body, "is_benar") {
		t.Errorf("KEBOCORAN: respon soal memuat kata 'is_benar': %s", body)
	}
	if strings.Contains(body, "\"kunci\"") {
		t.Errorf("KEBOCORAN: respon soal memuat kata 'kunci': %s", body)
	}
}

// TestAuditKeamanan_OtorisasiRoleEndpoint memverifikasi proteksi akses berdasarkan peran.
func TestAuditKeamanan_OtorisasiRoleEndpoint(t *testing.T) {
	driverName := namaDriverMock("mock_audit_auth")
	sql.Register(driverName, &auditMockDriver{})
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open mock db: %v", err)
	}
	defer db.Close()

	cfg := config.Config{Env: config.EnvTest, Timezone: time.UTC}
	sm := scs.New()
	deps := httpserver.Deps{
		DB:             db,
		Config:         cfg,
		SessionManager: sm,
	}
	h := httpserver.New(deps)

	// Skenario 1: Tamu (Unauthenticated) tidak boleh mengakses halaman/API terproteksi
	t.Run("Tamu Ditolak dari Endpoint Terproteksi", func(t *testing.T) {
		ruteTamu := []struct {
			method   string
			path     string
			wantCode int
		}{
			{"GET", "/ujian", http.StatusFound},             // redirect ke /login
			{"GET", "/api/sesi", http.StatusUnauthorized},   // 401
			{"GET", "/api/soal/1", http.StatusUnauthorized}, // 401
			{"GET", "/admin/dashboard", http.StatusFound},   // redirect /login-admin
			{"GET", "/guru/dashboard", http.StatusFound},    // redirect /login-guru
		}

		for _, r := range ruteTamu {
			req := httptest.NewRequest(r.method, r.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != r.wantCode {
				t.Errorf("%s %s code = %d, ingin %d", r.method, r.path, rec.Code, r.wantCode)
			}
		}
	})

	// Skenario 2: Siswa mencoba mengakses endpoint Admin & Guru
	t.Run("Siswa Dilarang Mengakses Admin dan Guru", func(t *testing.T) {
		reqInit := httptest.NewRequest("GET", "/", nil)
		ctx, err := sm.Load(reqInit.Context(), "")
		if err != nil {
			t.Fatalf("session load: %v", err)
		}
		sm.Put(ctx, "peran", "siswa")
		sm.Put(ctx, "siswaId", 1001)
		sm.Put(ctx, "ujianId", 10)
		sm.Put(ctx, "deviceToken", "token-device")
		token, _, _ := sm.Commit(ctx)
		cookie := &http.Cookie{Name: sm.Cookie.Name, Value: token}

		ruteAdminGuru := []struct {
			method   string
			path     string
			wantCode int
		}{
			{"GET", "/admin/dashboard", http.StatusFound}, // redirect login-admin
			{"GET", "/admin/guru", http.StatusFound},
			{"GET", "/admin/siswa", http.StatusFound},
			{"GET", "/guru/dashboard", http.StatusFound}, // redirect login-guru
			{"GET", "/guru/kelola-soal", http.StatusFound},
			{"GET", "/guru/api/soal/1", http.StatusForbidden}, // 403 API
		}

		for _, r := range ruteAdminGuru {
			req := httptest.NewRequest(r.method, r.path, nil)
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != r.wantCode {
				t.Errorf("Siswa akses %s %s code = %d, ingin %d", r.method, r.path, rec.Code, r.wantCode)
			}
		}
	})

	// Skenario 3: Guru mencoba mengakses endpoint Admin
	t.Run("Guru Dilarang Mengakses Rute Admin", func(t *testing.T) {
		reqInit := httptest.NewRequest("GET", "/", nil)
		ctx, err := sm.Load(reqInit.Context(), "")
		if err != nil {
			t.Fatalf("session load: %v", err)
		}
		sm.Put(ctx, "peran", "guru")
		sm.Put(ctx, "guruId", 5)
		token, _, _ := sm.Commit(ctx)
		cookie := &http.Cookie{Name: sm.Cookie.Name, Value: token}

		ruteAdmin := []struct {
			method   string
			path     string
			wantCode int
		}{
			{"GET", "/admin/dashboard", http.StatusFound}, // redirect login-admin
			{"GET", "/admin/guru", http.StatusFound},
			{"GET", "/admin/siswa", http.StatusFound},
			{"GET", "/admin/api/ujian-by-mapel/1", http.StatusForbidden}, // 403 API
		}

		for _, r := range ruteAdmin {
			req := httptest.NewRequest(r.method, r.path, nil)
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != r.wantCode {
				t.Errorf("Guru akses %s %s code = %d, ingin %d", r.method, r.path, rec.Code, r.wantCode)
			}
		}
	})
}

// TestAuditKeamanan_RateLimiting memverifikasi pembatasan frekuensi request login.
func TestAuditKeamanan_RateLimiting(t *testing.T) {
	ctx := context.Background()
	limiterNIS := ratelimit.New(ctx, 10, 15*time.Minute, 30*time.Minute)
	limiterIP := ratelimit.New(ctx, 300, 15*time.Minute, 30*time.Minute)
	limiterAdmin := ratelimit.New(ctx, 10, 15*time.Minute, 30*time.Minute)

	nisKey := "rate_nis:T9999"
	for i := 0; i < 10; i++ {
		if !limiterNIS.Allow(nisKey) {
			t.Fatalf("percobaan ke-%d harus diizinkan", i+1)
		}
	}
	if limiterNIS.Allow(nisKey) {
		t.Errorf("percobaan ke-11 untuk NIS yang sama harus ditolak oleh rate limiter")
	}

	adminKey := "rate_admin:127.0.0.1:admin"
	for i := 0; i < 10; i++ {
		if !limiterAdmin.Allow(adminKey) {
			t.Fatalf("admin percobaan ke-%d harus diizinkan", i+1)
		}
	}
	if limiterAdmin.Allow(adminKey) {
		t.Errorf("percobaan ke-11 login admin harus ditolak (429)")
	}

	ipKey := "rate_ip:192.168.1.50"
	for i := 0; i < 300; i++ {
		if !limiterIP.Allow(ipKey) {
			t.Fatalf("ip percobaan ke-%d harus diizinkan", i+1)
		}
	}
	if limiterIP.Allow(ipKey) {
		t.Errorf("percobaan ke-301 dari IP yang sama harus ditolak oleh IP rate limiter")
	}
}

// TestAuditKeamanan_SecurityHeaders memverifikasi disematkannya HTTP security headers.
func TestAuditKeamanan_SecurityHeaders(t *testing.T) {
	cfg := config.Config{Env: config.EnvTest, Timezone: time.UTC}
	deps := httpserver.Deps{
		Config: cfg,
	}
	h := httpserver.New(deps)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	headers := rec.Header()

	if got := headers.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, ingin 'nosniff'", got)
	}

	if got := headers.Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Errorf("Referrer-Policy = %q, ingin 'strict-origin-when-cross-origin'", got)
	}

	csp := headers.Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("Content-Security-Policy = %q tidak memuat 'default-src \\'self\\''", csp)
	}

	// G-019: seluruh aset disajikan lokal agar server tetap jalan tanpa internet.
	// CSP tidak boleh menyisakan izin ke origin luar (mis. Google Fonts).
	for _, origin := range []string{"https://", "http://", "//fonts."} {
		if strings.Contains(csp, origin) {
			t.Errorf("Content-Security-Policy = %q masih mengizinkan origin luar %q; aset wajib lokal (G-019)", csp, origin)
		}
	}
}

// TestAuditKeamanan_CSRF memverifikasi proteksi Cross-Origin Request pada method POST berbasis session.
func TestAuditKeamanan_CSRF(t *testing.T) {
	cfg := config.Config{Env: config.EnvTest, Timezone: time.UTC}
	sm := scs.New()
	deps := httpserver.Deps{
		Config:         cfg,
		SessionManager: sm,
	}
	h := httpserver.New(deps)

	// Buat session cookie
	reqInit := httptest.NewRequest("GET", "/", nil)
	ctx, err := sm.Load(reqInit.Context(), "")
	if err != nil {
		t.Fatalf("session load: %v", err)
	}
	sm.Put(ctx, "peran", "admin")
	sm.Put(ctx, "adminId", 1)
	token, _, _ := sm.Commit(ctx)
	cookie := &http.Cookie{Name: sm.Cookie.Name, Value: token}

	// Request POST dengan Cross-Site Origin jahat
	req := httptest.NewRequest("POST", "/admin/guru/tambah", strings.NewReader("nama=Hacker"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://evil-attacker.com")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Host = "cbt.sekolah.local"
	req.AddCookie(cookie)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// CrossOriginProtection menolak request cross-site dengan HTTP 403 Forbidden
	if rec.Code != http.StatusForbidden {
		t.Errorf("Request cross-site POST harus ditolak 403 Forbidden oleh CrossOriginProtection, dapat: %d", rec.Code)
	}
}

// TestAuditKeamanan_UploadSizeLimit memverifikasi pembatasan ukuran payload (1MB / 2MB)
// menggunakan http.MaxBytesReader sehingga mencegah serangan DoS memori.
func TestAuditKeamanan_UploadSizeLimit(t *testing.T) {
	cfg := config.Config{Env: config.EnvTest, Timezone: time.UTC}
	sm := scs.New()
	deps := httpserver.Deps{
		Config:         cfg,
		SessionManager: sm,
	}
	h := httpserver.New(deps)

	// Request sinkron jawaban dengan payload raksasa melebihi 1 MB
	hugePayload := make([]byte, 2*1024*1024) // 2 MB
	copy(hugePayload, []byte("[{\"soal_id\": 1, \"jawaban\": \""))
	for i := 25; i < len(hugePayload)-5; i++ {
		hugePayload[i] = 'A'
	}
	copy(hugePayload[len(hugePayload)-5:], []byte("\"}]"))

	req := httptest.NewRequest("POST", "/api/sinkron-jawaban", bytes.NewReader(hugePayload))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "cbt.sekolah.local"

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Harus ditolak dengan status error (401 bila belum login atau 413 jika payload diperiksa)
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Payload upload raksasa harus ditolak (401 atau 413), dapat: %d", rec.Code)
	}
}

// mockDriverSeq menomori nama driver mock agar selalu unik dalam satu proses uji.
// Sebelumnya nama dibentuk dari time.Now().UnixNano(); itu tidak aman karena di
// Windows dua panggilan berurutan dapat mengembalikan nilai yang sama, sehingga
// sql.Register panik dengan "Register called twice for driver ...".
var mockDriverSeq atomic.Uint64

// namaDriverMock mengembalikan nama driver unik dengan awalan yang diberikan.
func namaDriverMock(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, mockDriverSeq.Add(1))
}
