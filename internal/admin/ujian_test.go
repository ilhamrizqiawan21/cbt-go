package admin_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
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

type mockUjianDriver struct {
	lastInsertID int64
}

func (d *mockUjianDriver) Open(name string) (driver.Conn, error) {
	return &mockUjianConn{d: d}, nil
}

type mockUjianConn struct {
	d *mockUjianDriver
}

func (c *mockUjianConn) Prepare(query string) (driver.Stmt, error) {
	return &mockUjianStmt{conn: c, query: query}, nil
}
func (c *mockUjianConn) Close() error { return nil }
func (c *mockUjianConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

type mockUjianStmt struct {
	conn  *mockUjianConn
	query string
}

func (s *mockUjianStmt) Close() error  { return nil }
func (s *mockUjianStmt) NumInput() int { return -1 }
func (s *mockUjianStmt) Exec(args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.ExecContext(context.Background(), s.query, named)
}
func (s *mockUjianStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.QueryContext(context.Background(), s.query, named)
}

func (c *mockUjianConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "INSERT INTO ujian") {
		c.d.lastInsertID++
		return mockResult{id: c.d.lastInsertID}, nil
	}
	if strings.Contains(query, "UPDATE ujian") {
		return mockResult{id: 1}, nil
	}
	if strings.Contains(query, "DELETE FROM ujian") {
		return mockResult{id: 1}, nil
	}
	return mockResult{id: 1}, nil
}

func (c *mockUjianConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	now := time.Now()
	later := now.Add(2 * time.Hour)

	if strings.Contains(query, "WHERE p.mapel_id = ?") {
		// Mock kueri ujian by mapel
		for _, a := range args {
			if a.Value == int64(10) || a.Value == 10 {
				return &sliceRows{
					columns: []string{
						"id", "pengajaran_id", "nama_ujian", "durasi",
						"tanggal_mulai", "tanggal_selesai", "acak_soal", "acak_pilihan",
						"batas_pelanggaran", "token_ujian",
						"nama", "nama_mapel", "nama_kelas", "mapel_id",
					},
					rows: [][]driver.Value{
						{
							int64(1), int64(5), "PTS Matematika", int64(90),
							now, later, int64(1), int64(1),
							int64(3), "TOKEN123",
							"Guru A", "Matematika", "VII-A", int64(10),
						},
					},
				}, nil
			}
		}
		return &sliceRows{
			columns: []string{
				"id", "pengajaran_id", "nama_ujian", "durasi",
				"tanggal_mulai", "tanggal_selesai", "acak_soal", "acak_pilihan",
				"batas_pelanggaran", "token_ujian",
				"nama", "nama_mapel", "nama_kelas", "mapel_id",
			},
			rows: [][]driver.Value{},
		}, nil
	}

	if strings.Contains(query, "WHERE u.id = ?") {
		return &sliceRows{
			columns: []string{
				"id", "pengajaran_id", "nama_ujian", "durasi",
				"tanggal_mulai", "tanggal_selesai", "acak_soal", "acak_pilihan",
				"batas_pelanggaran", "token_ujian",
				"nama", "nama_mapel", "nama_kelas", "mapel_id",
			},
			rows: [][]driver.Value{
				{
					int64(1), int64(5), "PTS Matematika", int64(90),
					now, later, int64(1), int64(1),
					int64(3), "TOKEN123",
					"Guru A", "Matematika", "VII-A", int64(10),
				},
			},
		}, nil
	}

	if strings.Contains(query, "FROM ujian") {
		return &sliceRows{
			columns: []string{
				"id", "pengajaran_id", "nama_ujian", "durasi",
				"tanggal_mulai", "tanggal_selesai", "acak_soal", "acak_pilihan",
				"batas_pelanggaran", "token_ujian",
				"nama", "nama_mapel", "nama_kelas", "mapel_id",
			},
			rows: [][]driver.Value{
				{
					int64(1), int64(5), "PTS Matematika", int64(90),
					now, later, int64(1), int64(1),
					int64(3), "TOKEN123",
					"Guru A", "Matematika", "VII-A", int64(10),
				},
			},
		}, nil
	}

	if strings.Contains(query, "FROM pengajaran") {
		return &sliceRows{
			columns: []string{"id", "guru_id", "mapel_id", "kelas_id", "nama", "nama_mapel", "nama_kelas"},
			rows: [][]driver.Value{
				{int64(5), int64(1), int64(10), int64(1), "Guru A", "Matematika", "VII-A"},
			},
		}, nil
	}

	return nil, fmt.Errorf("mock ujian unknown query: %s", query)
}

func setupUjianMockDB(t *testing.T) *sql.DB {
	t.Helper()
	drv := &mockUjianDriver{}
	driverName := fmt.Sprintf("mock_ujian_%d", time.Now().UnixNano())
	sql.Register(driverName, drv)

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open mock ujian db: %v", err)
	}
	return db
}

func TestUjianValidasiDanCRUD(t *testing.T) {
	db := setupUjianMockDB(t)
	defer db.Close()
	ctx := context.Background()

	now := time.Now()
	earlier := now.Add(-1 * time.Hour)
	later := now.Add(2 * time.Hour)

	// 1. Validasi tanggal_selesai <= tanggal_mulai
	uSalahWaktu := admin.Ujian{
		PengajaranID:   5,
		NamaUjian:      "Ujian Salah Waktu",
		Durasi:         60,
		TanggalMulai:   now,
		TanggalSelesai: earlier,
	}
	_, err := admin.TambahUjian(ctx, db, uSalahWaktu)
	if !errors.Is(err, admin.ErrInvalidWaktu) {
		t.Errorf("harus ErrInvalidWaktu, dapat: %v", err)
	}

	// 2. Validasi durasi <= 0
	uSalahDurasi := admin.Ujian{
		PengajaranID:   5,
		NamaUjian:      "Ujian Salah Durasi",
		Durasi:         0,
		TanggalMulai:   now,
		TanggalSelesai: later,
	}
	_, err = admin.TambahUjian(ctx, db, uSalahDurasi)
	if !errors.Is(err, admin.ErrInvalidDurasi) {
		t.Errorf("harus ErrInvalidDurasi, dapat: %v", err)
	}

	// 3. Validasi pengajaran <= 0
	uSalahPengajaran := admin.Ujian{
		PengajaranID:   0,
		NamaUjian:      "Ujian Tanpa Pengajaran",
		Durasi:         60,
		TanggalMulai:   now,
		TanggalSelesai: later,
	}
	_, err = admin.TambahUjian(ctx, db, uSalahPengajaran)
	if !errors.Is(err, admin.ErrInvalidPengajaran) {
		t.Errorf("harus ErrInvalidPengajaran, dapat: %v", err)
	}

	// 4. Tambah Ujian Sukses
	token := "TOKEN99"
	uValid := admin.Ujian{
		PengajaranID:     5,
		NamaUjian:        "Ujian Valid",
		Durasi:           60,
		TanggalMulai:     now,
		TanggalSelesai:   later,
		AcakSoal:         true,
		AcakPilihan:      true,
		BatasPelanggaran: 3,
		TokenUjian:       &token,
	}
	id, err := admin.TambahUjian(ctx, db, uValid)
	if err != nil || id <= 0 {
		t.Fatalf("TambahUjian gagal: %v", err)
	}

	// 5. Get Ujian by ID
	uGet, err := admin.GetUjianByID(ctx, db, 1)
	if err != nil {
		t.Fatalf("GetUjianByID gagal: %v", err)
	}
	if uGet.NamaUjian != "PTS Matematika" {
		t.Errorf("nama ujian salah: %s", uGet.NamaUjian)
	}

	// 6. Edit Ujian Sukses
	uValid.ID = 1
	uValid.NamaUjian = "PTS Matematika Rev"
	err = admin.EditUjian(ctx, db, uValid)
	if err != nil {
		t.Fatalf("EditUjian gagal: %v", err)
	}

	// 7. List Ujian
	list, err := admin.ListUjian(ctx, db)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListUjian gagal: %v, len = %d", err, len(list))
	}

	// 8. Hapus Ujian
	err = admin.HapusUjian(ctx, db, 1)
	if err != nil {
		t.Fatalf("HapusUjian gagal: %v", err)
	}
}

func TestUjianByMapelAPI(t *testing.T) {
	db := setupUjianMockDB(t)
	defer db.Close()
	sm := scs.New()

	mux := http.NewServeMux()
	admin.RegisterAdminUjianRoutes(mux, db, sm, auth.RequireAdmin(sm), auth.RequireAdminAPI(sm))
	h := sm.LoadAndSave(mux)

	adminCtx := func(r *http.Request) *http.Request {
		ctx, _ := sm.Load(r.Context(), "")
		sm.Put(ctx, "peran", "admin")
		sm.Put(ctx, "adminId", 1)
		return r.WithContext(ctx)
	}

	// 1. Tanpa sesi admin -> API mengembalikan 401 Unauthorized
	reqUnauthorized := httptest.NewRequest(http.MethodGet, "/admin/api/ujian-by-mapel/10", nil)
	rrUnauthorized := httptest.NewRecorder()
	h.ServeHTTP(rrUnauthorized, reqUnauthorized)

	if rrUnauthorized.Code != http.StatusUnauthorized {
		t.Errorf("API tanpa sesi harus 401 Unauthorized, dapat %d", rrUnauthorized.Code)
	}

	// 2. Dengan sesi admin -> 200 OK dengan list ujian
	req := httptest.NewRequest(http.MethodGet, "/admin/api/ujian-by-mapel/10", nil)
	req = adminCtx(req)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status harus 200 OK, dapat %d", rr.Code)
	}

	var list []admin.Ujian
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("jumlah ujian harus 1, dapat %d", len(list))
	}
	if list[0].NamaUjian != "PTS Matematika" {
		t.Errorf("nama ujian salah: %s", list[0].NamaUjian)
	}
}

func TestUjianHTTPHandlers_TambahDanValidasi(t *testing.T) {
	db := setupUjianMockDB(t)
	defer db.Close()
	sm := scs.New()

	mux := http.NewServeMux()
	admin.RegisterAdminUjianRoutes(mux, db, sm, auth.RequireAdmin(sm), auth.RequireAdminAPI(sm))
	h := sm.LoadAndSave(mux)

	adminCtx := func(r *http.Request) *http.Request {
		ctx, _ := sm.Load(r.Context(), "")
		sm.Put(ctx, "peran", "admin")
		sm.Put(ctx, "adminId", 1)
		return r.WithContext(ctx)
	}

	// 1. POST /admin/ujian/tambah gagal karena tanggal_selesai sebelum tanggal_mulai
	form := url.Values{}
	form.Set("pengajaran_id", "5")
	form.Set("nama_ujian", "Ujian Gagal Tanggal")
	form.Set("durasi", "60")
	form.Set("tanggal_mulai", "2026-10-16T10:00")
	form.Set("tanggal_selesai", "2026-10-16T08:00")

	reqSalah := httptest.NewRequest(http.MethodPost, "/admin/ujian/tambah", strings.NewReader(form.Encode()))
	reqSalah.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqSalah.Header.Set("Accept", "application/json")
	reqSalah = adminCtx(reqSalah)

	rrSalah := httptest.NewRecorder()
	h.ServeHTTP(rrSalah, reqSalah)

	if rrSalah.Code != http.StatusBadRequest {
		t.Fatalf("harus 400 Bad Request, dapat %d", rrSalah.Code)
	}
	if !strings.Contains(rrSalah.Body.String(), "tanggal selesai harus lebih besar") {
		t.Errorf("pesan error salah: %s", rrSalah.Body.String())
	}

	// 2. POST /admin/ujian/tambah berhasil
	form.Set("tanggal_selesai", "2026-10-16T12:00")
	form.Set("token_ujian", "TOKEN123")
	reqSukses := httptest.NewRequest(http.MethodPost, "/admin/ujian/tambah", strings.NewReader(form.Encode()))
	reqSukses.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqSukses.Header.Set("Accept", "application/json")
	reqSukses = adminCtx(reqSukses)

	rrSukses := httptest.NewRecorder()
	h.ServeHTTP(rrSukses, reqSukses)

	if rrSukses.Code != http.StatusCreated {
		t.Fatalf("harus 201 Created, dapat %d (body: %s)", rrSukses.Code, rrSukses.Body.String())
	}
}
