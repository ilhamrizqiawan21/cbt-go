# CBT Sekolah (Go)

Penulisan ulang aplikasi **CBT Sekolah** (Computer Based Test untuk MTs Al-Ihsan Batujajar) dari Node.js ke **Go**. Konsep dan aturan bisnis sama dengan versi Node (`~/Projects/CBT-Sekolah`); teknologi berbeda. Tujuan: paritas perilaku, kapasitas **500 siswa serentak**, dan belajar Go.

> **Status:** kerangka + dokumen perancangan selesai; implementasi fitur belum dimulai. Lihat [docs/TODO.md](docs/TODO.md).

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

## Mulai (setelah T0.1 oleh user)
```bash
cp .env.example .env            # isi DB_*
make check                      # gofmt + vet + test
make run                        # lalu: curl localhost:3000/healthz
```
Prasyarat: Go ≥ 1.25, `make`, MySQL 8.x, gcc (untuk `make test-race`), database `cbt_sekolah_go` dan `cbt_sekolah_go_test` (lihat T0.1).

## Struktur
```
cmd/server/            entry point
internal/{config,db,httpserver}/   sudah ada; paket lain dibuat bertahap per item TODO
migrations/            skema (goose, SQL)
web/{templates,static}/  tampilan + aset
tools/loadtest/        uji beban (Node, hanya di mesin penguji)
scripts/backup.sh      backup database
docs/                  dokumen perancangan
```
