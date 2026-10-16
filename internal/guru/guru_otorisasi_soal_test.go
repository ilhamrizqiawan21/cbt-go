package guru_test

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/admin"
	"cbt-go/internal/auth"
	"cbt-go/internal/guru"
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

type mockOtorisasiDriver struct {
	lastInsertID int64
}

func (d *mockOtorisasiDriver) Open(name string) (driver.Conn, error) {
	return &mockOtorisasiConn{d: d}, nil
}

type mockOtorisasiConn struct {
	d *mockOtorisasiDriver
}

func (c *mockOtorisasiConn) Prepare(query string) (driver.Stmt, error) {
	return &mockOtorisasiStmt{conn: c, query: query}, nil
}
func (c *mockOtorisasiConn) Close() error { return nil }
func (c *mockOtorisasiConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

type mockOtorisasiStmt struct {
	conn  *mockOtorisasiConn
	query string
}

func (s *mockOtorisasiStmt) Close() error  { return nil }
func (s *mockOtorisasiStmt) NumInput() int { return -1 }
func (s *mockOtorisasiStmt) Exec(args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.ExecContext(context.Background(), s.query, named)
}
func (s *mockOtorisasiStmt) Query(args []driver.Value) (driver.Rows, error) {
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

func (c *mockOtorisasiConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "INSERT INTO soal") {
		c.d.lastInsertID++
		return mockResult{id: c.d.lastInsertID}, nil
	}
	if strings.Contains(query, "UPDATE soal") {
		return mockResult{id: 1}, nil
	}
	if strings.Contains(query, "DELETE FROM soal") {
		return mockResult{id: 1}, nil
	}
	return mockResult{id: 1}, nil
}

func (c *mockOtorisasiConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	// Cek otorisasi ujian guru: u.id = ? AND p.guru_id = ?
	if strings.Contains(query, "SELECT COUNT(*)") && strings.Contains(query, "FROM ujian u") {
		var ujianID, guruID int64
		if len(args) >= 2 {
			ujianID, _ = args[0].Value.(int64)
			guruID, _ = args[1].Value.(int64)
		}
		// Guru 1 berhak atas Ujian 10; Guru 2 berhak atas Ujian 20
		if (guruID == 1 && ujianID == 10) || (guruID == 2 && ujianID == 20) {
			return &sliceRows{columns: []string{"count"}, rows: [][]driver.Value{{int64(1)}}}, nil
		}
		return &sliceRows{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}}, nil
	}

	// Cek otorisasi soal guru: s.id = ? AND p.guru_id = ?
	if strings.Contains(query, "SELECT COUNT(*)") && strings.Contains(query, "FROM soal s") {
		var soalID, guruID int64
		if len(args) >= 2 {
			soalID, _ = args[0].Value.(int64)
			guruID, _ = args[1].Value.(int64)
		}
		// Soal 101 milik Ujian 10 (Guru 1); Soal 201 milik Ujian 20 (Guru 2)
		if (guruID == 1 && soalID == 101) || (guruID == 2 && soalID == 201) {
			return &sliceRows{columns: []string{"count"}, rows: [][]driver.Value{{int64(1)}}}, nil
		}
		return &sliceRows{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}}, nil
	}

	if strings.Contains(query, "FROM soal") && strings.Contains(query, "ujian_id = ?") {
		ans := "B"
		return &sliceRows{
			columns: []string{
				"id", "ujian_id", "tipe_soal", "teks_soal", "gambar", "poin",
				"pilihan_a", "pilihan_b", "pilihan_c", "pilihan_d", "jawaban_benar", "opsi_tambahan",
			},
			rows: [][]driver.Value{
				{
					int64(101), int64(10), "pg", "Berapa 1+1?", nil, int64(2),
					"1", "2", "3", "4", ans, nil,
				},
			},
		}, nil
	}

	if strings.Contains(query, "FROM soal") && strings.Contains(query, "id = ?") {
		ans := "B"
		return &sliceRows{
			columns: []string{
				"id", "ujian_id", "tipe_soal", "teks_soal", "gambar", "poin",
				"pilihan_a", "pilihan_b", "pilihan_c", "pilihan_d", "jawaban_benar", "opsi_tambahan",
			},
			rows: [][]driver.Value{
				{
					int64(101), int64(10), "pg", "Berapa 1+1?", nil, int64(2),
					"1", "2", "3", "4", ans, nil,
				},
			},
		}, nil
	}

	if strings.Contains(query, "FROM ujian u") && strings.Contains(query, "WHERE p.guru_id = ?") {
		return &sliceRows{
			columns: []string{"id", "nama_ujian", "durasi", "nama_mapel", "nama_kelas", "t_mulai", "t_selesai"},
			rows: [][]driver.Value{
				{int64(10), "PTS Matematika", int64(90), "Matematika", "VII-A", "2026-10-16 10:00", "2026-10-16 11:30"},
			},
		}, nil
	}

	return nil, fmt.Errorf("mock otorisasi unknown query: %s", query)
}

func setupOtorisasiMockDB(t *testing.T) *sql.DB {
	t.Helper()
	drv := &mockOtorisasiDriver{}
	driverName := fmt.Sprintf("mock_otorisasi_%d", time.Now().UnixNano())
	sql.Register(driverName, drv)

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open mock otorisasi db: %v", err)
	}
	return db
}

// TestGuruOtorisasiSoal memverifikasi bahwa guru hanya dapat melihat, menambah, mengubah, dan menghapus
// soal pada ujian yang diampunya, serta dilarang keras mengakses soal/ujian milik guru lain (403 Forbidden).
func TestGuruOtorisasiSoal(t *testing.T) {
	db := setupOtorisasiMockDB(t)
	defer db.Close()
	sm := scs.New()

	mux := http.NewServeMux()
	guru.RegisterGuruRoutes(mux, db, sm, auth.RequireGuru(sm), auth.RequireGuruAPI(sm))
	h := sm.LoadAndSave(mux)

	guruCtx := func(r *http.Request, guruID int) *http.Request {
		ctx, _ := sm.Load(r.Context(), "")
		sm.Put(ctx, "peran", "guru")
		sm.Put(ctx, "guruId", guruID)
		sm.Put(ctx, "namaGuru", fmt.Sprintf("Guru %d", guruID))
		return r.WithContext(ctx)
	}

	// 1. Guru 1 (pengampu Ujian 10) mengambil soal Ujian 10 -> Sukses 200
	reqG1 := httptest.NewRequest(http.MethodGet, "/guru/api/soal/10", nil)
	reqG1 = guruCtx(reqG1, 1)
	rrG1 := httptest.NewRecorder()
	h.ServeHTTP(rrG1, reqG1)

	if rrG1.Code != http.StatusOK {
		t.Fatalf("Guru 1 harus bisa akses soal ujian miliknya, status = %d (body: %s)", rrG1.Code, rrG1.Body.String())
	}

	// 2. Guru 2 (BUKAN pengampu Ujian 10) mencoba mengambil soal Ujian 10 -> Ditolak 403 Forbidden
	reqG2 := httptest.NewRequest(http.MethodGet, "/guru/api/soal/10", nil)
	reqG2 = guruCtx(reqG2, 2)
	rrG2 := httptest.NewRecorder()
	h.ServeHTTP(rrG2, reqG2)

	if rrG2.Code != http.StatusForbidden {
		t.Fatalf("Guru 2 harus ditolak 403 saat akses soal ujian Guru 1, dapat %d", rrG2.Code)
	}

	// 3. Guru 2 mencoba menambah soal di Ujian 10 -> Ditolak 403 Forbidden
	bodyTambah := bytes.NewBufferString(`{
		"ujian_id": 10,
		"tipe_soal": "pg",
		"teks_soal": "Soal curian Guru 2"
	}`)
	reqTambah := httptest.NewRequest(http.MethodPost, "/guru/soal/tambah", bodyTambah)
	reqTambah.Header.Set("Content-Type", "application/json")
	reqTambah = guruCtx(reqTambah, 2)
	rrTambah := httptest.NewRecorder()
	h.ServeHTTP(rrTambah, reqTambah)

	if rrTambah.Code != http.StatusForbidden {
		t.Fatalf("Guru 2 harus ditolak 403 saat tambah soal di ujian Guru 1, dapat %d", rrTambah.Code)
	}

	// 4. Guru 2 mencoba inspect/edit Soal 101 (milik Guru 1) via GET -> Ditolak 403 Forbidden
	reqGetEdit := httptest.NewRequest(http.MethodGet, "/guru/soal/edit/101", nil)
	reqGetEdit = guruCtx(reqGetEdit, 2)
	rrGetEdit := httptest.NewRecorder()
	h.ServeHTTP(rrGetEdit, reqGetEdit)

	if rrGetEdit.Code != http.StatusForbidden {
		t.Fatalf("Guru 2 harus ditolak 403 saat inspect soal milik Guru 1, dapat %d", rrGetEdit.Code)
	}

	// 5. Guru 2 mencoba edit Soal 101 (milik Guru 1) via POST -> Ditolak 403 Forbidden
	bodyEdit := bytes.NewBufferString(`{
		"tipe_soal": "pg",
		"teks_soal": "Percobaan edit oleh Guru 2"
	}`)
	reqEdit := httptest.NewRequest(http.MethodPost, "/guru/soal/edit/101", bodyEdit)
	reqEdit.Header.Set("Content-Type", "application/json")
	reqEdit = guruCtx(reqEdit, 2)
	rrEdit := httptest.NewRecorder()
	h.ServeHTTP(rrEdit, reqEdit)

	if rrEdit.Code != http.StatusForbidden {
		t.Fatalf("Guru 2 harus ditolak 403 saat edit soal milik Guru 1, dapat %d", rrEdit.Code)
	}

	// 6. Guru 2 mencoba hapus Soal 101 (milik Guru 1) -> Ditolak 403 Forbidden
	reqHapus := httptest.NewRequest(http.MethodPost, "/guru/soal/hapus/101", nil)
	reqHapus.Header.Set("Accept", "application/json")
	reqHapus = guruCtx(reqHapus, 2)
	rrHapus := httptest.NewRecorder()
	h.ServeHTTP(rrHapus, reqHapus)

	if rrHapus.Code != http.StatusForbidden {
		t.Fatalf("Guru 2 harus ditolak 403 saat hapus soal milik Guru 1, dapat %d", rrHapus.Code)
	}

	// 7. Guru 2 mencoba batch tambah soal di Ujian 10 -> Ditolak 403 Forbidden
	bodyBatch := bytes.NewBufferString(`{
		"ujian_id": 10,
		"soal": [
			{"tipe_soal": "pg", "teks_soal": "Batch Soal 1"}
		]
	}`)
	reqBatch := httptest.NewRequest(http.MethodPost, "/guru/soal/batch-tambah", bodyBatch)
	reqBatch.Header.Set("Content-Type", "application/json")
	reqBatch = guruCtx(reqBatch, 2)
	rrBatch := httptest.NewRecorder()
	h.ServeHTTP(rrBatch, reqBatch)

	if rrBatch.Code != http.StatusForbidden {
		t.Fatalf("Guru 2 harus ditolak 403 saat batch tambah di ujian Guru 1, dapat %d", rrBatch.Code)
	}

	// 8. Guru 1 melakukan edit pada soalnya sendiri (Soal 101) -> Sukses 200
	reqEditG1 := httptest.NewRequest(http.MethodPost, "/guru/soal/edit/101", bodyEdit)
	reqEditG1.Header.Set("Content-Type", "application/json")
	reqEditG1 = guruCtx(reqEditG1, 1)
	rrEditG1 := httptest.NewRecorder()
	h.ServeHTTP(rrEditG1, reqEditG1)

	if rrEditG1.Code != http.StatusOK {
		t.Fatalf("Guru 1 harus sukses edit soal miliknya sendiri, dapat %d", rrEditG1.Code)
	}

	// 9. Guru 1 melakukan hapus pada soalnya sendiri (Soal 101) -> Sukses 200
	reqHapusG1 := httptest.NewRequest(http.MethodPost, "/guru/soal/hapus/101", nil)
	reqHapusG1.Header.Set("Accept", "application/json")
	reqHapusG1 = guruCtx(reqHapusG1, 1)
	rrHapusG1 := httptest.NewRecorder()
	h.ServeHTTP(rrHapusG1, reqHapusG1)

	if rrHapusG1.Code != http.StatusOK {
		t.Fatalf("Guru 1 harus sukses hapus soal miliknya sendiri, dapat %d", rrHapusG1.Code)
	}
}

func TestAdminSoalAccess(t *testing.T) {
	db := setupOtorisasiMockDB(t)
	defer db.Close()
	sm := scs.New()

	mux := http.NewServeMux()
	admin.RegisterAdminSoalRoutes(mux, db, sm, auth.RequireAdmin(sm), auth.RequireAdminAPI(sm))
	h := sm.LoadAndSave(mux)

	adminCtx := func(r *http.Request) *http.Request {
		ctx, _ := sm.Load(r.Context(), "")
		sm.Put(ctx, "peran", "admin")
		sm.Put(ctx, "adminId", 1)
		return r.WithContext(ctx)
	}

	// 1. GET /admin/soal?ujian_id=10 (admin memiliki akses global ke seluruh ujian)
	req := httptest.NewRequest(http.MethodGet, "/admin/soal?ujian_id=10", nil)
	req.Header.Set("Accept", "application/json")
	req = adminCtx(req)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Admin harus bisa akses soal ujian secara global, status = %d", rr.Code)
	}

	// 2. POST /admin/soal/tambah
	bodyTambah := bytes.NewBufferString(`{
		"ujian_id": 10,
		"tipe_soal": "pg",
		"teks_soal": "Soal dibuat admin",
		"poin": 2
	}`)
	reqTambah := httptest.NewRequest(http.MethodPost, "/admin/soal/tambah", bodyTambah)
	reqTambah.Header.Set("Content-Type", "application/json")
	reqTambah = adminCtx(reqTambah)
	rrTambah := httptest.NewRecorder()
	h.ServeHTTP(rrTambah, reqTambah)

	if rrTambah.Code != http.StatusCreated {
		t.Fatalf("Admin harus sukses tambah soal, status = %d (body: %s)", rrTambah.Code, rrTambah.Body.String())
	}
}
