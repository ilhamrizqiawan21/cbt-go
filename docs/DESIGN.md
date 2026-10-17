# DESIGN — CBT Sekolah (versi Go)

Dokumen terkait: [PRD](PRD.md), [ERD](ERD.md), [DECISION](DECISION.md), [STACK](STACK.md), [PORTING_MAP](PORTING_MAP.md), [TODO](TODO.md)

Aturan bisnis (§3) dan kontrak API (§4) **identik dengan versi Node** (G-004). Yang baru: arsitektur Go (§2) dan realtime WebSocket (§5). Untuk detail perilaku yang tidak tertulis di sini, **kode Node adalah referensi** (lihat PORTING_MAP §1); bila dokumen ini bertentangan dengan kode Node, dokumen ini yang menang.

## 1. Arsitektur target
```
Siswa (HP/Laptop) ──HTTPS──► Tunnel (D-002) ──► Laptop server sekolah
Admin / Guru (browser) ─────HTTPS──────────────► ├─ cbt-go (1 binary: HTTP + WebSocket + job latar)
                                                  ├─ MySQL/MariaDB
                                                  └─ systemd + backup terjadwal
```
- **Monolit satu proses**, satu binary. Template dan aset statis di-embed (`go:embed`).
- Goroutine latar: job penutupan otomatis (§3.4) dan hub WebSocket (§5). Semuanya dihentikan rapi lewat `context` saat shutdown.

## 2. Struktur kode
Per fitur (G-010). Package dibuat **saat dibutuhkan item TODO**, bukan di muka.
```
cmd/server/            entry point: config → db → router → graceful shutdown
cmd/migrate/           CLI goose (up | down | status)            [T0.3]
cmd/seed/              seed data sintetis (hanya dev/uji)        [T0.5]
internal/config/       env → struct, divalidasi                  [ADA]
internal/db/           koneksi + pool                            [ADA]
internal/httpserver/   router, middleware (log, recover, ...)    [ADA]
internal/auth/         login siswa/admin/guru, session, role guard
internal/sesi/         timer, takeover, grace, sinkron jawaban, finalisasi
internal/penilaian/    fungsi MURNI penghitung nilai
internal/soal/         paket soal, acak deterministik, otorisasi guru
internal/monitor/      snapshot pemantauan (aksi admin + audit: internal/admin/aksi_sesi.go)
internal/realtime/     hub WebSocket (siswa & admin)
internal/excel/        import siswa, template, ekspor, impor essay
internal/ratelimit/    limiter per kunci (NIS, IP)
internal/testdb/       helper DB uji (migrasi + seed + bersih)
migrations/            goose, SQL murni
web/templates/         html/template
web/static/            css, js, font, gambar
```
Aturan struktur:
- Handler HTTP **tipis**: baca request → panggil fungsi paket bisnis → tulis respons.
- Logika bisnis yang bisa dibuat murni (input → output) **harus** murni dan diuji dengan tes berbasis tabel.
- Query SQL ada di paket fiturnya sendiri (tidak ada paket "repository" global). Selalu berparameter.
- Error dibungkus `fmt.Errorf("konteks: %w", err)`; dicatat di satu tempat (handler/job), tidak di setiap lapisan.

## 3. Alur utama (sama dengan Node)

### 3.1 Login dan lanjut sesi
1. Siswa mengirim NIS, PIN, `ujian_id` → server ambil siswa **berdasarkan NIS**, `bcrypt` bandingkan PIN.
2. Validasi: ujian dalam jendela waktu, **kelas siswa = kelas pengajaran ujian**, token ujian (jika diaktifkan), belum ada `nilai_ujian` final.
3. Cek `sesi_ujian`:
   - Tidak ada → buat sesi baru (`waktu_mulai = NOW()`, `batas_waktu`, `seed` acak, `device_token`).
   - Ada `sedang_ujian` → **ambil alih**: terbitkan `device_token` baru; perangkat lama menerima 409 dan keluar; catat di `audit_admin` (aksi `takeover`).
   - `keluar_paksa` → tolak, pesan hubungi pengawas.
   - `selesai` → tolak (sudah dikerjakan).
4. Klien menerima paket soal + jawaban tersimpan + `sisa_detik`.
- Pesan galat login **generik** ("NIS atau PIN salah") agar tidak membocorkan keberadaan NIS.

### 3.2 Timer server-authoritative
- `batas_waktu = waktu_mulai + durasi + tambahan_menit` (tidak melewati `ujian.tanggal_selesai`, D-007, kecuali admin menambah waktu).
- `sisa_detik = max(0, batas_waktu − NOW())` dihitung server di `GET /api/sesi`.
- Klien menampilkan hitung mundur lokal, **resinkron** tiap 30 detik, setiap sambung ulang, dan saat tab kembali aktif.

### 3.3 Sinkronisasi jawaban (offline-tolerant)
- Paket soal (tanpa kunci jawaban) diunduh sekali.
- `POST /api/sinkron-jawaban` mengirim batch `[{soal_id, jawaban, client_ts}]`; server **upsert** (idempoten), simpan yang `client_ts` terbaru per soal — satu transaksi per batch.
- Tolak bila sesi `selesai`/`keluar_paksa`. Setelah `batas_waktu`, masih diterima selama **grace 120 detik** hanya jika `client_ts ≤ batas_waktu` (D-009).
- Header/field `device_token` harus cocok; tidak cocok → 409 (perangkat lama tergeser).
- Heartbeat tiap 20 detik memperbarui `sesi_ujian.last_seen`.

### 3.4 Penutupan otomatis
Job (`time.Ticker`, 30 detik) memanggil `finalizeSesi` untuk sesi `sedang_ujian` dengan `NOW() > batas_waktu + grace`: hitung nilai, set `status='selesai'`, `selesai_pada`. Tombol "Selesai" siswa memanggil fungsi yang sama. Finalisasi **idempoten** (dua pemanggil bersamaan tidak boleh membuat nilai ganda → pakai `SELECT ... FOR UPDATE` atau `INSERT ... ON DUPLICATE KEY`).

### 3.5 Penilaian
`penilaian.Hitung(soal, jawaban, skorEssay)` — **fungsi murni** (aturan PRD §5):
- PG: `jawaban == jawaban_benar` → `poin`.
- Menjodohkan: kumpulan pasangan sama persis (urutan diabaikan sesuai perilaku Node) → `poin`, selain itu 0.
- Essay: skor dari `nilai_essay` (0..poin); belum diisi = belum dinilai.
- `nilai_akhir = round(total_didapat / total_poin_maks × 100)`; `status_koreksi = menunggu_essay` bila ada soal essay tanpa skor.
- Menyimpan skor essay memanggil ulang hitung.

### 3.6 Pemantauan admin
- Admin terhubung ke WebSocket admin, bergabung ke room `ujian:{id}`.
- Server menyiarkan `monitor:update` saat siswa masuk/sambung ulang/keluar, jawaban tersinkron (progres), pelanggaran, selesai.
- **Optimasi wajib:** bila room admin untuk ujian itu kosong, **jangan menjalankan query monitor sama sekali** (temuan MINOR beban di Node: 4 query per event walau tak ada admin).
- `GET /admin/api/monitor/:ujianId` mengembalikan snapshot awal.
- Status tampilan: `online` (last_seen < 45 dtk), `offline`, `selesai`, `terkunci`, `belum_masuk`.
- Aksi POST (hanya admin): `buka-kunci`, `tambah-waktu`, `paksa-selesai`; semua dicatat ke `audit_admin`.

### 3.7 Anti-curang per perangkat
- `device_type` (`hp`/`laptop`) ditentukan klien; server hanya memakainya memilih kebijakan, bukan keamanan.
- **Laptop**: fullscreen wajib; pindah tab/keluar fullscreen/copy-paste = pelanggaran. **HP**: pindah tab/aplikasi dihitung; copy-paste diblokir.
- Melewati `ujian.batas_pelanggaran` → `keluar_paksa` + `paksa-submit {terkunci: true}` ke siswa + notifikasi dashboard. Klien menampilkan layar terkunci tanpa mengumpulkan; siswa tidak bisa memfinalisasi sesi terkunci; setelah admin membuka kunci, siswa login ulang dan melanjutkan (G-017). Sesi terkunci yang melewati batas waktu dinilai oleh job penutupan otomatis.
- Pengunci utama: Exambro (HP) / SEB (laptop) (D-011).
- Acak soal/pilihan memakai `seed` di DB: `math/rand/v2` dengan sumber `rand.NewPCG(seed, ...)` agar urutan **deterministik** per siswa (urutan tidak harus sama dengan Node, hanya stabil).

## 4. Kontrak HTTP (identik dengan Node — G-004)
| Method | Path | Auth | Fungsi |
|--------|------|------|--------|
| GET | `/`, `/login` | publik | Landing / halaman login siswa |
| POST | `/login-siswa` | publik + rate limit (NIS & IP) | Login/lanjut sesi |
| GET | `/ujian` | siswa | Halaman ujian |
| GET | `/logout` | siswa | Keluar |
| GET/POST | `/login-admin`, `/login-guru` | publik + rate limit | Login admin/guru |
| GET | `/logout-admin`, `/logout-guru` | sesuai peran | Keluar |
| GET | `/api/daftar-ujian` | publik | Daftar ujian aktif untuk halaman login |
| GET | `/api/sesi` | siswa | `{status, sisa_detik, batas_waktu, jawaban_tersimpan}` |
| GET | `/api/soal/:ujianId` | siswa | Paket soal tanpa kunci; urutan sesuai seed |
| POST | `/api/sinkron-jawaban` | siswa + `device_token` | Upsert batch jawaban |
| POST | `/api/simpan-jawaban` | siswa | Lama; pembungkus ke logika sinkron (dipertahankan sementara) |
| POST | `/api/heartbeat` | siswa | Perbarui `last_seen` |
| POST | `/api/selesai-ujian` | siswa | Finalisasi |
| GET | `/api/cek-status-ujian` | publik | Status ujian (lihat Node `routes/api.js`) |
| GET | `/admin/api/monitor/:ujianId` | admin | Snapshot pemantauan |
| POST | `/admin/api/sesi/:id/buka-kunci` | admin | `keluar_paksa → sedang_ujian` |
| POST | `/admin/api/sesi/:id/tambah-waktu` | admin | `{menit}` |
| POST | `/admin/api/sesi/:id/paksa-selesai` | admin | Finalisasi paksa |
| * | `/admin/...`, `/guru/...` | admin / guru | Halaman CRUD (rute persis: PORTING_MAP §2) |
| GET | `/healthz` | publik | Kesehatan proses + DB (baru, tidak ada di Node) |

Bentuk JSON, nama field, dan status code **harus sama persis** dengan Node. Cara memastikan: baca handler Node, lalu tulis tes Go yang memeriksa bentuk respons.

## 5. Realtime (WebSocket — G-005)
Dua endpoint, autentikasi lewat cookie session yang sama:
- `GET /ws` — siswa (peran siswa + sesi ujian aktif).
- `GET /ws/admin` — admin.

Amplop pesan JSON dua arah: `{"event": "<nama>", "data": { ... }}`. Nama event **sama dengan Socket.io lama** agar porting `ujian.js` dan `monitor` mekanis.

| Arah | Event | Data | Perilaku |
|------|-------|------|----------|
| siswa→server | `siswa-siap` | `{}` | Server balas `mulai-ujian` |
| server→siswa | `mulai-ujian` | `{durasi, sisa_detik}` | |
| siswa→server | `pindah-tab`, `copy-paste`, `keluar-fullscreen` | `{}` | Catat `log_kecurangan`; lewat batas → `paksa-submit` |
| siswa→server | `selesai-ujian` | `{}` | Finalisasi |
| server→siswa | `paksa-submit` | `{message, terkunci?}` | `terkunci: true` → layar terkunci, tidak mengumpulkan (G-017); tanpa `terkunci` (paksa selesai admin) → klien mengirim jawaban terakhir lalu selesai |
| server→siswa | `tambah-waktu` | `{...}` | Klien resinkron timer |
| server→siswa | `peringatan` | `{pesan}` | Tampilkan peringatan |
| server→siswa | `error` | `{message}` | |
| admin→server | `admin:join` | `{ujian_id}` | Server balas `admin:joined` / `admin:error` |
| server→admin | `monitor:update` | payload siswa | Lihat Node `services/monitorService.js` |

Aturan:
- Satu goroutine pembaca + satu penulis per koneksi; hub memegang peta room dengan `sync.Mutex` (atau satu goroutine hub dengan channel) — pilih yang paling sederhana dan lulus `go test -race`.
- **Ping/pong** dan batas ukuran pesan wajib. Pemutusan koneksi → tandai `last_seen`, bukan menutup sesi (ketahanan sesi, G1).
- Klien menyambung ulang dengan backoff; setelah tersambung ulang mengirim `siswa-siap` lagi.
- Pemeriksaan `Origin` WebSocket dibiarkan aktif (default library); jangan dimatikan.
- Rate-limit pesan per koneksi untuk mencegah banjir event.

## 6. Antarmuka
- **Arab/RTL**: font lokal (Amiri) via `@font-face`; `dir="auto"` pada teks soal/pilihan; kontrol A−/A+ (disimpan di `localStorage`); harakat tampil benar.
- **Responsif**: target 360×640; tombol minimal 44px; tanpa scroll horizontal.
- **Essay**: kartu bacaan "Dijawab di lembar kertas"; tanpa textarea.
- **Indikator**: status sinkron, sisa waktu, jumlah terjawab.
- **Admin monitor**: tabel dengan filter kelas/status, penanda warna, aksi per baris.
- Template memakai `html/template` (escape otomatis). **Dilarang** memakai `template.HTML`/`template.JS` pada data pengguna.

## 7. Keamanan
- Session: library `scs` + store MySQL; cookie `HttpOnly`, `SameSite=Lax`, `Secure` bila produksi/`TRUST_PROXY`; token dirotasi saat login (`RenewToken`) untuk mencegah session fixation.
- Rate limit login: per **NIS** (10/15 menit) dan per IP longgar (300/15 menit); admin/guru per IP+username (10/15 menit). Respons 429 dalam Bahasa Indonesia.
- CSRF: `http.CrossOriginProtection` untuk semua POST berbasis session (G-009); izinkan pengecualian eksplisit hanya bila terbukti perlu.
- Kunci jawaban tidak pernah dikirim ke klien. Otorisasi di server (guru hanya mapel miliknya).
- Upload Excel: batas 2 MB, hanya `.xlsx`, ukuran dibatasi lewat `http.MaxBytesReader`, file sementara dihapus.
- Header keamanan dasar (`X-Content-Type-Options`, `Referrer-Policy`, `Content-Security-Policy`). Bootstrap & ikon disajikan lokal dari binary (G-019); CSP hanya mengizinkan Google Fonts sebagai pengecualian eksternal.
- Tanpa akun/endpoint demo; secret hanya dari environment.
- Log memakai `log/slog`; jangan mencatat PIN, hash, atau isi cookie.

## 8. Deployment
- Satu laptop khusus server: sleep/hibernate dimatikan, daya tersambung, jaringan kabel bila ada.
- `go build` → binary `bin/server` + `.env`; dijalankan **systemd** (`Restart=always`), mulai saat boot. Panduan di `docs/DEPLOYMENT.md` (dibuat di T4.2).
- Backup: `scripts/backup.sh` (disalin dari Node; sesuaikan nama DB), tiap 10 menit saat ujian.
- Akses publik via tunnel — belum diputuskan (D-002). `TRUST_PROXY=true` bila di balik proxy.
- Uji beban sebelum hari-H: `tools/loadtest/loadtest.js` (butuh Node hanya di mesin penguji, bukan di server).

## 9. Strategi pengujian
- Paket `testing` bawaan; **tes berbasis tabel** untuk fungsi murni; `httptest` untuk handler.
- Tes integrasi memakai DB `cbt_sekolah_go_test` lewat `internal/testdb`. Tes yang butuh DB dilewati (`t.Skip`) bila DB uji tidak tersedia, agar `go test ./...` tetap hijau di mesin tanpa MySQL.
- Gerbang kualitas: `make check` (gofmt, vet, test); `make test-race` untuk kode konkuren (butuh gcc/cgo).
- Padanan tes Node: PORTING_MAP §3.
- UI: Playwright (dari tooling Node yang sudah ada, diarahkan ke server Go) untuk 360px dan teks Arab (T4.4).
- Beban: 500 klien; bandingkan dengan Node.
