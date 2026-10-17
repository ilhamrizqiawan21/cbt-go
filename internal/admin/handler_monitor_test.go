package admin_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/admin"
	"cbt-go/internal/auth"
	"cbt-go/internal/monitor"
	"cbt-go/internal/sesi"
	"cbt-go/internal/testdb"
)

type dummyNotifier struct {
	sentSiswaID int
	sentUjianID int
	sentEvent   string
	sentData    any
}

func (d *dummyNotifier) SendToSiswa(siswaID, ujianID int, event string, data any) bool {
	d.sentSiswaID = siswaID
	d.sentUjianID = ujianID
	d.sentEvent = event
	d.sentData = data
	return true
}

func withSession(t *testing.T, sm *scs.SessionManager, req *http.Request, vals map[string]any) *http.Request {
	t.Helper()
	ctx, err := sm.Load(req.Context(), "")
	if err != nil {
		t.Fatalf("sm load: %v", err)
	}
	for k, v := range vals {
		sm.Put(ctx, k, v)
	}
	return req.WithContext(ctx)
}

// Otorisasi tidak butuh database: middleware menolak sebelum handler dipanggil.
func TestAdminMonitorAPI_Otorisasi(t *testing.T) {
	sm := scs.New()
	mux := http.NewServeMux()
	admin.RegisterAdminMonitorRoutes(mux, nil, sm, nil, nil, auth.RequireAdmin(sm), auth.RequireAdminAPI(sm))
	h := sm.LoadAndSave(mux)

	for _, aksi := range []string{"buka-kunci", "tambah-waktu", "paksa-selesai"} {
		path := "/admin/api/sesi/1/" + aksi

		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s tanpa login: status = %d, want 401", aksi, w.Code)
		}

		req := withSession(t, sm, httptest.NewRequest("POST", path, nil), map[string]any{"peran": "siswa", "siswaId": 99})
		w = httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s sebagai siswa: status = %d, want 403 (JSON, bukan redirect)", aksi, w.Code)
		}
	}
}

type monitorEnv struct {
	h        http.Handler
	db       *sql.DB
	sm       *scs.SessionManager
	notifier *dummyNotifier
	adminID  int
	sesiID   int
	siswaID  int
	ujianID  int
}

// setupMonitorEnv membuat satu siswa terkunci (keluar_paksa) dengan 1 jawaban dan 3 pelanggaran.
func setupMonitorEnv(t *testing.T) *monitorEnv {
	t.Helper()
	db := testdb.Open(t)
	testdb.Reset(t, db)
	ctx := context.Background()

	exec := func(what, query string, args ...any) int {
		t.Helper()
		res, err := db.ExecContext(ctx, query, args...)
		if err != nil {
			t.Fatalf("insert %s: %v", what, err)
		}
		id, _ := res.LastInsertId()
		return int(id)
	}

	adminID := exec("admin", "INSERT INTO admin (username, password) VALUES ('admin_uji', 'hash')")
	guruID := exec("guru", "INSERT INTO guru (nip, nama, username, password) VALUES ('19800101', 'Guru Uji', 'guru_uji', 'hash')")
	kelasID := exec("kelas", "INSERT INTO kelas (nama_kelas) VALUES ('VII-A')")
	mapelID := exec("mapel", "INSERT INTO mata_pelajaran (nama_mapel) VALUES ('Bahasa Arab')")
	siswaID := exec("siswa", "INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('1001', 'Nu''man Test', 'VII-A', 'hash')")
	pengajaranID := exec("pengajaran", "INSERT INTO pengajaran (guru_id, kelas_id, mapel_id) VALUES (?, ?, ?)", guruID, kelasID, mapelID)
	ujianID := exec("ujian", `
		INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai, batas_pelanggaran)
		VALUES (?, 'Ujian Monitor Realtime', 60, NOW() - INTERVAL 1 HOUR, NOW() + INTERVAL 2 HOUR, 3)
	`, pengajaranID)
	soalID := exec("soal", `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
		VALUES (?, 'pg', 'Soal 1', 2, 'a', 'b', 'c', 'd', 'A')
	`, ujianID)
	sesiID := exec("sesi", `
		INSERT INTO sesi_ujian (siswa_id, ujian_id, waktu_mulai, batas_waktu, status, device_type, last_seen, seed)
		VALUES (?, ?, NOW(), NOW() + INTERVAL 30 MINUTE, 'keluar_paksa', 'hp', NOW(), 42)
	`, siswaID, ujianID)
	exec("jawaban", `
		INSERT INTO jawaban_siswa (siswa_id, ujian_id, soal_id, jawaban_dipilih, client_ts)
		VALUES (?, ?, ?, 'A', NOW())
	`, siswaID, ujianID, soalID)
	for i := 0; i < 3; i++ {
		exec("log", `
			INSERT INTO log_kecurangan (siswa_id, ujian_id, jenis_kecurangan, device_type) VALUES (?, ?, 'pindah_tab', 'hp')
		`, siswaID, ujianID)
	}

	sm := scs.New()
	notifier := &dummyNotifier{}
	mux := http.NewServeMux()
	admin.RegisterAdminMonitorRoutes(mux, db, sm, monitor.NewService(db, nil), notifier, auth.RequireAdmin(sm), auth.RequireAdminAPI(sm))

	return &monitorEnv{
		h: sm.LoadAndSave(mux), db: db, sm: sm, notifier: notifier,
		adminID: adminID, sesiID: sesiID, siswaID: siswaID, ujianID: ujianID,
	}
}

func (e *monitorEnv) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Accept", "application/json")
	req = withSession(t, e.sm, req, map[string]any{"peran": "admin", "adminId": e.adminID, "username": "admin_uji"})
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	return w
}

func (e *monitorEnv) jumlahAudit(t *testing.T, aksi string) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow("SELECT COUNT(*) FROM audit_admin WHERE sesi_id = ? AND aksi = ? AND admin_id = ?", e.sesiID, aksi, e.adminID).Scan(&n); err != nil {
		t.Fatalf("query audit: %v", err)
	}
	return n
}

func TestAdminMonitorPage_DB(t *testing.T) {
	e := setupMonitorEnv(t)
	w := e.do(t, "GET", fmt.Sprintf("/admin/monitor/%d", e.ujianID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. Body: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{"Monitor Realtime Ujian", "Ujian Monitor Realtime", "1001"} {
		if !strings.Contains(body, want) {
			t.Errorf("halaman monitor tidak memuat %q", want)
		}
	}
}

func TestAdminBukaKunci_DB(t *testing.T) {
	e := setupMonitorEnv(t)
	ctx := context.Background()

	w := e.do(t, "POST", fmt.Sprintf("/admin/api/sesi/%d/buka-kunci", e.sesiID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. Body: %s", w.Code, w.Body.String())
	}

	var status string
	if err := e.db.QueryRow("SELECT status FROM sesi_ujian WHERE id = ?", e.sesiID).Scan(&status); err != nil {
		t.Fatalf("query sesi: %v", err)
	}
	if status != "sedang_ujian" {
		t.Errorf("status = %q, want sedang_ujian", status)
	}
	if n, err := sesi.HitungPelanggaran(ctx, e.db, e.siswaID, e.ujianID); err != nil || n != 0 {
		t.Errorf("pelanggaran aktif = %d (err %v), want 0 setelah buka kunci", n, err)
	}
	var jawaban int
	_ = e.db.QueryRow("SELECT COUNT(*) FROM jawaban_siswa WHERE siswa_id = ? AND ujian_id = ?", e.siswaID, e.ujianID).Scan(&jawaban)
	if jawaban != 1 {
		t.Errorf("jawaban = %d, want 1 (buka kunci tidak boleh menghapus jawaban)", jawaban)
	}
	if n := e.jumlahAudit(t, "buka-kunci"); n != 1 {
		t.Errorf("audit buka-kunci = %d, want 1", n)
	}

	// Sesi yang tidak terkunci ditolak dan tidak menambah audit.
	w = e.do(t, "POST", fmt.Sprintf("/admin/api/sesi/%d/buka-kunci", e.sesiID), "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("buka kunci kedua: status = %d, want 400", w.Code)
	}
	if n := e.jumlahAudit(t, "buka-kunci"); n != 1 {
		t.Errorf("audit buka-kunci setelah penolakan = %d, want 1", n)
	}

	w = e.do(t, "POST", "/admin/api/sesi/999999/buka-kunci", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("sesi tidak ada: status = %d, want 404", w.Code)
	}
}

func TestAdminTambahWaktu_DB(t *testing.T) {
	e := setupMonitorEnv(t)
	path := fmt.Sprintf("/admin/api/sesi/%d/tambah-waktu", e.sesiID)

	var sebelum time.Time
	_ = e.db.QueryRow("SELECT batas_waktu FROM sesi_ujian WHERE id = ?", e.sesiID).Scan(&sebelum)

	w := e.do(t, "POST", path, `{"menit":15}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. Body: %s", w.Code, w.Body.String())
	}
	var sesudah time.Time
	var tambahan int
	_ = e.db.QueryRow("SELECT batas_waktu, tambahan_menit FROM sesi_ujian WHERE id = ?", e.sesiID).Scan(&sesudah, &tambahan)
	if got := sesudah.Sub(sebelum); got != 15*time.Minute {
		t.Errorf("batas_waktu maju %v, want 15m", got)
	}
	if tambahan != 15 {
		t.Errorf("tambahan_menit = %d, want 15", tambahan)
	}
	if e.notifier.sentEvent != "tambah-waktu" || e.notifier.sentSiswaID != e.siswaID {
		t.Errorf("notifikasi siswa salah: %+v", e.notifier)
	}
	if data, _ := e.notifier.sentData.(map[string]any); data["pesan"] == nil {
		t.Errorf("event tambah-waktu harus memuat 'pesan' untuk ujian.js: %+v", e.notifier.sentData)
	}
	if n := e.jumlahAudit(t, "tambah-waktu"); n != 1 {
		t.Errorf("audit tambah-waktu = %d, want 1", n)
	}

	for _, body := range []string{`{"menit":-5}`, `{"menit":500}`, `{bukan json`} {
		if w := e.do(t, "POST", path, body); w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, w.Code)
		}
	}

	_, _ = e.db.Exec("UPDATE sesi_ujian SET status = 'selesai' WHERE id = ?", e.sesiID)
	if w := e.do(t, "POST", path, `{"menit":5}`); w.Code != http.StatusBadRequest {
		t.Errorf("sesi selesai: status = %d, want 400", w.Code)
	}
}

func TestAdminPaksaSelesai_DB(t *testing.T) {
	e := setupMonitorEnv(t)
	w := e.do(t, "POST", fmt.Sprintf("/admin/api/sesi/%d/paksa-selesai", e.sesiID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. Body: %s", w.Code, w.Body.String())
	}

	var status string
	_ = e.db.QueryRow("SELECT status FROM sesi_ujian WHERE id = ?", e.sesiID).Scan(&status)
	if status != "selesai" {
		t.Errorf("status = %q, want selesai", status)
	}
	var nilai int
	if err := e.db.QueryRow("SELECT nilai FROM nilai_ujian WHERE siswa_id = ? AND ujian_id = ?", e.siswaID, e.ujianID).Scan(&nilai); err != nil {
		t.Fatalf("nilai_ujian tidak tersimpan: %v", err)
	}
	if nilai != 100 {
		t.Errorf("nilai = %d, want 100 (1 soal PG benar)", nilai)
	}
	if e.notifier.sentEvent != "paksa-submit" {
		t.Errorf("event = %q, want paksa-submit (event yang ditangani ujian.js)", e.notifier.sentEvent)
	}
	if n := e.jumlahAudit(t, "paksa-selesai"); n != 1 {
		t.Errorf("audit paksa-selesai = %d, want 1", n)
	}
}
