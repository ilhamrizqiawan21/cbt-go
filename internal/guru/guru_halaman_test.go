package guru_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/auth"
	"cbt-go/internal/excel"
	"cbt-go/internal/guru"
	"cbt-go/internal/testdb"
)

type guruEnv struct {
	db                       *sql.DB
	guru1, guru2             int
	ujian, ujianLain         int
	soalEssay                int
	siswaSelesai, siswaUjian int
	siswaBelum               int
}

// setupGuruEnv: guru1 mengampu ujian (1 PG + 1 essay) di VII-A; guru2 mengampu ujianLain.
// Tiga siswa VII-A: selesai, sedang ujian, belum masuk.
func setupGuruEnv(t *testing.T) *guruEnv {
	t.Helper()
	db := testdb.Open(t)
	testdb.Reset(t, db)
	ctx := context.Background()
	exec := func(q string, args ...any) int {
		t.Helper()
		res, err := db.ExecContext(ctx, q, args...)
		if err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
		id, _ := res.LastInsertId()
		return int(id)
	}

	e := &guruEnv{db: db}
	e.guru1 = exec("INSERT INTO guru (nip, nama, username, password) VALUES ('1', 'Guru Satu', 'g1', 'x')")
	e.guru2 = exec("INSERT INTO guru (nip, nama, username, password) VALUES ('2', 'Guru Dua', 'g2', 'x')")
	kelas := exec("INSERT INTO kelas (nama_kelas) VALUES ('VII-A')")
	mapel := exec("INSERT INTO mata_pelajaran (nama_mapel) VALUES ('Fiqih')")
	p1 := exec("INSERT INTO pengajaran (guru_id, mapel_id, kelas_id) VALUES (?, ?, ?)", e.guru1, mapel, kelas)
	p2 := exec("INSERT INTO pengajaran (guru_id, mapel_id, kelas_id) VALUES (?, ?, ?)", e.guru2, mapel, kelas)
	e.ujian = exec("INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai) VALUES (?, 'UTS Fiqih', 60, NOW() - INTERVAL 2 HOUR, NOW() + INTERVAL 1 HOUR)", p1)
	e.ujianLain = exec("INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai) VALUES (?, 'Milik Guru Dua', 60, NOW(), NOW() + INTERVAL 1 HOUR)", p2)
	exec("INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, jawaban_benar) VALUES (?, 'pg', 'PG', 2, 'a', 'b', 'A')", e.ujian)
	e.soalEssay = exec("INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin) VALUES (?, 'essay', 'Jelaskan', 4)", e.ujian)
	e.siswaSelesai = exec("INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('S1', 'Selesai', 'VII-A', 'x')")
	e.siswaUjian = exec("INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('S2', 'Sedang', 'VII-A', 'x')")
	e.siswaBelum = exec("INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('S3', 'Belum', 'VII-A', 'x')")
	exec("INSERT INTO sesi_ujian (siswa_id, ujian_id, waktu_mulai, batas_waktu, status) VALUES (?, ?, NOW(), NOW(), 'selesai')", e.siswaSelesai, e.ujian)
	exec("INSERT INTO sesi_ujian (siswa_id, ujian_id, waktu_mulai, batas_waktu, status) VALUES (?, ?, NOW(), NOW() + INTERVAL 1 HOUR, 'sedang_ujian')", e.siswaUjian, e.ujian)
	exec("INSERT INTO jawaban_siswa (siswa_id, ujian_id, soal_id, jawaban_dipilih) SELECT ?, ?, id, 'A' FROM soal WHERE ujian_id = ? AND tipe_soal = 'pg'", e.siswaSelesai, e.ujian, e.ujian)
	return e
}

func (e *guruEnv) adaNilai(t *testing.T, siswaID int) bool {
	t.Helper()
	var n int
	if err := e.db.QueryRow("SELECT COUNT(*) FROM nilai_ujian WHERE siswa_id = ? AND ujian_id = ?", siswaID, e.ujian).Scan(&n); err != nil {
		t.Fatalf("hitung nilai_ujian: %v", err)
	}
	return n > 0
}

func TestEssay_HanyaSesiSelesai_DB(t *testing.T) {
	e := setupGuruEnv(t)
	ctx := context.Background()

	// Menilai siswa yang sedang/belum ujian ditolak dan TIDAK membuat nilai_ujian
	// (nilai_ujian membuat login siswa ditolak "sudah mengerjakan").
	for _, siswa := range []int{e.siswaUjian, e.siswaBelum} {
		_, err := guru.SimpanNilaiEssay(ctx, e.db, e.guru1, e.ujian, siswa, e.soalEssay, 3)
		if !errors.Is(err, guru.ErrSesiBelumSelesai) {
			t.Errorf("siswa %d: err = %v, want ErrSesiBelumSelesai", siswa, err)
		}
		if e.adaNilai(t, siswa) {
			t.Errorf("siswa %d: nilai_ujian terbentuk padahal belum selesai", siswa)
		}
	}

	hasil, err := guru.SimpanNilaiEssay(ctx, e.db, e.guru1, e.ujian, e.siswaSelesai, e.soalEssay, 4)
	if err != nil {
		t.Fatalf("nilai siswa selesai: %v", err)
	}
	if hasil.Nilai != 100 || string(hasil.StatusKoreksi) != "selesai" {
		t.Errorf("hasil = nilai %d status %s, want 100 selesai (PG 2 + essay 4 dari 6)", hasil.Nilai, hasil.StatusKoreksi)
	}

	// Lembar koreksi hanya memuat siswa yang selesai.
	rows, err := guru.GetJawabanEssayUjian(ctx, e.db, e.ujian)
	if err != nil {
		t.Fatalf("GetJawabanEssayUjian: %v", err)
	}
	if len(rows) != 1 || rows[0].SiswaID != e.siswaSelesai || rows[0].Skor == nil || *rows[0].Skor != 4 {
		t.Errorf("lembar koreksi = %+v, want 1 baris siswa selesai dengan skor 4", rows)
	}
}

func TestEssay_ImporMelaporkanBarisGagal_DB(t *testing.T) {
	e := setupGuruEnv(t)
	items := []excel.EssayImportItem{
		{Baris: 6, SiswaID: e.siswaSelesai, SoalID: e.soalEssay, Skor: 3},
		{Baris: 7, SiswaID: e.siswaUjian, SoalID: e.soalEssay, Skor: 3},
		{Baris: 8, SiswaID: e.siswaSelesai, SoalID: e.soalEssay, Skor: 9},
	}
	sukses, gagal, err := guru.BatchSimpanNilaiEssay(context.Background(), e.db, e.guru1, e.ujian, items)
	if err != nil {
		t.Fatalf("BatchSimpanNilaiEssay: %v", err)
	}
	if sukses != 1 || len(gagal) != 2 {
		t.Fatalf("sukses=%d gagal=%+v, want 1 sukses & 2 gagal dilaporkan", sukses, gagal)
	}
	if gagal[0].Baris != 7 || !strings.Contains(gagal[0].Pesan, "belum menyelesaikan") {
		t.Errorf("laporan baris 7 = %+v", gagal[0])
	}
}

func TestHalamanGuru_DB(t *testing.T) {
	e := setupGuruEnv(t)
	sm := scs.New()
	mux := http.NewServeMux()
	guru.RegisterGuruRoutes(mux, e.db, sm, auth.RequireGuru(sm), auth.RequireGuruAPI(sm))
	guru.RegisterGuruEssayRoutes(mux, e.db, sm, auth.RequireGuru(sm))
	guru.RegisterGuruHasilRoutes(mux, e.db, sm, auth.RequireGuru(sm))
	guru.RegisterGuruLogRoutes(mux, e.db, sm, auth.RequireGuru(sm))
	mux.Handle("GET /guru/dashboard", auth.RequireGuru(sm)(guru.GuruDashboardHandler(e.db, sm)))
	h := sm.LoadAndSave(mux)

	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("GET", path, nil)
		ctx, _ := sm.Load(req.Context(), "")
		sm.Put(ctx, "peran", "guru")
		sm.Put(ctx, "guruId", e.guru1)
		sm.Put(ctx, "namaGuru", "Guru Satu")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req.WithContext(ctx))
		return w
	}

	cases := []struct {
		path string
		code int
		want string
	}{
		{"/guru/dashboard", 200, "UTS Fiqih"},
		{"/guru/kelola-soal", 200, "Jelaskan"}, // default ke ujian guru1
		{fmt.Sprintf("/guru/kelola-soal?ujian_id=%d", e.ujianLain), 403, ""},
		{"/guru/essay", 200, "UTS Fiqih"},
		{fmt.Sprintf("/guru/essay/%d", e.ujian), 200, "Selesai"},
		{fmt.Sprintf("/guru/essay/%d", e.ujianLain), 403, ""},
		{"/guru/hasil-siswa", 200, "UTS Fiqih"},
		{fmt.Sprintf("/guru/hasil-siswa?ujian_id=%d", e.ujianLain), 403, ""},
		{"/guru/log-kecurangan", 200, "Log Pelanggaran"},
		{"/guru/kelola-soal/tambah-batch", 302, ""},
	}
	for _, c := range cases {
		w := get(c.path)
		if w.Code != c.code {
			t.Errorf("GET %s: status %d, want %d", c.path, w.Code, c.code)
			continue
		}
		if c.want != "" && !strings.Contains(w.Body.String(), c.want) {
			t.Errorf("GET %s: body tidak memuat %q", c.path, c.want)
		}
		if strings.Contains(w.Body.String(), "Milik Guru Dua") {
			t.Errorf("GET %s: menampilkan ujian guru lain", c.path)
		}
	}
	// Siswa yang belum selesai tidak muncul di lembar koreksi.
	if body := get(fmt.Sprintf("/guru/essay/%d", e.ujian)).Body.String(); strings.Contains(body, "Sedang") || strings.Contains(body, ">S3<") {
		t.Errorf("lembar koreksi memuat siswa yang belum selesai")
	}
}
