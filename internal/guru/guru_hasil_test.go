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

type mockGuruHasilDriver struct{}

func (d *mockGuruHasilDriver) Open(name string) (driver.Conn, error) {
	return &mockGuruHasilConn{}, nil
}

type mockGuruHasilConn struct{}

func (c *mockGuruHasilConn) Prepare(query string) (driver.Stmt, error) {
	return &mockGuruHasilStmt{query: query}, nil
}
func (c *mockGuruHasilConn) Close() error { return nil }
func (c *mockGuruHasilConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

type mockGuruHasilStmt struct {
	query string
}

func (s *mockGuruHasilStmt) Close() error  { return nil }
func (s *mockGuruHasilStmt) NumInput() int { return -1 }
func (s *mockGuruHasilStmt) Exec(args []driver.Value) (driver.Result, error) {
	return mockResult{id: 1}, nil
}
func (s *mockGuruHasilStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.QueryContext(context.Background(), named)
}

func (s *mockGuruHasilStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	// Cek otorisasi guru atas ujian: u.id = ? AND p.guru_id = ?
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

	now := time.Now()
	if strings.Contains(s.query, "FROM nilai_ujian nu") {
		return &sliceRows{
			columns: []string{
				"id", "nis", "nama", "kelas",
				"poin_otomatis", "poin_essay", "poin_maks", "nilai",
				"benar", "salah", "kosong", "status_koreksi", "selesai_pada",
				"id", "nama_ujian", "nama_mapel", "nama", "nama_kelas",
			},
			rows: [][]driver.Value{
				{
					int64(1), "1001", "Budi Santoso", "VII-A",
					int64(16), int64(14), int64(40), int64(75),
					int64(8), int64(2), int64(0), "selesai", now,
					int64(10), "PTS Matematika", "Matematika", "Guru A", "VII-A",
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

	if strings.Contains(s.query, "FROM ujian u") && strings.Contains(s.query, "WHERE p.guru_id = ?") {
		return &sliceRows{
			columns: []string{"id", "nama_ujian", "durasi", "nama_mapel", "nama_kelas", "t_mulai", "t_selesai"},
			rows: [][]driver.Value{
				{int64(10), "PTS Matematika", int64(90), "Matematika", "VII-A", "2026-10-16 10:00", "2026-10-16 11:30"},
			},
		}, nil
	}

	return nil, fmt.Errorf("mock guru hasil unknown query: %s", s.query)
}

func setupGuruHasilMockDB(t *testing.T) *sql.DB {
	t.Helper()
	drv := &mockGuruHasilDriver{}
	driverName := fmt.Sprintf("mock_guru_hasil_%d", time.Now().UnixNano())
	sql.Register(driverName, drv)

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open mock guru hasil db: %v", err)
	}
	return db
}

func TestGuruHasil_OtorisasiDanEkspor(t *testing.T) {
	db := setupGuruHasilMockDB(t)
	defer db.Close()
	sm := scs.New()

	mux := http.NewServeMux()
	guru.RegisterGuruHasilRoutes(mux, db, sm, auth.RequireGuru(sm))
	h := sm.LoadAndSave(mux)

	guruCtx := func(r *http.Request, guruID int) *http.Request {
		ctx, _ := sm.Load(r.Context(), "")
		sm.Put(ctx, "peran", "guru")
		sm.Put(ctx, "guruId", guruID)
		sm.Put(ctx, "namaGuru", fmt.Sprintf("Guru %d", guruID))
		return r.WithContext(ctx)
	}

	// 1. Guru 1 (pengampu Ujian 10) akses list hasil -> Sukses 200
	reqG1 := httptest.NewRequest(http.MethodGet, "/guru/hasil-siswa?ujian_id=10", nil)
	reqG1.Header.Set("Accept", "application/json")
	reqG1 = guruCtx(reqG1, 1)
	rrG1 := httptest.NewRecorder()
	h.ServeHTTP(rrG1, reqG1)

	if rrG1.Code != http.StatusOK {
		t.Fatalf("Guru 1 harus bisa akses hasil ujian miliknya, status = %d", rrG1.Code)
	}

	// 2. Guru 2 (BUKAN pengampu Ujian 10) akses list hasil -> Ditolak 403 Forbidden
	reqG2 := httptest.NewRequest(http.MethodGet, "/guru/hasil-siswa?ujian_id=10", nil)
	reqG2.Header.Set("Accept", "application/json")
	reqG2 = guruCtx(reqG2, 2)
	rrG2 := httptest.NewRecorder()
	h.ServeHTTP(rrG2, reqG2)

	if rrG2.Code != http.StatusForbidden {
		t.Fatalf("Guru 2 harus ditolak 403 saat akses hasil ujian Guru 1, status = %d", rrG2.Code)
	}

	// 3. Guru 2 mencoba ekspor hasil Ujian 10 -> Ditolak 403 Forbidden
	reqExpG2 := httptest.NewRequest(http.MethodGet, "/guru/hasil-siswa/export?ujian_id=10", nil)
	reqExpG2 = guruCtx(reqExpG2, 2)
	rrExpG2 := httptest.NewRecorder()
	h.ServeHTTP(rrExpG2, reqExpG2)

	if rrExpG2.Code != http.StatusForbidden {
		t.Fatalf("Guru 2 harus ditolak 403 saat ekspor hasil ujian Guru 1, status = %d", rrExpG2.Code)
	}

	// 4. Guru 1 ekspor hasil Ujian 10 -> Sukses 200 dengan file spreadsheet valid
	reqExpG1 := httptest.NewRequest(http.MethodGet, "/guru/hasil-siswa/export?ujian_id=10", nil)
	reqExpG1 = guruCtx(reqExpG1, 1)
	rrExpG1 := httptest.NewRecorder()
	h.ServeHTTP(rrExpG1, reqExpG1)

	if rrExpG1.Code != http.StatusOK {
		t.Fatalf("Guru 1 harus sukses ekspor hasil ujian miliknya, status = %d", rrExpG1.Code)
	}

	f, err := excelize.OpenReader(rrExpG1.Body)
	if err != nil {
		t.Fatalf("file export guru tidak dapat dibaca: %v", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	val, _ := f.GetCellValue(sheet, "B6")
	if val != "1001" {
		t.Errorf("NIS pada baris data pertama harus 1001, dapat %s", val)
	}
}
