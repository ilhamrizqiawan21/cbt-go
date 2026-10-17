package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"cbt-go/internal/httpserver"
	"cbt-go/internal/soal"
	"cbt-go/internal/testdb"
)

func cookieDenganKunci(t *testing.T, deps *httpserver.Deps, kv map[string]any) *http.Cookie {
	t.Helper()
	ctx, err := deps.SessionManager.Load(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range kv {
		deps.SessionManager.Put(ctx, k, v)
	}
	token, _, err := deps.SessionManager.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: deps.SessionManager.Cookie.Name, Value: token}
}

func berkasMultipart(t *testing.T, isi []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("gambar", "x.png")
	fw.Write(isi)
	mw.Close()
	return &buf, mw.FormDataContentType()
}

func TestGambarSoalAkses(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)

	asli := soal.DirGambar
	soal.DirGambar = filepath.Join(t.TempDir(), "soal")
	t.Cleanup(func() { soal.DirGambar = asli })

	handler, deps := setupAPISiswaTestApp(t, db)
	siswaID, ujianID, deviceToken := setupAPISiswaData(t, db)

	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	nama, err := soal.SimpanGambar(bytes.NewReader(pngBuf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE soal SET gambar = ? WHERE ujian_id = ? ORDER BY id LIMIT 1", nama, ujianID); err != nil {
		t.Fatal(err)
	}

	get := func(path string, c *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		if c != nil {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	siswa := createSiswaSessionCookie(t, deps, siswaID, ujianID, deviceToken)
	rec := get("/uploads/soal/"+nama, siswa)
	if rec.Code != http.StatusOK {
		t.Fatalf("siswa sesi aktif: status = %d", rec.Code)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "image/png") {
		t.Errorf("header salah: %v", rec.Header())
	}

	if rec := get("/uploads/soal/"+nama, nil); rec.Code != http.StatusForbidden {
		t.Errorf("anonim: status = %d, want 403", rec.Code)
	}

	// Siswa lain yang tidak punya sesi pada ujian ini.
	res, _ := db.Exec("INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('T0002', 'Siswa Lain', 'VII-A', 'hash')")
	lainID, _ := res.LastInsertId()
	lain := cookieDenganKunci(t, deps, map[string]any{"peran": "siswa", "siswaId": int(lainID), "ujianId": ujianID})
	if rec := get("/uploads/soal/"+nama, lain); rec.Code != http.StatusForbidden {
		t.Errorf("siswa tanpa sesi: status = %d, want 403", rec.Code)
	}

	admin := cookieDenganKunci(t, deps, map[string]any{"peran": "admin", "adminId": 1, "username": "a"})
	if rec := get("/uploads/soal/"+nama, admin); rec.Code != http.StatusOK {
		t.Errorf("admin: status = %d, want 200", rec.Code)
	}

	// Nama tidak valid / traversal → 404, bukan membaca berkas lain.
	for _, p := range []string{"/uploads/soal/..%2f..%2f.env", "/uploads/soal/abc.png", "/uploads/soal/" + strings.Repeat("a", 32) + ".svg"} {
		if rec := get(p, admin); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", p, rec.Code)
		}
	}

	// Setelah sesi selesai, siswa tidak lagi boleh mengambil gambar.
	if _, err := db.Exec("UPDATE sesi_ujian SET status = 'selesai' WHERE siswa_id = ?", siswaID); err != nil {
		t.Fatal(err)
	}
	if rec := get("/uploads/soal/"+nama, siswa); rec.Code != http.StatusForbidden {
		t.Errorf("sesi selesai: status = %d, want 403", rec.Code)
	}
}

func TestUnggahGambarSoal(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)

	asli := soal.DirGambar
	soal.DirGambar = filepath.Join(t.TempDir(), "soal")
	t.Cleanup(func() { soal.DirGambar = asli })

	handler, deps := setupAPISiswaTestApp(t, db)
	_, ujianID, _ := setupAPISiswaData(t, db)

	var guruID int
	if err := db.QueryRow("SELECT id FROM guru WHERE username = 'guru1'").Scan(&guruID); err != nil {
		t.Fatal(err)
	}
	res, _ := db.Exec("INSERT INTO guru (nip, nama, username, password) VALUES ('1', 'Guru 2', 'guru2', 'hash')")
	guru2, _ := res.LastInsertId()

	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 4, 4)))

	post := func(path string, c *http.Cookie, isi []byte) *httptest.ResponseRecorder {
		body, ct := berkasMultipart(t, isi)
		req := httptest.NewRequest("POST", path, body)
		req.Header.Set("Content-Type", ct)
		req.Header.Set("Accept", "application/json")
		if c != nil {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	guruCookie := func(id int) *http.Cookie {
		return cookieDenganKunci(t, deps, map[string]any{"peran": "guru", "guruId": id, "username": "g"})
	}
	u := strconv.Itoa(ujianID)

	rec := post("/guru/soal/gambar?ujian_id="+u, guruCookie(guruID), pngBuf.Bytes())
	if rec.Code != http.StatusCreated {
		t.Fatalf("guru pengampu: status = %d, body %s", rec.Code, rec.Body.String())
	}
	var out struct{ Gambar string }
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if !soal.NamaGambarValid(out.Gambar) {
		t.Fatalf("nama = %q", out.Gambar)
	}
	if _, err := os.Stat(filepath.Join(soal.DirGambar, out.Gambar)); err != nil {
		t.Errorf("berkas tidak ada: %v", err)
	}

	if rec := post("/guru/soal/gambar?ujian_id="+u, guruCookie(int(guru2)), pngBuf.Bytes()); rec.Code != http.StatusForbidden {
		t.Errorf("guru lain: status = %d, want 403", rec.Code)
	}
	if rec := post("/guru/soal/gambar", guruCookie(guruID), pngBuf.Bytes()); rec.Code != http.StatusForbidden {
		t.Errorf("tanpa ujian_id: status = %d, want 403", rec.Code)
	}
	if rec := post("/guru/soal/gambar?ujian_id="+u, guruCookie(guruID), []byte("<svg></svg>")); rec.Code != http.StatusBadRequest {
		t.Errorf("svg: status = %d, want 400", rec.Code)
	}
	if rec := post("/guru/soal/gambar?ujian_id="+u, guruCookie(guruID), bytes.Repeat([]byte{1}, soal.MaksUkuranGambar+100<<10)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("terlalu besar: status = %d, want 413", rec.Code)
	}

	admin := cookieDenganKunci(t, deps, map[string]any{"peran": "admin", "adminId": 1, "username": "a"})
	if rec := post("/admin/soal/gambar", admin, pngBuf.Bytes()); rec.Code != http.StatusCreated {
		t.Errorf("admin: status = %d, body %s", rec.Code, rec.Body.String())
	}
	if rec := post("/admin/soal/gambar", nil, pngBuf.Bytes()); rec.Code == http.StatusCreated {
		t.Errorf("tanpa login tidak boleh berhasil")
	}
}

func TestGambarSoalDibersihkanSaatSoalDiubahDanDihapus(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)

	asli := soal.DirGambar
	soal.DirGambar = filepath.Join(t.TempDir(), "soal")
	t.Cleanup(func() { soal.DirGambar = asli })

	_, ujianID, _ := setupAPISiswaData(t, db)
	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	baru := func() string {
		n, err := soal.SimpanGambar(bytes.NewReader(pngBuf.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	ada := func(n string) bool { _, err := os.Stat(filepath.Join(soal.DirGambar, n)); return err == nil }

	g1, g2 := baru(), baru()
	ctx := context.Background()
	id, err := soal.TambahSoal(ctx, db, soal.SoalKelola{UjianID: ujianID, TipeSoal: "essay", TeksSoal: "Soal bergambar", Gambar: &g1})
	if err != nil {
		t.Fatal(err)
	}

	// Mengganti gambar menghapus berkas lama, berkas baru tetap ada.
	if err := soal.EditSoal(ctx, db, soal.SoalKelola{ID: id, TipeSoal: "essay", TeksSoal: "Soal bergambar", Gambar: &g2}); err != nil {
		t.Fatal(err)
	}
	if ada(g1) || !ada(g2) {
		t.Errorf("setelah ganti: g1 ada=%v (harus tidak), g2 ada=%v (harus ada)", ada(g1), ada(g2))
	}

	// Menghapus soal menghapus berkasnya.
	if err := soal.HapusSoal(ctx, db, id); err != nil {
		t.Fatal(err)
	}
	if ada(g2) {
		t.Error("berkas yatim tidak dibersihkan setelah soal dihapus")
	}
}
