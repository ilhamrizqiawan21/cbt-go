# DECISION — Catatan Keputusan Desain (versi Go)

Format: ID · Status · Keputusan. Status: **Final**, **Ditunda**, **Usulan**. Perubahan keputusan Final harus dicatat sebagai entri baru yang menggantikan, jangan dihapus. **Hanya user yang boleh mengubah keputusan Final** (RULES_AI §1).

Keputusan `D-xxx` **diwarisi** dari proyek Node (tetap berlaku bila tidak digantikan). Keputusan `G-xxx` khusus proyek Go ini.

## A. Diwarisi dari proyek Node
| ID | Status | Keputusan |
|----|--------|-----------|
| D-001 | Final | Server berjalan di **satu laptop khusus milik sekolah**; tanpa VPS berbayar |
| D-002 | Final | Akses fleksibel: mendukung Tunnel HTTPS (`TRUST_PROXY=true`) maupun WiFi HTTP LAN lokal sekolah. Flag `COOKIE_SECURE` dapat disetel mandiri (default otomatis aktif pada production/tunnel, dan nonaktif bila `COOKIE_SECURE=false`) agar cookie sesi tidak ditolak browser saat diakses lewat HTTP IP laptop sekolah |
| D-003 | Final | Menjodohkan: satu soal = 2 poin jika **semua** pasangan benar, selain itu 0 |
| D-004 | Final | Bobot: PG 2, menjodohkan 2, essay 4 per soal (5 soal = 20) |
| D-005 | Final | Nilai akhir = total poin / total poin maks × 100, dibulatkan |
| D-006 | Final | Essay dijawab di kertas, dinilai guru **per soal (0–4)**; aplikasi tidak menerima jawaban essay |
| D-007 | Final | Timer **server-authoritative**; `batas_waktu` tidak melewati `tanggal_selesai` ujian kecuali admin menambah waktu |
| D-008 | Final | Login pada sesi aktif **mengambil alih** (perangkat lama keluar), bukan ditolak |
| D-009 | Final | Sinkron jawaban batch idempoten; grace 120 detik setelah batas, hanya untuk `client_ts ≤ batas_waktu` |
| D-011 | Final | Pengunci perangkat memakai Exambro (HP) / SEB (laptop); aplikasi tidak mengklaim mendeteksi HP kedua |
| D-014 | Final | Font Arab dilayani lokal (tanpa CDN) |
| D-017 | Usulan | Siswa tidak melihat nilai setelah ujian (flag `TAMPILKAN_NILAI_SISWA`, default false) |
| D-018 | Final | Rate limit login per NIS, bukan hanya per IP |
| D-019 | Final | Pemantauan hanya untuk **admin** |

Tidak diwarisi: D-010 (express-mysql-session → digantikan G-005), D-012 (tetap Express → digantikan G-001), D-013 (digantikan G-007), D-015 (peran, diulang di G-012), D-016 (khusus Node).

## B. Khusus proyek Go
| ID | Status | Keputusan |
|----|--------|-----------|
| G-001 | Final | Proyek Go **terpisah** di `~/Projects/cbt-go`. Proyek Node tidak diubah; berfungsi sebagai referensi perilaku (read-only) dan baseline performa |
| G-002 | Final | Tujuan ganda: **paritas perilaku** dan **belajar Go**. Definisi selesai ada di PRD §8 |
| G-003 | Final | Stack: lihat [STACK](STACK.md). Prinsip: **stdlib dulu**, tambah library hanya yang tercantum di item TODO, terawat aktif, tidak deprecated. Versi/API diverifikasi ke dokumentasi resmi sebelum dipakai |
| G-004 | Final | **Kontrak HTTP identik** dengan versi Node (path, method, bentuk JSON, status code) — DESIGN §4. Alasan: `tools/loadtest/loadtest.js` dan `web/static/js/ujian.js` dapat dipakai ulang, dan perbandingan Node vs Go adil |
| G-005 | Final | Realtime: **WebSocket polos** (`github.com/coder/websocket`) dengan amplop JSON `{"event","data"}` yang memakai nama event Socket.io lama. Satu proses, hub in-memory (selaras D-001). Tidak memakai Socket.io |
| G-006 | Final | Database **terpisah**: `cbt_sekolah_go` (dev) dan `cbt_sekolah_go_test` (uji). Skema = baseline Node + migrasi 001–003 digabung + tabel `sessions` untuk scs. Dilarang menyentuh `cbt_sekolah` (milik Node) |
| G-007 | Final | Migrasi dengan **goose** (SQL murni) di `migrations/`; folder ini adalah **sumber kebenaran skema** (tidak ada `schema.sql` terpisah agar tidak berselisih). Migrasi bernomor, tidak menghapus data |
| G-008 | Final | **Tanpa ORM**: SQL ditulis tangan lewat `database/sql`, selalu berparameter. `sqlc` dipertimbangkan hanya bila disetujui user |
| G-009 | Final | Perlindungan CSRF memakai `http.CrossOriginProtection` (stdlib, Go ≥ 1.25) menggantikan cek Origin manual |
| G-010 | Final | Struktur kode **per fitur** di `internal/` (bukan per lapisan teknis); logika bisnis sebagai fungsi murni yang diuji. Tidak ada interface "untuk berjaga-jaga": interface hanya didefinisikan di sisi **pemakai** dan hanya bila dibutuhkan tes |
| G-011 | Final | Tampilan: `html/template` + `go:embed`; Bootstrap 5 + JS vanilla dipertahankan. Tidak ada SPA/React/templ |
| G-012 | Final | Peran: **Gemini = eksekutor**, **Claude = perancang + reviewer**, **user = pemilik keputusan**. Karena user sedang belajar Go: kode harus idiomatik, dan laporan eksekusi memuat bagian "Konsep Go" |
| G-013 | Final | `go.mod` memakai `go 1.25` (minimum, karena `CrossOriginProtection`). Toolchain pengembangan: Go stabil terbaru (saat dokumen ini ditulis: 1.27.2) |
| G-014 | Final | Jalur panas (login, sesi, sinkron jawaban, heartbeat, selesai, WebSocket) dikerjakan **lebih dulu** (Fase 1–2) agar uji beban 500 klien bisa dijalankan lebih awal sebelum CRUD admin/guru |
| G-015 | Final | Deployment: **binary tunggal** (template dan aset di-embed) dijalankan **systemd**; tidak memakai PM2/Node di server |
| G-016 | Usulan | Pertahankan `siswa.kelas` sebagai nama kelas (bukan FK) demi paritas data dan kemudahan import Excel. Perbaikan ke FK ditunda ke setelah paritas |

### Keputusan 2026-10-17 (diputuskan Claude atas delegasi user; dapat ditinjau ulang user)
| ID | Status | Keputusan |
|----|--------|-----------|
| G-017 | Final | **Sesi terkunci dapat dilanjutkan** (PRD FR-12, PANDUAN_HARI_H): melewati batas pelanggaran → `keluar_paksa` + event `paksa-submit` dengan `terkunci: true`; klien menampilkan layar terkunci **tanpa mengumpulkan** dan tanpa menghapus antrean jawaban lokal. Siswa tidak dapat memfinalisasi sesi terkunci (`sesi.FinalizeOlehSiswa` → `ErrKeluarPaksa`; HTTP 403 `status: keluar_paksa`; event WS `selesai-ujian` diabaikan). Admin membuka kunci → siswa login ulang dan melanjutkan. Admin tetap bisa "paksa selesai"; job penutupan otomatis menilai sesi terkunci yang melewati `batas_waktu` + grace |
| G-018 | Final | **Validasi soal di server diperketat** (`soal.ValidasiDanNormalisasi`, berlaku admin, guru, dan batch): PG ≥2 pilihan + kunci A–D yang pilihannya terisi; menjodohkan ≥2 pasangan lengkap dengan sisi kiri unik, `jawaban_benar` selalu diturunkan server dari `opsi_tambahan.pasangan`; essay membersihkan pilihan/kunci. Alasan: soal cacat sebelumnya tersimpan diam-diam dan membuat semua siswa dinilai salah |
| G-019 | Final | **Bootstrap 5.3.3 dan Bootstrap Icons 1.11.3 disajikan lokal** (di-embed, `web/static/{css,js}/vendor`, hash diverifikasi terhadap jsDelivr). CSP tidak lagi mengizinkan `cdn.jsdelivr.net`. Google Fonts (Sora/DM Sans) tetap opsional dengan fallback font sistem. Alasan: D-001/G-015 — tampilan tidak boleh rusak saat internet sekolah putus |
| G-020 | Final | Mengganti nama kelas ikut memperbarui `siswa.kelas` dalam satu transaksi (konsekuensi G-016) |
| G-021 | Final | Nilai essay hanya dapat diberikan setelah sesi siswa `selesai`; lembar koreksi hanya memuat siswa tersebut. Alasan: menilai lebih awal membuat `nilai_ujian` yang memblokir login siswa |

### Keputusan 2026-10-17 (audit kelengkapan fitur; diputuskan Claude atas delegasi user: "decision making dari anda dulu"; dapat ditinjau ulang user)
| ID | Status | Keputusan |
|----|--------|-----------|
| G-022 | Final | Paritas Node dinyatakan tercapai (Fase 0–5 `DONE`), sehingga larangan "fitur baru di luar paritas" (PRD §4) dilonggarkan **hanya** untuk daftar G-023…G-028 dalam **Fase 6**. Fitur lain di luar daftar tetap dicatat di "Catatan eksekutor". Fase 6 tidak boleh mengganggu jalur panas (Fase 1–2) dan harus lolos `make check`, `make test-race`, serta uji beban ulang bila menyentuh `internal/sesi` |
| G-023 | Final | **Gambar soal**: unggah png/jpg/webp, maks 1 MB, tanpa SVG. Tipe diperiksa dari isi berkas (`http.DetectContentType`), bukan ekstensi; nama berkas acak (`crypto/rand`); disimpan di `uploads/soal/`; disajikan lewat rute yang mensyaratkan login (siswa hanya untuk ujian sesi aktifnya) dengan `X-Content-Type-Options: nosniff` dan `Cache-Control` agar hemat kuota. Tanpa resize di server (G-003: stdlib dulu); guru diminta mengecilkan gambar sebelum unggah. Berkas yatim dibersihkan saat soal dihapus. Guru hanya untuk mapel miliknya |
| G-024 | Final | **Impor soal dari Excel** (admin dan guru): template `.xlsx` untuk PG, menjodohkan, essay; batas 2 MB; validasi memakai `soal.ValidasiDanNormalisasi` (G-018); **atomik** (satu transaksi; satu baris salah → tidak ada yang tersimpan) dengan laporan galat per baris; otorisasi per mapel sama dengan input manual |
| G-025 | Final | **Satu ujian tetap satu kelas** (tidak mengubah skema `ujian`/`pengajaran`). Duplikasi diselesaikan dengan fitur **salin soal** dari ujian lain (dalam wewenang pengguna) dan **duplikat ujian** ke pengajaran/kelas lain. Alasan: mengubah relasi ujian–kelas menyentuh login, monitor, dan penilaian (risiko besar menjelang ujian) |
| G-026 | Final | **Ujian susulan per siswa** lewat tabel baru `izin_susulan (siswa_id, ujian_id, tanggal_selesai, dibuat_oleh)` (migrasi `00002`, tanpa menghapus data). Admin memberi izin; login siswa tersebut diizinkan di luar jendela ujian sampai `tanggal_selesai` izin; `batas_waktu` dibatasi izin itu (turunan D-007). Dicatat di `audit_admin`. `reset-ujian` tidak lagi menjadi satu-satunya jalan bagi siswa yang berhalangan |
| G-027 | Final | **Akun**: admin dapat mengganti password sendiri dan mereset password guru; guru dapat mengganti password sendiri (verifikasi password lama; bcrypt; sesi lain dirotasi). Menutup sisa F-7 (sebelumnya hanya dokumentasi) |
| G-028 | Final | **Laporan guru/admin**: (a) analisis butir sederhana per ujian: persentase benar, jumlah kosong, dan distribusi pilihan A–D per soal, dari `jawaban_siswa`, tanpa psikometri lanjutan; (b) **daftar hadir/berita acara cetak** per ujian dan kelas dari `sesi_ujian` (status, waktu mulai/selesai, jumlah pelanggaran, takeover) |
| G-029 | Final | **Ditolak/ditunda**: entitas ruang/gelombang (kebutuhan belum terbukti; tanyakan ke pengawas, buka lagi bila perlu); ujian multi-kelas (G-025); pengumuman ke peserta; paginasi monitor (sticky header F-11 sudah ada dan 410 baris dapat difilter); resize gambar di server. D-017 (nilai tidak tampil ke siswa) tetap berlaku |

### Koreksi saat implementasi Fase 6 (2026-10-17)
| ID | Status | Keputusan |
|----|--------|-----------|
| G-030 | Final | Menggantikan sebagian G-026/G-027. (a) Izin susulan **tidak** dicatat di `audit_admin` (kolom `sesi_id` NOT NULL, sesi belum ada); jejaknya (`dibuat_oleh`, `alasan`, `created_at`) disimpan di `izin_susulan`. (b) **Reset password guru oleh admin sudah ada** sejak Fase 3 (Edit Guru → password baru); audit awal keliru menyebutnya tidak ada, sehingga G-027 hanya menambah ganti password **mandiri** (admin dan guru). (c) Daftar ujian publik (`/api/daftar-ujian`) menampilkan ujian yang punya izin susulan aktif, karena siswa memilih ujian sebelum mengisi NIS; login tetap menolak siswa tanpa izin. (d) Backup (`scripts/backup.sh`) kini juga mengarsipkan `uploads/` karena dump database tidak memuat gambar soal |
