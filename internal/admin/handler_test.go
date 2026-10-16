package admin_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
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

	"cbt-go/internal/admin"
	"cbt-go/internal/auth"
)

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

// mockMasterDriver melayani kueri handler CRUD admin
type mockMasterDriver struct {
	lastInsertID int64
}

func (d *mockMasterDriver) Open(name string) (driver.Conn, error) {
	return &mockMasterConn{d: d}, nil
}

type mockMasterConn struct {
	d *mockMasterDriver
}

func (c *mockMasterConn) Prepare(query string) (driver.Stmt, error) {
	return &mockMasterStmt{conn: c, query: query}, nil
}
func (c *mockMasterConn) Close() error { return nil }
func (c *mockMasterConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

type mockMasterStmt struct {
	conn  *mockMasterConn
	query string
}

func (s *mockMasterStmt) Close() error  { return nil }
func (s *mockMasterStmt) NumInput() int { return -1 }
func (s *mockMasterStmt) Exec(args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.ExecContext(context.Background(), s.query, named)
}
func (s *mockMasterStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.QueryContext(context.Background(), s.query, named)
}

type mockResult struct {
	id int64
}

func (r mockResult) LastInsertId() (int64, error) { return r.id, nil }
func (r mockResult) RowsAffected() (int64, error) { return 1, nil }

func (c *mockMasterConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "INSERT INTO guru") {
		// Jika username 'duplikat', simulasikan duplicate key error
		for _, a := range args {
			if a.Value == "duplikat" {
				return nil, errors.New("Error 1062: Duplicate entry 'duplikat' for key 'username'")
			}
		}
		c.d.lastInsertID++
		return mockResult{id: c.d.lastInsertID}, nil
	}
	if strings.Contains(query, "INSERT INTO kelas") {
		for _, a := range args {
			if a.Value == "VII-DUPLIKAT" {
				return nil, errors.New("Error 1062: Duplicate entry 'VII-DUPLIKAT' for key 'nama_kelas'")
			}
		}
		c.d.lastInsertID++
		return mockResult{id: c.d.lastInsertID}, nil
	}
	return mockResult{id: 1}, nil
}

func (c *mockMasterConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "FROM guru") {
		return &sliceRows{
			columns: []string{"id", "nip", "nama", "username", "password"},
			rows: [][]driver.Value{
				{int64(1), "123", "Guru Test", "gurutest", "hash"},
			},
		}, nil
	}
	if strings.Contains(query, "FROM kelas") {
		return &sliceRows{
			columns: []string{"id", "nama_kelas"},
			rows: [][]driver.Value{
				{int64(1), "VII-A"},
			},
		}, nil
	}
	if strings.Contains(query, "FROM mata_pelajaran") {
		return &sliceRows{
			columns: []string{"id", "nama_mapel"},
			rows: [][]driver.Value{
				{int64(1), "Matematika"},
			},
		}, nil
	}
	if strings.Contains(query, "FROM pengajaran") {
		return &sliceRows{
			columns: []string{"id", "guru_id", "mapel_id", "kelas_id", "nama", "nama_mapel", "nama_kelas"},
			rows: [][]driver.Value{
				{int64(1), int64(1), int64(1), int64(1), "Guru Test", "Matematika", "VII-A"},
			},
		}, nil
	}
	return nil, fmt.Errorf("query tidak dikenal: %s", query)
}

func setupMasterMockDB(t *testing.T) *sql.DB {
	t.Helper()
	drv := &mockMasterDriver{}
	driverName := fmt.Sprintf("mock_master_%d", time.Now().UnixNano())
	sql.Register(driverName, drv)

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open mock master db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestAdminHandlers_CRUD(t *testing.T) {
	db := setupMasterMockDB(t)
	sm := scs.New()
	mux := http.NewServeMux()

	admin.RegisterAdminMasterRoutes(mux, db, sm, auth.RequireAdmin(sm))
	h := sm.LoadAndSave(mux)

	adminCtx := func(r *http.Request) *http.Request {
		ctx, _ := sm.Load(r.Context(), "")
		sm.Put(ctx, "peran", "admin")
		sm.Put(ctx, "adminId", 1)
		return r.WithContext(ctx)
	}

	// 1. GET /admin/guru dengan Accept: application/json
	req := httptest.NewRequest("GET", "/admin/guru", nil)
	req.Header.Set("Accept", "application/json")
	req = adminCtx(req)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /admin/guru status = %d, want 200", w.Code)
	}
	var guruList []admin.Guru
	if err := json.NewDecoder(w.Body).Decode(&guruList); err != nil {
		t.Fatalf("decode json guru: %v", err)
	}
	if len(guruList) != 1 || guruList[0].Username != "gurutest" {
		t.Errorf("hasil decode guru tidak cocok: %+v", guruList)
	}

	// 2. POST /admin/guru/tambah berhasil
	form := url.Values{}
	form.Set("nip", "999")
	form.Set("nama", "Guru Baru")
	form.Set("username", "gurubaru")
	form.Set("password", "pass123")
	req = httptest.NewRequest("POST", "/admin/guru/tambah", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req = adminCtx(req)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("POST /admin/guru/tambah status = %d, want 201, body: %s", w.Code, w.Body.String())
	}

	// 3. POST /admin/guru/tambah duplikat -> ditolak 400 dengan pesan jelas
	form = url.Values{}
	form.Set("nip", "999")
	form.Set("nama", "Guru Duplikat")
	form.Set("username", "duplikat")
	form.Set("password", "pass123")
	req = httptest.NewRequest("POST", "/admin/guru/tambah", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req = adminCtx(req)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST duplikat status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "username sudah digunakan") {
		t.Errorf("pesan duplikat tidak sesuai: %s", w.Body.String())
	}

	// 4. POST /admin/kelas/tambah duplikat -> ditolak 400 dengan pesan jelas
	form = url.Values{}
	form.Set("nama_kelas", "VII-DUPLIKAT")
	req = httptest.NewRequest("POST", "/admin/kelas/tambah", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req = adminCtx(req)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST duplikat kelas status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "nama kelas sudah ada") {
		t.Errorf("pesan duplikat kelas tidak sesuai: %s", w.Body.String())
	}
}
