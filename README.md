# CBT Sekolah (Go)

Penulisan ulang aplikasi **CBT Sekolah** (Computer Based Test untuk MTs Al-Ihsan Batujajar) dari Node.js ke **Go**. Konsep dan aturan bisnis sama dengan versi Node (`~/Projects/CBT-Sekolah`); teknologi berbeda. Tujuan: paritas perilaku, kapasitas **500 siswa serentak**, dan belajar Go.

> **Status:** Fase 0 s.d. Fase 5 selesai (paritas dengan versi Node). Fase 6 (gambar soal, impor Excel, salin soal, ujian susulan, ganti password, analisis butir, daftar hadir) selesai diimplementasikan, **belum direview** (R-FINAL-GO belum dijalankan). Setelah memperbarui kode, jalankan `./bin/migrate up` (migrasi `00002_izin_susulan`). Lihat [docs/TODO.md](docs/TODO.md).

## Dokumentasi
| Dokumen | Isi |
|---------|-----|
| [docs/PRD.md](docs/PRD.md) | Kebutuhan produk, aturan penilaian, kriteria selesai |
| [docs/DECISION.md](docs/DECISION.md) | Keputusan (D-xxx warisan Node, G-xxx khusus Go) |
| [docs/DESIGN.md](docs/DESIGN.md) | Arsitektur, alur, kontrak HTTP, WebSocket, keamanan |
| [docs/ERD.md](docs/ERD.md) | Skema data (sumber: [migrations/](migrations/)) |
| [docs/STACK.md](docs/STACK.md) | **Go disandingkan dengan apa saja** + konsep Go untuk pemula |
| [docs/PORTING_MAP.md](docs/PORTING_MAP.md) | Peta berkas/rute/tes Node → Go |
| [docs/RULES_AI.md](docs/RULES_AI.md) | Aturan wajib untuk agen AI |
| [docs/TODO.md](docs/TODO.md) | Rencana implementasi per item + status |
| [GEMINI.md](GEMINI.md) · [CLAUDE.md](CLAUDE.md) · [AGENTS.md](AGENTS.md) | Panduan per agen |
| [docs/MOCKUP_UI.md](docs/MOCKUP_UI.md) | Spesifikasi mockup UI 6 layar (token desain, responsive) |
| [docs/REVIEW_KESIAPAN.md](docs/REVIEW_KESIAPAN.md) | Laporan review kesiapan pra-ujian dan mitigasi temuan |
| [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) | Panduan instalasi server, unit systemd, dan manajemen akun |
| [docs/BACKUP_RESTORE.md](docs/BACKUP_RESTORE.md) | Panduan backup otomatis cron 10 menit dan prosedur restore |
| [GEMINI.md](GEMINI.md) · [CLAUDE.md](CLAUDE.md) · [AGENTS.md](AGENTS.md) | Panduan peran per agen |

## Mulai (setelah T0.1 oleh user)
## Mulai
```bash
cp .env.example .env            # isi DB_*
cp .env.example .env            # sesuaikan DB_* dan COOKIE_SECURE
make check                      # gofmt + vet + test
make run                        # lalu: curl localhost:3000/healthz
make test-race                  # detektor race condition (butuh gcc)
make build                      # kompilasi binary ke bin/server
make run                        # jalankan server: curl localhost:3000/healthz
```
Prasyarat: Go ≥ 1.25, `make`, MySQL 8.x, gcc (untuk `make test-race`), database `cbt_sekolah_go` dan `cbt_sekolah_go_test` (lihat T0.1).
Prasyarat: Go ≥ 1.25, `make`, MySQL 8.x / MariaDB, gcc (untuk `make test-race`), database `cbt_sekolah_go` dan `cbt_sekolah_go_test`.

## Struktur
## Struktur Direktori
```
cmd/server/            entry point
internal/{config,db,httpserver}/   sudah ada; paket lain dibuat bertahap per item TODO
migrations/            skema (goose, SQL)
web/{templates,static}/  tampilan + aset
tools/loadtest/        uji beban (Node, hanya di mesin penguji)
scripts/backup.sh      backup database
docs/                  dokumen perancangan
cmd/server/            entry point server aplikasi
cmd/migrate/           cli migrasi database (goose)
cmd/seed/              cli seeding data awal
internal/              logika bisnis (admin, auth, config, db, excel, guru, httpserver, monitor, penilaian, ratelimit, realtime, sesi, soal, testdb)
migrations/            skema basis data (SQL goose berurutan)
web/templates/         template HTML responsif Go html/template
web/static/            aset statis lokal (CSS, JS, Font Sora/DM Sans/Amiri, vendor Bootstrap)
scripts/               skrip pendukung (backup.sh, restore)
tests/ui/              pengujian antarmuka Playwright (Arab 360px HP & monitor admin)
docs/                  arsitektur, dokumen perancangan, dan panduan operasional
```
