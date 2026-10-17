package httpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"cbt-go/internal/testdb"
)

func TestUjianSusulanPerSiswa(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)
	siswaID, ujianID, _, _, _ := setupKetahananE2EData(t, db)
	app, deps := setupAPISiswaTestApp(t, db)

	// Siswa kedua sekelas (PIN sama) dan siswa dari kelas lain.
	if _, err := db.Exec(`INSERT INTO siswa (nis, nama, kelas, pin_ujian) SELECT 'T0002', 'Siswa Dua', kelas, pin_ujian FROM siswa WHERE id = ?`, siswaID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO siswa (nis, nama, kelas, pin_ujian) SELECT 'T0003', 'Siswa Kelas Lain', 'IX-Z', pin_ujian FROM siswa WHERE id = ?`, siswaID); err != nil {
		t.Fatal(err)
	}
	var dua, lain int
	db.QueryRow("SELECT id FROM siswa WHERE nis = 'T0002'").Scan(&dua)
	db.QueryRow("SELECT id FROM siswa WHERE nis = 'T0003'").Scan(&lain)

	// Jendela ujian sudah lewat.
	if _, err := db.Exec("UPDATE ujian SET tanggal_mulai = NOW() - INTERVAL 3 HOUR, tanggal_selesai = NOW() - INTERVAL 1 HOUR WHERE id = ?", ujianID); err != nil {
		t.Fatal(err)
	}

	login := func(nis string) *httptest.ResponseRecorder {
		form := url.Values{"nis": {nis}, "pin": {e2eDefaultPIN}, "ujian_id": {strconv.Itoa(ujianID)}, "device_type": {"laptop"}}
		req := httptest.NewRequest(http.MethodPost, "/login-siswa", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		return rec
	}
	resAdmin, err := db.Exec("INSERT INTO admin (username, password) VALUES ('admin_uji', 'hash')")
	if err != nil {
		t.Fatal(err)
	}
	adminID, _ := resAdmin.LastInsertId()
	adminC := cookieDenganKunci(t, deps, map[string]any{"peran": "admin", "adminId": int(adminID), "username": "a"})
	adminPost := func(path string, f url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(f.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		req.AddCookie(adminC)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		return rec
	}
	u := strconv.Itoa(ujianID)

	// 1. Tanpa izin: login ditolak, dan ujian tidak muncul di daftar publik.
	if rec := login("T0001"); rec.Code == http.StatusFound {
		t.Fatal("login di luar jendela tanpa izin seharusnya ditolak")
	}
	req := httptest.NewRequest("GET", "/api/daftar-ujian", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "Ujian E2E") {
		t.Error("ujian lewat jadwal tidak boleh tampil tanpa izin")
	}

	// 2. Validasi pemberian izin.
	if rec := adminPost("/admin/susulan/tambah", url.Values{"ujian_id": {u}, "siswa_id": {strconv.Itoa(lain)}}); rec.Code != http.StatusBadRequest {
		t.Errorf("siswa kelas lain: status = %d, want 400", rec.Code)
	}
	lampau := time.Now().Add(-time.Hour).Format("2006-01-02T15:04")
	if rec := adminPost("/admin/susulan/tambah", url.Values{"ujian_id": {u}, "siswa_id": {strconv.Itoa(siswaID)}, "sampai": {lampau}}); rec.Code != http.StatusBadRequest {
		t.Errorf("waktu lampau: status = %d, want 400", rec.Code)
	}
	// Non-admin tidak boleh.
	guruC := cookieDenganKunci(t, deps, map[string]any{"peran": "guru", "guruId": 1, "username": "g"})
	reqG := httptest.NewRequest(http.MethodPost, "/admin/susulan/tambah", strings.NewReader(url.Values{"ujian_id": {u}, "siswa_id": {strconv.Itoa(siswaID)}}.Encode()))
	reqG.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqG.AddCookie(guruC)
	recG := httptest.NewRecorder()
	app.ServeHTTP(recG, reqG)
	if recG.Code == http.StatusCreated || recG.Code == http.StatusOK {
		t.Errorf("guru tidak boleh memberi izin susulan, status = %d", recG.Code)
	}

	// 3. Izin untuk siswa pertama (batas default = durasi + 30 menit).
	if rec := adminPost("/admin/susulan/tambah", url.Values{"ujian_id": {u}, "siswa_id": {strconv.Itoa(siswaID)}, "alasan": {"Sakit"}}); rec.Code != http.StatusCreated {
		t.Fatalf("beri izin: status = %d, body %s", rec.Code, rec.Body.String())
	}

	// 4. Siswa berizin: login, paket soal terbuka, timer = durasi penuh (tidak terpotong jadwal lama).
	rec = login("T0001")
	if rec.Code != http.StatusFound {
		t.Fatalf("login siswa berizin: status = %d, body %s", rec.Code, rec.Body.String())
	}
	cookie := rec.Result().Cookies()[0]
	var sisaMenit float64
	if err := db.QueryRow("SELECT TIMESTAMPDIFF(SECOND, NOW(), batas_waktu) / 60 FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?", siswaID, ujianID).Scan(&sisaMenit); err != nil {
		t.Fatal(err)
	}
	if sisaMenit < 58 || sisaMenit > 61 {
		t.Errorf("sisa waktu = %.1f menit, want ~60 (durasi ujian)", sisaMenit)
	}
	reqSoal := httptest.NewRequest("GET", "/api/soal/"+u, nil)
	reqSoal.AddCookie(cookie)
	recSoal := httptest.NewRecorder()
	app.ServeHTTP(recSoal, reqSoal)
	if recSoal.Code != http.StatusOK {
		t.Errorf("paket soal siswa berizin: status = %d, body %s", recSoal.Code, recSoal.Body.String())
	}
	reqCek := httptest.NewRequest("GET", "/api/cek-status-ujian", nil)
	reqCek.AddCookie(cookie)
	recCek := httptest.NewRecorder()
	app.ServeHTTP(recCek, reqCek)
	var cek struct{ Valid bool }
	_ = json.Unmarshal(recCek.Body.Bytes(), &cek)
	if !cek.Valid {
		t.Errorf("cek-status-ujian harus valid untuk siswa berizin: %s", recCek.Body.String())
	}

	// 5. Siswa lain sekelas tetap ditolak; daftar publik menampilkan ujian.
	if rec := login("T0002"); rec.Code == http.StatusFound {
		t.Error("siswa tanpa izin tidak boleh login")
	}
	recL := httptest.NewRecorder()
	app.ServeHTTP(recL, httptest.NewRequest("GET", "/api/daftar-ujian", nil))
	if !strings.Contains(recL.Body.String(), "Ujian E2E") {
		t.Error("ujian dengan izin aktif harus tampil di daftar publik")
	}

	// 6. Izin kedaluwarsa tidak berlaku; izin dapat dicabut; siswa bernilai tidak dapat diberi izin.
	db.Exec("UPDATE izin_susulan SET tanggal_selesai = NOW() - INTERVAL 1 MINUTE WHERE siswa_id = ?", siswaID)
	db.Exec("DELETE FROM sesi_ujian WHERE siswa_id = ?", siswaID)
	if rec := login("T0001"); rec.Code == http.StatusFound {
		t.Error("izin kedaluwarsa tidak boleh dipakai login baru")
	}
	var izinID int
	db.QueryRow("SELECT id FROM izin_susulan WHERE siswa_id = ?", siswaID).Scan(&izinID)
	if rec := adminPost("/admin/susulan/hapus/"+strconv.Itoa(izinID), url.Values{}); rec.Code != http.StatusOK {
		t.Errorf("cabut izin: status = %d", rec.Code)
	}
	db.Exec("INSERT INTO nilai_ujian (siswa_id, ujian_id, poin_otomatis, poin_essay, poin_maks, nilai, benar, salah, kosong, status_koreksi) VALUES (?, ?, 0, 0, 6, 0, 0, 0, 3, 'selesai')", dua, ujianID)
	if rec := adminPost("/admin/susulan/tambah", url.Values{"ujian_id": {u}, "siswa_id": {strconv.Itoa(dua)}}); rec.Code != http.StatusBadRequest {
		t.Errorf("siswa bernilai: status = %d, want 400", rec.Code)
	}

	// 7. Halaman admin dapat dirender.
	reqP := httptest.NewRequest("GET", "/admin/susulan?ujian_id="+u, nil)
	reqP.AddCookie(adminC)
	recP := httptest.NewRecorder()
	app.ServeHTTP(recP, reqP)
	if recP.Code != http.StatusOK || !strings.Contains(recP.Body.String(), "Siswa Dua") {
		t.Errorf("halaman susulan: status = %d", recP.Code)
	}
}
