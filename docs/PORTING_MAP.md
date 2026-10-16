# PORTING_MAP — Peta Node → Go

Referensi untuk eksekutor. Proyek Node ada di **`/home/ilhamrizqiawan/Projects/CBT-Sekolah`** (read-only, RULES_AI §1.5). Dokumen ini menunjuk **di mana perilaku asli berada** dan **ke mana padanannya di Go**. Bila PORTING_MAP bertentangan dengan DESIGN/PRD, DESIGN/PRD menang.

> Cara memakai: sebelum mengerjakan item TODO, baca file Node yang tercantum di kolom "Sumber Node" untuk item itu, **pahami perilakunya**, lalu tulis tes Go (TDD), lalu implementasi. Jangan menerjemahkan baris-per-baris; tulis ulang secara idiomatik Go.

## 1. Berkas Node → paket Go
| Sumber Node | Isi | Padanan Go | Item TODO |
|-------------|-----|-----------|-----------|
| `app.js` (441 baris) | Express, session, Socket.io (event siswa & admin), job, error handler | `cmd/server`, `internal/httpserver`, `internal/realtime` | T0.6, T2.x |
| `models/db.js` | Pool MySQL | `internal/db` (ADA) | — |
| `controllers/authController.js` | Login siswa | `internal/auth` | T1.3 |
| `middleware/auth.js` | `isSiswaAPI`, `isAdmin`, `isGuru`, dll. | `internal/auth` (middleware) | T0.6, T3.1 |
| `middleware/csrfProtection.js` | Cek Origin | `http.CrossOriginProtection` | T0.6 |
| `middleware/rateLimiter.js` | Limiter login per NIS/IP | `internal/ratelimit` | T0.8 |
| `services/penilaianService.js` | `hitung`, `cekJawabanPG`, `cekJawabanMenjodohkan` | `internal/penilaian` | T1.1 |
| `services/sesiService.js` | `hitungBatasWaktu`, `hitungSisaDetik`, `bolehTerimaJawaban`, `ambilAlih`, `perpanjang`, `mulaiAtauLanjut`, `hitungPelanggaran` | `internal/sesi` | T1.2, T1.3 |
| `services/finalizeService.js` | `hitungUlangNilai`, `finalizeSesi`, `tutupSesiKadaluarsa`, `startAutoFinalizeJob` | `internal/sesi` (finalisasi + job) | T1.6, T1.7 |
| `services/monitorService.js` | `hitungStatusTurunan`, `getMonitorSnapshot`, `siarkanUpdateSiswa` | `internal/monitor` | T2.4 |
| `routes/index.js` | Landing, login (siswa/admin/guru), `/ujian`, logout | `internal/auth` + `internal/httpserver` | T0.7, T1.3, T3.1 |
| `routes/api.js` (618) | API siswa: sesi, soal, sinkron, heartbeat, selesai | `internal/sesi`, `internal/soal` | T1.4–T1.6 |
| `routes/admin.js` (839) | CRUD admin, monitor, hasil, reset | `internal/admin*` (per fitur) | T2.5, T3.x |
| `routes/guru.js` (1182) | Soal guru, hasil, log, essay | `internal/guru*` (per fitur) | T3.5–T3.8 |
| `utils/excelExport.js` | Ekspor Excel | `internal/excel` | T3.3, T3.6 |
| `utils/helper.js`, `utils/logger.js` | Util, winston | `log/slog` | — |
| `scripts/migrate.js` | Migrator | `cmd/migrate` (goose) | T0.3 |
| `scripts/seed.js` | Seed dev | `cmd/seed` | T0.5 |
| `scripts/loadtest.js` | Uji beban 410 klien | `tools/loadtest/loadtest.js` (disalin) | T1.8, T4.3 |
| `scripts/backup.sh` | Backup mysqldump | `scripts/backup.sh` (disalin) | T4.2 |
| `views/**/*.ejs` (36) | Tampilan | `web/templates/**/*.gohtml` | T0.7, tiap fitur |
| `public/**` | Aset statis | `web/static/**` (disalin) | — |
| `public/js/ujian.js` (736) | Klien ujian; memakai Socket.io | `web/static/js/ujian.js` — **ganti bagian socket ke WebSocket** | T2.3 |

## 2. Rute admin & guru (apa adanya di Node)
Semua di bawah prefix `/admin` (file `routes/admin.js`) atau `/guru` (`routes/guru.js`). Nomor = baris di file Node.
**Admin:** `GET /dashboard`(14) · guru: `GET /guru`(37) `POST /guru/tambah`(41) `/guru/hapus/:id`(49) `/guru/edit/:id`(53) · kelas: `GET /kelas`(70) `POST /kelas/tambah`(74) `/kelas/hapus/:id`(78) `/kelas/edit/:id`(82) · mapel: `GET /mapel`(92) `POST /mapel/tambah`(96) `/mapel/hapus/:id`(100) `/mapel/edit/:id`(104) · ujian: `GET /ujian`(114) `POST /ujian/tambah`(130) `GET /ujian/edit/:id`(140) `POST /ujian/edit/:id`(146) `POST /ujian/hapus/:id`(156) `GET /api/ujian-by-mapel/:mapelId`(160) · soal: `GET /soal`(170) `POST /soal/tambah`(182) `GET /soal/edit/:id`(193) `POST /soal/edit/:id`(199) `POST /soal/hapus/:id`(210) `GET /soal/tambah-batch`(214) `POST /soal/batch-tambah`(218) · pengajaran: `GET /pengajaran`(243) `POST /pengajaran/tambah`(250) `/pengajaran/hapus/:id`(256) · siswa: `GET /siswa`(297) `POST /siswa/tambah`(312) `/siswa/hapus/:id`(322) `GET /siswa/template`(327) `POST /siswa/import`(349) `GET /siswa/edit/:id`(412) `POST /siswa/edit/:id`(419) · hasil: `GET /hasil`(436) `GET /hasil/export`(805) `GET /hasil/cetak/:ujianId`(813) · `GET /log-kecurangan`(452) · monitor: `GET /monitor`(467) `GET /monitor/:ujianId`(484) `GET /api/monitor/:ujianId`(521) `POST /api/sesi/:id/buka-kunci`(543) `/tambah-waktu`(622) `/paksa-selesai`(708) · reset: `POST /reset-ujian`(777) `POST /reset-ujian-semua`(794).

**Guru:** `GET /dashboard`(11) · `GET /kelola-soal`(111) `GET /kelola-soal/tambah-batch`(165) `GET /api/ujian-by-guru`(180) `POST /soal/batch-tambah`(195) `GET /soal/edit/:id`(266) `POST /soal/edit/:id`(289) `POST /soal/hapus/:id`(324) `GET /api/soal/:ujianId`(344) · `GET /hasil-siswa`(350) `/hasil-siswa/export`(448) · `GET /log-kecurangan`(527) `/log-kecurangan/export`(627) · essay: `GET /essay`(742) `GET /essay/:ujianId`(775) `POST /essay/:ujianId`(841) `GET /essay/:ujianId/export`(944) `POST /essay/:ujianId/import`(1033).

Catatan: halaman guru memeriksa **otorisasi per mapel** (guru hanya soal/ujian miliknya) — tes Node: `guru_otorisasi_soal.test.js`.

## 3. Tes Node → tes Go (padanan wajib)
Tandai ✔ saat padanan Go lulus. `N/A` hanya dengan alasan tertulis.

| Tes Node | Perilaku yang diuji | Paket Go | Item | ✔ |
|----------|---------------------|----------|------|---|
| `penilaian_service.test.js` | Hitung nilai PG/menjodohkan/essay, contoh PRD §5 | `penilaian` | T1.1 | ✔ |
| `penilaian_fase3.test.js` | Penilaian + essay + status koreksi | `penilaian`, `sesi`, `guru` | T3.7 | ✔ |
| `sesi_service.test.js` | Batas waktu, sisa detik, grace, takeover, perpanjang | `sesi` | T1.2 | ✔ |
| `auth_db.test.js` | Login siswa (PIN, kelas, pesan generik) | `auth` | T1.3 | ✔ |
| `takeover_login.test.js` | Login ulang mengambil alih | `auth`, `sesi` | T1.3 | ✔ |
| `token_ujian.test.js` | Token ujian opsional | `auth` | T1.3 | ✔ |
| `soal_arab_hp.test.js` | Paket soal/Arab, tanpa kunci jawaban | `soal` | T1.4 | ✔ |
| `menjodohkan.test.js` | Soal menjodohkan: format & nilai | `penilaian`, `soal` | T1.1, T1.4 | ✔ |
| `sinkron_jawaban.test.js` | Upsert idempoten, grace, device_token | `sesi` | T1.5 | ✔ |
| `heartbeat_client.test.js` | Heartbeat / klien | `sesi` (+ JS) | T1.6 | ✔ |
| `finalize_service.test.js` | Finalisasi idempoten, auto-close | `sesi` | T1.6, T1.7 | ✔ |
| `ketahanan_e2e.test.js` | Putus-sambung: progres & timer tidak hilang | integrasi | T1.8 | ✔ |
| `sesi_endpoint_socket.test.js` | Endpoint sesi + event socket | `realtime`, `sesi` | T2.2 | ✔ |
| `socket_monitor.test.js` | Broadcast monitor, room admin | `realtime`, `monitor` | T2.4 | ✔ |
| `monitor_service.test.js` | Status turunan (online/offline/…) | `monitor` | T2.4 | ✔ |
| `halaman_monitor.test.js` | Halaman monitor, 410 baris | `monitor` | T2.6 | |
| `aksi_admin_audit.test.js` | Buka kunci/tambah waktu/paksa selesai + audit | `monitor` | T2.5 | |
| `buka_kunci.test.js` | Buka kunci tanpa hapus data | `monitor` | T2.5 | |
| `device_type.test.js` | Kebijakan hp vs laptop | `realtime`/`sesi` | T2.2 | ✔ |
| `kebijakan_perangkat.test.js` | Batas pelanggaran per perangkat | `sesi` | T2.2 | ✔ |
| `guru_otorisasi_soal.test.js` | Guru hanya soal mapelnya | `guru*` | T3.5 | ✔ |
| `security_audit.test.js` | Audit keamanan (kunci jawaban, rate limit, cookie) | lintas paket | T4.1 | |
| `production_env.test.js` | Konfigurasi produksi | `config` | T4.1 | |
| `migrate.test.js` | Migrator idempoten | `cmd/migrate` | T0.3 | ✔ |
| `persistent_seed.test.js` | Seed tetap setelah restart | `sesi` | T1.4 | ✔ |
| `backup_restore.test.js` | Backup & restore | skrip | T4.2 | |
| `loadtest.test.js` | Skrip loadtest | N/A (Node tool disalin) — alasan: alat uji, bukan fitur | — | N/A |
| `landing.test.js` | Halaman landing | `httpserver` | T0.7 | ✔ |
| `rfinal_minor.test.js` | Perbaikan MINOR review akhir Node | lintas | T4.1 | |
| `tests/ui/*.ui.js` | Playwright 360px Arab, monitor | dipakai ulang vs server Go | T4.4 | |
| `tests/helpers/db.js` | Helper DB uji | `internal/testdb` | T0.4 | |

Cara membaca isi tes Node: `sed -n 1,120p ~/Projects/CBT-Sekolah/tests/<nama>.test.js`. Gunakan sebagai **spesifikasi** (input → hasil yang diharapkan), bukan untuk disalin.

## 4. Perilaku Node yang SENGAJA berbeda di Go
| Hal | Node | Go | Alasan |
|-----|------|----|--------|
| Realtime | Socket.io | WebSocket polos | G-005 |
| Session store | `express-mysql-session` | `scs` + `mysqlstore` | G-003 |
| CSRF | `verifySameOrigin` manual | `http.CrossOriginProtection` | G-009 |
| Query monitor saat tak ada admin | Tetap dijalankan (MINOR di review Node) | **Dilewati** | DESIGN §3.6 |
| `GET /healthz` | Tidak ada | Ada | Operasional |
| Urutan acak soal | PRNG Node | `math/rand/v2` PCG | Hanya perlu stabil per `seed` |

Hal lain yang berbeda **harus dicatat di sini** oleh eksekutor beserta alasannya (dan ditandai `NEEDS_USER` bila mengubah perilaku yang terlihat pengguna).
