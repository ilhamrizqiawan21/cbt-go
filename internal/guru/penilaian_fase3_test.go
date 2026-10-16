package guru_test

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
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

type mockEssayDriver struct {
	skorEssayMap map[string]int // key: "siswaID:soalID"
}

func (d *mockEssayDriver) Open(name string) (driver.Conn, error) {
	return &mockEssayConn{d: d}, nil
}

type mockEssayConn struct {
	d *mockEssayDriver
}

func (c *mockEssayConn) Prepare(query string) (driver.Stmt, error) {
	return &mockEssayStmt{conn: c, query: query}, nil
}
func (c *mockEssayConn) Close() error { return nil }
func (c *mockEssayConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

type mockEssayStmt struct {
	conn  *mockEssayConn
	query string
}

func (s *mockEssayStmt) Close() error  { return nil }
func (s *mockEssayStmt) NumInput() int { return -1 }
func (s *mockEssayStmt) Exec(args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.ExecContext(context.Background(), s.query, named)
}
func (s *mockEssayStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.QueryContext(context.Background(), s.query, named)
}

func (c *mockEssayConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "INSERT INTO nilai_essay") {
		// args: siswa_id, ujian_id, soal_id, skor, dinilai_oleh
		if len(args) >= 4 {
			sID := args[0].Value.(int64)
			soalID := args[2].Value.(int64)
			skor := args[3].Value.(int64)
			key := fmt.Sprintf("%d:%d", sID, soalID)
			c.d.skorEssayMap[key] = int(skor)
		}
		return mockResult{id: 1}, nil
	}
	if strings.Contains(query, "INSERT INTO nilai_ujian") {
		return mockResult{id: 1}, nil
	}
	return mockResult{id: 1}, nil
}

func (c *mockEssayConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	// 1. Cek otorisasi guru: u.id = ? AND p.guru_id = ?
	if strings.Contains(query, "SELECT COUNT(*)") && strings.Contains(query, "FROM ujian u") {
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

	// 2. Query soal essay poin: SELECT poin, tipe_soal FROM soal WHERE id = ? AND ujian_id = ?
	if strings.Contains(query, "SELECT poin, tipe_soal") && strings.Contains(query, "FROM soal") {
		var soalID int64
		if len(args) >= 1 {
			soalID, _ = args[0].Value.(int64)
		}
		if soalID == 999 {
			// Soal bukan essay untuk pengujian
			return &sliceRows{columns: []string{"poin", "tipe_soal"}, rows: [][]driver.Value{{int64(2), "pg"}}}, nil
		}
		// Soal essay poin maksimal 4
		return &sliceRows{columns: []string{"poin", "tipe_soal"}, rows: [][]driver.Value{{int64(4), "essay"}}}, nil
	}

	// 3. Query seluruh soal untuk HitungUlangNilai
	if strings.Contains(query, "SELECT id, tipe_soal, poin") && strings.Contains(query, "FROM soal") {
		return &sliceRows{
			columns: []string{"id", "tipe_soal", "poin", "jawaban_benar"},
			rows: [][]driver.Value{
				{int64(1), "pg", int64(2), "A"},
				{int64(2), "essay", int64(4), ""},
			},
		}, nil
	}

	// 4. Query jawaban siswa untuk HitungUlangNilai
	if strings.Contains(query, "FROM jawaban_siswa") {
		return &sliceRows{
			columns: []string{"soal_id", "jawaban_dipilih"},
			rows: [][]driver.Value{
				{int64(1), "A"}, // PG dijawab benar
				{int64(2), "Jawaban essay siswa"},
			},
		}, nil
	}

	// 5. Query nilai essay yang sudah diberikan
	if strings.Contains(query, "FROM nilai_essay") {
		var sID int64
		if len(args) >= 1 {
			sID, _ = args[0].Value.(int64)
		}
		key := fmt.Sprintf("%d:%d", sID, 2)
		if skor, exists := c.d.skorEssayMap[key]; exists {
			return &sliceRows{
				columns: []string{"soal_id", "skor"},
				rows: [][]driver.Value{
					{int64(2), int64(skor)},
				},
			}, nil
		}
		return &sliceRows{columns: []string{"soal_id", "skor"}, rows: [][]driver.Value{}}, nil
	}

	// 6. Query jawaban essay untuk halaman detail atau ekspor
	if strings.Contains(query, "FROM soal so") && strings.Contains(query, "WHERE so.ujian_id = ?") {
		return &sliceRows{
			columns: []string{
				"id", "nis", "nama", "kelas",
				"id", "teks_soal", "jawaban_dipilih", "poin", "skor",
			},
			rows: [][]driver.Value{
				{
					int64(100), "1001", "Budi Santoso", "VII-A",
					int64(2), "Teks Soal Essay", "Jawaban Essay", int64(4), int64(4),
				},
			},
		}, nil
	}

	if strings.Contains(query, "FROM ujian u") && strings.Contains(query, "WHERE u.id = ?") {
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
					int64(10), int64(1), "PTS IPA", int64(90),
					now, now.Add(2 * time.Hour), int64(1), int64(1),
					int64(3), "TOKEN123",
					"Guru A", "IPA", "VII-A", int64(1),
				},
			},
		}, nil
	}

	return nil, fmt.Errorf("mock essay unknown query: %s", query)
}

func setupEssayMockDB(t *testing.T) *sql.DB {
	t.Helper()
	drv := &mockEssayDriver{
		skorEssayMap: make(map[string]int),
	}
	driverName := fmt.Sprintf("mock_essay_%d", time.Now().UnixNano())
	sql.Register(driverName, drv)

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open mock essay db: %v", err)
	}
	return db
}

// TestPenilaianFase3 adalah padanan resmi dari tests/penilaian_fase3.test.js:
// 1. Validasi 0 <= skor <= poin maksimal (skor di luar rentang ditolak)
// 2. Otorisasi guru pengampu (guru lain ditolak 403)
// 3. Status koreksi otomatis berubah menjadi 'selesai' dan nilai akhir terhitung benar
// 4. Impor Excel essay idempoten
func TestPenilaianFase3(t *testing.T) {
	db := setupEssayMockDB(t)
	defer db.Close()
	sm := scs.New()

	mux := http.NewServeMux()
	guru.RegisterGuruEssayRoutes(mux, db, sm, auth.RequireGuru(sm))
	h := sm.LoadAndSave(mux)

	guruCtx := func(r *http.Request, guruID int) *http.Request {
		ctx, _ := sm.Load(r.Context(), "")
		sm.Put(ctx, "peran", "guru")
		sm.Put(ctx, "guruId", guruID)
		sm.Put(ctx, "namaGuru", fmt.Sprintf("Guru %d", guruID))
		return r.WithContext(ctx)
	}

	// 1. Validasi: Skor negatif (-1) ditolak
	bodyNegatif := bytes.NewBufferString(`{"siswa_id": 100, "soal_id": 2, "skor": -1}`)
	reqNegatif := httptest.NewRequest(http.MethodPost, "/guru/essay/10", bodyNegatif)
	reqNegatif.Header.Set("Content-Type", "application/json")
	reqNegatif = guruCtx(reqNegatif, 1)
	rrNegatif := httptest.NewRecorder()
	h.ServeHTTP(rrNegatif, reqNegatif)

	if rrNegatif.Code != http.StatusBadRequest {
		t.Fatalf("skor negatif harus 400 Bad Request, dapat %d", rrNegatif.Code)
	}

	// 2. Validasi: Skor melebihi poin maksimal (5 > 4) ditolak
	bodyLebih := bytes.NewBufferString(`{"siswa_id": 100, "soal_id": 2, "skor": 5}`)
	reqLebih := httptest.NewRequest(http.MethodPost, "/guru/essay/10", bodyLebih)
	reqLebih.Header.Set("Content-Type", "application/json")
	reqLebih = guruCtx(reqLebih, 1)
	rrLebih := httptest.NewRecorder()
	h.ServeHTTP(rrLebih, reqLebih)

	if rrLebih.Code != http.StatusBadRequest {
		t.Fatalf("skor melebihi poin harus 400 Bad Request, dapat %d", rrLebih.Code)
	}

	// 3. Otorisasi: Guru 2 (bukan pengampu) ditolak 403 Forbidden
	bodyValid := bytes.NewBufferString(`{"siswa_id": 100, "soal_id": 2, "skor": 4}`)
	reqG2 := httptest.NewRequest(http.MethodPost, "/guru/essay/10", bodyValid)
	reqG2.Header.Set("Content-Type", "application/json")
	reqG2 = guruCtx(reqG2, 2)
	rrG2 := httptest.NewRecorder()
	h.ServeHTTP(rrG2, reqG2)

	if rrG2.Code != http.StatusForbidden {
		t.Fatalf("guru non-pengampu harus 403 Forbidden, dapat %d", rrG2.Code)
	}

	// 4. Koreksi Sukses: Guru 1 memberi skor 4
	// Soal: PG (poin 2, benar) + Essay (poin 4, skor 4) -> Total didapat: 6 / 6 * 100 = 100.
	// Seluruh essay selesai dinilai -> status_koreksi = 'selesai'
	bodySukses := bytes.NewBufferString(`{"siswa_id": 100, "soal_id": 2, "skor": 4}`)
	reqSukses := httptest.NewRequest(http.MethodPost, "/guru/essay/10", bodySukses)
	reqSukses.Header.Set("Content-Type", "application/json")
	reqSukses = guruCtx(reqSukses, 1)
	rrSukses := httptest.NewRecorder()
	h.ServeHTTP(rrSukses, reqSukses)

	if rrSukses.Code != http.StatusOK {
		t.Fatalf("penilaian sukses harus 200 OK, dapat %d (body: %s)", rrSukses.Code, rrSukses.Body.String())
	}
	if !strings.Contains(rrSukses.Body.String(), `"status_koreksi":"selesai"`) {
		t.Errorf("status koreksi harus 'selesai', respons: %s", rrSukses.Body.String())
	}
	if !strings.Contains(rrSukses.Body.String(), `"nilai_akhir":100`) {
		t.Errorf("nilai akhir harus 100, respons: %s", rrSukses.Body.String())
	}

	// 5. Impor Excel Essay Idempoten
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	for colIdx, h := range []string{"No", "Siswa ID", "NIS", "Nama", "Kelas", "Soal ID", "Soal", "Jawaban", "Poin Maks", "Skor"} {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 5)
		_ = f.SetCellValue(sheet, cell, h)
	}
	// Baris 6: Siswa 100, Soal 2, Skor 4
	_ = f.SetCellValue(sheet, "B6", "100")
	_ = f.SetCellValue(sheet, "F6", "2")
	_ = f.SetCellValue(sheet, "J6", "4")

	var excelBuf bytes.Buffer
	_ = f.Write(&excelBuf)

	bodyImport := &bytes.Buffer{}
	writer := multipart.NewWriter(bodyImport)
	part, _ := writer.CreateFormFile("file", "koreksi_essay.xlsx")
	_, _ = io.Copy(part, &excelBuf)
	_ = writer.Close()

	reqImport := httptest.NewRequest(http.MethodPost, "/guru/essay/10/import", bodyImport)
	reqImport.Header.Set("Content-Type", writer.FormDataContentType())
	reqImport.Header.Set("Accept", "application/json")
	reqImport = guruCtx(reqImport, 1)
	rrImport := httptest.NewRecorder()
	h.ServeHTTP(rrImport, reqImport)

	if rrImport.Code != http.StatusOK {
		t.Fatalf("impor excel essay harus 200 OK, dapat %d (body: %s)", rrImport.Code, rrImport.Body.String())
	}
	if !strings.Contains(rrImport.Body.String(), `"sukses":1`) {
		t.Errorf("hasil impor harus mencatat 1 sukses, dapat: %s", rrImport.Body.String())
	}
}
