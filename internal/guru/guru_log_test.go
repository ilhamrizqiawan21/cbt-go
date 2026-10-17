package guru_test

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

	"cbt-go/internal/auth"
	"cbt-go/internal/guru"
)

type mockGuruLogDriver struct{}

func (d *mockGuruLogDriver) Open(name string) (driver.Conn, error) {
	return &mockGuruLogConn{}, nil
}

type mockGuruLogConn struct{}

func (c *mockGuruLogConn) Prepare(query string) (driver.Stmt, error) {
	return &mockGuruLogStmt{query: query}, nil
}
func (c *mockGuruLogConn) Close() error { return nil }
func (c *mockGuruLogConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

type mockGuruLogStmt struct {
	query string
}

func (s *mockGuruLogStmt) Close() error  { return nil }
func (s *mockGuruLogStmt) NumInput() int { return -1 }
func (s *mockGuruLogStmt) Exec(args []driver.Value) (driver.Result, error) {
	return mockResult{id: 1}, nil
}
func (s *mockGuruLogStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.QueryContext(context.Background(), named)
}

func (s *mockGuruLogStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	// Cek otorisasi guru atas ujian
	if strings.Contains(s.query, "SELECT COUNT(*)") && strings.Contains(s.query, "FROM ujian u") {
		var ujianID, guruID int64
		if len(args) >= 2 {
			ujianID, _ = args[0].Value.(int64)
			guruID, _ = args[1].Value.(int64)
		}
		if guruID == 1 && ujianID == 10 {
			return &sliceRows{columns: []string{"count"}, rows: [][]driver.Value{{int64(1)}}}, nil
		}
		return &sliceRows{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}}, nil
	}

	if strings.Contains(s.query, "FROM log_kecurangan lk") {
		return &sliceRows{
			columns: []string{
				"id", "siswa_id", "nis", "nama", "kelas",
				"ujian_id", "nama_ujian", "jenis_kecurangan", "device_type", "timestamp",
			},
			rows: [][]driver.Value{
				{
					int64(1), int64(10), "1001", "Budi Santoso", "VII-A",
					int64(10), "PTS Fisika", "pindah_tab", "laptop", time.Now(),
				},
			},
		}, nil
	}

	if strings.Contains(s.query, "FROM ujian u") && strings.Contains(s.query, "WHERE u.id = ?") {
		now := time.Now()
		return &sliceRows{
			columns: []string{
				"id", "pengajaran_id", "nama_ujian", "durasi",
				"tanggal_mulai", "tanggal_selesai", "acak_soal", "acak_pilihan",
				"batas_pelanggaran", "token_ujian",
				"nama", "nama_mapel", "nama_kelas", "mapel_id",
			},
			rows: [][]driver.Value{
				{
					int64(10), int64(1), "PTS Fisika", int64(90),
					now, now.Add(2 * time.Hour), int64(1), int64(1),
					int64(3), "TOKEN123",
					"Guru A", "Fisika", "VII-A", int64(1),
				},
			},
		}, nil
	}

	if strings.Contains(s.query, "FROM ujian u") && strings.Contains(s.query, "WHERE p.guru_id = ?") {
		return &sliceRows{
			columns: []string{"id", "nama_ujian", "durasi", "nama_mapel", "nama_kelas", "t_mulai", "t_selesai"},
			rows: [][]driver.Value{
				{int64(10), "PTS Fisika", int64(90), "Fisika", "VII-A", "2026-10-16 10:00", "2026-10-16 11:30"},
			},
		}, nil
	}

	return nil, fmt.Errorf("mock guru log unknown query: %s", s.query)
}

func setupGuruLogMockDB(t *testing.T) *sql.DB {
	t.Helper()
	drv := &mockGuruLogDriver{}
	driverName := namaDriverMock("mock_guru_log")
	sql.Register(driverName, drv)

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open mock guru log db: %v", err)
	}
	return db
}

func TestGuruLogKecurangan_Otorisasi(t *testing.T) {
	db := setupGuruLogMockDB(t)
	defer db.Close()
	sm := scs.New()

	mux := http.NewServeMux()
	guru.RegisterGuruLogRoutes(mux, db, sm, auth.RequireGuru(sm))
	h := sm.LoadAndSave(mux)

	guruCtx := func(r *http.Request, guruID int) *http.Request {
		ctx, _ := sm.Load(r.Context(), "")
		sm.Put(ctx, "peran", "guru")
		sm.Put(ctx, "guruId", guruID)
		sm.Put(ctx, "namaGuru", fmt.Sprintf("Guru %d", guruID))
		return r.WithContext(ctx)
	}

	// 1. Guru 1 (pengampu Ujian 10) akses log -> Sukses 200
	reqG1 := httptest.NewRequest(http.MethodGet, "/guru/log-kecurangan?ujian_id=10", nil)
	reqG1.Header.Set("Accept", "application/json")
	reqG1 = guruCtx(reqG1, 1)
	rrG1 := httptest.NewRecorder()
	h.ServeHTTP(rrG1, reqG1)

	if rrG1.Code != http.StatusOK {
		t.Fatalf("Guru 1 harus bisa akses log kecurangan ujian miliknya, dapat %d", rrG1.Code)
	}

	// 2. Guru 2 (BUKAN pengampu Ujian 10) akses log -> Ditolak 403 Forbidden
	reqG2 := httptest.NewRequest(http.MethodGet, "/guru/log-kecurangan?ujian_id=10", nil)
	reqG2.Header.Set("Accept", "application/json")
	reqG2 = guruCtx(reqG2, 2)
	rrG2 := httptest.NewRecorder()
	h.ServeHTTP(rrG2, reqG2)

	if rrG2.Code != http.StatusForbidden {
		t.Fatalf("Guru 2 harus ditolak 403 saat akses log ujian Guru 1, dapat %d", rrG2.Code)
	}

	// 3. Guru 2 mencoba ekspor log Ujian 10 -> Ditolak 403 Forbidden
	reqExpG2 := httptest.NewRequest(http.MethodGet, "/guru/log-kecurangan/export?ujian_id=10", nil)
	reqExpG2 = guruCtx(reqExpG2, 2)
	rrExpG2 := httptest.NewRecorder()
	h.ServeHTTP(rrExpG2, reqExpG2)

	if rrExpG2.Code != http.StatusForbidden {
		t.Fatalf("Guru 2 harus ditolak 403 saat ekspor log ujian Guru 1, dapat %d", rrExpG2.Code)
	}

	// 4. Guru 1 ekspor log Ujian 10 -> Sukses 200 dengan Excel valid
	reqExpG1 := httptest.NewRequest(http.MethodGet, "/guru/log-kecurangan/export?ujian_id=10", nil)
	reqExpG1 = guruCtx(reqExpG1, 1)
	rrExpG1 := httptest.NewRecorder()
	h.ServeHTTP(rrExpG1, reqExpG1)

	if rrExpG1.Code != http.StatusOK {
		t.Fatalf("Guru 1 harus sukses ekspor log ujian miliknya, dapat %d", rrExpG1.Code)
	}

	f, err := excelize.OpenReader(rrExpG1.Body)
	if err != nil {
		t.Fatalf("file export log guru tidak dapat dibuka: %v", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	val, _ := f.GetCellValue(sheet, "C6")
	if val != "1001" {
		t.Errorf("NIS pada baris pertama harus 1001, dapat %s", val)
	}
}
