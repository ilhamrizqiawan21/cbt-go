package httpserver_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/xuri/excelize/v2"

	"cbt-go/internal/testdb"
)

func xlsxBytes(t *testing.T, baris [][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	sh := f.GetSheetName(0)
	_ = f.SetSheetRow(sh, "A1", &[]any{"Tipe", "Soal", "Poin", "A", "B", "C", "D", "Kunci", "Pasangan", "Pengecoh"})
	for i, b := range baris {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		_ = f.SetSheetRow(sh, cell, &b)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImporSoalExcel(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)
	handler, deps := setupAPISiswaTestApp(t, db)
	_, ujianID, _ := setupAPISiswaData(t, db)

	var guruID int
	if err := db.QueryRow("SELECT id FROM guru WHERE username = 'guru1'").Scan(&guruID); err != nil {
		t.Fatal(err)
	}
	res, _ := db.Exec("INSERT INTO guru (nip, nama, username, password) VALUES ('2', 'Guru Lain', 'guru9', 'hash')")
	lain, _ := res.LastInsertId()

	jumlahSoal := func() (n int) {
		_ = db.QueryRow("SELECT COUNT(*) FROM soal WHERE ujian_id = ?", ujianID).Scan(&n)
		return
	}
	awal := jumlahSoal()

	kirim := func(path string, c *http.Cookie, nama string, isi []byte, ujian int) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.WriteField("ujian_id", strconv.Itoa(ujian))
		fw, _ := mw.CreateFormFile("file", nama)
		fw.Write(isi)
		mw.Close()
		req := httptest.NewRequest("POST", path, &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Accept", "application/json")
		if c != nil {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	guruC := func(id int) *http.Cookie {
		return cookieDenganKunci(t, deps, map[string]any{"peran": "guru", "guruId": id, "username": "g"})
	}
	adminC := cookieDenganKunci(t, deps, map[string]any{"peran": "admin", "adminId": 1, "username": "a"})

	valid := xlsxBytes(t, [][]any{
		{"pg", "Soal 1", "", "a", "b", "", "", "A"},
		{"essay", "Soal 2"},
	})
	salah := xlsxBytes(t, [][]any{
		{"pg", "Soal valid", "", "a", "b", "", "", "A"},
		{"pg", "Soal rusak", "", "a", "", "", "", "A"},
	})

	// Atomik: satu baris salah → 422 dan tidak ada yang tersimpan.
	rec := kirim("/guru/soal/import", guruC(guruID), "s.xlsx", salah, ujianID)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("baris salah: status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Errors []struct {
			Baris int `json:"baris"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Errors) != 1 || body.Errors[0].Baris != 3 {
		t.Errorf("galat = %+v, want baris 3", body.Errors)
	}
	if jumlahSoal() != awal {
		t.Fatal("impor tidak atomik: ada soal tersimpan padahal ada baris salah")
	}

	// Guru yang bukan pengampu ditolak.
	if rec := kirim("/guru/soal/import", guruC(int(lain)), "s.xlsx", valid, ujianID); rec.Code != http.StatusForbidden {
		t.Errorf("guru lain: status = %d, want 403", rec.Code)
	}
	if jumlahSoal() != awal {
		t.Fatal("guru lain berhasil menyimpan soal")
	}

	// Ekstensi salah.
	if rec := kirim("/guru/soal/import", guruC(guruID), "s.xls", valid, ujianID); rec.Code != http.StatusBadRequest {
		t.Errorf("ekstensi salah: status = %d, want 400", rec.Code)
	}

	// Sukses oleh guru pengampu, lalu oleh admin.
	if rec := kirim("/guru/soal/import", guruC(guruID), "s.xlsx", valid, ujianID); rec.Code != http.StatusCreated {
		t.Fatalf("guru pengampu: status = %d, body %s", rec.Code, rec.Body.String())
	}
	if jumlahSoal() != awal+2 {
		t.Errorf("jumlah soal = %d, want %d", jumlahSoal(), awal+2)
	}
	if rec := kirim("/admin/soal/import", adminC, "s.xlsx", valid, ujianID); rec.Code != http.StatusCreated {
		t.Errorf("admin: status = %d, body %s", rec.Code, rec.Body.String())
	}
	if rec := kirim("/admin/soal/import", nil, "s.xlsx", valid, ujianID); rec.Code == http.StatusCreated {
		t.Error("tanpa login tidak boleh berhasil")
	}

	// Template dapat diunduh.
	req := httptest.NewRequest("GET", "/guru/soal/template", nil)
	req.AddCookie(guruC(guruID))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Header().Get("Content-Disposition") == "" {
		t.Errorf("template: status = %d", rr.Code)
	}
}
