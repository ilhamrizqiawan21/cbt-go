package admin_test

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/xuri/excelize/v2"

	"cbt-go/internal/admin"
	"cbt-go/internal/auth"
)

type mockSiswaDriver struct {
	lastInsertID int64
}

func (d *mockSiswaDriver) Open(name string) (driver.Conn, error) {
	return &mockSiswaConn{d: d}, nil
}

type mockSiswaConn struct {
	d *mockSiswaDriver
}

func (c *mockSiswaConn) Prepare(query string) (driver.Stmt, error) {
	return &mockSiswaStmt{conn: c, query: query}, nil
}
func (c *mockSiswaConn) Close() error { return nil }
func (c *mockSiswaConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

type mockSiswaStmt struct {
	conn  *mockSiswaConn
	query string
}

func (s *mockSiswaStmt) Close() error  { return nil }
func (s *mockSiswaStmt) NumInput() int { return -1 }
func (s *mockSiswaStmt) Exec(args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.ExecContext(context.Background(), s.query, named)
}
func (s *mockSiswaStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.QueryContext(context.Background(), s.query, named)
}

func (c *mockSiswaConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "INSERT INTO siswa") {
		for _, a := range args {
			if a.Value == "100-DUP" {
				return nil, errors.New("Error 1062: Duplicate entry '100-DUP' for key 'nis'")
			}
		}
		c.d.lastInsertID++
		return mockResult{id: c.d.lastInsertID}, nil
	}
	if strings.Contains(query, "UPDATE siswa") {
		for _, a := range args {
			if a.Value == "100-DUP" {
				return nil, errors.New("Error 1062: Duplicate entry '100-DUP' for key 'nis'")
			}
		}
		return mockResult{id: 1}, nil
	}
	if strings.Contains(query, "DELETE FROM siswa") {
		return mockResult{id: 1}, nil
	}
	return mockResult{id: 1}, nil
}

func (c *mockSiswaConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "SELECT id FROM siswa WHERE nis = ?") {
		for _, a := range args {
			if a.Value == "NIS-EXISTING" || a.Value == "100-DUP" {
				return &sliceRows{
					columns: []string{"id"},
					rows: [][]driver.Value{
						{int64(99)},
					},
				}, nil
			}
		}
		// tidak ditemukan -> ErrNoRows
		return &sliceRows{columns: []string{"id"}, rows: [][]driver.Value{}}, nil
	}
	if strings.Contains(query, "FROM siswa WHERE id = ?") {
		return &sliceRows{
			columns: []string{"id", "nis", "nama", "kelas", "pin_ujian"},
			rows: [][]driver.Value{
				{int64(1), "1001", "Budi Santoso", "VII-A", "$2a$10$xyz"},
			},
		}, nil
	}
	if strings.Contains(query, "FROM siswa") {
		return &sliceRows{
			columns: []string{"id", "nis", "nama", "kelas", "pin_ujian"},
			rows: [][]driver.Value{
				{int64(1), "1001", "Budi Santoso", "VII-A", "$2a$10$xyz"},
				{int64(2), "1002", "Siti Aminah", "VII-A", "$2a$10$abc"},
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
	return nil, fmt.Errorf("mock siswa unknown query: %s", query)
}

func setupSiswaMockDB(t *testing.T) *sql.DB {
	t.Helper()
	drv := &mockSiswaDriver{}
	driverName := fmt.Sprintf("mock_siswa_%d", time.Now().UnixNano())
	sql.Register(driverName, drv)

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open mock siswa db: %v", err)
	}
	return db
}

func TestSiswaCRUD(t *testing.T) {
	db := setupSiswaMockDB(t)
	defer db.Close()

	ctx := context.Background()

	// 1. List
	list, err := admin.ListSiswa(ctx, db, "")
	if err != nil {
		t.Fatalf("ListSiswa gagal: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("harus 2 siswa, dapat %d", len(list))
	}

	// 2. Get by ID
	s, err := admin.GetSiswaByID(ctx, db, 1)
	if err != nil || s == nil {
		t.Fatalf("GetSiswaByID gagal: %v", err)
	}
	if s.Nama != "Budi Santoso" {
		t.Errorf("nama salah: %s", s.Nama)
	}

	// 3. Tambah sukses
	id, err := admin.TambahSiswa(ctx, db, "1003", "Andi", "VII-A", "123456")
	if err != nil || id <= 0 {
		t.Fatalf("TambahSiswa gagal: %v", err)
	}

	// 4. Tambah duplikat
	_, err = admin.TambahSiswa(ctx, db, "100-DUP", "Andi", "VII-A", "123456")
	if !errors.Is(err, admin.ErrDuplicateNIS) {
		t.Errorf("harus ErrDuplicateNIS, dapat: %v", err)
	}

	// 5. Edit sukses
	err = admin.EditSiswa(ctx, db, 1, "1001", "Budi Updated", "VII-B", "newpin")
	if err != nil {
		t.Errorf("EditSiswa gagal: %v", err)
	}

	// 6. Hapus
	err = admin.HapusSiswa(ctx, db, 1)
	if err != nil {
		t.Errorf("HapusSiswa gagal: %v", err)
	}
}

func TestSiswaTemplateHandler(t *testing.T) {
	handler := admin.SiswaTemplateHandler()

	req := httptest.NewRequest(http.MethodGet, "/admin/siswa/template", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status template handler harus 200, dapat %d", rr.Code)
	}
	contentType := rr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "spreadsheetml") {
		t.Errorf("Content-Type salah: %s", contentType)
	}

	// Pastikan data yang dihasilkan dapat dibaca oleh excelize
	f, err := excelize.OpenReader(rr.Body)
	if err != nil {
		t.Fatalf("output template bukan file excel yang valid: %v", err)
	}
	defer f.Close()

	val, err := f.GetCellValue("Sheet1", "A1")
	if err != nil || val != "NIS" {
		t.Errorf("header A1 salah: %s (err: %v)", val, err)
	}
}

func TestSiswaImportHandler_Sukses3Baris(t *testing.T) {
	db := setupSiswaMockDB(t)
	defer db.Close()
	sm := scs.New()

	// Buat berkas excel 3 baris
	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "NIS")
	_ = f.SetCellValue(sheet, "B1", "Nama")
	_ = f.SetCellValue(sheet, "C1", "Kelas")
	_ = f.SetCellValue(sheet, "D1", "PIN")

	_ = f.SetCellValue(sheet, "A2", "2001")
	_ = f.SetCellValue(sheet, "B2", "Siswa 1")
	_ = f.SetCellValue(sheet, "C2", "VII-A")
	_ = f.SetCellValue(sheet, "D2", "111111")

	_ = f.SetCellValue(sheet, "A3", "2002")
	_ = f.SetCellValue(sheet, "B3", "Siswa 2")
	_ = f.SetCellValue(sheet, "C3", "VII-A")
	_ = f.SetCellValue(sheet, "D3", "222222")

	_ = f.SetCellValue(sheet, "A4", "2003")
	_ = f.SetCellValue(sheet, "B4", "Siswa 3")
	_ = f.SetCellValue(sheet, "C4", "VII-B")
	_ = f.SetCellValue(sheet, "D4", "333333")

	var excelBuf bytes.Buffer
	if err := f.Write(&excelBuf); err != nil {
		t.Fatalf("tulis excel: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "siswa_3baris.xlsx")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = io.Copy(part, &excelBuf)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/siswa/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	handler := admin.SiswaImportHandler(db, sm)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status import harus 200, dapat %d (body: %s)", rr.Code, rr.Body.String())
	}

	var res map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if int(res["berhasil"].(float64)) != 3 {
		t.Errorf("berhasil harus 3, dapat %v", res["berhasil"])
	}
}

func TestSiswaImportHandler_FormatBukanXlsx(t *testing.T) {
	db := setupSiswaMockDB(t)
	defer db.Close()
	sm := scs.New()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "catatan.csv")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = part.Write([]byte("nis,nama,kelas,pin\n1001,Budi,VII-A,123456"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/siswa/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	handler := admin.SiswaImportHandler(db, sm)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("harus 400 Bad Request untuk file non-xlsx, dapat %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), ".xlsx") {
		t.Errorf("pesan error harus menyebutkan .xlsx, dapat: %s", rr.Body.String())
	}
}

func TestSiswaImportHandler_UkuranMelebihi2MB(t *testing.T) {
	db := setupSiswaMockDB(t)
	defer db.Close()
	sm := scs.New()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "terlalu_besar.xlsx")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	// Buat payload dummy > 2 MB (misal 2.5 MB)
	largeData := make([]byte, int(2.5*1024*1024))
	_, _ = part.Write(largeData)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/siswa/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	handler := admin.SiswaImportHandler(db, sm)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("harus ditolak 400 Bad Request karena melebihi 2MB, dapat %d", rr.Code)
	}
}

func TestSiswaImportHandler_DuplikatNIS(t *testing.T) {
	db := setupSiswaMockDB(t)
	defer db.Close()
	sm := scs.New()

	// File dengan NIS yang sudah ada di DB ('NIS-EXISTING')
	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "NIS")
	_ = f.SetCellValue(sheet, "B1", "Nama")
	_ = f.SetCellValue(sheet, "C1", "Kelas")
	_ = f.SetCellValue(sheet, "D1", "PIN")

	_ = f.SetCellValue(sheet, "A2", "NIS-EXISTING")
	_ = f.SetCellValue(sheet, "B2", "Siswa Duplikat")
	_ = f.SetCellValue(sheet, "C2", "VII-A")
	_ = f.SetCellValue(sheet, "D2", "123456")

	_ = f.SetCellValue(sheet, "A3", "NIS-BARU")
	_ = f.SetCellValue(sheet, "B3", "Siswa Baru")
	_ = f.SetCellValue(sheet, "C3", "VII-A")
	_ = f.SetCellValue(sheet, "D3", "123456")

	var excelBuf bytes.Buffer
	_ = f.Write(&excelBuf)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "duplikat.xlsx")
	_, _ = io.Copy(part, &excelBuf)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/admin/siswa/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	handler := admin.SiswaImportHandler(db, sm)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status harus 200 dengan laporan gagal, dapat %d", rr.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if int(res["berhasil"].(float64)) != 1 {
		t.Errorf("berhasil harus 1, dapat %v", res["berhasil"])
	}
	gagalList := res["gagal"].([]any)
	if len(gagalList) != 1 {
		t.Fatalf("laporan gagal harus 1, dapat %d", len(gagalList))
	}
}

func TestSiswaHTTPRoutesIntegration(t *testing.T) {
	db := setupSiswaMockDB(t)
	defer db.Close()
	sm := scs.New()

	mux := http.NewServeMux()
	admin.RegisterAdminSiswaRoutes(mux, db, sm, auth.RequireAdmin(sm))
	h := sm.LoadAndSave(mux)

	adminCtx := func(r *http.Request) *http.Request {
		ctx, _ := sm.Load(r.Context(), "")
		sm.Put(ctx, "peran", "admin")
		sm.Put(ctx, "adminId", 1)
		return r.WithContext(ctx)
	}

	// 1. GET /admin/siswa tanpa sesi -> ditolak redirect /login-admin
	req := httptest.NewRequest(http.MethodGet, "/admin/siswa", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Errorf("tanpa sesi harus redirect 302, dapat %d", rr.Code)
	}

	// 2. POST /admin/siswa/tambah dengan sesi admin
	form := url.Values{}
	form.Set("nis", "9901")
	form.Set("nama", "Siswa Baru")
	form.Set("kelas", "VII-A")
	form.Set("pin", "123456")

	reqTambah := httptest.NewRequest(http.MethodPost, "/admin/siswa/tambah", strings.NewReader(form.Encode()))
	reqTambah.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqTambah.Header.Set("Accept", "application/json")
	reqTambah = adminCtx(reqTambah)

	rrTambah := httptest.NewRecorder()
	h.ServeHTTP(rrTambah, reqTambah)

	if rrTambah.Code != http.StatusCreated {
		t.Errorf("status tambah siswa harus 201 Created, dapat %d (body: %s)", rrTambah.Code, rrTambah.Body.String())
	}
}
