# REVIEW KESIAPAN — CBT Sekolah (Go)

| | |
|---|---|
| Tanggal | 17 Oktober 2026 |
| Pelaksana | Claude, atas permintaan user |
| Sifat | **Tinjauan baca-saja atas kode dan dokumen. Bukan R-FINAL-GO.** |
| Dokumen terkait | [TODO](TODO.md) · [DEPLOYMENT](DEPLOYMENT.md) · [PANDUAN_HARI_H](PANDUAN_HARI_H.md) · [LAPORAN_PERBAIKAN_ADMIN_GURU](LAPORAN_PERBAIKAN_ADMIN_GURU.md) · [MOCKUP_UI](MOCKUP_UI.md) |

## 1. Batas tinjauan (baca dulu)
Dibaca: README, CLAUDE, PRD, STACK, DECISION, DESIGN (daftar rute), ERD, TODO (daftar status dan laporan T4.x), DEPLOYMENT, PANDUAN_HARI_H, LAPORAN_PERBAIKAN_ADMIN_GURU, `style.css`, beberapa template, `internal/httpserver/server.go`, `internal/config/config.go`, `internal/admin/handler*.go`, `internal/guru/handler_soal.go`, `internal/auth/admin_guru.go`, `.env.example`, dan screenshot di `tests/ui/out/`. Berkas `.env` **tidak dibaca**.

**Tidak dilakukan:** `make check`, `make test-race`, menjalankan server, `govulncheck`, uji beban, uji di browser atau HP. Semua angka tes dan beban di bawah dikutip dari laporan Gemini dan Claude di TODO.md, bukan hasil ulang. Protokol review CLAUDE.md mewajibkan menjalankan sendiri; itu belum terpenuhi.

Temuan bertanda "dari kode" diperoleh dengan membaca kode, belum dibuktikan dengan menjalankan aplikasi.

## 2. Ringkasan
- Status TODO: T0.1–T4.4 semua `DONE`. `R-FINAL-GO` masih `TODO`. Bagian "Hasil review" di TODO.md masih kosong.
- Fungsi inti sudah ada: login siswa dan takeover, timer server, sinkron jawaban idempoten, auto-finalize, WebSocket, monitor admin, CRUD master data, soal admin dan guru, hasil dan ekspor Excel, penilaian essay, log dan reset, backup, systemd.
- Laporan uji beban T4.3: 500 klien, p95 `sinkron-jawaban` 208 ms (ambang 500 ms), pembanding Node 4.095 ms. Dijalankan ke `127.0.0.1`.
- Ada satu kemungkinan penghalang login di produksi (F-1) dan satu keputusan terbuka yang menentukan cara siswa mengakses server (F-2). Keduanya perlu ditutup sebelum hari-H.

## 3. Temuan
Tingkat mengikuti CLAUDE.md: BLOCKER, MAJOR, MINOR.

### BLOCKER
**F-1 — Cookie sesi `Secure` dapat membuat login gagal lewat HTTP biasa (dari kode).**
- Bukti: `internal/httpserver/server.go:56` → `sm.Cookie.Secure = cfg.IsProduction() || cfg.TrustProxy`. Tidak ada `ListenAndServeTLS` di `cmd/server/main.go` maupun `server.go` (hasil pencarian kosong).
- Skenario: server produksi (`APP_ENV=production`) diakses siswa lewat `http://<IP-laptop>:3000`. Browser menolak menyimpan cookie `Secure` dari origin HTTP (selain localhost), sehingga sesi tidak terbentuk dan siswa tidak bisa lanjut setelah login.
- `DEPLOYMENT.md` baris 61 menyebut Secure aktif "jika menggunakan HTTPS/proxy", sedangkan kode mengaktifkannya untuk semua mode produksi. Dokumen dan kode tidak selaras.
- Perbaikan yang disarankan (pilih satu, keputusan user): (a) produksi selalu di belakang HTTPS atau tunnel dengan `TRUST_PROXY=true`; atau (b) tentukan `Secure` dari skema permintaan, bukan dari `APP_ENV`. Apa pun pilihannya: tambah tes yang meniru akses HTTP LAN dengan mode production, dan uji sekali di HP.

**F-2 — Cara akses publik (D-002) belum diputuskan, padahal PRD §3 menyebut siswa sering memakai kuota sendiri dan WiFi sekolah tidak mampu menampung semua.**
- Bukti: DECISION D-002 berstatus **Ditunda**; PRD §9 O-1 "Ditunda".
- Dampak: tanpa jalur akses dari luar jaringan sekolah, sebagian siswa tidak dapat mengikuti ujian. Keputusan ini juga menentukan F-1 (HTTPS), rate limit per IP di belakang proxy (`X-Forwarded-For` tidak boleh dipercaya mentah, TODO T0.6), dan latensi.
- Tindakan: putuskan metode (tunnel gratis atau lainnya) lalu catat sebagai entri baru di DECISION.

### MAJOR
**F-3 — Uji beban belum mewakili kondisi hari-H.**
- Bukti: laporan T4.3 memakai `--url=http://127.0.0.1:3001` pada mesin yang sama dengan server dan klien penguji.
- Belum terbukti: kinerja lewat WiFi atau tunnel, jumlah koneksi WebSocket nyata, Exambro (HP) dan SEB (laptop) termasuk Fullscreen API dan deteksi pindah tab (D-011).
- Tindakan: simulasi di jaringan yang sama dengan perangkat nyata, minimal satu kelas, dua jenis perangkat.

**F-4 — Lingkungan build tidak stabil dan satu tes masih ter-skip.**
- Bukti: LAPORAN_PERBAIKAN_ADMIN_GURU §5 mencatat SIGSEGV acak pada compiler atau linker dan build cache korup; `TestMigrateIntegration` masih `Skip` karena pencarian `.env`.
- Tindakan: jalankan memtest dan pasang ulang Go; bangun binary produksi di mesin sehat. Setelah `make test`, periksa `go test -v ./... | grep -- '--- SKIP'` dan selesaikan.

**F-5 — Belum ada alur kartu peserta dan PIN massal (dari kode).**
- Bukti: rute siswa hanya `GET /admin/siswa`, `tambah`, `edit`, `hapus`, `template`, `import` (`internal/admin/handler_siswa.go:18-23`). PIN diisi manual atau dari Excel. Tidak ada pembuatan PIN acak massal, reset PIN massal, atau cetak kartu NIS+PIN.
- Dampak: ±410 siswa; pembagian PIN akan manual dan rawan salah.

**F-6 — Soal bergambar tidak dapat dibuat (dari kode).**
- Bukti: ERD memuat kolom `soal.gambar` dan ada folder `uploads/`, tetapi pencarian `upload|gambar|FormFile|multipart` pada `internal/admin/handler_soal.go`, `internal/guru/handler_soal.go`, dan `internal/soal/*.go` tidak menemukan apa pun. Folder `uploads/` hanya berisi `.gitignore`.
- Tindakan: tanyakan apakah ujian nyata memerlukan gambar. Bila ya, buat item baru (batas ukuran, tipe berkas, nama acak, penyajian dengan header aman).

### MINOR
- **F-7** — Tidak ditemukan rute ganti password admin atau guru; `internal/auth/admin_guru.go` hanya memuat login dan logout. Dokumentasikan cara membuat dan mengganti akun admin produksi.
- **F-8** — Tidak ditemukan cetak berita acara dan daftar hadir ujian (hanya `admin_hasil_cetak.gohtml`).
- **F-9** — `ujian.js` memasukkan `teks_soal` ke `innerHTML` tanpa escape (LAPORAN §5). Putuskan: soal boleh HTML atau tidak.
- **F-10** — Progres monitor menghitung soal essay dalam total sehingga tidak mencapai 100% (LAPORAN §5).
- **F-11** — Monitor merender seluruh siswa sekaligus. Screenshot `tests/ui/out/monitor_admin.png` berukuran 1280×9990 untuk 410 baris. Tambah paginasi atau header lengket.
- **F-12** — Google Fonts (Sora, DM Sans) masih dimuat dari luar di login dan ujian. Tampilan tetap berfungsi dengan font cadangan, tetapi selaraskan dengan G-019 (lokal).
- **F-13** — `README.md` masih menyatakan "implementasi fitur belum dimulai" dan tabel dokumen tidak memuat dokumen baru. Perbarui.
- **F-14** — Backup 10 menit saat ujian bergantung pada cron yang dipasang manual (BACKUP_RESTORE.md). Tambahkan bukti latihan restore ke DB uji pada hari sebelum ujian.

## 4. Yang sudah baik (berdasarkan dokumen, belum diuji ulang)
- Timer server-authoritative, takeover, dan penutupan otomatis beserta tes konkurensi.
- Kunci jawaban tidak dikirim ke klien; SQL berparameter; rate limit per NIS; CrossOriginProtection.
- `govulncheck` dilaporkan bersih; 18 paket dan 274 tes dilaporkan lulus (LAPORAN §4).
- Aset Bootstrap dan ikon sudah lokal dengan tes penjaga (`web/vendor_test.go`).

## 5. Urutan kerja yang disarankan
1. Putuskan F-2, lalu tutup F-1 beserta tesnya.
2. Bereskan F-4, lalu jalankan `make check` dan `make test-race` dan tempel keluarannya di TODO.md.
3. Uji nyata F-3 (HP, Exambro, SEB, WiFi atau tunnel).
4. Kerjakan F-5 dan, bila perlu, F-6.
5. Implementasikan mockup per [MOCKUP_UI](MOCKUP_UI.md) §4.
6. Setelah itu baru R-FINAL-GO: jalankan seluruh protokol CLAUDE.md dan isi "Hasil review" di TODO.md.

## 6. Usulan item TODO (belum ditambahkan ke TODO.md)
Item berikut menunggu persetujuan user agar eksekutor tidak mengerjakan di luar lingkup (RULES_AI).
- T5.1 Keputusan akses publik dan HTTPS (F-1, F-2) — Owner: U lalu G.
- T5.2 Stabilkan toolchain dan tutup tes ter-skip (F-4) — Owner: U dan G.
- T5.3 Uji lapangan: HP, Exambro, SEB, WiFi atau tunnel (F-3) — Owner: U dan G.
- T5.4 Kartu peserta, PIN massal, dan reset PIN (F-5) — Owner: G.
- T5.5 Upload gambar soal, bila dibutuhkan (F-6) — Owner: G.
- T5.6 Implementasi mockup UI per layar — Owner: G.
- T5.7 Perbaikan minor F-7 sampai F-14 — Owner: G.
