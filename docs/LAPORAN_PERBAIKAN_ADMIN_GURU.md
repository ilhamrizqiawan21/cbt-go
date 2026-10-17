# Laporan Perbaikan Menu Admin & Guru

| | |
|---|---|
| Tanggal | 17 Oktober 2026 |
| Pelaksana | Claude (atas permintaan user; biasanya peran Claude = perancang/reviewer, G-012) |
| Commit | `e9657d2` "Complete the teacher menus" (di-commit user; ikut memuat pekerjaan Gemini T2.5/T2.6 yang belum di-commit saat pekerjaan ini dimulai) |
| Dokumen terkait | [DECISION](DECISION.md) G-017–G-021 · [TODO](TODO.md) (laporan per putaran) · [PORTING_MAP](PORTING_MAP.md) §3–§4 · [PANDUAN_HARI_H](PANDUAN_HARI_H.md) |

## 1. Ringkasan

Pekerjaan berlangsung dalam tiga putaran: (1) perbaikan & pelengkapan menu admin, (2) halaman guru, (3) keputusan yang tertunda, yang diambil Claude atas delegasi user. Hasilnya:

- **Temuan terpenting:** laporan sebelumnya menyatakan semua tes "PASS", padahal **seluruh tes yang butuh database selalu dilewati** (`internal/testdb` mencari `.env` di direktori paket, bukan di root repo). Setelah ini diperbaiki, beberapa bug BLOCKER langsung terlihat. Yang paling serius: monitor admin gagal total di database nyata, dan halaman Hasil kosong begitu ada nilai.
- Menu admin dan guru kini lengkap dan dapat dipakai end-to-end. Alur ini sudah diuji di browser terhadap server sungguhan: CRUD master data, ujian, bank soal (PG/menjodohkan/essay), monitor realtime beserta aksinya, hasil, log & reset, koreksi essay.
- Lima keputusan desain dicatat sebagai **G-017–G-021**. Yang terpenting: siswa yang terkunci kini benar-benar bisa melanjutkan setelah dibuka kuncinya (PRD FR-12).
- Gerbang kualitas: `make check` dan `make test-race` lulus, dengan **18 paket** dan **274 tes** dijalankan terhadap DB uji yang aktif.

## 2. Temuan dan perbaikan

Tingkat: **BLOCKER** = fitur tidak bisa dipakai / data salah; **MAJOR** = salah perilaku berdampak nyata; **MINOR** = kualitas/keamanan ringan.

### 2.1 Infrastruktur tes
| Tingkat | Temuan | Perbaikan |
|---|---|---|
| BLOCKER | `testdb.Open` memanggil `godotenv.Load()` dari direktori paket, sehingga `.env` tidak ditemukan, kredensial default ditolak, dan semua tes DB di-`Skip`. | `.env` dicari di root modul (`cariEnvRoot`). Tes DB kini benar-benar berjalan pada `make test`. |
| MAJOR | Tes monitor admin (`handler_monitor_test.go`) ditulis terhadap skema fiktif (`mulai_pada`, `ip_address`, `jawaban_siswa.sesi_id`, `pilihan_ganda`). Mock menyembunyikan bug nyata. | Ditulis ulang terhadap skema `migrations/` dengan DB nyata. |
| MAJOR | `tests/ui/monitor.ui.js` dan `arab_hp.ui.js` memakai `page.setContent(...)` dengan HTML statis, sehingga tidak pernah menguji server Go. | Ditulis ulang agar login dan membuka halaman server sungguhan. |

### 2.2 Admin
| Tingkat | Temuan | Perbaikan |
|---|---|---|
| BLOCKER | `monitor.Snapshot` memakai kolom yang tidak ada (`su.jumlah_pelanggaran`, `js.sesi_id`); buka-kunci meng-UPDATE `jumlah_pelanggaran`. | Query sesuai skema: pelanggaran dihitung dari `log_kecurangan` setelah penanda `buka_kunci` terakhir (aturan yang sama dengan `sesi.HitungPelanggaran`), tetap satu round-trip. Aksi buka kunci/tambah waktu dipindah ke `internal/admin/aksi_sesi.go` dalam transaksi (`FOR UPDATE`, `defer Rollback`) dan dicatat di audit. |
| BLOCKER | Halaman Hasil gagal render bila ada nilai (`printf "%.1f"` dan `ge … 75.0` pada `int`). Error render dibuang (`_ = web.Render`), sehingga yang tampil halaman kosong. Status dicek `final`, padahal di DB `selesai`. | Template diperbaiki. Helper `render` mencatat error dan menampilkan halaman 500. |
| BLOCKER | Form Tambah Soal mengirim `pilihan_ganda` (selalu ditolak) dengan poin default 5. Belum ada editor menjodohkan maupun edit soal. | Editor soal lengkap (PG + kunci, menjodohkan pasangan/pengecoh, essay, edit), poin default 2/2/4 (D-004). |
| MAJOR | Tombol Tambah Ujian/Siswa membuka modal yang tidak ada; tidak ada fitur edit ujian/siswa; reset siswa harus mengetik ID. | Modal tambah/edit; reset per baris di Hasil, Log, dan Monitor. |
| MAJOR | Event `ujian-selesai`/`kunci-terbuka` tidak dikenali `ujian.js`; tambah waktu tidak menambah `tambahan_menit`. | Memakai event `paksa-submit` dan `tambah-waktu` (+`pesan`); `tambahan_menit` ikut bertambah. |
| MAJOR | Mengganti nama kelas memutus `siswa.kelas` (yang menyimpan **nama** kelas, G-016). | Diperbarui dalam satu transaksi (G-020). |
| MAJOR | `siswa.kelas` yang NULL membuat seluruh daftar siswa/hasil/log gagal dimuat. | `COALESCE` pada query. |
| MINOR | Detail SQL bocor ke klien; hash PIN ikut terkirim di JSON daftar siswa; perangkat dicek `mobile/desktop` (data sebenarnya `hp/laptop`); `innerHTML` memuat nama siswa (berisiko XSS dan rusak bila nama mengandung apostrof). | `serverError` (detail hanya di log); hash tidak di-SELECT; render DOM memakai `textContent`. |

Fitur baru di sisi admin: halaman **Dashboard** (paritas dengan Node `GET /admin/dashboard`), monitor dengan sisa waktu, penyegaran berkala 20 detik di samping WebSocket, dan notifikasi toast.

### 2.3 Guru
| Tingkat | Temuan | Perbaikan |
|---|---|---|
| MAJOR | Menilai essay siswa yang belum atau sedang ujian membuat baris `nilai_ujian`, padahal login siswa menolak siapa pun yang sudah punya nilai ("Anda sudah mengerjakan ujian ini"). | Penilaian hanya untuk sesi `selesai`; lembar koreksi hanya memuat siswa tersebut (G-021). |
| MAJOR | `BatchSimpanNilaiEssay` membuang daftar baris gagal, sehingga impor Excel selalu tampak sukses. | Baris gagal dilaporkan beserta alasannya, tanpa membocorkan detail DB. |
| MAJOR | Template guru masih kerangka: Kelola Soal hanya mendaftar ujian, halaman essay tanpa input skor, `/guru/essay` kosong, tanpa navbar/logout. | Dashboard guru, Kelola Soal (memakai editor yang sama dengan admin lewat partial `soal_bank.gohtml`), lembar koreksi dengan input skor per baris (simpan tanpa reload, Enter pindah ke baris berikutnya, filter), Hasil, Log. |
| MINOR | Error ditelan atau bocor; redirect setelah aksi soal selalu ke daftar umum. | Pola `render`/`serverError`/`forbidden`; redirect kembali ke ujian yang sedang dibuka. |

### 2.4 Jalur ujian siswa (dampak dari keputusan G-017)
| Tingkat | Temuan | Perbaikan |
|---|---|---|
| MAJOR | Event WebSocket `selesai-ujian` langsung mengubah status menjadi `selesai` **tanpa menghitung nilai**. Bila `/api/selesai-ujian` gagal, siswa tidak pernah dinilai, karena job penutupan hanya memproses `sedang_ujian`. | Memakai `sesi.FinalizeOlehSiswa` (menghitung nilai, idempoten). |
| MAJOR | Saat terkunci, klien langsung mengumpulkan ujian dan menghapus antrean jawaban lokal, sehingga "Buka kunci" (FR-12) tidak ada gunanya. | Layar terkunci tanpa mengumpulkan; antrean dipertahankan; siswa login ulang setelah kunci dibuka (G-017). |

## 3. Keputusan (diputuskan Claude atas delegasi user)

Rincian lengkap ada di [DECISION](DECISION.md). Semua keputusan ini dapat ditinjau ulang oleh user.

| ID | Keputusan | Alasan singkat |
|---|---|---|
| G-017 | Sesi terkunci dapat dilanjutkan; siswa tidak bisa memfinalisasi sesi terkunci; job menilai sesi terkunci yang kedaluwarsa. | PRD FR-12, PANDUAN_HARI_H |
| G-018 | Validasi soal di server diperketat (`soal.ValidasiDanNormalisasi`). | Soal cacat sebelumnya membuat semua siswa dinilai salah tanpa peringatan |
| G-019 | Bootstrap 5.3.3 dan Icons 1.11.3 disajikan lokal; CSP tanpa jsDelivr. | Tampilan tidak boleh rusak saat internet putus (D-001, G-015) |
| G-020 | Ganti nama kelas ikut memperbarui `siswa.kelas`. | Konsekuensi G-016 |
| G-021 | Essay dinilai hanya setelah sesi selesai. | Mencegah siswa terblokir dari login |

Perbedaan perilaku dengan Node dicatat di PORTING_MAP §4. Repo Node tidak tersedia di mesin ini, sehingga perilaku Node untuk butir-butir tersebut **tidak dapat diverifikasi**.

## 4. Verifikasi

| Pemeriksaan | Hasil |
|---|---|
| `make check` (gofmt, vet, test) | PASS: 18 paket, 274 tes, DB uji aktif |
| `make test-race` | PASS, tanpa data race |
| Tes baru utama | `internal/admin/handler_monitor_test.go`, `internal/guru/guru_halaman_test.go`, `internal/soal/validasi_test.go` (13 kasus), `TestSesiTerkunci_FinalisasiDanPenutupan`, `TestSnapshot_410Siswa_DB`, `web/templates_admin_test.go` (render semua template admin & guru), `web/vendor_test.go` |
| Skala monitor | Snapshot 410 siswa ≈ 15 ms (DB nyata) |
| `tests/ui/monitor.ui.js` | LULUS terhadap server (410 baris, WebSocket "Realtime aktif", filter status) |
| `tests/ui/arab_hp.ui.js` | LULUS (360×640, 3 soal Arab tampil RTL, `dir="auto"`, tanpa scroll horizontal) |
| E2E admin (browser) | Tambah/edit soal, tambah ujian/siswa; siswa terkunci → buka kunci → +15 menit → paksa selesai; 3 baris audit; 0 error konsol |
| E2E guru (browser) | Lembar koreksi hanya memuat siswa yang selesai; menilai siswa yang masih ujian ditolak; skor 3/4 → nilai 30, status selesai; ujian guru lain 403 |
| E2E sesi terkunci | 3 pelanggaran via WebSocket → layar terkunci, status `keluar_paksa`, 0 nilai, jawaban tersimpan → buka kunci → login ulang ke `/ujian`, jawaban tersimpan dimuat, timer berlanjut |

Semua uji E2E memakai DB **`cbt_sekolah_go_test`** dengan data seed. DB dev `cbt_sekolah_go` hanya dibaca, tidak diubah.

Cara menjalankan tes UI (server harus berjalan dengan data seed):
```bash
DB_NAME=cbt_sekolah_go_test go run ./cmd/seed
DB_NAME=cbt_sekolah_go_test PORT=3917 go run ./cmd/server &
cd tests/ui && CBT_URL=http://127.0.0.1:3917 npm test
```

## 5. Sisa pekerjaan dan risiko

| Prioritas | Butir | Saran |
|---|---|---|
| Tinggi | Toolchain Go di mesin pengembangan beberapa kali crash acak (SIGSEGV di compiler/linker, build cache korup). Gejala ini mengarah ke masalah RAM atau instalasi Go, bukan kode. | Jalankan memtest dan instal ulang Go. Pastikan build produksi dibuat di mesin yang sehat. |
| Sedang | `TestMigrateIntegration` masih ter-skip karena memuat konfigurasi sendiri (masalah `.env` yang sama dengan `testdb`). | Pakai pencarian `.env` di root modul yang sama. |
| Sedang | `ujian.js` menyisipkan `teks_soal` ke `innerHTML` tanpa escape (sumber: admin/guru). | Escape, kecuali soal memang sengaja memuat HTML. Perlu diputuskan. |
| Rendah | Google Fonts (Sora/DM Sans) di halaman login/ujian masih eksternal. | Opsional: sajikan lokal seperti G-019. |
| Rendah | Progres monitor menghitung soal essay dalam total, sehingga tidak mencapai 100% walau semua PG terjawab. | Pertimbangkan total = soal non-essay. |

## 6. Catatan untuk eksekutor (Gemini)
- Jangan menyatakan "PASS" bila tes DB ter-`Skip`. Periksa keluaran `go test -v` untuk baris `--- SKIP`.
- Tes harus memakai skema dari `migrations/`, bukan skema rekaan. Mock tidak menggantikan tes DB untuk query SQL.
- Halaman admin/guru baru: pakai partial `app_head`/`app_flash`/`app_scripts` (`layout.gohtml`), helper `render`/`serverError`, dan tambahkan kasus render di `web/templates_admin_test.go`.
- Aset front-end disajikan lokal (`/css/vendor`, `/js/vendor`). Jangan menambah referensi CDN; `web/vendor_test.go` akan gagal bila ada.
