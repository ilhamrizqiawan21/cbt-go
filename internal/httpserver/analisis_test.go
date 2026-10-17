package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"cbt-go/internal/analisis"
	"cbt-go/internal/testdb"
)

func TestAnalisisButirSoal(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)
	app, deps := setupAPISiswaTestApp(t, db)
	siswa1, ujianID, _ := setupAPISiswaData(t, db) // soal 1 = PG kunci A (jawaban siswa1 = 'A'), soal 2 menjodohkan, soal 3 essay

	var soalPG, soalJodoh, soalEssay, guruID int
	db.QueryRow("SELECT id FROM soal WHERE ujian_id = ? AND tipe_soal = 'pg'", ujianID).Scan(&soalPG)
	db.QueryRow("SELECT id FROM soal WHERE ujian_id = ? AND tipe_soal = 'menjodohkan'", ujianID).Scan(&soalJodoh)
	db.QueryRow("SELECT id FROM soal WHERE ujian_id = ? AND tipe_soal = 'essay'", ujianID).Scan(&soalEssay)
	db.QueryRow("SELECT guru_id FROM pengajaran p JOIN ujian u ON u.pengajaran_id = p.id WHERE u.id = ?", ujianID).Scan(&guruID)

	// Dua siswa lagi; siswa3 tidak menjawab apa pun. Siswa4 belum selesai (tidak boleh dihitung).
	ids := []int{siswa1}
	for _, nis := range []string{"T0002", "T0003", "T0004"} {
		res, err := db.Exec("INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES (?, ?, 'VII-A', 'h')", nis, "Siswa "+nis)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		ids = append(ids, int(id))
	}
	jawab := func(siswa, soal int, j string, benar int) {
		if _, err := db.Exec("INSERT INTO jawaban_siswa (siswa_id, ujian_id, soal_id, jawaban_dipilih, is_benar, client_ts) VALUES (?, ?, ?, ?, ?, NOW()) ON DUPLICATE KEY UPDATE jawaban_dipilih = VALUES(jawaban_dipilih), is_benar = VALUES(is_benar)", siswa, ujianID, soal, j, benar); err != nil {
			t.Fatal(err)
		}
	}
	jawab(ids[1], soalPG, "B", 0)
	jawab(ids[1], soalJodoh, `[{"kiri":"A","kanan":"1"}]`, 1)
	jawab(ids[3], soalPG, "A", 1) // belum selesai → diabaikan
	for _, s := range ids[:3] {
		if _, err := db.Exec("INSERT INTO nilai_ujian (siswa_id, ujian_id, poin_otomatis, poin_essay, poin_maks, nilai, benar, salah, kosong, status_koreksi) VALUES (?, ?, 0, 0, 8, 50, 0, 0, 0, 'selesai')", s, ujianID); err != nil {
			t.Fatal(err)
		}
	}
	db.Exec("INSERT INTO nilai_essay (siswa_id, ujian_id, soal_id, skor) VALUES (?, ?, ?, 4), (?, ?, ?, 2)", ids[0], ujianID, soalEssay, ids[1], ujianID, soalEssay)

	h, err := analisis.Hitung(t.Context(), db, ujianID)
	if err != nil {
		t.Fatal(err)
	}
	if h.Peserta != 3 || h.RataNilai != 50 || len(h.Butir) != 3 {
		t.Fatalf("peserta=%d rata=%v butir=%d", h.Peserta, h.RataNilai, len(h.Butir))
	}
	pg, jd, es := h.Butir[0], h.Butir[1], h.Butir[2]
	if pg.Benar != 1 || pg.PersenBenar != 33 || pg.PilihanA != 1 || pg.PilihanB != 1 || pg.Kosong != 1 || pg.Kunci != "A" {
		t.Errorf("PG salah: %+v (siswa belum selesai tidak boleh dihitung)", pg)
	}
	if jd.Benar != 1 || jd.PersenBenar != 33 || jd.Kosong != 2 {
		t.Errorf("menjodohkan salah: %+v", jd)
	}
	if es.PersenBenar != -1 || !es.AdaSkor || es.RataSkor != 3 {
		t.Errorf("essay salah: %+v", es)
	}

	// Akses: admin 200; guru pengampu 200; guru lain 403; anonim dialihkan.
	res, _ := db.Exec("INSERT INTO guru (nip, nama, username, password) VALUES ('9', 'Lain', 'gl', 'h')")
	lain, _ := res.LastInsertId()
	get := func(path string, kv map[string]any) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		if kv != nil {
			req.AddCookie(cookieDenganKunci(t, deps, kv))
		}
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		return rec
	}
	if rec := get("/admin/analisis?ujian_id="+strconv.Itoa(ujianID), map[string]any{"peran": "admin", "adminId": 1, "username": "a"}); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "33%") {
		t.Errorf("admin: status = %d", rec.Code)
	}
	if rec := get("/guru/analisis?ujian_id="+strconv.Itoa(ujianID), map[string]any{"peran": "guru", "guruId": guruID, "username": "g"}); rec.Code != http.StatusOK {
		t.Errorf("guru pengampu: status = %d", rec.Code)
	}
	if rec := get("/guru/analisis?ujian_id="+strconv.Itoa(ujianID), map[string]any{"peran": "guru", "guruId": int(lain), "username": "g"}); rec.Code != http.StatusForbidden {
		t.Errorf("guru lain: status = %d, want 403", rec.Code)
	}
	if rec := get("/guru/analisis", nil); rec.Code != http.StatusFound {
		t.Errorf("anonim: status = %d, want 302", rec.Code)
	}
	if rec := get("/admin/analisis", map[string]any{"peran": "guru", "guruId": guruID, "username": "g"}); rec.Code == http.StatusOK {
		t.Error("guru tidak boleh membuka analisis admin")
	}
}
