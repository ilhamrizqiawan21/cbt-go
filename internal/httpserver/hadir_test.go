package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"cbt-go/internal/admin"
	"cbt-go/internal/testdb"
)

func TestDaftarHadirDanBeritaAcara(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)
	app, deps := setupAPISiswaTestApp(t, db)
	siswa1, ujianID, _ := setupAPISiswaData(t, db) // siswa1 punya sesi 'sedang_ujian'; kelas VII-A

	db.Exec("INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('T0002', 'Tidak Hadir <b>', 'VII-A', 'h'), ('T0009', 'Kelas Lain', 'IX-Z', 'h')")
	db.Exec(`INSERT INTO log_kecurangan (siswa_id, ujian_id, jenis_kecurangan, device_type) VALUES
		(?, ?, 'pindah_tab', 'hp'), (?, ?, 'copy_paste', 'hp'), (?, ?, 'ambil_alih_sesi', 'hp'), (?, ?, 'buka_kunci', 'hp')`,
		siswa1, ujianID, siswa1, ujianID, siswa1, ujianID, siswa1, ujianID)

	list, rekap, err := admin.GetDaftarHadir(t.Context(), db, ujianID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("hanya siswa kelas ujian yang tampil, got %d baris", len(list))
	}
	var hadir, absen admin.HadirRow
	for _, b := range list {
		if b.NIS == "T0001" {
			hadir = b
		} else {
			absen = b
		}
	}
	if hadir.Status != "sedang_ujian" || hadir.Mulai == nil || hadir.Pelanggaran != 2 || hadir.AmbilAlih != 1 {
		t.Errorf("baris hadir salah (buka_kunci/ambil_alih tidak dihitung pelanggaran): %+v", hadir)
	}
	if absen.Status != "" || absen.Mulai != nil || absen.Pelanggaran != 0 {
		t.Errorf("baris tidak hadir salah: %+v", absen)
	}
	if rekap.Total != 2 || rekap.Hadir != 1 || rekap.TidakHadir != 1 || rekap.BelumSelesai != 1 || rekap.TotalPelanggaran != 2 {
		t.Errorf("rekap salah: %+v", rekap)
	}

	get := func(kv map[string]any) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/admin/hasil/hadir/"+strconv.Itoa(ujianID), nil)
		if kv != nil {
			req.AddCookie(cookieDenganKunci(t, deps, kv))
		}
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		return rec
	}
	rec := get(map[string]any{"peran": "admin", "adminId": 1, "username": "a"})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Tidak hadir") || !strings.Contains(rec.Body.String(), "Tidak Hadir &lt;b&gt;") {
		t.Errorf("admin: status = %d (nama harus ter-escape)", rec.Code)
	}
	if rec := get(map[string]any{"peran": "guru", "guruId": 1, "username": "g"}); rec.Code == http.StatusOK {
		t.Error("guru tidak boleh membuka daftar hadir admin")
	}
	if rec := get(nil); rec.Code == http.StatusOK {
		t.Error("anonim tidak boleh membuka daftar hadir")
	}
	req := httptest.NewRequest("GET", "/admin/hasil/hadir/999999", nil)
	req.AddCookie(cookieDenganKunci(t, deps, map[string]any{"peran": "admin", "adminId": 1, "username": "a"}))
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("ujian tak ada: status = %d, want 404", rr.Code)
	}
}
