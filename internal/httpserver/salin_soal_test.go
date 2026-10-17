package httpserver_test

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"cbt-go/internal/soal"
	"cbt-go/internal/testdb"
)

func TestSalinSoalDanDuplikatUjian(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)

	asli := soal.DirGambar
	soal.DirGambar = filepath.Join(t.TempDir(), "soal")
	t.Cleanup(func() { soal.DirGambar = asli })

	handler, deps := setupAPISiswaTestApp(t, db)
	_, sumber, _ := setupAPISiswaData(t, db) // 3 soal

	// Beri gambar pada satu soal sumber.
	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	gambarSumber, err := soal.SimpanGambar(bytes.NewReader(pngBuf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	db.Exec("UPDATE soal SET gambar = ? WHERE ujian_id = ? ORDER BY id LIMIT 1", gambarSumber, sumber)

	var pengajaranID, guruID int
	if err := db.QueryRow("SELECT pengajaran_id FROM ujian WHERE id = ?", sumber).Scan(&pengajaranID); err != nil {
		t.Fatal(err)
	}
	db.QueryRow("SELECT guru_id FROM pengajaran WHERE id = ?", pengajaranID).Scan(&guruID)
	res, _ := db.Exec(`INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai)
		VALUES (?, 'Ujian Tujuan', 60, NOW(), NOW() + INTERVAL 1 DAY)`, pengajaranID)
	tujuan64, _ := res.LastInsertId()
	tujuan := int(tujuan64)

	hitung := func(ujian int) (n int) {
		db.QueryRow("SELECT COUNT(*) FROM soal WHERE ujian_id = ?", ujian).Scan(&n)
		return
	}
	post := func(path string, c *http.Cookie, f url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(f.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
	form := func(kv ...string) url.Values {
		v := url.Values{}
		for i := 0; i < len(kv); i += 2 {
			v.Set(kv[i], kv[i+1])
		}
		return v
	}

	// Guru lain tidak boleh menyalin.
	res, _ = db.Exec("INSERT INTO guru (nip, nama, username, password) VALUES ('7', 'G7', 'guru7', 'h')")
	lain, _ := res.LastInsertId()
	f := form("ujian_id", strconv.Itoa(tujuan), "dari_ujian_id", strconv.Itoa(sumber))
	if rec := post("/guru/soal/salin", guruC(int(lain)), f); rec.Code != http.StatusForbidden {
		t.Errorf("guru lain: status = %d, want 403", rec.Code)
	}

	// Sumber = tujuan ditolak.
	if rec := post("/admin/soal/salin", adminC, form("ujian_id", strconv.Itoa(sumber), "dari_ujian_id", strconv.Itoa(sumber))); rec.Code != http.StatusBadRequest {
		t.Errorf("sumber=tujuan: status = %d, want 400", rec.Code)
	}

	// Guru pengampu menyalin: 3 soal pindah, sumber tetap utuh, gambar punya berkas sendiri.
	if rec := post("/guru/soal/salin", guruC(guruID), f); rec.Code != http.StatusCreated {
		t.Fatalf("salin: status = %d, body %s", rec.Code, rec.Body.String())
	}
	if hitung(tujuan) != 3 || hitung(sumber) != 3 {
		t.Fatalf("jumlah soal tujuan=%d sumber=%d, want 3 dan 3", hitung(tujuan), hitung(sumber))
	}
	var gambarSalinan string
	db.QueryRow("SELECT gambar FROM soal WHERE ujian_id = ? AND gambar IS NOT NULL", tujuan).Scan(&gambarSalinan)
	if gambarSalinan == "" || gambarSalinan == gambarSumber {
		t.Fatalf("gambar salinan = %q, harus berkas baru (sumber %q)", gambarSalinan, gambarSumber)
	}
	if _, err := os.Stat(filepath.Join(soal.DirGambar, gambarSalinan)); err != nil {
		t.Errorf("berkas salinan tidak ada: %v", err)
	}
	// Menghapus soal salinan tidak merusak gambar sumber.
	var idSalinan int
	db.QueryRow("SELECT id FROM soal WHERE ujian_id = ? AND gambar IS NOT NULL", tujuan).Scan(&idSalinan)
	if err := soal.HapusSoal(t.Context(), db, idSalinan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(soal.DirGambar, gambarSumber)); err != nil {
		t.Errorf("gambar sumber ikut terhapus: %v", err)
	}

	// Duplikat ujian (admin): ujian baru + 3 soal, token tidak ikut kecuali diisi.
	d := form("pengajaran_id", strconv.Itoa(pengajaranID), "nama_ujian", "Ujian Salinan", "durasi", "45",
		"tanggal_mulai", "2030-01-01T08:00", "tanggal_selesai", "2030-01-01T10:00", "batas_pelanggaran", "2")
	rec := post("/admin/ujian/duplikat/"+strconv.Itoa(sumber), adminC, d)
	if rec.Code != http.StatusCreated {
		t.Fatalf("duplikat: status = %d, body %s", rec.Code, rec.Body.String())
	}
	var baruID, durasi int
	var token *string
	if err := db.QueryRow("SELECT id, durasi, token_ujian FROM ujian WHERE nama_ujian = 'Ujian Salinan'").Scan(&baruID, &durasi, &token); err != nil {
		t.Fatal(err)
	}
	if durasi != 45 || token != nil || hitung(baruID) != 3 {
		t.Errorf("duplikat: durasi=%d token=%v soal=%d", durasi, token, hitung(baruID))
	}

	// Sumber kosong → ujian baru dibatalkan (tidak tersisa ujian kosong).
	d.Set("nama_ujian", "Ujian Gagal")
	if rec := post("/admin/ujian/duplikat/"+strconv.Itoa(tujuan+999), adminC, d); rec.Code != http.StatusBadRequest {
		t.Errorf("sumber kosong: status = %d, want 400", rec.Code)
	}
	var sisa int
	db.QueryRow("SELECT COUNT(*) FROM ujian WHERE nama_ujian = 'Ujian Gagal'").Scan(&sisa)
	if sisa != 0 {
		t.Errorf("ujian gagal tidak dibatalkan: %d tersisa", sisa)
	}
	// Guru tidak boleh membuat duplikat ujian.
	if rec := post("/admin/ujian/duplikat/"+strconv.Itoa(sumber), guruC(guruID), d); rec.Code == http.StatusCreated {
		t.Error("guru tidak boleh menduplikat ujian")
	}
}
