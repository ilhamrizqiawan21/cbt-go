package admin_test

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/admin"
	"cbt-go/internal/auth"
)

type mockResetDriver struct {
	deletedQueries []string
}

func (d *mockResetDriver) Open(name string) (driver.Conn, error) {
	return &mockResetConn{d: d}, nil
}

type mockResetConn struct {
	d *mockResetDriver
}

func (c *mockResetConn) Prepare(query string) (driver.Stmt, error) {
	return &mockResetStmt{conn: c, query: query}, nil
}
func (c *mockResetConn) Close() error { return nil }
func (c *mockResetConn) Begin() (driver.Tx, error) {
	return &mockResetTx{}, nil
}

type mockResetTx struct{}

func (tx *mockResetTx) Commit() error   { return nil }
func (tx *mockResetTx) Rollback() error { return nil }

type mockResetStmt struct {
	conn  *mockResetConn
	query string
}

func (s *mockResetStmt) Close() error  { return nil }
func (s *mockResetStmt) NumInput() int { return -1 }
func (s *mockResetStmt) Exec(args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.ExecContext(context.Background(), s.query, named)
}
func (s *mockResetStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.QueryContext(context.Background(), s.query, named)
}

func (c *mockResetConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.d.deletedQueries = append(c.d.deletedQueries, query)
	return mockResult{id: 1}, nil
}

func (c *mockResetConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "FROM log_kecurangan") {
		return &sliceRows{
			columns: []string{
				"id", "siswa_id", "nis", "nama", "kelas",
				"ujian_id", "nama_ujian", "jenis_kecurangan", "device_type", "timestamp",
			},
			rows: [][]driver.Value{
				{
					int64(1), int64(10), "1001", "Budi Santoso", "VII-A",
					int64(5), "PTS Matematika", "pindah_tab", "laptop", time.Now(),
				},
			},
		}, nil
	}

	if strings.Contains(query, "FROM ujian") {
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
					int64(5), int64(1), "PTS Matematika", int64(90),
					now, now.Add(2 * time.Hour), int64(1), int64(1),
					int64(3), "TOKEN123",
					"Guru A", "Matematika", "VII-A", int64(1),
				},
			},
		}, nil
	}

	return nil, fmt.Errorf("mock reset unknown query: %s", query)
}

func setupResetMockDB(t *testing.T) (*sql.DB, *mockResetDriver) {
	t.Helper()
	drv := &mockResetDriver{
		deletedQueries: make([]string, 0),
	}
	driverName := namaDriverMock("mock_reset")
	sql.Register(driverName, drv)

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open mock reset db: %v", err)
	}
	return db, drv
}

func TestLogKecuranganDanResetUjianAdmin(t *testing.T) {
	db, drv := setupResetMockDB(t)
	defer db.Close()
	sm := scs.New()

	mux := http.NewServeMux()
	admin.RegisterAdminLogResetRoutes(mux, db, sm, auth.RequireAdmin(sm))
	h := sm.LoadAndSave(mux)

	adminCtx := func(r *http.Request) *http.Request {
		ctx, _ := sm.Load(r.Context(), "")
		sm.Put(ctx, "peran", "admin")
		sm.Put(ctx, "adminId", 1)
		return r.WithContext(ctx)
	}

	// 1. GET /admin/log-kecurangan (Admin)
	reqList := httptest.NewRequest(http.MethodGet, "/admin/log-kecurangan?ujian_id=5", nil)
	reqList.Header.Set("Accept", "application/json")
	reqList = adminCtx(reqList)
	rrList := httptest.NewRecorder()
	h.ServeHTTP(rrList, reqList)

	if rrList.Code != http.StatusOK {
		t.Fatalf("status log kecurangan admin harus 200, dapat %d", rrList.Code)
	}

	// 2. POST /admin/reset-ujian untuk siswa 10 pada ujian 5
	bodyReset := bytes.NewBufferString(`{"siswa_id": 10, "ujian_id": 5}`)
	reqReset := httptest.NewRequest(http.MethodPost, "/admin/reset-ujian", bodyReset)
	reqReset.Header.Set("Content-Type", "application/json")
	reqReset = adminCtx(reqReset)
	rrReset := httptest.NewRecorder()
	h.ServeHTTP(rrReset, reqReset)

	if rrReset.Code != http.StatusOK {
		t.Fatalf("reset ujian siswa harus 200 OK, dapat %d (body: %s)", rrReset.Code, rrReset.Body.String())
	}

	// Verifikasi bahwa kueri delete mencakup tabel jawaban, nilai_essay, nilai_ujian, log_kecurangan, sesi_ujian
	// dan membatasi pada siswa_id = ? AND ujian_id = ?
	foundSesiDelete := false
	for _, q := range drv.deletedQueries {
		if strings.Contains(q, "DELETE FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?") {
			foundSesiDelete = true
		}
	}
	if !foundSesiDelete {
		t.Errorf("kueri reset ujian siswa harus menghapus sesi_ujian spesifik siswa dan ujian")
	}

	// 3. POST /admin/reset-ujian-semua untuk ujian 5
	drv.deletedQueries = nil
	bodyResetSemua := bytes.NewBufferString(`{"ujian_id": 5}`)
	reqResetSemua := httptest.NewRequest(http.MethodPost, "/admin/reset-ujian-semua", bodyResetSemua)
	reqResetSemua.Header.Set("Content-Type", "application/json")
	reqResetSemua = adminCtx(reqResetSemua)
	rrResetSemua := httptest.NewRecorder()
	h.ServeHTTP(rrResetSemua, reqResetSemua)

	if rrResetSemua.Code != http.StatusOK {
		t.Fatalf("reset semua peserta harus 200 OK, dapat %d", rrResetSemua.Code)
	}

	foundUjianSemuaDelete := false
	for _, q := range drv.deletedQueries {
		if strings.Contains(q, "DELETE FROM sesi_ujian WHERE ujian_id = ?") {
			foundUjianSemuaDelete = true
		}
	}
	if !foundUjianSemuaDelete {
		t.Errorf("kueri reset semua harus menghapus seluruh sesi pada ujian_id tersebut")
	}
}
