package admin_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/xuri/excelize/v2"

	"cbt-go/internal/admin"
	"cbt-go/internal/auth"
)

type mockHasilDriver struct{}

func (d *mockHasilDriver) Open(name string) (driver.Conn, error) {
	return &mockHasilConn{}, nil
}

type mockHasilConn struct{}

func (c *mockHasilConn) Prepare(query string) (driver.Stmt, error) {
	return &mockHasilStmt{query: query}, nil
}
func (c *mockHasilConn) Close() error { return nil }
func (c *mockHasilConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

type mockHasilStmt struct {
	query string
}

func (s *mockHasilStmt) Close() error  { return nil }
func (s *mockHasilStmt) NumInput() int { return -1 }
func (s *mockHasilStmt) Exec(args []driver.Value) (driver.Result, error) {
	return mockResult{id: 1}, nil
}
func (s *mockHasilStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.QueryContext(context.Background(), named)
}

func (s *mockHasilStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	now := time.Now()
	if strings.Contains(s.query, "FROM nilai_ujian nu") {
		return &sliceRows{
			columns: []string{
				"id", "nis", "nama", "kelas",
				"poin_otomatis", "poin_essay", "poin_maks", "nilai",
				"benar", "salah", "kosong", "status_koreksi", "selesai_pada",
				"id", "nama_ujian", "nama_mapel", "nama", "nama_kelas", "siswa_id",
			},
			rows: [][]driver.Value{
				{
					int64(1), "1001", "Budi Santoso", "VII-A",
					int64(16), int64(14), int64(40), int64(75),
					int64(8), int64(2), int64(0), "selesai", now,
					int64(10), "PTS Matematika", "Matematika", "Guru A", "VII-A", int64(5),
				},
			},
		}, nil
	}

	if strings.Contains(s.query, "WHERE u.id = ?") {
		return &sliceRows{
			columns: []string{
				"id", "pengajaran_id", "nama_ujian", "durasi",
				"tanggal_mulai", "tanggal_selesai", "acak_soal", "acak_pilihan",
				"batas_pelanggaran", "token_ujian",
				"nama", "nama_mapel", "nama_kelas", "mapel_id",
			},
			rows: [][]driver.Value{
				{
					int64(10), int64(1), "PTS Matematika", int64(90),
					now, now.Add(2 * time.Hour), int64(1), int64(1),
					int64(3), "TOKEN123",
					"Guru A", "Matematika", "VII-A", int64(1),
				},
			},
		}, nil
	}

	if strings.Contains(s.query, "FROM ujian") {
		return &sliceRows{
			columns: []string{
				"id", "pengajaran_id", "nama_ujian", "durasi",
				"tanggal_mulai", "tanggal_selesai", "acak_soal", "acak_pilihan",
				"batas_pelanggaran", "token_ujian",
				"nama", "nama_mapel", "nama_kelas", "mapel_id",
			},
			rows: [][]driver.Value{
				{
					int64(10), int64(1), "PTS Matematika", int64(90),
					now, now.Add(2 * time.Hour), int64(1), int64(1),
					int64(3), "TOKEN123",
					"Guru A", "Matematika", "VII-A", int64(1),
				},
			},
		}, nil
	}

	return nil, fmt.Errorf("mock hasil unknown query: %s", s.query)
}

func setupHasilMockDB(t *testing.T) *sql.DB {
	t.Helper()
	drv := &mockHasilDriver{}
	driverName := fmt.Sprintf("mock_hasil_%d", time.Now().UnixNano())
	sql.Register(driverName, drv)

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open mock hasil db: %v", err)
	}
	return db
}

func TestAdminHasil_ListDanExport(t *testing.T) {
	db := setupHasilMockDB(t)
	defer db.Close()
	sm := scs.New()

	mux := http.NewServeMux()
	admin.RegisterAdminHasilRoutes(mux, db, sm, auth.RequireAdmin(sm))
	h := sm.LoadAndSave(mux)

	adminCtx := func(r *http.Request) *http.Request {
		ctx, _ := sm.Load(r.Context(), "")
		sm.Put(ctx, "peran", "admin")
		sm.Put(ctx, "adminId", 1)
		return r.WithContext(ctx)
	}

	// 1. GET /admin/hasil?ujian_id=10 JSON
	reqList := httptest.NewRequest(http.MethodGet, "/admin/hasil?ujian_id=10", nil)
	reqList.Header.Set("Accept", "application/json")
	reqList = adminCtx(reqList)
	rrList := httptest.NewRecorder()
	h.ServeHTTP(rrList, reqList)

	if rrList.Code != http.StatusOK {
		t.Fatalf("status list hasil harus 200, dapat %d", rrList.Code)
	}

	// 2. GET /admin/hasil/export?ujian_id=10
	reqExport := httptest.NewRequest(http.MethodGet, "/admin/hasil/export?ujian_id=10", nil)
	reqExport = adminCtx(reqExport)
	rrExport := httptest.NewRecorder()
	h.ServeHTTP(rrExport, reqExport)

	if rrExport.Code != http.StatusOK {
		t.Fatalf("status export hasil harus 200, dapat %d", rrExport.Code)
	}

	f, err := excelize.OpenReader(rrExport.Body)
	if err != nil {
		t.Fatalf("file export excel tidak dapat dibuka: %v", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	val, _ := f.GetCellValue(sheet, "B6")
	if val != "1001" {
		t.Errorf("NIS pada baris pertama harus 1001, dapat %s", val)
	}

	// 3. GET /admin/hasil/cetak/10
	reqCetak := httptest.NewRequest(http.MethodGet, "/admin/hasil/cetak/10", nil)
	reqCetak = adminCtx(reqCetak)
	rrCetak := httptest.NewRecorder()
	h.ServeHTTP(rrCetak, reqCetak)

	if rrCetak.Code != http.StatusOK {
		t.Fatalf("status cetak hasil harus 200, dapat %d", rrCetak.Code)
	}
	if !strings.Contains(rrCetak.Body.String(), "DAFTAR NILAI UJIAN BERBASIS KOMPUTER") {
		t.Errorf("konten halaman cetak tidak sesuai")
	}
}
