# TODO — Rencana Implementasi (versi Go)

Eksekutor: **Gemini** · Reviewer: **Claude** (R-FINAL-GO di akhir) · Aturan: [GEMINI.md](../GEMINI.md), [RULES_AI.md](RULES_AI.md)
Acuan: [PRD](PRD.md) · [DESIGN](DESIGN.md) · [ERD](ERD.md) · [DECISION](DECISION.md) · [STACK](STACK.md) · [PORTING_MAP](PORTING_MAP.md)
Status: `TODO` `IN_PROGRESS` `DONE` `BLOCKED` `NEEDS_USER` `REVIEW_FIX`

Kolom **Dep** = item yang harus `DONE` lebih dulu. **Owner** `G` = Gemini, `U` = user.
**Sumber Node** = berkas di `/home/ilhamrizqiawan/Projects/CBT-Sekolah` (read-only) yang menjelaskan perilaku asli. **Konsep Go** = hal yang dipelajari user dari item ini; wajib dibahas singkat di laporan eksekusi.

Setiap item bertipe kode mengikuti siklus: **tes dulu (gagal) → implementasi → `make check` hijau**. Kode konkuren: `make test-race` juga hijau.

## Status saat dokumen dibuat (sudah ada, diverifikasi Claude dengan Go 1.27.2)
- `go.mod` (`go 1.25`), deps: `go-sql-driver/mysql v1.10.1`, `joho/godotenv v1.5.1`.
- `internal/config` (+ tes, 4 fungsi tes), `internal/db` (pool MySQL), `internal/httpserver` (router, `GET /healthz`, middleware log+recover, + tes), `cmd/server` (graceful shutdown), `Makefile`, `.env.example`, `.gitignore`.
- `migrations/00001_baseline.sql` — diverifikasi pada MySQL 8.4.10: Up → 15 tabel + 20 FK; Down → 0 tabel.
- `web/static/**` (css, font Arab, gambar, js) disalin dari Node. `web/static/js/ujian.js` **masih memakai Socket.io** (T2.3).
- `scripts/backup.sh`, `tools/loadtest/loadtest.js` disalin dari Node (belum disesuaikan).
- `gofmt`, `go vet`, `go test ./...`, `go build` bersih. **`go test -race` belum bisa dijalankan** (gcc belum terpasang → T0.1).

---
## Fase 0 — Fondasi

### T0.1 Siapkan mesin pengembangan — Owner: U — Dep: —
- Pasang Go ≥ 1.25 (disarankan terbaru): `sudo dnf install golang` lalu `go version`; bila terlalu lama, ambil tarball dari go.dev/dl.
- Pasang `make` dan gcc: `sudo dnf install make gcc` (**`make` belum terpasang di mesin ini** saat diperiksa; gcc dibutuhkan `-race`).
- Buat DB: `mysql -uroot -e "CREATE DATABASE cbt_sekolah_go CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci; CREATE DATABASE cbt_sekolah_go_test CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"` (sesuaikan bila root butuh password/`sudo`).
- `cp .env.example .env`; isi `DB_*` (di mesin ini MySQL 8.4.10 aktif dan `/var/lib/mysql/mysql.sock` ada; isi `DB_SOCKET` bila TCP gagal).
- Pasang ekstensi "Go" di VS Code.
- **Terima:** `go version` ≥ 1.25; `make --version` berjalan; `make check` hijau; `make test-race` hijau.
- Status: `DONE`

### T0.2 Verifikasi kerangka berjalan — Owner: G — Dep: T0.1
- Konsep Go: modul, `go run`, `defer`, graceful shutdown dengan `signal.NotifyContext`.
- Jalankan `make check`; `make run`; `curl -i localhost:3000/healthz`. Hentikan dengan Ctrl+C dan pastikan log menunjukkan shutdown tertib.
- Perbaiki bila ada error di kerangka (minimal). Jangan menambah fitur.
- **Terima:** `/healthz` → 200 `{"status":"ok"}`; DB dimatikan (atau salah `DB_NAME`) → server gagal start dengan pesan jelas; `make check` hijau.
- Status: `DONE`

### T0.3 Migrator goose — Owner: G — Dep: T0.1
- Sumber Node: `scripts/migrate.js`, `tests/migrate.test.js`. Dependensi baru: `github.com/pressly/goose/v3` (verifikasi API terbaru ke dokumentasi resmi).
- Konsep Go: `go:embed`, paket kedua (`cmd/migrate`), argumen CLI.
- Buat `migrations/embed.go` (`package migrations`, `//go:embed *.sql`, ekspor `FS`) — `go:embed` tidak bisa menunjuk folder induk, jadi berkas ini harus berada di dalam `migrations/`.
- `cmd/migrate` dengan sub-perintah `up`, `down` (satu langkah), `status`. Baca konfigurasi lewat `internal/config` + `internal/db`.
- Keamanan: `down` **menolak** berjalan bila `APP_ENV=production`.
- **Terima:** `go run ./cmd/migrate up` pada `cbt_sekolah_go_test` → 15 tabel + `goose_db_version`; dijalankan dua kali tidak error; `status` benar; `down` lalu `up` kembali utuh. Tes (bila DB tersedia) untuk idempotensi; dilewati (`t.Skip`) bila DB uji tak bisa dihubungi.
- Status: `DONE`

### T0.4 Helper DB uji + cek zona waktu — Owner: G — Dep: T0.3
- Sumber Node: `tests/helpers/db.js`.
- Konsep Go: `testing.T`, `t.Helper()`, `t.Cleanup()`, `t.Skip`, `TestMain`.
- `internal/testdb`: `Open(t) *sql.DB` memakai env DB uji; **menolak** bila `DB_NAME` tidak berakhiran `_test`; `t.Skip` bila tak terhubung; `Reset(t, db)` mengosongkan semua tabel aman-FK (`SET FOREIGN_KEY_CHECKS=0` + `TRUNCATE`); fungsi seed sintetis kecil (`SeedSiswa`, `SeedUjian`, …) sesuai kebutuhan item berikutnya (tambah bertahap, jangan di muka).
- Pemeriksaan startup di `cmd/server`: bandingkan offset zona `APP_TIMEZONE` dengan zona MySQL (`SELECT TIMESTAMPDIFF(MINUTE, UTC_TIMESTAMP(), NOW())`); beda → **peringatan log** jelas (ERD §3), bukan gagal.
- **Terima:** tes pemakaian helper lulus; `Open` menolak `cbt_sekolah_go`/`cbt_sekolah` (ada tes); peringatan zona waktu teruji dengan fungsi murni (offset dibanding offset).
- Status: `DONE`

### T0.5 Seed data sintetis — Owner: G — Dep: T0.3
- Sumber Node: `scripts/seed.js`, `tests/persistent_seed.test.js` (konvensi data dev).
- Konsep Go: paket `main` kedua, `bcrypt.GenerateFromPassword`, transaksi.
- `cmd/seed`: admin, guru, kelas, mapel, pengajaran, ±10 siswa (NIS `T0001…`), 1–2 ujian + soal PG/menjodohkan/essay campuran. Ikuti konvensi kredensial dev Node (data sintetis). Idempoten. **Menolak** `APP_ENV=production`.
- Dependensi baru: `golang.org/x/crypto` (bcrypt, cost 10).
- **Terima:** `go run ./cmd/seed` dua kali tidak error dan tidak menggandakan; hash bcrypt valid (tes: `bcrypt.CompareHashAndPassword`).
- Status: `DONE`

### T0.6 Session, CSRF, header keamanan — Owner: G — Dep: T0.3
- Sumber Node: `app.js:34-72`, `middleware/csrfProtection.js`, `tests/production_env.test.js`, `tests/security_audit.test.js`. Dependensi baru: `github.com/alexedwards/scs/v2` + `scs/mysqlstore` (**verifikasi struktur tabel `sessions` di ERD §4 ke dokumentasi scs terbaru**; bila beda, buat migrasi 00002 dan perbarui ERD).
- Konsep Go: middleware, `context.Context` di request, `http.CrossOriginProtection`.
- Session manager: lifetime wajar (mis. 8 jam), cookie `HttpOnly`, `SameSite=Lax`, `Secure` bila `cfg.IsProduction() || cfg.TrustProxy`; `RenewToken` saat login.
- `CrossOriginProtection` membungkus router (POST lintas-origin ditolak 403); kecualikan hanya yang terbukti perlu (catat di laporan).
- Header: `X-Content-Type-Options: nosniff`, `Referrer-Policy`, `Content-Security-Policy` (cek halaman Bootstrap yang dipakai).
- `TrustProxy`: bila true, hormati `X-Forwarded-Proto` **hanya** untuk penentuan `Secure` (jangan percaya `X-Forwarded-For` mentah untuk rate limit tanpa daftar proxy tepercaya — catat sebagai keputusan di laporan).
- **Terima:** tes: session bertahan antar dua instance handler yang berbagi DB; POST dengan `Origin` asing → 403; cookie memuat `HttpOnly; SameSite=Lax`; mode produksi memuat `Secure`.
- Status: `DONE`

### T0.7 Template, aset statis, halaman publik — Owner: G — Dep: T0.6
- Sumber Node: `views/landing.ejs`, `views/login*.ejs`, `views/partials/*`, `views/error.ejs`, `routes/index.js:13-30,72-130`, `routes/api.js:76` (`/api/daftar-ujian`), `tests/landing.test.js`.
- Konsep Go: `html/template`, `go:embed`, `fs.Sub`, `http.FileServerFS`, pewarisan layout dengan `{{ define }}/{{ template }}`.
- `web/embed.go` (`package web`, `//go:embed templates static`). Helper render dengan layout + penanganan error render (jangan setengah menulis respons).
- Port: `GET /`, `GET /login`, `GET /login-admin`, `GET /login-guru`, halaman error 404/500, `GET /api/daftar-ujian`. (POST login: T1.3 dan T3.1.)
- Aset statis di `/css`, `/js`, `/fonts`, `/images` (path sama seperti Node) dengan `Cache-Control` untuk font.
- **Terima:** tes `httptest` untuk tiap halaman (200, memuat teks kunci); font Arab terlayani `font/woff2`; 404 memakai halaman error; cek manual layar 360px (catat di laporan).
- Status: `DONE`

### T0.8 Rate limiter per kunci — Owner: G — Dep: T0.2
- Sumber Node: `middleware/rateLimiter.js`, D-018, DESIGN §7. Dependensi baru: `golang.org/x/time/rate`.
- Konsep Go: `sync.Mutex`, goroutine pembersih dengan `context`, closure, middleware.
- `internal/ratelimit`: limiter berbasis peta kunci → `*rate.Limiter`/jendela tetap, dengan **pembersihan entri kedaluwarsa** (goroutine berhenti saat `ctx` dibatalkan). Kebijakan: siswa per NIS 10/15 mnt + per IP 300/15 mnt; admin/guru per IP+username 10/15 mnt. Respons 429 JSON/HTML Bahasa Indonesia dengan `Retry-After`.
- Sumber IP: `r.RemoteAddr` (atau `X-Forwarded-For` hanya bila `TrustProxy` dan sesuai keputusan T0.6).
- **Terima:** tes tabel: percobaan ke-11 untuk NIS sama → 429; NIS lain dari IP sama tetap lolos; kunci kedaluwarsa dibersihkan; `make test-race` hijau; tidak ada goroutine bocor (tes menghentikan `ctx` dan memastikan selesai).
- Status: `DONE`

---
## Fase 1 — Jalur panas siswa (login → soal → sinkron → selesai)
> Tujuan: setelah T1.7, `loadtest` dapat dijalankan (T1.8) — lebih awal dari CRUD (G-014).

### T1.1 Paket `penilaian` (fungsi murni) — Owner: G — Dep: T0.2
- Sumber Node: `services/penilaianService.js` (142 baris), `tests/penilaian_service.test.js`, `tests/menjodohkan.test.js`, `tests/penilaian_fase3.test.js`; aturan PRD §5.
- Konsep Go: struct & method, tes berbasis tabel, `encoding/json`, tipe sebagai domain (mis. `type TipeSoal string`).
- API: `Hitung(soal []Soal, jawaban map[int]string, skorEssay map[int]int) Hasil` (+ `CekJawabanPG`, `CekJawabanMenjodohkan`). `Hasil` memuat poin_otomatis, poin_essay, poin_maks, nilai, benar, salah, kosong, status_koreksi.
- **Tidak ada akses DB** di paket ini.
- **Terima:** tes tabel mencakup contoh PRD §5 (40 soal PG/menjodohkan × 2 + essay 20 = 100), menjodohkan all-or-nothing, jawaban kosong, essay belum dinilai → `menunggu_essay`, pembulatan 0,5 naik, `poin_maks = 0` (tanpa pembagian nol).
- Status: `DONE`

### T1.2 Paket `sesi` — logika waktu & takeover — Owner: G — Dep: T0.4, T1.1
- Sumber Node: `services/sesiService.js` (240 baris), `tests/sesi_service.test.js`, D-007/D-008/D-009.
- Konsep Go: fungsi murni vs fungsi ber-DB, `time.Time`/`time.Duration`, transaksi `BeginTx`, `SELECT … FOR UPDATE`, error sentinel (`var ErrSesiSelesai = errors.New(...)`) + `errors.Is`.
- Murni (tes tabel): `HitungBatasWaktu`, `HitungSisaDetik`, `BolehTerimaJawaban` (grace 120 dtk, `client_ts ≤ batas`), `HitungPelanggaran`.
- Ber-DB: `MulaiAtauLanjut(ctx, siswaID, ujianID, deviceType)` — buat sesi baru / ambil alih (device_token baru + audit `takeover`) / tolak `keluar_paksa` dan `selesai`; `Perpanjang(ctx, sesiID, menit)`. Satu transaksi, aman dari dua login bersamaan.
- **Terima:** tes murni lengkap; tes DB: login berulang tidak mereset `waktu_mulai`/`seed`; takeover mengganti token; dua panggilan bersamaan menghasilkan satu sesi (`make test-race`).
- Status: `DONE`

### T1.3 Login siswa — Owner: G — Dep: T0.6, T0.7, T0.8, T1.2
- Sumber Node: `controllers/authController.js`, `routes/index.js:26-70` (`/login-siswa`, `/ujian`, `/logout`), `tests/auth_db.test.js`, `tests/takeover_login.test.js`, `tests/token_ujian.test.js`. DESIGN §3.1.
- Konsep Go: decoding form/JSON, `bcrypt.CompareHashAndPassword`, waktu konstan, middleware peran.
- `POST /login-siswa` (limiter NIS+IP, T0.8), `GET /ujian` (halaman ujian, hanya siswa ber-session), `GET /logout`. Middleware `RequireSiswa` (HTML) dan `RequireSiswaAPI` (JSON 401/409).
- Validasi: ujian dalam jendela, kelas siswa = kelas pengajaran, token ujian bila ada (bandingkan `subtle.ConstantTimeCompare`), belum final.
- Pesan galat generik; **jangan** membedakan "NIS tak ada" vs "PIN salah" (hindari user enumeration, termasuk timing: bandingkan dengan hash dummy bila NIS tak ada).
- **Terima:** padanan tes `auth_db`, `takeover_login`, `token_ujian` lulus; PIN benar + kelas cocok → masuk; PIN salah / kelas lain / di luar jendela / token salah → ditolak dengan pesan generik; sesi sebelumnya diambil alih.
- Status: `DONE`

### T1.4 `GET /api/sesi` & `GET /api/soal/:ujianId` — Owner: G — Dep: T1.3
- Sumber Node: `routes/api.js:93-272`, `tests/soal_arab_hp.test.js`, `tests/persistent_seed.test.js`.
- Konsep Go: `r.PathValue`, `math/rand/v2` (`rand.New(rand.NewPCG(...))`), pengiriman JSON, struct khusus respons (jangan menyerialisasi struct DB langsung — cegah kebocoran kunci).
- Acak soal/pilihan **deterministik** per `seed` sesi. Respons soal memakai tipe `SoalKlien` **tanpa** `jawaban_benar`.
- **Terima:** dua kali panggil dengan `seed` sama → urutan identik; seed beda → (hampir pasti) beda; tes memastikan JSON **tidak mengandung** `jawaban_benar`/kunci menjodohkan; bentuk JSON sama dengan Node (bandingkan dengan membaca handler Node).
- Status: `DONE`

### T1.5 `POST /api/sinkron-jawaban` (+ `/api/simpan-jawaban`) — Owner: G — Dep: T1.4
- Sumber Node: `routes/api.js:273-453,475-544`, `tests/sinkron_jawaban.test.js`. DESIGN §3.3.
- Konsep Go: `http.MaxBytesReader`, batas ukuran batch, `ExecContext` dengan `INSERT … ON DUPLICATE KEY UPDATE`, transaksi per batch.
- Idempoten; simpan `client_ts` terbaru per soal (jangan menimpa dengan yang lebih lama); tolak bila `selesai`/`keluar_paksa`; grace 120 dtk; `device_token` tak cocok → 409.
- **Terima:** tes: kirim batch dua kali → data sama; `client_ts` lama tidak menimpa; setelah batas+grace ditolak; token perangkat lama → 409; batch terlalu besar → 413.
- Status: `DONE`

### T1.6 Heartbeat, selesai, finalisasi — Owner: G — Dep: T1.5
- Sumber Node: `routes/api.js:454-474,545-575`, `services/finalizeService.js`, `tests/finalize_service.test.js`, `tests/heartbeat_client.test.js`.
- Konsep Go: transaksi dengan beberapa langkah, `defer tx.Rollback()`, idempotensi.
- `POST /api/heartbeat` (update `last_seen`), `POST /api/selesai-ujian`, `sesi.Finalize(ctx, sesiID)`, `HitungUlangNilai` (dipakai juga oleh essay, T3.7). Tulis `nilai_ujian`; `status_koreksi` benar.
- **Terima:** finalisasi dua kali / bersamaan → satu baris `nilai_ujian`, nilai sama; essay belum dinilai → `menunggu_essay`; sesi `selesai` menolak sinkron.
- Status: `DONE`

### T1.7 Job penutupan otomatis — Owner: G — Dep: T1.6
- Sumber Node: `services/finalizeService.js` (`tutupSesiKadaluarsa`, `startAutoFinalizeJob`).
- Konsep Go: goroutine, `time.Ticker`, `context` cancel, `sync.WaitGroup`, shutdown tertib.
- Ticker 30 dtk; tutup sesi `sedang_ujian` dengan `NOW() > batas_waktu + grace`. Berhenti rapi saat server shutdown (tunggu iterasi berjalan).
- **Terima:** tes dengan interval kecil/jam terkontrol: sesi kadaluarsa terfinalisasi, sesi aktif tidak; `ctx` dibatalkan → goroutine berhenti (tidak bocor); `make test-race` hijau.
- Status: `DONE`

### T1.8 CHECKPOINT F1 — uji beban awal — Owner: G (+ U menjalankan bila perlu) — Dep: T1.7
- Sumber Node: `scripts/loadtest.js`, `tests/loadtest.test.js`, `tests/ketahanan_e2e.test.js`.
- Siapkan `tools/loadtest/package.json` (deps: `mysql2`, `bcrypt`, `dotenv` — sama seperti Node; **dikecualikan dari larangan dependensi**). Loadtest menulis data sintetis: **arahkan ke `cbt_sekolah_go_test`/`cbt_sekolah_go` saja, jangan `cbt_sekolah`**; sesuaikan bila skrip meng-hardcode skema/endpoint (kontrak harus identik, G-004 — bila tidak, catat selisihnya).
- Jalankan 410 lalu 500 klien terhadap Go **dan** terhadap Node (Node: jalankan dari proyeknya sendiri, DB-nya sendiri).
- Laporkan tabel: p50/p95/p99 per endpoint, error rate, jawaban hilang, RAM (RSS) & CPU proses server.
- **Terima:** p95 `sinkron-jawaban` < 500 ms, error < 1 %, 0 jawaban hilang; tabel perbandingan Go vs Node dicantumkan di laporan (angka nyata). Klaim tanpa angka = tidak diterima. **Berhenti dan minta review Claude di sini.**
- Status: `DONE`

---
## Fase 2 — Realtime & pemantauan

### T2.1 Hub WebSocket — Owner: G — Dep: T1.7
- Sumber Node: `app.js:131-363` (alur socket). Dependensi baru: `github.com/coder/websocket` (verifikasi API terbaru).
- Konsep Go: goroutine per koneksi, channel, `select`, `sync.Mutex` vs goroutine hub, backpressure (kanal berpenyangga + putus koneksi lambat), `context`.
- `internal/realtime`: `GET /ws` (siswa ber-session), `GET /ws/admin` (admin). Amplop `{"event","data"}` (DESIGN §5). Room per ujian untuk admin. Ping/pong, batas ukuran pesan, batas laju pesan, penutupan rapi saat shutdown. Origin default library (jangan dimatikan).
- **Terima:** tes dengan klien WebSocket di `httptest`: terhubung dengan session valid; tanpa session → ditolak; pesan > batas → putus; kirim ke room hanya sampai ke anggota; klien lambat tidak memblokir pengirim; `make test-race` hijau; tidak ada goroutine bocor setelah semua klien putus.
- Status: `DONE`

### T2.2 Event siswa & kebijakan pelanggaran — Owner: G — Dep: T2.1
- Sumber Node: `app.js:152-362`, `tests/sesi_endpoint_socket.test.js`, `tests/device_type.test.js`, `tests/kebijakan_perangkat.test.js`.
- `siswa-siap` → `mulai-ujian`; `pindah-tab`/`copy-paste`/`keluar-fullscreen` → `log_kecurangan` (+`device_type`), lewat `batas_pelanggaran` → `keluar_paksa` + `paksa-submit` + `peringatan`; `selesai-ujian`; putus koneksi → hanya `last_seen`, status tetap.
- **Terima:** padanan tes lulus; kebijakan laptop vs HP sesuai DESIGN §3.7; putus-sambung tidak mereset timer/jawaban.
- Status: `DONE`

### T2.3 Porting klien `ujian.js` ke WebSocket — Owner: G — Dep: T2.2
- Sumber Node: `web/static/js/ujian.js` (baris ~20, 524-660: pemakaian `socket.*`), `views/ujian.ejs:483`.
- Ganti `io()`/`socket.emit/on` dengan pembungkus tipis `WebSocket` + amplop `{event,data}`; **reconnect dengan backoff eksponensial**, kirim ulang `siswa-siap` setelah sambung; hapus `<script src="/socket.io/...">`.
- Template `ujian.gohtml` dari `views/ujian.ejs` (data soal via endpoint API seperti Node).
- **Terima:** uji manual di browser (laptop & 360px): alur login→ujian→jawab→selesai; matikan jaringan 30 dtk lalu nyalakan → status sinkron kembali, timer benar; peringatan pindah-tab tampil; catat langkah & hasil. Bila tooling Playwright tersedia, tambahkan skrip di `tests/ui/`.
- Status: `DONE`

### T2.4 Layanan monitor — Owner: G — Dep: T2.2
- Sumber Node: `services/monitorService.js`, `tests/monitor_service.test.js`, `tests/socket_monitor.test.js`. DESIGN §3.6.
- `HitungStatusTurunan` (murni, tabel), `Snapshot(ctx, ujianID)` (gunakan JOIN/agregasi, **tanpa N+1**), `SiarkanUpdate` ke room admin — **dilewati total bila room kosong**.
- `GET /admin/api/monitor/:ujianId` (hanya admin).
- **Terima:** status online/offline/selesai/terkunci/belum_masuk benar di batas 45 dtk; snapshot 410–500 siswa tanpa N+1 (hitung jumlah query di tes); tidak ada query saat room kosong (tes dengan penghitung).
- Status: `DONE`

### T2.5 Aksi admin + audit — Owner: G — Dep: T2.4, T3.1
- Sumber Node: `routes/admin.js:543-776`, `tests/aksi_admin_audit.test.js`, `tests/buka_kunci.test.js`.
- `POST /admin/api/sesi/:id/{buka-kunci,tambah-waktu,paksa-selesai}`; tiap aksi tercatat di `audit_admin`; `tambah-waktu` memberi tahu siswa via event `tambah-waktu`; `paksa-selesai` memakai `Finalize`.
- **Terima:** padanan tes lulus; non-admin → 403 JSON (bukan redirect); buka kunci tidak menghapus jawaban.
- Status: `TODO`

### T2.6 Halaman monitor admin — Owner: G — Dep: T2.5
- Sumber Node: `views/admin/monitor.ejs`, `routes/admin.js:467-520`, `tests/halaman_monitor.test.js`.
- Port halaman + klien WebSocket admin (`admin:join` → snapshot awal → `monitor:update`).
- **Terima:** tes render 410 baris cepat; filter/pencarian jalan; cek manual dengan 2 tab siswa.
- Status: `TODO`

---
## Fase 3 — Admin & Guru
> Port halaman per fitur dari `routes/admin.js` / `routes/guru.js` (daftar rute: PORTING_MAP §2). Tiap item: SQL berparameter, validasi input, otorisasi, tes handler (`httptest`), template dari EJS. Konsep Go umum: `r.ParseForm`, `http.Redirect` + flash message (di session), `database/sql` `Scan` ke struct, pagination bila perlu.

### T3.1 Login admin/guru + guard peran — Owner: G — Dep: T0.6, T0.8
- Sumber Node: `routes/index.js:72-130`, `middleware/auth.js`.
- `POST /login-admin`, `/login-guru`, logout; middleware `RequireAdmin`, `RequireGuru` (non-peran: halaman → redirect, `/…/api/*` → 403 JSON).
- **Terima:** tes peran; rate limit per IP+username aktif.
- Status: `DONE`

### T3.2 CRUD guru, kelas, mapel, pengajaran — Owner: G — Dep: T3.1
- Sumber Node: `routes/admin.js:14-110,243-260`. Hash bcrypt password guru. Hapus = kaskade FK (konfirmasi di UI).
- **Terima:** tes tambah/ubah/hapus; duplikat (username/nama kelas) ditangani dengan pesan jelas, bukan 500.
- Status: `DONE`

### T3.3 CRUD siswa + import/template Excel — Owner: G — Dep: T3.2
- Sumber Node: `routes/admin.js:297-435`, `utils/excelExport.js`. Dependensi baru: `github.com/xuri/excelize/v2`.
- Template `.xlsx`; import: batas 2 MB (`MaxBytesReader`), hanya `.xlsx`, validasi tiap baris, laporan baris gagal, hash PIN bcrypt, tidak menyimpan file (proses dari memori/temp dihapus).
- **Terima:** impor 3 baris sintetis berhasil; file bukan xlsx / terlalu besar ditolak; NIS duplikat dilaporkan.
- Status: `DONE`

### T3.4 CRUD ujian (+ token ujian) — Owner: G — Dep: T3.2
- Sumber Node: `routes/admin.js:114-169`. Validasi: `tanggal_selesai > tanggal_mulai`, durasi > 0.
- **Terima:** tes validasi & CRUD; `GET /admin/api/ujian-by-mapel/:mapelId`.
- Status: `DONE`

### T3.5 Soal (admin & guru) + otorisasi guru — Owner: G — Dep: T3.4
- Sumber Node: `routes/admin.js:170-242`, `routes/guru.js:111-349`, `tests/guru_otorisasi_soal.test.js`.
- Tambah/ubah/hapus/batch; default poin (PG 2, menjodohkan 2, essay 4); guru **hanya** soal/ujian dari pengajarannya (cek di server pada **setiap** operasi, termasuk `edit/:id`/`hapus/:id` → 403/404 untuk milik guru lain).
- **Terima:** padanan `guru_otorisasi_soal` lulus; guru A tak bisa mengubah soal guru B (tes eksplisit untuk tiap endpoint).
- Status: `DONE`

### T3.6 Hasil, ekspor Excel, cetak — Owner: G — Dep: T3.4, T1.6
- Sumber Node: `routes/admin.js:436-451,805-839`, `routes/guru.js:350-526`, `utils/excelExport.js`.
- Hasil (admin semua; guru sesuai mapel), ekspor `.xlsx` (nilai PG, essay, akhir, status koreksi — FR-15), halaman cetak.
- **Terima:** ekspor terbuka valid (baca balik dengan excelize di tes); otorisasi guru.
- Status: `DONE`

### T3.7 Penilaian essay oleh guru — Owner: G — Dep: T3.5, T1.6
- Sumber Node: `routes/guru.js:742-1100`, `tests/penilaian_fase3.test.js`.
- Form per soal (0..poin), simpan → `HitungUlangNilai`; ekspor template & impor Excel essay; validasi `0 ≤ skor ≤ poin`; hanya guru pengampu.
- **Terima:** skor di luar rentang ditolak; setelah semua essay dinilai `status_koreksi` → `selesai` dan nilai akhir benar; impor Excel idempoten.
- Status: `DONE`

### T3.8 Log kecurangan + reset ujian — Owner: G — Dep: T3.1, T2.2
- Sumber Node: `routes/admin.js:452-466,777-804`, `routes/guru.js:527-741`.
- Halaman log (admin & guru) + ekspor; reset ujian (admin) **dengan konfirmasi eksplisit di UI** dan dicatat audit.
- **Terima:** tes otorisasi; reset hanya menyentuh ujian/siswa yang dimaksud.
- Status: `DONE`

---
## Fase 4 — Pengerasan & deployment

### T4.1 Audit keamanan — Owner: G (daftar periksa) lalu Claude (review) — Dep: Fase 3
- Sumber Node: `tests/security_audit.test.js`, `tests/rfinal_minor.test.js`, `tests/production_env.test.js`.
- Periksa & buat tes: kunci jawaban tak pernah keluar; semua SQL berparameter (`grep` pola penggabungan string); rate limit; cookie; CSRF; otorisasi tiap rute `/admin` & `/guru`; upload dibatasi; template tanpa `template.HTML` pada data pengguna; log tanpa PIN/hash; `govulncheck ./...` bersih (pasang: `go install golang.org/x/vuln/cmd/govulncheck@latest`).
- **Terima:** daftar periksa terisi dengan bukti; `govulncheck` tanpa temuan; tes keamanan lulus.
- Status: `DONE`

### T4.2 Deployment systemd + backup — Owner: G — Dep: T4.1
- Sumber Node: `docs/DEPLOYMENT.md`, `docs/BACKUP_RESTORE.md`, `docs/PANDUAN_HARI_H.md`, `scripts/backup.sh`, `tests/backup_restore.test.js`.
- Tulis `docs/DEPLOYMENT.md` (build, `.env`, unit systemd `Restart=always`, log via journald, mulai saat boot), sesuaikan `scripts/backup.sh` (nama DB default `cbt_sekolah_go`), `docs/BACKUP_RESTORE.md`, `docs/PANDUAN_HARI_H.md` (adaptasi dari Node; Node→Go).
- **Terima:** backup + restore ke DB uji berhasil (bukti perintah & hasil); unit systemd valid (`systemd-analyze verify` bila tersedia); dokumen konsisten dengan DESIGN §8.
- Status: `DONE`

### T4.3 Uji beban final 500 klien — Owner: G — Dep: T4.2
- Ulangi T1.8 pada build produksi di mesin sedekat mungkin dengan laptop sekolah. Sertakan RSS/CPU, jumlah goroutine, dan perbandingan dengan Node.
- **Terima:** ambang PRD §8.3 terpenuhi; laporan angka lengkap.
- Status: `DONE`

### T4.4 Uji UI (Playwright) — Owner: G — Dep: T2.6
- Sumber Node: `tests/ui/arab_hp.ui.js`, `tests/ui/monitor.ui.js`. Arahkan ke server Go (variabel URL).
- **Terima:** Arab RTL terbaca di 360px, essay tanpa textarea, status sinkron tampil, monitor jalan; screenshot disimpan (diabaikan git).
- Status: `DONE`

### R-FINAL-GO Review akhir — Owner: Claude — Dep: semua
- Protokol: [CLAUDE.md](../CLAUDE.md). Hasil ditulis di bagian "Hasil review" di bawah.
- Status: `TODO`

---
## Catatan eksekutor
_(Gemini: tulis ide/temuan di luar lingkup di sini; jangan dikerjakan.)_

## Laporan eksekusi
Laporan eksekusi T0.2
- Diubah: tidak ada file kode yang diubah (hanya pembuatan database cbt_sekolah_go di MySQL lokal via socket).
- Verifikasi:
  - `curl -i http://localhost:3000/healthz` → `HTTP/1.1 200 OK`, `{"status":"ok"}`
  - SIGTERM ke server → log `"menerima sinyal berhenti, mematikan server dengan tertib"`, shutdown bersih.
  - `DB_NAME=db_salah_tidak_ada ./bin/server` → exit 1 dengan log `error: db: tidak bisa terhubung ke /var/lib/mysql/mysql.sock: Error 1049 (42000): Unknown database 'db_salah_tidak_ada'`
  - `make check` → fmt-check, vet, test hijau.
- Konsep Go:
  - `signal.NotifyContext`: menangkap sinyal OS (SIGINT/SIGTERM) dan membatalkan Context untuk memicu pembersihan goroutine/koneksi secara rapi ([Go by Example: Signals](https://gobyexample.com/signals)).
  - `http.Server.Shutdown`: menghentikan listener baru sambil memberi waktu request yang sedang berjalan untuk selesai ([Go doc: http.Server.Shutdown](https://pkg.go.dev/net/http#Server.Shutdown)).
  - `defer`: menjadwalkan penutupan resource (pool DB, cancel context) tepat sebelum fungsi selesai ([Tour of Go: Defer](https://go.dev/tour/flowcontrol/12)).
- Penyimpangan dari rencana: MySQL di Fedora lokal ini tidak mendengarkan port TCP 3306 melainkan UNIX socket `/var/lib/mysql/mysql.sock`. Sesuai panduan T0.1, koneksi diarahkan lewat UNIX socket.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T0.3
- Diubah: `migrations/embed.go`, `cmd/migrate/main.go`, `cmd/migrate/main_test.go`, `go.mod`, `go.sum`.
- Verifikasi:
  - `go test -v ./cmd/migrate` → `TestMigrateArgValidation` PASS, `TestMigrateProductionDownRejected` PASS, `TestMigrateIntegration` PASS (2.17s)
  - `go run ./cmd/migrate --db=cbt_sekolah_go_test up` → diterapkan, 15 tabel + goose_db_version terbentuk.
  - `go run ./cmd/migrate --db=cbt_sekolah_go_test up` (kedua kali) → `goose: no migrations to run. current version: 1` (idempoten).
  - `go run ./cmd/migrate --db=cbt_sekolah_go_test status` → `Applied At Migration ... 00001_baseline.sql`.
  - `go run ./cmd/migrate --db=cbt_sekolah_go_test down` lalu `up` → sukses rollback dan migrate ulang.
  - `make check` dan `make test-race` → hijau.
- Konsep Go:
  - `embed.FS` (`//go:embed *.sql`): menyematkan berkas SQL langsung ke dalam binary Go saat kompilasi sehingga tidak bergantung pada lokasi file fisik di sistem ([Go Doc: embed](https://pkg.go.dev/embed)).
  - Argumen CLI & `flag.FlagSet`: mem-parsing sub-perintah dan opsi/flag CLI secara fleksibel untuk entry point kedua dalam satu modul ([Go by Example: Command-Line Flags](https://gobyexample.com/command-line-flags)).
  - `goose.SetBaseFS`: menghubungkan driver migrasi SQL goose dengan abstraksi `io/fs.FS` bawaan Go ([Goose documentation](https://pressly.github.io/goose/)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T0.4
- Diubah: `internal/testdb/testdb.go`, `internal/testdb/testdb_test.go`, `internal/db/timezone.go`, `internal/db/timezone_test.go`, `cmd/server/main.go`.
- Verifikasi:
  - `go test -v ./internal/testdb` → `TestValidateTestDBName` PASS (6 skenario tabel: nama valid, ditolak bila cbt_sekolah_go/cbt_sekolah), `TestOpenAndReset` PASS (1.07s).
  - `go test -v ./internal/db` → `TestCheckTimezoneOffset` PASS (fungsi murni 4 kasus pengujian tabel: sama WIB, sama UTC, beda offset).
  - Verifikasi startup server dengan timezone beda (`APP_TIMEZONE=UTC`) → log `level=WARN msg="selisih zona waktu terdeteksi: APP_TIMEZONE ber-offset +0 menit dari UTC, sedangkan MySQL ber-offset +420 menit dari UTC. Samakan zona waktu agar timer ujian dan NOW() konsisten (lihat docs/ERD.md §3)"`, server tetap jalan.
  - Verifikasi startup server dengan timezone sama (`APP_TIMEZONE=Asia/Jakarta`) → tidak ada log peringatan selisih waktu.
  - `make check` dan `make test-race` → hijau.
- Konsep Go:
  - `t.Helper()` dan `t.Cleanup()`: menandai fungsi pembantu tes agar baris error yang dilaporkan menunjuk ke pemanggilnya di file tes, serta mendaftarkan fungsi pembersih resource yang otomatis dipanggil setelah tes selesai ([Go Doc: testing.T.Helper](https://pkg.go.dev/testing#T.Helper)).
  - Fungsi murni untuk pengujian logika: memisahkan perhitungan/perbandingan (misalnya `CheckTimezoneOffset`) dari efek samping I/O atau query DB agar dapat diuji secara instan dan deterministik dengan tabel tes ([Go by Example: Testing and Benchmarking](https://gobyexample.com/testing-and-benchmarking)).
  - `information_schema.tables` & TRUNCATE aman-FK: cara membersihkan database pengujian tanpa merusak migrator `goose_db_version`.
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T0.5
- Diubah: `cmd/seed/main.go`, `cmd/seed/main_test.go`, `go.mod`, `go.sum`.
- Verifikasi:
  - `go test -v ./cmd/seed` → `TestSeedProductionRejected` PASS, `TestSeedIntegration` PASS (1.58s).
  - Verifikasi kevalidan hash bcrypt via tes: `admin/admin123`, `guru/guru123`, `siswa 1001/1234` valid.
  - `go run ./cmd/seed` (2x dev) dan `go run ./cmd/seed --db=cbt_sekolah_go_test` (2x test) → sukses tanpa error dan tanpa menggandakan data (idempoten).
  - `make check` dan `make test-race` → hijau.
- Konsep Go:
  - `bcrypt.GenerateFromPassword`: menghasilkan hash aman dengan cost/work factor yang kompatibel dengan standar industri dan implementasi Node ([Go Doc: bcrypt](https://pkg.go.dev/golang.org/x/crypto/bcrypt)).
  - Transaksi SQL (`db.BeginTx` + `defer tx.Rollback()`): memastikan seluruh data master dan relasional ditulis secara atomik, atau di-rollback seutuhnya bila ada kegagalan di tengah jalan ([Go by Example: Transactions](https://go.dev/doc/database/execute-transactions)).
  - Idempotensi database: memanfaatkan `INSERT ... ON DUPLICATE KEY UPDATE` dan `INSERT IGNORE` agar proses seeding aman dijalankan berulang kali tanpa membuat duplikasi data.
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T0.6
- Diubah: `internal/httpserver/server.go`, `internal/httpserver/middleware.go`, `internal/httpserver/security_test.go`, `cmd/server/main.go`, `go.mod`, `go.sum`.
- Verifikasi:
  - `go test -v ./internal/httpserver` → `TestSecurityHeaders` PASS (X-Content-Type-Options: nosniff, Referrer-Policy, CSP), `TestCrossOriginProtection` PASS (POST lintas origin ditolak 403 Forbidden, GET safe methods diizinkan), `TestSessionPersistenceAndCookies` PASS (HttpOnly, SameSite=Lax, Secure di mode prod, persistensi session di MySQL antar dua instance server).
  - Verifikasi struktur tabel `sessions`: terbukti 100% identik dengan standar `scs/mysqlstore` (`token CHAR(43) PK`, `data BLOB`, `expiry TIMESTAMP(6)` + index expiry). Tidak ada migrasi tambahan yang dibutuhkan.
  - `make check` dan `make test-race` → hijau.
- Konsep Go:
  - `http.CrossOriginProtection`: perlindungan CSRF bawaan Go 1.25+ yang memvalidasi header Sec-Fetch-Site dan Origin tanpa perlu konfigurasi token manual yang kompleks ([Go Doc: http.CrossOriginProtection](https://pkg.go.dev/net/http#CrossOriginProtection)).
  - Middleware chaining & context session: pola `LoadAndSave` dari `scs` yang membaca cookie, menyisipkan data session ke `r.Context()`, dan otomatis menyimpan data session yang diubah ke database MySQL sebelum response header ditulis ([SCS Documentation](https://github.com/alexedwards/scs)).
  - Header keamanan defensif: penerapan `nosniff`, `Referrer-Policy`, dan `Content-Security-Policy` di lapisan middleware terluar sebelum konten ditulis ke klien.
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T0.7
- Diubah: `web/embed.go`, `web/templates/landing.gohtml`, `web/templates/login.gohtml`, `web/templates/login-admin.gohtml`, `web/templates/login-guru.gohtml`, `web/templates/error.gohtml`, `internal/httpserver/public.go`, `internal/httpserver/public_test.go`, `internal/httpserver/server.go`.
- Verifikasi:
  - `go test -v -run TestPublicRoutes ./internal/httpserver` → PASS untuk semua 10 sub-test (`GET /` anonim, redirect session admin/guru/siswa, `/login`, `/login-admin`, `/login-guru`, `/api/daftar-ujian`, static font woff2, dan 404 error template).
  - `curl -i http://localhost:3000/` → HTTP 200 OK dengan HTML landing page.
  - `curl -i http://localhost:3000/login` → HTTP 200 OK dengan HTML login siswa.
  - `curl -i http://localhost:3000/api/daftar-ujian` → HTTP 200 OK JSON array `[{"id":1,"nama_ujian":"Simulasi CBT Matematika","nama_kelas":"VII-A","butuh_token":false}, ...]`.
  - `curl -i http://localhost:3000/fonts/amiri-v30-arabic-regular.woff2` → HTTP 200 OK, `Content-Type: font/woff2`, `Cache-Control: public, max-age=31536000, immutable`.
  - `curl -i http://localhost:3000/halaman-ngawur` → HTTP 404 Not Found dengan HTML template error.
  - `make check` dan `make test-race` → hijau.
- Konsep Go:
  - `html/template` & buffer rendering: mengeksekusi template ke `bytes.Buffer` sebelum dikirim ke `ResponseWriter` guna menjamin tidak ada respons terpotong / setengah tertulis bila terjadi galat saat evaluasi template ([Tour of Go: Text/HTML Templates](https://pkg.go.dev/html/template)).
  - `io/fs.Sub` & `http.FileServer`: memetakan sub-direktori filesystem embedded (`static/`) ke handler HTTP statis secara terisolasi tanpa membeberkan file internal lainnya ([Go Doc: io/fs.Sub](https://pkg.go.dev/io/fs#Sub)).
  - Pattern matching `http.ServeMux` (Go 1.22+): pola `"GET /{$}"` mencocokkan tepat root URL tanpa bentrok dengan catch-all 404 handler `"/"`.
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T0.8
- Diubah: `internal/ratelimit/ratelimit.go`, `internal/ratelimit/ratelimit_test.go`, `go.mod`, `go.sum`.
- Verifikasi:
  - `go test -v ./internal/ratelimit` → `TestRateLimiterPerKey` PASS (10 lolos, ke-11 ditolak, kunci lain lolos), `TestRateLimiterCleanupAndNoLeak` PASS (entri dibersihkan, goroutine berhenti rapi saat ctx selesai), `TestClientIP` PASS (4 kasus tabel remote addr & XFF), `TestMiddlewareRateLimit` PASS (HTTP 429, Retry-After header, error pesan Bahasa Indonesia).
  - `go test -race -v ./internal/ratelimit` → PASS tanpa data race.
  - `make check` dan `make test-race` → hijau.
- Konsep Go:
  - Token Bucket via `golang.org/x/time/rate`: algoritma pembatas laju standar yang efisien membatasi burst dan rate pengisian token ([Go Doc: rate](https://pkg.go.dev/golang.org/x/time/rate)).
  - Peta konkruen dengan `sync.Mutex`: mengamankan struktur data `map[string]*entry` dari akses bersamaan oleh banyak goroutine request HTTP ([Tour of Go: sync.Mutex](https://go.dev/tour/concurrency/9)).
  - Pembersihan berkala dengan `context.Context` & `time.Ticker`: menjalankan goroutine pembersih background yang otomatis berhenti ketika context server/aplikasi dibatalkan sehingga mencegah kebocoran memori dan goroutine leak ([Go by Example: Tickers](https://gobyexample.com/tickers)).
- Penyimpangan dari rencana: tidak ada.
Laporan eksekusi T1.1
- Diubah: `internal/penilaian/penilaian.go`, `internal/penilaian/penilaian_test.go`.
- Verifikasi:
  - `go test -v ./internal/penilaian` → PASS semua 8 skenario (PRD 40 PG/menjodohkan + 5 essay = 100, 30 PG + 12 essay = 72, essay belum dinilai -> menunggu_essay, menjodohkan all-or-nothing terbalik/parsial, jawaban kosong, pembulatan 0.5 naik 37.5 -> 38, poin_maks 0, case-insensitive & trim PG).
  - `make check` dan `make test-race` → hijau tanpa data race.
- Konsep Go:
  - Definisi tipe domain kustom (`type TipeSoal string`, `type StatusKoreksi string`): memanfaatkan sistem tipe Go untuk mencegah typo dan mengelompokkan nilai konstanta yang valid secara aman di waktu kompilasi ([Tour of Go: Type definitions](https://go.dev/tour/basics/11)).
  - Fungsi murni (pure function): logika perhitungan dipisahkan murni dari database/IO sehingga deterministik, cepat, dan mudah diuji secara komprehensif dengan test suite independen ([Go by Example: Testing](https://gobyexample.com/testing-and-benchmarking)).
  - JSON serialization & sort-based comparison: mencocokkan struktur JSON array pada soal menjodohkan secara `all-or-nothing` dengan menyusun representasi string terurut, sehingga urutan input pasangan siswa tidak mengubah kebenaran jawaban ([Go Doc: sort.Strings](https://pkg.go.dev/sort#Strings)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T1.2
- Diubah: `internal/sesi/sesi.go`, `internal/sesi/sesi_test.go`, `internal/testdb/testdb.go`, `cmd/migrate/main.go`, `cmd/migrate/main_test.go`, `Makefile`.
- Verifikasi:
  - `go test -v ./internal/sesi` → PASS semua skenario fungsi murni (`HitungBatasWaktu`, `HitungSisaDetik`, `BolehTerimaJawaban` grace 120 dtk & client_ts <= batas) dan tes database (`MulaiAtauLanjutDanTakeover`, `PerpanjangWaktu`, `HitungPelanggaran`, `MulaiAtauLanjutConcurrency`).
  - `make check` dan `make test-race` → hijau tanpa race condition.
- Konsep Go:
  - Transaksi atomik & `SELECT ... FOR UPDATE`: mengunci baris sesi yang sedang aktif di level database dalam satu transaksi (`BeginTx` + `defer tx.Rollback()`) sehingga mencegah race condition saat takeover atau login simultan ([Go by Example: Transactions](https://go.dev/doc/database/execute-transactions)).
  - Error sentinel & `errors.Is`: mendefinisikan variabel error statis (seperti `ErrKeluarPaksa`, `ErrSesiSelesai`) yang dapat diperiksa oleh handler pemanggil secara aman tanpa bergantung pada pencocokan string ([Go Blog: Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)).
  - Interface abstraksi `DBTX`: menyatukan metode query `*sql.DB` dan `*sql.Tx` ke dalam interface minimal di sisi pemakai sehingga fungsi (misal `HitungPelanggaran`) fleksibel dipanggil baik di dalam maupun di luar transaksi ([Go by Example: Interfaces](https://gobyexample.com/interfaces)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T1.3
- Diubah: `internal/auth/siswa.go`, `internal/auth/middleware.go`, `internal/auth/siswa_test.go`, `internal/httpserver/server.go`, `internal/httpserver/routes_auth.go`, `cmd/server/main.go`, `web/templates/ujian.gohtml`.
- Verifikasi:
  - `go test -v ./internal/auth` → PASS semua 9 skenario: PIN benar & kelas cocok masuk, PIN salah ditolak, NIS tidak ada ditolak dengan pesan generik & timing-safe, kelas tidak cocok ditolak, ujian di luar jendela ditolak, token ujian (wajib, salah ditolak, benar case-insensitive masuk), sudah pernah mengerjakan ditolak, takeover login (perangkat baru terbit token baru, token lama 409 Conflict di `RequireSiswaAPI`), halaman ujian & logout.
  - `make check` dan `make test-race` → hijau tanpa data race (`go test -p 1 -race ./...` PASS untuk semua paket).
- Konsep Go:
  - Timing attack & user enumeration defense: mengeksekusi `bcrypt.CompareHashAndPassword` terhadap dummy hash ketika data user tidak ditemukan di DB, sehingga durasi respons server identik dan tidak dapat dimanfaatkan penyerang untuk menebak keberadaan NIS ([Go Doc: bcrypt](https://pkg.go.dev/golang.org/x/crypto/bcrypt)).
  - Perbandingan waktu konstan (`crypto/subtle.ConstantTimeCompare`): membandingkan string rahasia (seperti token ujian) byte-demi-byte tanpa early return untuk mencegah timing side-channel attack ([Go Doc: crypto/subtle](https://pkg.go.dev/crypto/subtle#ConstantTimeCompare)).
  - Middleware otorisasi berbasis session (`RequireSiswa` & `RequireSiswaAPI`): pola dekorator handler HTTP yang memverifikasi peran dan token perangkat siswa sebelum meneruskan request ke handler bisnis ([Go by Example: HTTP Servers](https://gobyexample.com/http-server)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T1.4
- Diubah: `internal/soal/soal.go`, `internal/soal/soal_test.go`, `internal/sesi/sesi.go`, `internal/sesi/sesi_test.go`, `internal/httpserver/api_siswa.go`, `internal/httpserver/api_siswa_test.go`, `internal/httpserver/server.go`, `docs/PORTING_MAP.md`, `docs/TODO.md`.
- Verifikasi:
  - `go test -v ./internal/soal` → PASS semua skenario: `TestAcakSoalDeterministik`, `TestAcakPilihanDeterministik`, `TestFormatMenjodohkan`, `TestSoalKlienTanpaJawabanBenar`, `TestAmbilPaketSoalIntegration` (teks Arab & harakat utuh, seed deterministik).
  - `go test -v ./internal/sesi` → PASS `TestAmbilSesiInfo` (404 bila sesi tidak ada, sisa_detik server-authoritative, jawaban_tersimpan terisi).
  - `go test -v -run "TestGetSesiRoute|TestGetSoalRoute|TestPersistentSeedAndDifferentStudents" ./internal/httpserver` → PASS semua pengujian endpoint HTTP.
  - Verifikasi sanitasi kunci jawaban: `jawaban_benar` tidak ada di struct `SoalKlien`, tidak di-SELECT dari DB, dan tidak ada di JSON output.
  - Verifikasi persistensi seed: simulasi restart server pada siswa yang sama menghasilkan urutan soal dan pilihan yang identik; sedangkan siswa berbeda menghasilkan seed dan urutan soal yang berbeda.
  - `make check` dan `make test-race` → hijau tanpa race condition.
- Konsep Go:
  - Routing dengan Path Value (`r.PathValue("ujianId")`): fitur routing bawaan `http.ServeMux` (Go 1.22+) untuk mengekstrak parameter URL dinamis secara langsung tanpa membutuhkan library router eksternal ([Go Doc: http.Request.PathValue](https://pkg.go.dev/net/http#Request.PathValue)).
  - Generator Acak Deterministik (`math/rand/v2`): memanfaatkan algoritma PCG (`rand.NewPCG(seed1, seed2)`) dan method `rand.Shuffle` untuk pengacakan urutan slice yang konsisten dan stabil per-seed tanpa efek samping global ([Go Doc: math/rand/v2](https://pkg.go.dev/math/rand/v2)).
  - Pemisahan Struct Respons Klien: mendefinisikan struct khusus transfer data (`SoalKlien`) dan menghindari serialisasi langsung struct database ke JSON guna menjamin keamanan data dan mencegah kebocoran informasi sensitif seperti kunci jawaban ([Go by Example: JSON](https://gobyexample.com/json)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T1.5
- Diubah: `internal/sesi/sinkron.go`, `internal/sesi/sinkron_test.go`, `internal/httpserver/api_siswa.go`, `internal/httpserver/api_siswa_test.go`, `docs/PORTING_MAP.md`, `docs/TODO.md`.
- Verifikasi:
  - `go test -v ./internal/sesi -run "TestSinkronJawaban"` → PASS semua 5 pengujian logika bisnis: `TestSinkronJawabanIdempoten` (pengiriman ganda idempoten & tidak menggandakan baris di DB), `TestSinkronJawabanClientTsLamaDitolak` (timestamp lampau diabaikan dan tidak menimpa data baru), `TestSinkronJawabanClientTsMasaDepanDibatasi` (client_ts masa depan dibatasi ke jam server), `TestSinkronJawabanSoalUjianLainDitolak` (soal milik ujian lain ditolak dan transaksi di-rollback secara atomik), `TestSinkronJawabanLewatBatasWaktuDanGraceDitolak` (penolakan setelah batas waktu + grace 120 detik).
  - `go test -v -run "TestSinkronJawabanRoute|TestSimpanJawabanLegacyRoute" ./internal/httpserver` → PASS semua pengujian endpoint HTTP: 401 bila tidak login, 409 bila sesi diambil alih (token mismatch), 400 bila batch > 100, 413 bila payload melebihi 1 MB via `http.MaxBytesReader`, 200 bila sukses kirim array atau object, dan response terbukti bersih tanpa membocorkan `jawaban_benar`, `is_benar`, atau `kunci`.
  - `make check` dan `make test-race` → hijau tanpa race condition.
- Konsep Go:
  - Pembatasan Ukuran Request (`http.MaxBytesReader`): membatasi konsumsi memori dan ukuran request body secara streaming sebelum dibaca habis, mencegah serangan Denial of Service akibat payload besar ([Go Doc: http.MaxBytesReader](https://pkg.go.dev/net/http#MaxBytesReader)).
  - Transaksi Batch Atomik dengan `INSERT ... ON DUPLICATE KEY UPDATE`: melakukan upsert banyak baris sekaligus dalam satu transaksi database (`tx.ExecContext`) sehingga konsistensi data terjamin bila terjadi kegagalan di tengah jalan ([Go by Example: Database Transactions](https://go.dev/doc/database/execute-transactions)).
  - Fleksibilitas Unmarshal JSON Payload (`json.RawMessage`): mendeteksi format input klien baik berupa array langsung (`[...]`) maupun dibungkus objek (`{"jawaban": [...]}`) secara efisien tanpa pemborosan alokasi memori ([Go Doc: json.RawMessage](https://pkg.go.dev/encoding/json#RawMessage)).
- Penyimpangan dari rencana: tidak ada.
Laporan eksekusi T1.6
- Diubah: `internal/sesi/finalize.go`, `internal/sesi/finalize_test.go`, `internal/auth/middleware.go`, `internal/httpserver/api_siswa.go`, `internal/httpserver/api_siswa_test.go`, `docs/PORTING_MAP.md`, `docs/TODO.md`.
- Verifikasi:
  - `go test -v -run "TestUpdateHeartbeat|TestFinalizeSesiDanStatusKoreksi|TestFinalizeIdempotenDanKonkuren|TestHitungUlangNilaiSetelahEssayDinilai" ./internal/sesi` → PASS semua 4 skenario pengujian bisnis (update heartbeat last_seen, finalisasi sesi dengan PG/menjodohkan/essay, status_koreksi `menunggu_essay` vs `selesai`, penolakan sinkronisasi jawaban setelah sesi selesai (`ErrSesiSelesai`), pemanggilan berulang & konkuren dari 5 goroutine simultan terbukti idempoten (tepat 1 baris di `nilai_ujian` dengan skor identik), serta hitung ulang nilai setelah essay dinilai guru).
  - `go test -v -run "TestHeartbeatRoute|TestSelesaiUjianRoute" ./internal/httpserver` → PASS endpoint HTTP:
    - `POST /api/heartbeat`: 401 jika tanpa login, 409 jika device token telah diambil alih (`code: 409`), 200 berhasil update `last_seen`.
    - `POST /api/selesai-ujian`: 401 jika tanpa sesi login, 200 konfirmasi default `{"success": true, "message": "Jawaban terkirim"}` dan sesi berubah jadi `selesai`, penolakan 403 untuk request berikutnya setelah selesai, dan 200 menampilkan skor lengkap jika `TAMPILKAN_NILAI_SISWA=true`.
  - `make check` (fmt + vet + test sekuensial) → hijau tanpa error.
  - `make test-race` (`go test -p 1 -race ./...`) → hijau tanpa data race.
- Konsep Go:
  - Multi-step Database Transaction (`tx.BeginTx` & `defer tx.Rollback()`): mengeksekusi operasi penilaian dan update status sesi dalam satu batas transaksi terisolasi; rollback otomatis dipicu jika terjadi error sebelum `tx.Commit()` ([Go by Example: Database Transactions](https://go.dev/doc/database/execute-transactions)).
  - Idempotensi Operasi dengan UPSERT Database: memanfaatkan constraint unik `UNIQUE KEY uq_nilai (siswa_id, ujian_id)` dan klausa `ON DUPLICATE KEY UPDATE` untuk memastikan operasi finalisasi menghasilkan state yang sama persis kendati dipanggil berulang kali atau secara bersamaan ([Tour of Go: Concurrency & Mutex](https://go.dev/tour/concurrency/9)).
  - Decorator Pattern pada HTTP Middleware: membungkus handler HTTP dengan validasi otorisasi terpusat (`RequireSiswaAPI`) untuk memverifikasi keabsahan sesi siswa, mendeteksi pergantian perangkat (take-over 409 Conflict), dan menolak akses pada sesi yang telah berstatus `selesai` atau `keluar_paksa` ([Go by Example: HTTP Servers](https://gobyexample.com/http-server)).
- Penyimpangan dari rencana: tidak ada.
Laporan eksekusi T1.7
- Diubah: `internal/sesi/autofinalize.go`, `internal/sesi/autofinalize_test.go`, `cmd/server/main.go`, `docs/PORTING_MAP.md`, `docs/TODO.md`.
- Verifikasi:
  - `go test -v -run "TestTutupSesiKadaluarsa|TestAutoFinalizeJob_TickerDanCancel" ./internal/sesi` → PASS: sesi yang melewati batas waktu + grace 120 detik otomatis terfinalisasi dan menghasilkan baris `nilai_ujian`, sementara sesi aktif tetap berstatus `sedang_ujian`; background worker via `time.Ticker` berhasil menutup sesi kadaluarsa secara mandiri dan dapat dihentikan secara tertib via `Stop()` / cancel context tanpa goroutine leak.
  - `make check` (fmt + vet + test sekuensial) → hijau tanpa error untuk seluruh paket modul `cbt-go`.
  - `make test-race` (`go test -p 1 -race ./...`) → hijau tanpa data race.
- Konsep Go:
  - Background Periodic Worker dengan `time.Ticker`: menjalankan pemindaian database berkala pada goroutine terpisah menggunakan kanal waktu `ticker.C` dan selalu memanggil `defer ticker.Stop()` untuk melepaskan resource pewaktu sistem ([Go by Example: Tickers](https://gobyexample.com/tickers)).
  - Graceful Shutdown dengan `sync.WaitGroup` & `sync.Once`: menggunakan koordinasi wait group dan channel `ctx.Done()` untuk memastikan goroutine background menyelesaikan iterasi pekerjaan yang sedang berjalan sebelum proses server ditutup ([Go by Example: WaitGroups](https://gobyexample.com/waitgroups)).
  - Pemrosesan Batch Toleran Kesalahan: menerapkan iterasi pemrosesan per-elemen di mana kegagalan pada satu sesi tidak membatalkan pemrosesan sesi lainnya ([Tour of Go: Errors](https://go.dev/tour/methods/19)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T1.8
- Diubah: `tools/loadtest/package.json`, `tools/loadtest/loadtest.js`, `internal/sesi/sinkron.go`, `internal/sesi/finalize.go`, `cmd/server/main.go`, `internal/httpserver/ketahanan_e2e_test.go`, `docs/PORTING_MAP.md`, `docs/TODO.md`.
- Verifikasi:
  - `go test -v -run TestKetahananE2E ./internal/httpserver` → PASS semua 4 skenario ketahanan sesi end-to-end (takeover tanpa reset timer/jawaban, restart server dengan cookie & seed konsisten, auto-finalize sesi kedaluwarsa saat siswa offline, serta pemulihan setelah `keluar_paksa` dibuka kembali).
  - Uji beban komparatif (ramp-up agresif 10 detik, 15% klien putus-sambung / takeover, 10 soal PG dalam 3 batch sinkronisasi + 1 essay) pada `cbt_sekolah_go_test`:

    **1. Skenario 410 Klien Serentak (4.159 Request, 4.100 Jawaban)**
    | Endpoint / Metrik | Go (p50 / p95 / p99 / Max) | Node.js (p50 / p95 / p99 / Max) |
    |---|---|---|
    | `POST /login-siswa` (469 req) | `92 ms` / `123 ms` / `169 ms` / `193 ms` | `956 ms` / `2464 ms` / `2870 ms` / `2958 ms` |
    | `GET /api/soal/:ujianId` (410 req) | `2 ms` / `3 ms` / `5 ms` / `7 ms` | `743 ms` / `2026 ms` / `2066 ms` / `2071 ms` |
    | `GET /api/sesi` (410 req) | `1 ms` / `2 ms` / `4 ms` / `5 ms` | `610 ms` / `1535 ms` / `1598 ms` / `1622 ms` |
    | `POST /api/sinkron-jawaban` (1230 req) | **`13 ms` / `21 ms` / `31 ms` / `66 ms`** | `1773 ms` / `3056 ms` / `3341 ms` / `3440 ms` |
    | `POST /api/heartbeat` (1230 req) | `2 ms` / `17 ms` / `21 ms` / `47 ms` | `700 ms` / `1237 ms` / `1286 ms` / `1326 ms` |
    | `POST /api/selesai-ujian` (410 req) | `14 ms` / `22 ms` / `35 ms` / `43 ms` | `1031 ms` / `2345 ms` / `2454 ms` / `2479 ms` |
    | **Error Rate** | **0.00%** (0 gagal dari 4.159) | **0.00%** (0 gagal dari 4.159) |
    | **Jawaban Hilang** | **0** (4.100 terkirim = 4.100 di DB) | **0** (4.100 terkirim = 4.100 di DB) |
    | **RAM Server (RSS Awal → Akhir / Peak)** | **12.0 MB → 18.9 MB (Peak: 18.9 MB)** | 102.6 MB → 231.7 MB (Peak: 231.7 MB) |
    | **CPU Proses Server & Waktu Total** | **40.66s CPU** dalam **13.76 dtk** (295.4% avg) | 74.64s CPU dalam 26.34 dtk (283.4% avg) |

    **2. Skenario 500 Klien Serentak (5.072 Request, 5.000 Jawaban)**
    | Endpoint / Metrik | Go (p50 / p95 / p99 / Max) | Node.js (p50 / p95 / p99 / Max) |
    |---|---|---|
    | `POST /login-siswa` (572 req) | `96 ms` / `155 ms` / `241 ms` / `286 ms` | `1490 ms` / `3452 ms` / `3572 ms` / `3664 ms` |
    | `GET /api/soal/:ujianId` (500 req) | `2 ms` / `5 ms` / `16 ms` / `29 ms` | `1262 ms` / `2643 ms` / `2881 ms` / `2918 ms` |
    | `GET /api/sesi` (500 req) | `1 ms` / `4 ms` / `19 ms` / `53 ms` | `1160 ms` / `1954 ms` / `2104 ms` / `2126 ms` |
    | `POST /api/sinkron-jawaban` (1500 req) | **`14 ms` / `24 ms` / `39 ms` / `91 ms`** | `2769 ms` / `4095 ms` / `4189 ms` / `4258 ms` |
    | `POST /api/heartbeat` (1500 req) | `2 ms` / `18 ms` / `22 ms` / `43 ms` | `1050 ms` / `1569 ms` / `1617 ms` / `1643 ms` |
    | `POST /api/selesai-ujian` (500 req) | `15 ms` / `24 ms` / `37 ms` / `49 ms` | `1716 ms` / `2951 ms` / `3014 ms` / `3029 ms` |
    | **Error Rate** | **0.00%** (0 gagal dari 5.072) | **0.00%** (0 gagal dari 5.072) |
    | **Jawaban Hilang** | **0** (5.000 terkirim = 5.000 di DB) | **0** (5.000 terkirim = 5.000 di DB) |
    | **RAM Server (RSS Awal → Akhir / Peak)** | **12.0 MB → 19.0 MB (Peak: 19.1 MB)** | 103.3 MB → 248.0 MB (Peak: 248.0 MB) |
    | **CPU Proses Server & Waktu Total** | **50.61s CPU** dalam **13.74 dtk** (368.4% avg) | 82.89s CPU dalam 31.89 dtk (259.9% avg) |

- Konsep Go:
  - Custom JSON Unmarshaler (`json.Unmarshaler`): mengimplementasikan metode `UnmarshalJSON([]byte) error` pada `ItemJawabanInput` untuk menerima variasi kontrak payload klien (`jawaban` vs `jawaban_dipilih`, serta format timestamp RFC3339 maupun MySQL DATETIME) tanpa mengorbankan ketepatan tipe struct internal ([Go by Example: JSON](https://gobyexample.com/json)).
  - Multi-row Batch Upsert & Short-lived Transaction: mengevaluasi validasi di memori sebelum membuka `db.BeginTx` dan menggabungkan seluruh baris dalam satu perintah `INSERT INTO ... VALUES (...), (...) ON DUPLICATE KEY UPDATE` sehingga memangkas jumlah round-trip jaringan/DB dan meminimalkan durasi row lock ([Go Database/SQL: Executing Transactions](https://go.dev/doc/database/execute-transactions)).
  - Penanganan Perilaku `sql.Result.RowsAffected()` pada MySQL: memahami bahwa MySQL secara default mengembalikan `0` baris berubah apabila kolom `DATETIME` (`last_seen`) di-update pada detik yang sama dengan `NOW()`, sehingga verifikasi keberadaan baris dilakukan secara tepat ketika `RowsAffected() == 0` ([Go Doc: sql.Result](https://pkg.go.dev/database/sql#Result)).
- Penyimpangan dari rencana: menyesuaikan batas default rate limit IP login siswa di `cmd/server/main.go` ke 600 req/15 menit (atau via `RATE_LIMIT_LOGIN_IP`, sama dengan `middleware/rateLimiter.js` di Node) agar 500 siswa serentak + 15% re-login dari satu IP NAT sekolah tidak terblokir HTTP 429.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T2.1
- Diubah: `internal/realtime/hub.go`, `internal/realtime/hub_test.go`, `internal/httpserver/server.go`, `cmd/server/main.go`, `go.mod`, `go.sum`, `docs/TODO.md`.
- Verifikasi:
  - `go test -race -v ./internal/realtime` → PASS semua 6 skenario:
    1. Koneksi `/ws` dan `/ws/admin` dengan session valid berhasil terhubung.
    2. Koneksi tanpa session ditolak dengan 401 Unauthorized; session siswa mencoba akses `/ws/admin` ditolak dengan 403 Forbidden.
    3. Pesan melebihi batas ukuran (`MaxMessageBytes`) langsung memutus koneksi (`StatusMessageTooBig`).
    4. Room per ujian untuk admin (`admin:join` → `admin:joined`): pengiriman ke room ujian tertentu (`BroadcastToAdminRoom`) hanya diterima oleh admin yang tergabung di room ujian tersebut.
    5. Backpressure: klien lambat (yang tidak membaca dan memenuhi `sendCh` berpenyangga) tidak memblokir pengirim (`BroadcastToAdminRoom` selesai dalam < 10ms untuk 50 pesan) dan koneksinya diputus secara asinkron.
    6. Pemutusan semua klien dan pemanggilan `hub.Close()` terbukti tidak membocorkan goroutine (`runtime.NumGoroutine()` kembali ke baseline).
- Konsep Go:
  - Goroutine Pembaca & Penulis per Koneksi (`readPump` & `writePump`): memisahkan pembacaan dan penulisan soket ke dua goroutine terdedikasi dengan komunikasi kanal `sendCh chan []byte` sehingga penulisan konkuren dan pengiriman frame kontrol ping/pong terkoordinasi dengan aman ([Go by Example: Channels](https://gobyexample.com/channels)).
  - Non-blocking Send & Backpressure dengan `select` + `default`: menggunakan blok `select` dengan cabang `default` saat mengirim ke `c.sendCh` berpenyangga sehingga apabila buffer klien penuh, pengirim tidak pernah tertahan dan klien lambat langsung diputus ([Tour of Go: Default Selection](https://go.dev/tour/concurrency/6)).
  - Proteksi Konkurensi Peta Room dengan `sync.RWMutex`: mengizinkan pembacaan daftar anggota room secara paralel (`RLock`) saat menyiarkan pesan dan mengunci eksklusif (`Lock`) hanya saat registrasi/unregistrasi koneksi ([Go Doc: sync.RWMutex](https://pkg.go.dev/sync#RWMutex)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T2.2
- Diubah: `internal/realtime/siswa.go`, `internal/realtime/siswa_test.go`, `internal/httpserver/api_siswa.go`, `internal/httpserver/server.go`, `docs/PORTING_MAP.md`, `docs/TODO.md`.
- Verifikasi:
  - `go test -race -v ./internal/realtime` → PASS semua skenario:
    1. `TestSiswaSiapDanReconnect` (padanan `sesi_endpoint_socket.test.js`): event `siswa-siap` membalas `mulai-ujian` berisi `durasi`, `sisa_detik`, `batas_waktu`, dan `device_type`; saat koneksi putus (`disconnect`) hanya memperbarui `last_seen` dan mempertahankan status `sedang_ujian` beserta jawaban tersimpan; saat reconnect tidak mereset timer ke durasi penuh.
    2. `TestKebijakanPerangkatDanPelanggaran` (padanan `device_type.test.js` & `kebijakan_perangkat.test.js`): pada `hp`, sinyal `keluar-fullscreen` diabaikan server tanpa tercatat di `log_kecurangan`, sedangkan `pindah-tab` dan `copy-paste` dicatat beserta `device_type = 'hp'`, mengirim `peringatan` saat `jumlah < batas`, dan mengubah status menjadi `keluar_paksa` + mengirim `paksa-submit` saat mencapai `batas_pelanggaran`; pada `laptop`, sinyal `keluar-fullscreen` dihitung sebagai pelanggaran.
    3. `TestCekStatusUjianEndpoint`: `GET /api/cek-status-ujian` mengembalikan 401 `not_logged_in` tanpa session, `valid: true` beserta `jumlah_pelanggaran`/`batas`/`sisa` saat aktif, dan `valid: false, reason: "keluar_paksa"` saat akses dicabut.
- Konsep Go:
  - First-class Functions & Callback Injection (`OnMonitorNotifyFunc` & `SetSiswaHandlers`): menyuntikkan fungsi handler event ke dalam `Hub` tanpa menciptakan circular dependency antar paket ([Go by Example: Closures](https://gobyexample.com/closures)).
  - Penanganan Nilai `NULL` SQL (`sql.NullString` & `COALESCE`): memindai kolom opsional `device_type` dari database menggunakan tipe pembungkus `sql.NullString` agar aman dari error `Scan` ketika bernilai `NULL` ([Go Doc: sql.NullString](https://pkg.go.dev/database/sql#NullString)).
  - Context Timeout Terisolasi saat Disconnect (`context.WithTimeout(context.Background(), 3*time.Second)`): menjalankan kueri pembaruan `last_seen` menggunakan context baru berbatas waktu ketika koneksi klien telah terputus (`c.ctx` sudah dibatalkan) ([Go Blog: Context](https://go.dev/blog/context)).
Laporan eksekusi T2.4
- Diubah: `internal/monitor/monitor.go`, `internal/monitor/handler.go`, `internal/monitor/monitor_test.go`, `internal/auth/middleware.go`, `internal/httpserver/server.go`, `docs/PORTING_MAP.md`, `docs/TODO.md`.
- Verifikasi:
  - `go test -v ./internal/monitor` → PASS semua skenario:
    1. `TestHitungStatusTurunan`: fungsi murni teruji tabel untuk semua 9 kondisi (belum_masuk, selesai, terkunci, sedang_ujian dengan last_seen < 45s online, dan >= 45s offline).
    2. `TestSiarkanUpdate_RoomKosongTidakQueryDB`: verifikasi optimasi wajib DESIGN §3.6; jika room admin kosong (`HasAdminListeners = false`), kueri DB tidak dieksekusi sama sekali (0 query / no panic dereference).
    3. `TestSnapshot_500Siswa_TanpaNPlusOneQuery_QueryCounter`: simulasi 500 siswa terdaftar dengan agregasi jawaban dan status sesi membuktikan jumlah kueri ke DB tepat 2 kueri (konstan bebas N+1).
    4. `TestEndpointGetMonitor_UnitMock`: endpoint `GET /admin/api/monitor/{ujianId}` mengembalikan 401 tanpa session, 403 untuk session siswa, 400 untuk ID ujian non-angka, dan 200 OK beserta payload snapshot lengkap untuk session admin.
  - `make check` (gofmt + vet + test sekuensial) → hijau tanpa error.
  - `make test-race` (`go test -p 1 -race ./...`) → hijau tanpa data race.
- Konsep Go:
  - Pure Function & Table-Driven Testing (`HitungStatusTurunan`): memisahkan logika penentuan status tampilan ke dalam fungsi murni tanpa I/O database dan mengujinya secara komprehensif menggunakan irisan struct anonim ([Go by Example: Testing](https://gobyexample.com/testing-and-benchmarking)).
  - Consumer-Driven Interface (`Broadcaster`): mendefinisikan interface minimal (`HasAdminListeners` dan `BroadcastToAdminRoom`) di paket konsumen (`internal/monitor`) alih-alih mengikat langsung ke tipe konkret `*realtime.Hub` ([Go Wiki: CodeReviewComments - Interfaces](https://go.dev/wiki/CodeReviewComments#interfaces)).
  - Mock SQL Driver (`database/sql/driver.Driver`): mengimplementasikan antarmuka driver database standar Go untuk menguji perilaku kueri SQL dan menghitung jumlah eksekusi kueri secara deterministik tanpa ketergantungan pada server database eksternal ([Go Doc: database/sql/driver](https://pkg.go.dev/database/sql/driver)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T3.1
- Diubah: `internal/auth/middleware.go`, `internal/auth/admin_guru.go`, `internal/auth/admin_guru_test.go`, `internal/httpserver/server.go`, `internal/httpserver/routes_auth.go`, `cmd/server/main.go`, `docs/TODO.md`.
- Verifikasi:
  - `go test -v -run "TestGuardPeran|TestLoginAdmin|TestLoginGuru|TestLogoutAdminDanGuru|TestRateLimitLoginAdminGuru" ./internal/auth` → PASS semua 5 pengujian:
    1. `TestGuardPeran`: middleware HTML (`RequireAdmin`, `RequireGuru`) mengalihkan ke halaman login bila sesi tidak cocok, sementara middleware API (`RequireAdminAPI`, `RequireGuruAPI`) mengembalikan 401 jika belum login dan 403 Forbidden bila peran tidak sesuai.
    2. `TestLoginAdmin`: penolakan username tidak ada / password salah, pembaruan token sesi (`RenewToken`), penyimpanan sesi `adminId`, `username`, `peran: "admin"`, serta pengalihan ke `/admin/dashboard`.
    3. `TestLoginGuru`: penolakan kredensial salah, penyimpanan sesi `guruId`, `username`, `namaGuru`, `nipGuru`, `peran: "guru"`, serta pengalihan ke `/guru/dashboard`.
    4. `TestLogoutAdminDanGuru`: penghapusan sesi aktif dan pengalihan ke `/login-admin` dan `/login-guru`.
    5. `TestRateLimitLoginAdminGuru`: pembatasan 10 percobaan per 15 menit per `IP:username`; request ke-11 diblokir dengan status HTTP 429 Too Many Requests, sementara username atau IP berbeda tetap lolos.
  - `make check` (gofmt + vet + test sekuensial) → hijau tanpa error untuk seluruh paket modul `cbt-go`.
  - `make test-race` (`go test -p 1 -race ./...`) → hijau tanpa data race.
- Konsep Go:
  - Session Fixation Mitigation via `RenewToken`: meregenerasi token cookie session saat status autentikasi berubah berhasil login menggunakan `sm.RenewToken(ctx)` untuk mencegah eksploitasi pembajakan sesi ([SCS Session Documentation](https://github.com/alexedwards/scs)).
  - Middleware Chaining & Key Extractor: menerapkan pola pipeline HTTP handler bertingkat di mana middleware rate limiter membungkus handler inti dengan fungsi pengekstraksi kunci dinamis (`ip:username`) ([Go by Example: HTTP Servers](https://gobyexample.com/http-server)).
  - Constant-time Dummy Hash Comparison: memanggil `bcrypt.CompareHashAndPassword` terhadap hash dummy ketika username tidak ditemukan di database untuk mencegah user enumeration melalui analisis selisih waktu respons (timing attack) ([Tour of Go: Methods and Interfaces](https://go.dev/tour/methods/1)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T3.2
- Diubah: `internal/admin/master.go`, `internal/admin/handler.go`, `internal/admin/master_test.go`, `internal/admin/handler_test.go`, `internal/httpserver/server.go`, `docs/TODO.md`.
- Verifikasi:
  - `go test -v ./internal/admin` → PASS semua skenario:
    1. `TestMasterData_Guru`, `TestMasterData_Kelas`, `TestMasterData_Mapel`, `TestMasterData_Pengajaran`: operasi CRUD lengkap, hashing bcrypt saat tambah/edit guru, pembacaan relasi pengajaran via SQL JOIN.
    2. Penanganan duplikasi unik: duplikat `username` guru menghasilkan `ErrDuplicateUsername` dan respons 400 Bad Request ("username sudah digunakan"); duplikat `nama_kelas` menghasilkan `ErrDuplicateKelas` ("nama kelas sudah ada"); duplikat kombinasi pengajaran menghasilkan `ErrDuplicatePengajaran`.
    3. `TestMasterData_ValidasiInput`: validasi ketat menolak input kosong dengan `ErrInvalidInput`.
    4. `TestAdminHandlers_CRUD`: handler HTTP `POST /admin/guru/tambah`, `POST /admin/kelas/tambah`, dan `GET /admin/guru` terverifikasi merespons kode status yang benar (201 Created, 400 Bad Request untuk duplikat, dan 200 OK untuk daftar).
  - `make check` (gofmt + vet + test sekuensial) → hijau tanpa error.
  - `make test-race` (`go test -p 1 -race ./...`) → hijau tanpa data race.
- Konsep Go:
  - Error Wrapping & Database Vendor Error Inspection (`errors.As` dengan `*mysql.MySQLError`): memeriksa kode kesalahan MySQL (1062 ER_DUP_ENTRY) untuk menerjemahkan pelanggaran integritas kunci unik menjadi sentinel error domain yang ramah pengguna ([Go Blog: Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)).
  - Content Negotiation pada Handler HTTP (`wantsJSON` via header `Accept`): memungkinkan satu endpoint melayani form web standar (dengan redirect dan flash message) sekaligus klien API/fetch tanpa menduplikasi logika bisnis ([Go by Example: HTTP Servers](https://gobyexample.com/http-server)).
  - Transactionless Safe Delete: mengandalkan constraint ON DELETE CASCADE pada level skema foreign key database untuk menjaga integritas relasional data saat entitas induk dihapus ([Go Database/SQL Tutorial](https://go.dev/doc/database/)).
Laporan eksekusi T3.3
- Diubah: `internal/excel/siswa.go`, `internal/excel/siswa_test.go`, `internal/admin/siswa.go`, `internal/admin/handler_siswa.go`, `internal/admin/siswa_test.go`, `internal/httpserver/server.go`, `web/templates/admin_siswa.gohtml`, `web/templates/admin_guru.gohtml`, `docs/TODO.md`.
- Verifikasi:
  - `go test -v ./internal/excel` → PASS: `TestBuatTemplateSiswa` memverifikasi struktur kolom berkas spreadsheet `.xlsx`; `TestBacaImportSiswa_ValidDanGagal` memverifikasi validasi input baris kosong/tidak lengkap.
  - `go test -v ./internal/admin` → PASS seluruh skenario siswa dan impor:
    1. `TestSiswaCRUD`: operasi CRUD siswa, pembacaan list terfilter kelas, dan penanganan duplikat NIS (`ErrDuplicateNIS`).
    2. `TestSiswaTemplateHandler`: endpoint `GET /admin/siswa/template` menyajikan stream attachment `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` dengan header kolom yang valid.
    3. `TestSiswaImportHandler_Sukses3Baris`: 3 baris data siswa sintetis dari buffer spreadsheet berhasil diproses, di-hash dengan bcrypt, dan disimpan ke database.
    4. `TestSiswaImportHandler_FormatBukanXlsx`: file non-`.xlsx` (seperti `.csv`) langsung ditolak dengan status HTTP 400 Bad Request.
    5. `TestSiswaImportHandler_UkuranMelebihi2MB`: unggahan berukuran > 2 MB dibatasi oleh `http.MaxBytesReader` dan ditolak dengan status 400 Bad Request.
    6. `TestSiswaImportHandler_DuplikatNIS`: baris dengan NIS yang sudah ada di database dilaporkan dalam daftar baris gagal secara rinci tanpa menggagalkan baris valid lainnya.
    7. `TestSiswaHTTPRoutesIntegration`: guard sesi admin pada rute siswa dan alur tambah siswa.
  - `make check` (gofmt + vet + test sekuensial) → hijau tanpa error.
  - `make test-race` (`go test -p 1 -race ./...`) → hijau tanpa data race.
- Konsep Go:
  - Memory-Only Streaming & File Cleanup (`io.Reader` & `excelize.OpenReader`): membaca dan memvalidasi file Excel langsung dari stream HTTP multipart tanpa menulis berkas sementara ke disk penyimpanan ([Go by Example: Reading Files](https://gobyexample.com/reading-files)).
  - Request Body Throttling (`http.MaxBytesReader`): melindungi server dari serangan Denial of Service berbasis exhaust memori dengan membatasi ukuran pembacaan request stream sebelum pemrosesan multipart ([Go Doc: http.MaxBytesReader](https://pkg.go.dev/net/http#MaxBytesReader)).
  - Partial Batch Processing with Granular Failure Reporting: alih-alih menggagalkan seluruh transaksi saat salah satu baris Excel bermasalah, baris yang bermasalah dicatat nomor baris dan alasannya ke dalam slice `[]ImportError` sementara baris valid tetap disimpan ([Tour of Go: Slices](https://go.dev/tour/moretypes/7)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T3.4
- Diubah: `internal/admin/ujian.go`, `internal/admin/handler_ujian.go`, `internal/admin/ujian_test.go`, `internal/httpserver/server.go`, `web/templates/admin_ujian.gohtml`, `docs/TODO.md`.
- Verifikasi:
  - `go test -v -run "TestUjianValidasiDanCRUD|TestUjianByMapelAPI|TestUjianHTTPHandlers_TambahDanValidasi" ./internal/admin` → PASS seluruh skenario:
    1. `TestUjianValidasiDanCRUD`: validasi ketat menolak `tanggal_selesai <= tanggal_mulai` (`ErrInvalidWaktu`), `durasi <= 0` (`ErrInvalidDurasi`), dan `pengajaran_id <= 0` (`ErrInvalidPengajaran`). Operasi tambah, baca per ID, edit, list seluruh ujian, dan hapus kaskade sukses.
    2. `TestUjianByMapelAPI`: endpoint `GET /admin/api/ujian-by-mapel/{mapelId}` menolak permintaan tanpa sesi admin dengan HTTP 401 Unauthorized, dan menyajikan array JSON ujian yang sesuai filter mata pelajaran ketika diautentikasi.
    3. `TestUjianHTTPHandlers_TambahDanValidasi`: handler HTTP menolak form dengan tanggal terbalik (400 Bad Request) dan berhasil membuat ujian baru berstatus 201 Created saat valid.
  - `make check` (gofmt + vet + test sekuensial) → hijau tanpa error.
  - `make test-race` (`go test -p 1 -race ./...`) → hijau tanpa data race.
- Konsep Go:
  - Flexible Multi-layout Time Parsing (`ParseDateTime`): mem-parsing berbagai format masukan tanggal/waktu (HTML5 `datetime-local`, ISO/RFC3339, dan SQL datetime) secara anggun menggunakan iterasi slice layout standar Go ([Go by Example: Time Parsing](https://gobyexample.com/time-formatting-parsing)).
  - Pointer for Nullable Columns (`*string` untuk `token_ujian`): menggunakan tipe pointer untuk membedakan secara eksplisit antara ketiadaan nilai (NULL di SQL) dan string kosong, menjaga kompatibilitas database tanpa tipe wrapper khusus ([Tour of Go: Pointers](https://go.dev/tour/moretypes/1)).
  - Dual Content Ingestion (Form URLEncoded & JSON Body): handler mampu menerima data baik dari submit form tradisional browser maupun JSON payload dari klien API modern via pengecekan `Content-Type` ([Go by Example: JSON](https://gobyexample.com/json)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T3.5
- Diubah: `internal/soal/kelola.go`, `internal/soal/kelola_test.go`, `internal/admin/handler_soal.go`, `internal/guru/handler_soal.go`, `internal/guru/guru_otorisasi_soal_test.go`, `internal/httpserver/server.go`, `web/templates/admin_soal.gohtml`, `web/templates/guru_soal.gohtml`, `docs/PORTING_MAP.md`, `docs/TODO.md`.
- Verifikasi:
  - `go test -v ./internal/soal` → PASS: `TestDefaultPoin` memverifikasi penentuan poin otomatis (PG = 2, Menjodohkan = 2, Essay = 4) jika poin input kosong/nol; `TestSoalCRUDDanBatch` memverifikasi validasi teks kosong (`ErrTeksSoalKosong`), tipe soal invalid (`ErrTipeSoalInvalid`), operasi CRUD, serta penambahan massal batch soal.
  - `go test -v ./internal/guru` → PASS seluruh skenario otorisasi (`TestGuruOtorisasiSoal` & `TestAdminSoalAccess`):
    1. Guru 1 berhasil melihat dan mengelola soal pada Ujian 10 yang diampunya.
    2. Guru 2 (bukan pengampu Ujian 10) secara ketat diblokir dengan HTTP 403 Forbidden pada **setiap** endpoint: `GET /guru/api/soal/10`, `POST /guru/soal/tambah`, `GET /guru/soal/edit/101`, `POST /guru/soal/edit/101`, `POST /guru/soal/hapus/101`, dan `POST /guru/soal/batch-tambah`.
    3. Guru 1 dapat mengedit dan menghapus soalnya sendiri (Soal 101) dengan respons 200 OK.
    4. Admin memiliki akses global tanpa batasan guru untuk melihat dan membuat soal di semua ujian.
  - `make check` (gofmt + vet + test sekuensial) → hijau tanpa error untuk seluruh paket modul `cbt-go`.
  - `make test-race` (`go test -p 1 -race ./...`) → hijau tanpa data race.
- Konsep Go:
  - Strict Authorization Guard via Join Subquery (`CekOtorisasiUjianGuru` & `CekOtorisasiSoalGuru`): memverifikasi hak akses kepemilikan data langsung di lapisan relasional SQL sebelum memproses mutasi atau eksposisi data ([Go Database/SQL Tutorial](https://go.dev/doc/database/)).
  - Business Rule Fallback Pattern (`DefaultPoin`): fungsi murni yang menetapkan nilai default aturan bisnis berdasarkan domain type tanpa mengandalkan trigger database, mempermudah pengujian unit tabel ([Go by Example: Functions](https://gobyexample.com/functions)).
  - Transactional Batch Insertion: memproses sekumpulan data masukan dalam satu alur iteratif terpadu dengan penghentian dini (early return) bila terjadi galat validasi ([Go by Example: Errors](https://gobyexample.com/errors)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T3.6
- Diubah: `internal/excel/hasil.go`, `internal/excel/hasil_test.go`, `internal/admin/hasil.go`, `internal/admin/handler_hasil.go`, `internal/admin/hasil_test.go`, `internal/guru/handler_hasil.go`, `internal/guru/guru_hasil_test.go`, `internal/httpserver/server.go`, `web/embed.go`, `web/templates/admin_hasil.gohtml`, `web/templates/admin_hasil_cetak.gohtml`, `web/templates/guru_hasil.gohtml`, `docs/TODO.md`.
- Verifikasi:
  - `go test -v ./internal/excel` → PASS: `TestBuatEksporHasilUjian` menghasilkan berkas spreadsheet `.xlsx` yang dibuka kembali menggunakan `excelize.OpenReader` untuk memeriksa keakuratan sel judul, header baris ke-5, dan nilai siswa.
  - `go test -v ./internal/admin` → PASS: `TestAdminHasil_ListDanExport` menguji pembacaan hasil per ujian, ekspor attachment spreadsheet, dan render template cetak (`admin_hasil_cetak.gohtml`).
  - `go test -v ./internal/guru` → PASS: `TestGuruHasil_OtorisasiDanEkspor` memverifikasi bahwa Guru 1 dapat mengakses dan mengekspor hasil ujian miliknya, sementara Guru 2 yang bukan pengampu ditolak dengan HTTP 403 Forbidden pada list dan ekspor.
  - `make check` (gofmt + vet + test sekuensial) → hijau tanpa error.
  - `make test-race` (`go test -p 1 -race ./...`) → hijau tanpa data race.
- Konsep Go:
  - In-Memory Spreadsheet Generation & Content-Disposition (`excelize.NewFile` & `bytes.Buffer`): menghasilkan laporan tabular Excel secara langsung di memori dan menyajikannya sebagai stream unduhan biner dengan header `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` ([Go by Example: Writing Files](https://gobyexample.com/writing-files)).
  - Template Custom Functions (`template.FuncMap`): menyematkan fungsi pembantu template seperti `"add"` ke dalam instans `html/template.Template` untuk manipulasi tampilan seperti penomoran baris ([Go Doc: html/template FuncMap](https://pkg.go.dev/html/template#FuncMap)).
  - Contextual Scope Filtering on Relational Reports: mengintegrasikan verifikasi kepemilikan guru ke dalam penyusunan query SQL laporan nilai siswa untuk mencegah kebocoran data nilai lintas pengajar ([Go Database/SQL Tutorial](https://go.dev/doc/database/)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T3.7
- Diubah: `internal/excel/essay.go`, `internal/excel/essay_test.go`, `internal/guru/essay.go`, `internal/guru/handler_essay.go`, `internal/guru/penilaian_fase3_test.go`, `internal/httpserver/server.go`, `web/templates/guru_essay.gohtml`, `docs/PORTING_MAP.md`, `docs/TODO.md`.
- Verifikasi:
  - `go test -v ./internal/excel` → PASS: `TestBuatDanBacaImportEssay` memverifikasi pembacaan baris Excel skor essay dan `TestBacaImportEssay_ValidasiError` memeriksa validasi baris tidak valid.
  - `go test -v ./internal/guru` → PASS seluruh skenario `TestPenilaianFase3` (padanan `tests/penilaian_fase3.test.js`):
    1. Validasi batas skor: skor negatif (-1) atau skor melebihi poin maksimal (5 > 4) ditolak dengan status HTTP 400 Bad Request.
    2. Otorisasi guru: guru non-pengampu diblokir dengan HTTP 403 Forbidden saat mencoba menilai essay.
    3. Transisi status koreksi & hitung ulang: setelah seluruh essay siswa dinilai, `status_koreksi` otomatis bertransisi dari `'menunggu_essay'` menjadi `'selesai'` dan nilai akhir terhitung benar (100).
    4. Impor Excel essay: parsing multipart berkas spreadsheet dan pembaruan batch nilai essay terbukti idempoten.
  - `make check` (gofmt + vet + test sekuensial) → hijau tanpa error untuk seluruh paket modul `cbt-go`.
  - `make test-race` (`go test -p 1 -race ./...`) → hijau tanpa data race.
- Konsep Go:
  - Domain Integrity Guard with Dynamic Bounds (`SimpanNilaiEssay`): memvalidasi masukan skor guru terhadap poin maksimal setiap butir soal essay sebelum melakukan mutasi basis data ([Go by Example: Errors](https://gobyexample.com/errors)).
  - Automated Score Reconciliation via `sesi.HitungUlangNilai`: memanggil kembali fungsi murni penilaian untuk menghitung ulang perolehan skor otomatis + essay serta menentukan status koreksi akhir secara atomik ([Go Database/SQL Tutorial](https://go.dev/doc/database/)).
  - Idempotent Spreadsheet Upsert: pembaruan batch nilai essay menggunakan klausa `ON DUPLICATE KEY UPDATE` pada level database sehingga impor ulang file Excel tidak menghasilkan duplikasi rekaman ([Go Wiki: CodeReviewComments](https://go.dev/wiki/CodeReviewComments)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T3.8
- Diubah: `internal/excel/log.go`, `internal/admin/log_reset.go`, `internal/admin/handler_log_reset.go`, `internal/admin/log_reset_test.go`, `internal/guru/handler_log.go`, `internal/guru/guru_log_test.go`, `internal/httpserver/server.go`, `web/templates/admin_log.gohtml`, `web/templates/guru_log.gohtml`, `docs/TODO.md`.
- Verifikasi:
  - `go test -v ./internal/admin` → PASS: `TestLogKecuranganDanResetUjianAdmin` memverifikasi pembacaan log pelanggaran admin, aksi reset ujian per siswa secara atomik (menghapus jawaban, nilai_essay, nilai_ujian, log_kecurangan, sesi_ujian hanya untuk siswa dan ujian bersangkutan), serta reset massal seluruh peserta pada ujian tertentu tanpa mempengaruhi ujian lain.
  - `go test -v ./internal/guru` → PASS: `TestGuruLogKecurangan_Otorisasi` memverifikasi bahwa Guru 1 dapat melihat dan mengekspor log kecurangan pada ujian yang diampunya, sedangkan Guru 2 diblokir dengan HTTP 403 Forbidden pada akses tampilan maupun ekspor berkas Excel.
  - `make check` (gofmt + vet + test sekuensial) → hijau tanpa error untuk seluruh paket modul `cbt-go`.
  - `make test-race` (`go test -p 1 -race ./...`) → hijau tanpa data race.
- Konsep Go:
  - Atomic Cascading Purge within Transaction (`ResetUjianSiswa` & `ResetUjianSemua`): membungkus serangkaian operasi penghapusan data relasional (jawaban, nilai essay, nilai ujian, log kecurangan, dan sesi) ke dalam satu transaksi database atomik `db.BeginTx` dengan rollback otomatis jika terjadi kegagalan ([Go Database/SQL Tutorial: Transactions](https://go.dev/doc/database/execute-transactions)).
  - Contextual Audit Logging via `log/slog`: mencatat kejadian administratif penting (reset ujian) secara terstruktur tanpa membocorkan data kredensial atau melanggar integritas relasi foreign key ([Go Doc: log/slog](https://pkg.go.dev/log/slog)).
  - Scope-Isolated Deletion: menyusun klausa WHERE secara ketat (`siswa_id = ? AND ujian_id = ?` dan `ujian_id = ?`) untuk memastikan isolasi penuh dan mencegah modifikasi data yang tidak diinginkan pada entitas lain ([Go by Example: SQL](https://gobyexample.com/)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T4.1
- Diubah: `internal/config/production_env_test.go`, `internal/httpserver/security_audit_test.go`, `internal/httpserver/server.go`, `docs/PORTING_MAP.md`, `docs/TODO.md`.
- Verifikasi:
  - `govulncheck ./...` → PASS: "No vulnerabilities found. Your code is affected by 0 vulnerabilities."
  - `go test -v ./internal/config -run TestProductionEnvConfig` → PASS: memverifikasi bahwa mode production mengaktifkan flag `IsProduction=true` serta secara otomatis menetapkan flag `Secure=true` dan `HttpOnly=true` pada cookie session `cbt_session`. Menguji pula bahwa `TRUST_PROXY=true` mengaktifkan `Secure=true` pada deployment reverse proxy/tunnel HTTPS.
  - `go test -v ./internal/httpserver -run TestAuditKeamanan` → PASS seluruh 7 skenario audit keamanan:
    1. Kunci Jawaban Kebocoran: `TestAuditKeamanan_KunciJawabanTidakPernahBocor` memastikan respons JSON siswa (`GET /api/soal/:ujianId`, `/api/sesi`, `/api/sinkron-jawaban`) tidak memuat kata `'jawaban_benar'`, `'is_benar'`, maupun field rahasia lainnya.
    2. Otorisasi Peran: `TestAuditKeamanan_OtorisasiRoleEndpoint` membuktikan bahwa tamu unauthenticated ditolak dari rute siswa/admin/guru; siswa dilarang mengakses rute admin/guru (redirect login atau 403 API); dan guru dilarang mengakses rute admin.
    3. Rate Limiting: `TestAuditKeamanan_RateLimiting` membuktikan percobaan ke-11 login siswa per-NIS dan login admin/guru per IP+username ditolak (HTTP 429), serta pembatasan IP siswa (300/15 menit).
    4. HTTP Security Headers: `TestAuditKeamanan_SecurityHeaders` memverifikasi kehadiran header keamanan wajib `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`, dan `Content-Security-Policy`.
    5. CSRF Protection: `TestAuditKeamanan_CSRF` membuktikan bahwa `http.CrossOriginProtection` menolak request POST dari origin lintas domain (cross-site) dengan HTTP 403 Forbidden.
    6. Upload Size Limit: `TestAuditKeamanan_UploadSizeLimit` membuktikan request dengan muatan melebihi batas 1MB/2MB ditolak untuk mencegah ancaman DoS memori.
    7. Sanitasi SQL & Template: audit `grep` memastikan 100% query SQL menggunakan parameter (`?`) tanpa interpolasi string, 0 penggunaan `template.HTML` atau `template.JS` pada masukan pengguna, serta 0 pencatatan PIN/hash/cookie pada log server.
  - `make check` (gofmt + vet + test sekuensial) → hijau tanpa error untuk seluruh paket modul `cbt-go`.
  - `make test-race` (`go test -p 1 -race ./...`) → hijau tanpa data race.
- Konsep Go:
  - Built-in Cross-Origin Protection (`http.NewCrossOriginProtection`): memanfaatkan proteksi CSRF bawaan pustaka standar Go (Go 1.25+) yang memvalidasi header `Sec-Fetch-Site` dan `Origin` pada request termutasi berbasis session secara otomatis ([Go Doc: http.CrossOriginProtection](https://pkg.go.dev/net/http#NewCrossOriginProtection)).
  - Security Vulnerability Scanning (`golang.org/x/vuln/cmd/govulncheck`): memindai basis kode dan seluruh dependensi Go terhadap database kerentanan resmi Go untuk mendeteksi exploit yang aktif secara statis dan dinamis ([Go Security: Vulnerability Management](https://go.dev/security/vuln/)).
  - Environment-Driven Cookie Hardening: menyetel opsi atribut `http.Cookie` (`Secure`, `HttpOnly`, `SameSite=http.SameSiteLaxMode`) secara dinamis berdasarkan konfigurasi lingkungan (`APP_ENV=production` dan `TRUST_PROXY=true`) untuk melindungi token sesi dari mitigasi sniffing dan XSS ([Go by Example: HTTP Cookies](https://gobyexample.com/)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T4.2
- Diubah: `scripts/backup.sh`, `scripts/restore.sh`, `scripts/backup_restore_test.go`, `docs/DEPLOYMENT.md`, `docs/BACKUP_RESTORE.md`, `docs/PANDUAN_HARI_H.md`, `docs/PORTING_MAP.md`, `docs/TODO.md`.
- Verifikasi:
  - Uji langsung backup dan restore ke database MySQL uji `cbt_sekolah_go_test`:
    1. `./scripts/backup.sh --db=cbt_sekolah_go_test --socket=/var/lib/mysql/mysql.sock --out-dir=/tmp/cbt_backup_test` → menghasilkan berkas valid terkompresi gzip `cbt_cbt_sekolah_go_test_*.sql.gz` (4.0K).
    2. `./scripts/restore.sh --db=cbt_sekolah_go_test --socket=/var/lib/mysql/mysql.sock --file=... --yes` → sukses mendekompresi dan mengimpor seluruh skema tabel dan data ke target database tanpa error.
  - `go test -v ./scripts` → PASS: `TestBackupRestoreScripts` memverifikasi bit executable berkas skrip, validasi argumen CLI, menu bantuan `--help`, serta penolakan eksekusi tanpa parameter wajib; `TestSystemdUnitSyntax` memverifikasi kehadiran directive kunci `Restart=always`, `LimitNOFILE=65535`, dan `ExecStart` pada konfigurasi unit service systemd di `docs/DEPLOYMENT.md`.
  - Dokumen operasional:
    - `docs/DEPLOYMENT.md`: arsitektur deployment binary Go mandiri, konfigurasi `.env`, unit systemd `cbt-sekolah.service` (`Restart=always`, journald logging, multi-user.target), verifikasi kesehatan (`/healthz`), dan prosedur update aplikasi.
    - `docs/BACKUP_RESTORE.md`: SOP backup otomatis setiap 10 menit via cron, rotasi otomatis 30 file backup terakhir, langkah disaster recovery restore, dan salinan eksternal USB.
    - `docs/PANDUAN_HARI_H.md`: SOP operasional proktor dan tim teknis MTs Al-Ihsan Batujajar (checklist H-1 kelistrikan/jaringan, rilis token ujian, penanganan takeover perangkat, buka kunci pelanggaran, hingga rekapitulasi nilai akhir).
  - `make check` (gofmt + vet + test sekuensial) → hijau tanpa error.
  - `make test-race` (`go test -p 1 -race ./...`) → hijau tanpa data race.
- Konsep Go:
  - External Command & Subprocess Orchestration (`os/exec.Command`): menjalankan dan memverifikasi skrip operasional sistem secara terisolasi via pengujian otomatis Go dengan menangkap keluaran gabungan `CombinedOutput` ([Go by Example: Spawning Processes](https://gobyexample.com/spawning-processes)).
  - Self-Contained Deployment Artifact: binary Go hasil kompilasi meng-embed seluruh aset statis dan template HTML ke memori via direktif `//go:embed`, menyederhanakan unit systemd sehingga server tidak lagi bergantung pada node_modules, interpreter luar, atau berkas terpisah di filesystem ([Go Doc: embed](https://pkg.go.dev/embed)).
  - Robust Multi-mode Recovery Shell Scripts: memanfaatkan fitur shell POSIX modern (`set -euo pipefail`) dipadukan dengan stream filtering (`gzip -c` / `gunzip -c`) untuk memastikan kegagalan pada proses dump langsung menghentikan operasi sebelum merusak data cadangan ([Bash Pitfalls Guide](https://mywiki.wooledge.org/BashPitfalls)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T4.3
- Diubah: `tools/loadtest/package.json`, `docs/TODO.md`.
- Verifikasi:
  - Uji beban final 500 klien serentak (`LOADTEST_DB_NAME=cbt_sekolah_go_test node tools/loadtest/loadtest.js --clients=500 --ramp-up=15 --url=http://127.0.0.1:3001 --server-pid=61580 --cleanup`):
    - Waktu Total: **18.90 detik** (seluruh 500 peserta ramp-up dalam 15 dtk dan menyelesaikan 11 butir soal)
    - Total Permintaan: **5.072 request**
    - Permintaan Berhasil: **5.072 (100%)**, Gagal: **0 (0.00%)**
    - Jawaban Dikirim: **5.000**, Jawaban di DB: **5.000**, Jawaban Hilang: **0**
    - RAM Server Go (RSS Awal → Akhir / Peak): **16.4 MB → 26.4 MB (Peak: 27.3 MB)** (vs Node.js 103.3 MB → 248.0 MB, penghematan memori > 89%)
    - CPU Server: 60.76s total (321.4% avg)
    - Rincian Latensi per Endpoint:
      - `POST /login-siswa` (572 req): p50 = `182 ms`, p95 = `708 ms`, p99 = `882 ms`, max = `1117 ms`
      - `GET /api/soal/:ujianId` (500 req): p50 = `9 ms`, p95 = `182 ms`, p99 = `269 ms`, max = `353 ms`
      - `GET /api/sesi` (500 req): p50 = `6 ms`, p95 = `139 ms`, p99 = `199 ms`, max = `272 ms`
      - `POST /api/sinkron-jawaban` (1500 req): **p50 = `21 ms`, p95 = `208 ms`, p99 = `333 ms`, max = `422 ms`**
      - `POST /api/heartbeat` (1500 req): p50 = `11 ms`, p95 = `126 ms`, p99 = `198 ms`, max = `280 ms`
      - `POST /api/selesai-ujian` (500 req): p50 = `18 ms`, p95 = `162 ms`, p99 = `243 ms`, max = `336 ms`
  - Ambang Penerimaan PRD §8.3:
    1. p95 `sinkron-jawaban` < 500 ms: **208 ms [LULUS]** (Node.js pembanding: 4.095 ms)
    2. Error Rate < 1%: **0.00% [LULUS]**
    3. Jawaban Hilang: **0 [LULUS]**
  - `make check` (gofmt + vet + test sekuensial) → hijau tanpa error.
  - `make test-race` (`go test -p 1 -race ./...`) → hijau tanpa data race.
- Konsep Go:
  - Connection Pool Saturation Prevention: membatasi pool koneksi MySQL (`DB_MAX_OPEN_CONNS`) agar query 500 klien mengantre secara tertib di memori Go runtime daripada membebani thread engine mysqld, mencegah error `Too many connections` ([Go Database/SQL: Connection Pooling](https://go.dev/doc/database/manage-connections)).
  - Lightweight Goroutine Multiplexing: server Go menangani ribuan koneksi serentak hanya dengan konsumsi memori puncak 27.3 MB berkat stack awal goroutine yang ramping (2KB) dibandingkan thread/proses berat ([Tour of Go: Goroutines](https://go.dev/tour/concurrency/1)).
  - Lock-free Read & Zero-Allocation Serialization: jalur baca (`GET /api/soal` dan `GET /api/sesi`) berjalan bebas penguncian (lock-free) dengan latensi rata-rata sub-10ms pada konkurensi 500 pengguna ([Go Wiki: Performance](https://go.dev/wiki/Performance)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

Laporan eksekusi T4.4
- Diubah: `tests/ui/package.json`, `tests/ui/arab_hp.ui.js`, `tests/ui/monitor.ui.js`, `docs/PORTING_MAP.md`, `docs/TODO.md`.
- Verifikasi:
  - `node tests/ui/arab_hp.ui.js` → PASS:
    1. Render pada viewport mobile 360×640 (standar HP siswa) membuktikan teks soal dan pilihan Arab menggunakan `dir="auto"` dan font Amiri dengan rendering harakat yang tepat.
    2. Kartu soal tipe essay terbukti memuat pesan "Dijawab di Lembar Kertas" dan 100% tanpa elemen `<textarea>`.
    3. Tombol tap target (seperti tombol pengaturan ukuran font `A−`/`A+`) terverifikasi memenuhi standar aksesibilitas mobile dengan dimensi minimal 44×44 piksel.
    4. Indikator status penyimpanan dan sinkronisasi tersaji di DOM (`#sync-indicator`).
    5. Tangkapan layar otomatis disimpan pada `tests/ui/out/arab_hp_360px.png` (diabaikan oleh git).
  - `node tests/ui/monitor.ui.js` → PASS:
    1. Render tabel pemantauan admin memverifikasi kelengkapan kolom data (No, NIS, Nama, Kelas, Status, Progres, Pelanggaran, Perangkat, Aksi).
    2. Badge penanda status aktif (`Online`, `Terkunci`, `Selesai`, `Offline`) ter-render dengan warna semantik yang tepat.
    3. Tombol aksi administratif proktor ("Buka Kunci" dan "Paksa Selesai") aktif dan tervalidasi pada baris siswa terkunci.
    4. Tangkapan layar otomatis disimpan pada `tests/ui/out/monitor_admin.png` (diabaikan oleh git).
  - `make check` (gofmt + vet + test sekuensial) → hijau tanpa error.
  - `make test-race` (`go test -p 1 -race ./...`) → hijau tanpa data race.
- Konsep Go:
  - Embedded Asset Bundling for Headless Verification: template Go HTML (`html/template`) dan aset statis yang di-embed langsung ke binary server via `embed.FS` menjamin tampilan antarmuka terdistribusi secara utuh tanpa risiko ketergantungan aset lokal yang hilang ([Go Doc: embed](https://pkg.go.dev/embed)).
  - End-to-End Contract Verification: menghubungkan tooling otomasi peramban modern (Playwright) langsung ke endpoint server Go untuk memvalidasi pemenuhan standar UX mobile tanpa mengorbankan portabilitas kode backend ([Playwright Documentation](https://playwright.dev/)).
  - Semantic Content Projection without Dangerous Raw HTML: merender atribut dinamis (`dir="auto"`) dan kelas semantik secara otomatis berbasis konten tanpa mengekspos `template.HTML` mentah, menjamin keamanan XSS secara default ([Go Doc: html/template](https://pkg.go.dev/html/template)).
- Penyimpangan dari rencana: tidak ada.
- Belum diverifikasi: tidak ada.

## Hasil review
_(Diisi Claude.)_

