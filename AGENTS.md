# AGENTS — Panduan untuk Agen AI di Proyek CBT Sekolah (Go)

Berlaku untuk semua agen (Claude, Gemini, lainnya). File khusus: [GEMINI.md](GEMINI.md), [CLAUDE.md](CLAUDE.md). Aturan wajib: [docs/RULES_AI.md](docs/RULES_AI.md).

## Ringkasan proyek
Penulisan ulang **CBT Sekolah** (Computer Based Test untuk MTs Al-Ihsan Batujajar, ±410 siswa, target 500 serentak; HP dan laptop; server di laptop sekolah) dari Node.js ke **Go**. Konsep, aturan bisnis, dan kontrak HTTP **sama** dengan versi Node; teknologi berbeda. Peran pengguna: admin (memantau), guru (soal dan nilai essay), siswa (ujian). Detail: [PRD](docs/PRD.md).

Proyek Node ada di `~/Projects/CBT-Sekolah` — **read-only**, dipakai sebagai referensi perilaku dan pembanding performa.

## Urutan baca sebelum bekerja
1. [PRD](docs/PRD.md) — apa dan mengapa
2. [DECISION](docs/DECISION.md) — keputusan yang tidak boleh dilanggar
3. [DESIGN](docs/DESIGN.md) — bagaimana (alur, kontrak HTTP, realtime, keamanan)
4. [ERD](docs/ERD.md) — skema data (sumber: `migrations/`)
5. [STACK](docs/STACK.md) — padanan Node→Go dan konsep Go
6. [PORTING_MAP](docs/PORTING_MAP.md) — di mana perilaku asli berada
7. [TODO](docs/TODO.md) — pekerjaan dan statusnya

## Pembagian peran (G-012)
| Peran | Agen | Tugas |
|-------|------|-------|
| Perancang & reviewer | Claude | Menyusun dokumen, mereview hasil, menjaga konsistensi dengan DECISION |
| **Eksekutor** | **Gemini** | Mengerjakan item TODO satu per satu |
| Pemilik keputusan | User | Menyetujui perubahan keputusan, aksi destruktif, commit/push; sedang belajar Go |

## Stack dan perintah
- Go ≥ 1.25 (`go.mod`), `net/http`, `html/template`, `database/sql` + `go-sql-driver/mysql`, goose, scs, `coder/websocket`, excelize, `log/slog`. MySQL/MariaDB.
- `make run` · `make build` · `make test` · `make test-race` (butuh gcc) · `make vet` · `make fmt` · `make check`.
- Konfigurasi: salin `.env.example` ke `.env`. **Jangan** membaca atau menampilkan `.env`.

## Struktur
```
cmd/server/  cmd/migrate/  cmd/seed/        entry point
internal/<fitur>/                           config db httpserver auth sesi penilaian soal monitor realtime excel ratelimit testdb
migrations/                                 goose, SQL murni (sumber kebenaran skema)
web/templates/  web/static/                 html/template + aset (di-embed)
tools/loadtest/  scripts/                   uji beban (Node), backup
docs/                                       dokumen perancangan
```
Detail dan paket mana yang sudah ada: [DESIGN §2](docs/DESIGN.md).

## Konvensi kode
- `gofmt` selalu. Idiom Go; error dibungkus `%w`; `context` sebagai parameter pertama untuk operasi DB/IO.
- Query SQL **selalu** berparameter (`?`), tanpa penggabungan string dari input.
- Logika bisnis sebagai fungsi murni yang diuji (tes berbasis tabel); handler HTTP tipis.
- Identifier dan commit message dalam Bahasa Inggris. Komentar kode dalam Bahasa Indonesia (mengikuti kerangka yang ada). Teks antarmuka dan pesan ke pengguna dalam Bahasa Indonesia.
- Komentar seperlunya: jelaskan *mengapa*, bukan *apa*; plus penjelasan idiom Go untuk pemula.

## Hal yang sering salah di proyek ini
- PIN siswa disimpan **hash bcrypt**; jangan bandingkan dengan teks biasa.
- `siswa.kelas` berisi **nama** kelas, bukan id.
- Kunci jawaban (`soal.jawaban_benar`) tidak boleh dikirim ke klien — pakai struct respons khusus.
- Waktu dihitung dengan jam **database** (`NOW()`); `APP_TIMEZONE` harus sama dengan zona MySQL.
- `*.sql` diabaikan git kecuali `migrations/*.sql`.
- IP sekolah dipakai bersama banyak siswa; jangan rate-limit hanya per IP.
- Jangan menyentuh database `cbt_sekolah` (milik Node).
