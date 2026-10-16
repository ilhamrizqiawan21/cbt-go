package auth_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"cbt-go/internal/auth"
	"cbt-go/internal/ratelimit"
)

// mockAuthDriver menyediakan kueri tabel admin dan guru untuk pengujian unit deterministik.
type mockAuthDriver struct {
	adminPasswordHash string
	guruPasswordHash  string
}

func (d *mockAuthDriver) Open(name string) (driver.Conn, error) {
	return &mockAuthConn{d: d}, nil
}

type mockAuthConn struct {
	d *mockAuthDriver
}

type mockStmt struct {
	conn  *mockAuthConn
	query string
}

func (s *mockStmt) Close() error                                    { return nil }
func (s *mockStmt) NumInput() int                                   { return -1 }
func (s *mockStmt) Exec(args []driver.Value) (driver.Result, error) { return nil, nil }
func (s *mockStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.QueryContext(context.Background(), s.query, named)
}

func (c *mockAuthConn) Prepare(query string) (driver.Stmt, error) {
	return &mockStmt{conn: c, query: query}, nil
}
func (c *mockAuthConn) Close() error { return nil }
func (c *mockAuthConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

func (c *mockAuthConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	return c.QueryContext(context.Background(), query, nil)
}

func (c *mockAuthConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	var username string
	for _, a := range args {
		if s, ok := a.Value.(string); ok {
			username = s
			break
		}
	}

	if strings.Contains(query, "FROM admin") {
		if username == "admin" {
			return &sliceRows{
				columns: []string{"id", "password"},
				rows: [][]driver.Value{
					{int64(1), c.d.adminPasswordHash},
				},
			}, nil
		}
		return &sliceRows{columns: []string{"id", "password"}}, nil
	}

	if strings.Contains(query, "FROM guru") {
		if username == "guru" {
			return &sliceRows{
				columns: []string{"id", "nip", "nama", "password"},
				rows: [][]driver.Value{
					{int64(2), "19850101", "Budi Santoso, S.Pd.", c.d.guruPasswordHash},
				},
			}, nil
		}
		return &sliceRows{columns: []string{"id", "nip", "nama", "password"}}, nil
	}

	return nil, fmt.Errorf("kueri tidak dikenali: %s", query)
}

type sliceRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r *sliceRows) Columns() []string { return r.columns }
func (r *sliceRows) Close() error      { return nil }
func (r *sliceRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	for i, v := range r.rows[r.idx] {
		dest[i] = v
	}
	r.idx++
	return nil
}

func setupMockAuthDB(t *testing.T) (*sql.DB, *mockAuthDriver) {
	t.Helper()
	adminHash, err := bcrypt.GenerateFromPassword([]byte("admin123"), 10)
	if err != nil {
		t.Fatalf("hash admin: %v", err)
	}
	guruHash, err := bcrypt.GenerateFromPassword([]byte("guru123"), 10)
	if err != nil {
		t.Fatalf("hash guru: %v", err)
	}

	drv := &mockAuthDriver{
		adminPasswordHash: string(adminHash),
		guruPasswordHash:  string(guruHash),
	}
	driverName := fmt.Sprintf("mock_auth_%d", time.Now().UnixNano())
	sql.Register(driverName, drv)

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open mock db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, drv
}

func TestGuardPeran(t *testing.T) {
	sm := scs.New()

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// 1. RequireAdmin (HTML): redirect /login-admin bila bukan admin
	adminH := sm.LoadAndSave(auth.RequireAdmin(sm)(okHandler))

	// a. Tanpa session -> redirect 302 ke /login-admin
	req := httptest.NewRequest("GET", "/admin/dashboard", nil)
	w := httptest.NewRecorder()
	adminH.ServeHTTP(w, req)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/login-admin" {
		t.Errorf("RequireAdmin tanpa session: status = %d, loc = %q, want 302 -> /login-admin", w.Code, w.Header().Get("Location"))
	}

	// b. Session guru -> redirect 302 ke /login-admin
	req = httptest.NewRequest("GET", "/admin/dashboard", nil)
	ctxGuru, _ := sm.Load(req.Context(), "")
	sm.Put(ctxGuru, "peran", "guru")
	sm.Put(ctxGuru, "guruId", 2)
	req = req.WithContext(ctxGuru)
	w = httptest.NewRecorder()
	adminH.ServeHTTP(w, req)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/login-admin" {
		t.Errorf("RequireAdmin session guru: status = %d, want redirect ke /login-admin", w.Code)
	}

	// c. Session admin valid -> 200 OK
	req = httptest.NewRequest("GET", "/admin/dashboard", nil)
	ctxAdmin, _ := sm.Load(req.Context(), "")
	sm.Put(ctxAdmin, "peran", "admin")
	sm.Put(ctxAdmin, "adminId", 1)
	req = req.WithContext(ctxAdmin)
	w = httptest.NewRecorder()
	adminH.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("RequireAdmin session admin: status = %d, want 200", w.Code)
	}

	// 2. RequireAdminAPI: 401 jika belum login, 403 jika bukan admin
	adminAPIH := sm.LoadAndSave(auth.RequireAdminAPI(sm)(okHandler))

	req = httptest.NewRequest("GET", "/admin/api/test", nil)
	w = httptest.NewRecorder()
	adminAPIH.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("RequireAdminAPI tanpa login: status = %d, want 401", w.Code)
	}

	req = httptest.NewRequest("GET", "/admin/api/test", nil)
	req = req.WithContext(ctxGuru)
	w = httptest.NewRecorder()
	adminAPIH.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("RequireAdminAPI session guru: status = %d, want 403", w.Code)
	}

	req = httptest.NewRequest("GET", "/admin/api/test", nil)
	req = req.WithContext(ctxAdmin)
	w = httptest.NewRecorder()
	adminAPIH.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("RequireAdminAPI session admin: status = %d, want 200", w.Code)
	}

	// 3. RequireGuru (HTML): redirect /login-guru bila bukan guru
	guruH := sm.LoadAndSave(auth.RequireGuru(sm)(okHandler))

	req = httptest.NewRequest("GET", "/guru/dashboard", nil)
	w = httptest.NewRecorder()
	guruH.ServeHTTP(w, req)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/login-guru" {
		t.Errorf("RequireGuru tanpa session: status = %d, loc = %q, want 302 -> /login-guru", w.Code, w.Header().Get("Location"))
	}

	req = httptest.NewRequest("GET", "/guru/dashboard", nil)
	req = req.WithContext(ctxAdmin)
	w = httptest.NewRecorder()
	guruH.ServeHTTP(w, req)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/login-guru" {
		t.Errorf("RequireGuru session admin: status = %d, want redirect ke /login-guru", w.Code)
	}

	req = httptest.NewRequest("GET", "/guru/dashboard", nil)
	req = req.WithContext(ctxGuru)
	w = httptest.NewRecorder()
	guruH.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("RequireGuru session guru: status = %d, want 200", w.Code)
	}

	// 4. RequireGuruAPI: 401 jika belum login, 403 jika bukan guru
	guruAPIH := sm.LoadAndSave(auth.RequireGuruAPI(sm)(okHandler))

	req = httptest.NewRequest("GET", "/guru/api/test", nil)
	w = httptest.NewRecorder()
	guruAPIH.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("RequireGuruAPI tanpa login: status = %d, want 401", w.Code)
	}

	req = httptest.NewRequest("GET", "/guru/api/test", nil)
	req = req.WithContext(ctxAdmin)
	w = httptest.NewRecorder()
	guruAPIH.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("RequireGuruAPI session admin: status = %d, want 403", w.Code)
	}

	req = httptest.NewRequest("GET", "/guru/api/test", nil)
	req = req.WithContext(ctxGuru)
	w = httptest.NewRecorder()
	guruAPIH.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("RequireGuruAPI session guru: status = %d, want 200", w.Code)
	}
}

func TestLoginAdmin(t *testing.T) {
	db, _ := setupMockAuthDB(t)
	sm := scs.New()

	handler := sm.LoadAndSave(auth.LoginAdminHandler(db, sm, nil))

	// 1. Password salah -> render ulang / redirect dengan error
	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "salah123")
	req := httptest.NewRequest("POST", "/login-admin", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusUnauthorized {
		t.Errorf("login admin password salah: status = %d, want 200/401", w.Code)
	}

	// 2. Username tidak terdaftar -> ditolak
	form = url.Values{}
	form.Set("username", "admin_hacker")
	form.Set("password", "admin123")
	req = httptest.NewRequest("POST", "/login-admin", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusUnauthorized {
		t.Errorf("login admin username tidak ada: status = %d, want 200/401", w.Code)
	}

	// 3. Kredensial benar -> redirect ke /admin/dashboard, session tersimpan
	form = url.Values{}
	form.Set("username", "admin")
	form.Set("password", "admin123")
	req = httptest.NewRequest("POST", "/login-admin", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("login admin sukses: status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/admin/dashboard" && loc != "/admin/monitor" {
		t.Errorf("redirect location = %q, want /admin/dashboard atau /admin/monitor", loc)
	}

	// Cek session
	cookie := w.Header().Get("Set-Cookie")
	if cookie == "" {
		t.Fatalf("Set-Cookie tidak ada")
	}

	reqCheck := httptest.NewRequest("GET", "/admin/dashboard", nil)
	reqCheck.Header.Set("Cookie", cookie)
	ctx, err := sm.Load(reqCheck.Context(), extractCookieToken(cookie, sm.Cookie.Name))
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if sm.GetInt(ctx, "adminId") != 1 {
		t.Errorf("session adminId = %d, want 1", sm.GetInt(ctx, "adminId"))
	}
	if sm.GetString(ctx, "peran") != "admin" {
		t.Errorf("session peran = %q, want admin", sm.GetString(ctx, "peran"))
	}
}

func TestLoginGuru(t *testing.T) {
	db, _ := setupMockAuthDB(t)
	sm := scs.New()

	handler := sm.LoadAndSave(auth.LoginGuruHandler(db, sm, nil))

	// 1. Password salah
	form := url.Values{}
	form.Set("username", "guru")
	form.Set("password", "salah")
	req := httptest.NewRequest("POST", "/login-guru", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusUnauthorized {
		t.Errorf("login guru password salah: status = %d, want 200/401", w.Code)
	}

	// 2. Kredensial benar -> redirect ke /guru/dashboard, session tersimpan
	form = url.Values{}
	form.Set("username", "guru")
	form.Set("password", "guru123")
	req = httptest.NewRequest("POST", "/login-guru", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("login guru sukses: status = %d, want 302", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/guru/dashboard" {
		t.Errorf("redirect location = %q, want /guru/dashboard", loc)
	}

	cookie := w.Header().Get("Set-Cookie")
	if cookie == "" {
		t.Fatalf("Set-Cookie tidak ada")
	}

	reqCheck := httptest.NewRequest("GET", "/guru/dashboard", nil)
	reqCheck.Header.Set("Cookie", cookie)
	ctx, err := sm.Load(reqCheck.Context(), extractCookieToken(cookie, sm.Cookie.Name))
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if sm.GetInt(ctx, "guruId") != 2 {
		t.Errorf("session guruId = %d, want 2", sm.GetInt(ctx, "guruId"))
	}
	if sm.GetString(ctx, "peran") != "guru" {
		t.Errorf("session peran = %q, want guru", sm.GetString(ctx, "peran"))
	}
	if sm.GetString(ctx, "namaGuru") != "Budi Santoso, S.Pd." {
		t.Errorf("session namaGuru = %q, want Budi Santoso, S.Pd.", sm.GetString(ctx, "namaGuru"))
	}
}

func TestLogoutAdminDanGuru(t *testing.T) {
	sm := scs.New()

	logoutAdminH := sm.LoadAndSave(auth.LogoutAdminHandler(sm))
	logoutGuruH := sm.LoadAndSave(auth.LogoutGuruHandler(sm))

	// Logout Admin
	req := httptest.NewRequest("GET", "/logout-admin", nil)
	ctx, _ := sm.Load(req.Context(), "")
	sm.Put(ctx, "adminId", 1)
	sm.Put(ctx, "peran", "admin")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	logoutAdminH.ServeHTTP(w, req)

	if w.Code != http.StatusFound || w.Header().Get("Location") != "/login-admin" {
		t.Errorf("logout admin: status = %d, loc = %q, want 302 -> /login-admin", w.Code, w.Header().Get("Location"))
	}
	if sm.GetInt(ctx, "adminId") != 0 {
		t.Errorf("setelah logout admin, adminId harus 0, got %d", sm.GetInt(ctx, "adminId"))
	}

	// Logout Guru
	req = httptest.NewRequest("GET", "/logout-guru", nil)
	ctxGuru, _ := sm.Load(req.Context(), "")
	sm.Put(ctxGuru, "guruId", 2)
	sm.Put(ctxGuru, "peran", "guru")
	req = req.WithContext(ctxGuru)
	w = httptest.NewRecorder()
	logoutGuruH.ServeHTTP(w, req)

	if w.Code != http.StatusFound || w.Header().Get("Location") != "/login-guru" {
		t.Errorf("logout guru: status = %d, loc = %q, want 302 -> /login-guru", w.Code, w.Header().Get("Location"))
	}
	if sm.GetInt(ctxGuru, "guruId") != 0 {
		t.Errorf("setelah logout guru, guruId harus 0, got %d", sm.GetInt(ctxGuru, "guruId"))
	}
}

func TestRateLimitLoginAdminGuru(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	limiter := ratelimit.New(ctx, 10, 15*time.Minute, 15*time.Minute)
	handler := limiter.Middleware(func(r *http.Request) string {
		ip := ratelimit.ClientIP(r, false)
		username := strings.TrimSpace(r.FormValue("username"))
		return fmt.Sprintf("%s:%s", ip, username)
	}, "Terlalu banyak percobaan login", "login-admin.gohtml")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))

	for i := 1; i <= 10; i++ {
		form := url.Values{}
		form.Set("username", "admin")
		req := httptest.NewRequest("POST", "/login-admin", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "192.168.1.50:1234"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request ke-%d harus lolos, got %d", i, w.Code)
		}
	}

	// Request ke-11 untuk IP+username sama harus diblokir (429)
	form := url.Values{}
	form.Set("username", "admin")
	req := httptest.NewRequest("POST", "/login-admin", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "192.168.1.50:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("request ke-11 harus 429 Too Many Requests, got %d", w.Code)
	}

	// Request dari IP lain atau username lain tetap lolos
	form = url.Values{}
	form.Set("username", "guru")
	req = httptest.NewRequest("POST", "/login-guru", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "192.168.1.50:1234"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("username lain dari IP sama harus lolos, got %d", w.Code)
	}
}

func extractCookieToken(cookieHeader, cookieName string) string {
	parts := strings.Split(cookieHeader, ";")
	for _, p := range parts {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) == 2 && kv[0] == cookieName {
			return kv[1]
		}
	}
	return ""
}
