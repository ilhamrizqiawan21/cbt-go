package web_test

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"cbt-go/internal/admin"
	"cbt-go/internal/excel"
	"cbt-go/internal/guru"
	"cbt-go/internal/monitor"
	"cbt-go/internal/soal"
	"cbt-go/web"
)

func strPtr(s string) *string { return &s }
func intPtr(n int) *int       { return &n }

// Handler admin dulu membuang error web.Render, sehingga template yang gagal dieksekusi
// (mis. membandingkan int dengan float) hanya menghasilkan halaman kosong. Tes ini
// merender setiap halaman admin dengan data representatif agar kesalahan seperti itu
// tertangkap saat `go test`, bukan saat hari-H.
// Catatan: html/template mengapit nilai di konteks JS dengan spasi (mis. `=  3 ;`).
func TestAdminTemplates_Render(t *testing.T) {
	now := time.Date(2026, 10, 17, 8, 0, 0, 0, time.Local)
	sesiID := 7
	nama := "Nu'man <b>Uji</b>" // apostrof & tag: harus ter-escape, bukan merusak HTML/JS
	base := func(menu string, extra map[string]any) map[string]any {
		m := map[string]any{"Username": "admin", "ActiveMenu": menu, "Error": "galat uji", "Success": "sukses uji"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	guruBase := func(menu string, extra map[string]any) map[string]any {
		m := base(menu, extra)
		m["NamaGuru"] = "Ustadz Uji"
		return m
	}
	ujian := []admin.Ujian{{ID: 3, PengajaranID: 1, NamaUjian: "PTS Fiqih", Durasi: 60, TanggalMulai: now, TanggalSelesai: now.Add(time.Hour),
		AcakSoal: true, BatasPelanggaran: 3, TokenUjian: strPtr("ABC"), NamaGuru: "Guru", NamaMapel: "Fiqih", NamaKelas: "VII-A"}}
	kelas := []admin.Kelas{{ID: 1, NamaKelas: "VII-A"}}

	cases := []struct {
		name string
		tmpl string
		data map[string]any
		want []string
	}{
		{"dashboard", "admin_dashboard.gohtml", base("dashboard", map[string]any{
			"Ringkasan": &admin.RingkasanDashboard{Siswa: 410, UjianAktif: 1},
			"UjianList": []admin.UjianDashboard{{ID: 3, NamaUjian: "PTS Fiqih", TanggalMulai: now, TanggalSelesai: now, Berlangsung: true}},
		}), []string{"410", "Berlangsung", "/admin/monitor/3"}},
		{"ujian", "admin_ujian.gohtml", base("ujian", map[string]any{
			"UjianList": ujian, "PengajaranList": []admin.Pengajaran{{ID: 1, NamaGuru: "Guru", NamaMapel: "Fiqih", NamaKelas: "VII-A"}},
		}), []string{"PTS Fiqih", `data-mulai="2026-10-17T08:00"`, "modalUjian"}},
		{"siswa", "admin_siswa.gohtml", base("siswa", map[string]any{
			"SiswaList": []admin.Siswa{{ID: 1, NIS: "1001", Nama: nama, Kelas: "VII-A"}, {ID: 2, NIS: "1002", Nama: "Tanpa Kelas"}},
			"KelasList": kelas, "Filter": "VII-A",
		}), []string{"1001", "modalSiswa", "Nu&#39;man &lt;b&gt;Uji&lt;/b&gt;"}},
		{"soal", "admin_soal.gohtml", base("soal", map[string]any{
			"UjianList": ujian, "UjianID": 3, "SoalBase": "/admin/soal", "SoalPage": "/admin/soal",
			"SoalList": []soal.SoalKelola{
				{ID: 1, TipeSoal: "pg", TeksSoal: "1+1?", Poin: 2, PilihanA: strPtr("1"), PilihanB: strPtr("2"), JawabanBenar: strPtr("B")},
				{ID: 2, TipeSoal: "pg", TeksSoal: "Tanpa kunci", Poin: 2}, // pointer nil
				{ID: 3, TipeSoal: "menjodohkan", TeksSoal: "Jodohkan", Poin: 2, OpsiTambahan: strPtr(`{"pasangan":[{"kiri":"a","kanan":"b"}]}`)},
				{ID: 4, TipeSoal: "essay", TeksSoal: "Jelaskan", Poin: 4},
			},
		}), []string{"Pilihan ganda", "Menjodohkan", "Kunci belum diisi", `class="opsi-benar"`, "pilihan_d", `action="/admin/soal/hapus/1"`, `data-base="/admin/soal"`}},
		{"soal tanpa ujian", "admin_soal.gohtml", base("soal", map[string]any{"UjianList": []admin.Ujian{}, "UjianID": 0, "SoalBase": "/admin/soal", "SoalPage": "/admin/soal"}), []string{"Belum ada ujian"}},
		{"hasil", "admin_hasil.gohtml", base("hasil", map[string]any{
			"UjianList": ujian, "UjianID": 3, "KelasList": kelas,
			"HasilList": []admin.HasilUjianRow{
				{SiswaID: 9, NIS: "1001", NamaSiswa: nama, Kelas: "VII-A", NilaiAkhir: 85, PoinOtomatis: 10, PoinEssay: 7, PoinMaks: 20, StatusKoreksi: "selesai", SelesaiPada: now},
				{SiswaID: 10, NIS: "1002", NamaSiswa: "B", StatusKoreksi: "menunggu_essay"},
			},
		}), []string{"85", "Menunggu essay", `name="siswa_id" value="9"`, "/admin/hasil?ujian_id=3"}},
		{"log", "admin_log.gohtml", base("log", map[string]any{
			"UjianList": ujian, "UjianID": 3, "Filter": "pindah_tab",
			"LogList": []admin.LogKecuranganRow{{SiswaID: 9, UjianID: 3, NIS: "1001", NamaSiswa: nama, JenisKecurangan: "pindah_tab", DeviceType: "hp", Timestamp: now}},
		}), []string{"Pindah tab", "bi-phone", `name="siswa_id" value="9"`}},
		{"monitor dengan snapshot", "admin_monitor.gohtml", base("monitor", map[string]any{
			"UjianList": ujian, "UjianID": 3,
			"Snapshot": &monitor.SnapshotData{UjianID: 3, Siswa: []monitor.SiswaMonitor{{SiswaID: 9, NIS: "1001", Nama: nama, SesiID: &sesiID, Status: monitor.StatusTerkunci}}},
		}), []string{"const UJIAN_ID =  3 ;", `"nis":"1001"`, "modalTambahWaktu"}},
		{"monitor tanpa snapshot", "admin_monitor.gohtml", base("monitor", map[string]any{"UjianList": ujian, "UjianID": 3, "Snapshot": (*monitor.SnapshotData)(nil)}),
			[]string{"let snapshot =  null ;"}},
		{"guru", "admin_guru.gohtml", base("guru", map[string]any{"GuruList": []admin.Guru{{ID: 1, Nama: nama, Username: "g1"}}}), []string{"btn-edit-guru"}},
		{"kelas", "admin_kelas.gohtml", base("kelas", map[string]any{"KelasList": kelas}), []string{"VII-A", "/admin/siswa?kelas=VII-A"}},
		{"mapel", "admin_mapel.gohtml", base("mapel", map[string]any{"MapelList": []admin.MataPelajaran{{ID: 1, NamaMapel: "Fiqih"}}}), []string{"Fiqih"}},
		{"pengajaran kosong", "admin_pengajaran.gohtml", base("pengajaran", map[string]any{}), []string{"data guru", "disabled"}},
		// ---------- Halaman guru ----------
		{"guru dashboard", "guru_dashboard.gohtml", guruBase("dashboard", map[string]any{
			"TotalMenunggu": 2,
			"UjianList":     []guru.UjianRingkasan{{ID: 3, NamaUjian: "PTS Fiqih", TanggalMulai: now, TanggalSelesai: now, Berakhir: true, JumlahSoal: 5, JumlahEssay: 1, Selesai: 10, MenungguEssay: 2}},
		}), []string{"Ustadz Uji", "menunggu koreksi essay", "/guru/essay/3", "Berakhir"}},
		{"guru soal", "guru_soal.gohtml", guruBase("soal", map[string]any{
			"UjianList": []soal.UjianGuruItem{{ID: 3, NamaUjian: "PTS Fiqih", NamaKelas: "VII-A"}}, "UjianID": 3,
			"SoalBase": "/guru/soal", "SoalPage": "/guru/kelola-soal",
			"SoalList": []soal.SoalKelola{{ID: 1, TipeSoal: "pg", TeksSoal: "1+1?", Poin: 2, JawabanBenar: strPtr("A")}},
		}), []string{`action="/guru/soal/hapus/1"`, `data-base="/guru/soal"`, `action="/guru/kelola-soal"`, "CBT Guru"}},
		{"guru soal tanpa ujian", "guru_soal.gohtml", guruBase("soal", map[string]any{"UjianList": []soal.UjianGuruItem{}, "UjianID": 0, "SoalBase": "/guru/soal", "SoalPage": "/guru/kelola-soal"}),
			[]string{"Hubungi admin"}},
		{"guru essay list", "guru_essay_list.gohtml", guruBase("essay", map[string]any{
			"UjianList": []guru.UjianRingkasan{{ID: 3, NamaUjian: "PTS", JumlahEssay: 2, Selesai: 4, MenungguEssay: 1}, {ID: 4, NamaUjian: "Tanpa essay"}},
		}), []string{"width: 75%", "Nilai (1)", "Tidak ada soal essay"}},
		{"guru essay detail", "guru_essay.gohtml", guruBase("essay", map[string]any{
			"Ujian": ujian[0], "UjianID": 3, "JumlahSiswa": 1, "BelumDinilai": 1,
			"SoalList":    []guru.SoalEssayRingkas{{ID: 9, Nomor: 1, TeksSoal: "Jelaskan", PoinMaks: 4}},
			"JawabanList": []excel.EssayExportRow{{SiswaID: 5, NIS: "1001", NamaSiswa: nama, SoalID: 9, PoinMaks: 4}, {SiswaID: 6, NIS: "1002", NamaSiswa: "B", SoalID: 9, PoinMaks: 4, Skor: intPtr(0)}},
		}), []string{`data-siswa="5"`, `max="4"`, `data-status="belum"`, `data-status="sudah"`, `value="0"`, "/guru/essay/3/import"}},
		{"guru hasil", "guru_hasil.gohtml", guruBase("hasil", map[string]any{
			"UjianList": []soal.UjianGuruItem{{ID: 3, NamaUjian: "PTS"}}, "UjianID": 3,
			"HasilList": []admin.HasilUjianRow{{NIS: "1001", NamaSiswa: nama, NilaiAkhir: 70, StatusKoreksi: "menunggu_essay"}},
		}), []string{"Menunggu essay", "/guru/hasil-siswa/export?ujian_id=3"}},
		{"guru log", "guru_log.gohtml", guruBase("log", map[string]any{
			"UjianList": []soal.UjianGuruItem{{ID: 3, NamaUjian: "PTS"}}, "UjianID": 0, "Filter": "",
			"LogList": []admin.LogKecuranganRow{{NIS: "1001", NamaSiswa: nama, JenisKecurangan: "copy_paste", DeviceType: "laptop", Timestamp: now}},
		}), []string{"Copy-paste", "bi-laptop", "pilih ujian untuk ekspor"}},
		{"cetak", "admin_hasil_cetak.gohtml", map[string]any{
			"Ujian": ujian[0], "TanggalCetak": "17 October 2026",
			"HasilList": []admin.HasilUjianRow{{NIS: "1001", NamaSiswa: "A", StatusKoreksi: "selesai"}},
		}, []string{"Selesai"}},
		{"ujian_siswa", "ujian.gohtml", map[string]any{
			"UjianID": 3, "NamaUjian": "PAS Bahasa Arab", "NamaMapel": "Bahasa Arab", "DeviceType": "laptop",
			"Siswa": admin.Siswa{ID: 1, NIS: "1001", Nama: nama, Kelas: "VII-A"},
		}, []string{"PAS Bahasa Arab", "Navigator Soal", "btn-selesai", "fs-indicator", "timer"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			if err := web.Render(w, tc.tmpl, tc.data); err != nil {
				t.Fatalf("render %s: %v", tc.tmpl, err)
			}
			body := w.Body.String()
			for _, want := range tc.want {
				if !strings.Contains(body, want) {
					t.Errorf("hasil render %s tidak memuat %q", tc.tmpl, want)
				}
			}
			if strings.Contains(body, "<b>Uji</b>") {
				t.Errorf("nama siswa tidak di-escape di %s", tc.tmpl)
			}
		})
	}
}

// Padanan halaman_monitor.test.js (render): 410 siswa ikut terkirim ke halaman monitor.
func TestAdminMonitor_410Baris(t *testing.T) {
	siswa := make([]monitor.SiswaMonitor, 410)
	for i := range siswa {
		siswa[i] = monitor.SiswaMonitor{SiswaID: i + 1, NIS: "N" + strconv.Itoa(i+1), Nama: "Siswa", Status: monitor.StatusBelumMasuk}
	}
	w := httptest.NewRecorder()
	err := web.Render(w, "admin_monitor.gohtml", map[string]any{
		"ActiveMenu": "monitor", "UjianList": []admin.Ujian{{ID: 1, NamaUjian: "U"}}, "UjianID": 1,
		"Snapshot": &monitor.SnapshotData{UjianID: 1, Siswa: siswa},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if n := strings.Count(w.Body.String(), `"nis":`); n != 410 {
		t.Errorf("halaman memuat %d siswa, want 410", n)
	}
}
