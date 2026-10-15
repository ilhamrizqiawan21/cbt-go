# STACK — Go disandingkan dengan apa saja

Panduan untuk pemula Go. Menjawab: "Go ini dipasangkan dengan apa saja untuk menggantikan tumpukan Node/Express?" Keputusan resmi ada di [DECISION](DECISION.md) G-003; dokumen ini menjelaskan **mengapa** dan **padanannya**.

> Status verifikasi: versi library di bawah **belum dikunci**. Sebelum menambahkan sebuah library, eksekutor wajib memeriksa dokumentasi resmi/terbaru (dan memastikan tidak deprecated), lalu memakai `go get <modul>@latest`. Yang sudah terpasang dan teruji di kerangka: `go-sql-driver/mysql v1.10.1`, `joho/godotenv v1.5.1`.

## 1. Prinsip: Go suka "bawaan dulu"
Berbeda dengan Node (ekosistem npm besar, hampir semuanya library), Go punya **standard library yang sangat lengkap**. Untuk web server, JSON, template, logging, tes, dan kriptografi dasar Anda tidak perlu framework. Karena itu daftar dependensi proyek ini sengaja pendek.

## 2. Peta padanan Node → Go
| Kebutuhan | Versi Node | Versi Go | Jenis | Catatan untuk pemula |
|-----------|-----------|----------|-------|----------------------|
| Web server + routing | `express` | `net/http` (`http.ServeMux`) | **bawaan** | Sejak Go 1.22 mendukung `"GET /ujian/{id}"` dan `r.PathValue("id")` |
| Middleware | `app.use(...)` | fungsi `func(http.Handler) http.Handler` | bawaan | Pola "pembungkus"; contoh ada di `internal/httpserver/middleware.go` |
| Template HTML | `ejs` | `html/template` | **bawaan** | Escape XSS otomatis. Sintaks `{{ .Nama }}`, `{{ range }}`, `{{ if }}` |
| Embed aset ke binary | — | `embed` (`//go:embed`) | bawaan | Template + CSS + font ikut masuk satu file binary |
| JSON | `JSON.parse` | `encoding/json` | bawaan | Struct dengan tag `json:"nama"` |
| Logging | `winston` | `log/slog` | bawaan | Terstruktur (key=value / JSON) |
| Tes | `node:test` | `testing` + `net/http/httptest` | bawaan | File `*_test.go`; `go test ./...` |
| Env / konfigurasi | `dotenv` | `os.Getenv` + `godotenv` | library kecil | Sudah terpasang |
| Koneksi MySQL | `mysql2` | `database/sql` + `go-sql-driver/mysql` | bawaan + driver | Driver mendaftar diri; kode memakai `database/sql` |
| Migrasi DB | `scripts/migrate.js` | `pressly/goose` | library | SQL murni `-- +goose Up/Down` (G-007) |
| Hash password | `bcrypt` | `golang.org/x/crypto/bcrypt` | resmi Go (x/) | Kompatibel dengan hash bcrypt yang sudah ada |
| Session login | `express-session` + `express-mysql-session` | `alexedwards/scs` + `mysqlstore` | library | Cookie + penyimpanan di MySQL (tabel `sessions`) |
| CSRF | cek Origin manual | `http.CrossOriginProtection` | **bawaan** (Go ≥1.25) | Memeriksa header Origin/Sec-Fetch-Site |
| Rate limit | `express-rate-limit` | `golang.org/x/time/rate` | resmi Go (x/) | Dibungkus peta limiter per kunci (NIS/IP) |
| Realtime | `socket.io` | `coder/websocket` | library | WebSocket polos; bukan Socket.io (G-005) |
| Excel | `exceljs` | `xuri/excelize/v2` | library | Baca/tulis `.xlsx` |
| UUID (device token) | `crypto.randomUUID` | `crypto/rand` (+ format UUID) atau `google/uuid` | bawaan / kecil | Pilih yang paling sederhana |
| Jadwal job latar | `setInterval` | `time.Ticker` + goroutine | bawaan | Dihentikan lewat `context` |
| Proses manager | `pm2` | `systemd` | OS | Auto-restart + mulai saat boot |
| Uji beban | `scripts/loadtest.js` | tetap `tools/loadtest/loadtest.js` | Node (alat uji) | Hanya di mesin penguji |

## 3. Konsep Go yang akan Anda temui (urutan kemunculan)
1. **Package & module** — `go.mod`, `internal/` (paket di dalamnya tak bisa diimpor dari luar modul).
2. **Error sebagai nilai** — tidak ada `try/catch`; fungsi mengembalikan `(hasil, error)` dan Anda memeriksa `if err != nil`. Bungkus dengan `%w`.
3. **Struct & method** — pengganti class; tidak ada pewarisan.
4. **Interface (implisit)** — tipe memenuhi interface otomatis bila punya method-nya. Definisikan interface **di sisi pemakai**, kecil, hanya bila perlu (contoh: `Pinger`).
5. **`defer`** — jalankan sesuatu saat fungsi selesai (tutup koneksi, rollback).
6. **`context.Context`** — pembawa pembatalan/timeout yang diteruskan ke DB, HTTP, job. Dasar *graceful shutdown*.
7. **Goroutine & channel** — konkurensi ringan. Job penutupan otomatis dan hub WebSocket memakainya.
8. **`sync.Mutex` / `-race`** — melindungi data bersama; `go test -race` mendeteksi balapan data.
9. **Tes berbasis tabel** — pola tes idiomatik Go.
10. **Transaksi `database/sql`** — `BeginTx` → `defer tx.Rollback()` → `Commit()`.

## 4. Yang sengaja TIDAK dipakai (dan alasannya)
| Tidak dipakai | Alasan |
|---------------|--------|
| Framework web (Gin, Echo, Fiber) | `net/http` sudah cukup sejak Go 1.22; lebih sedikit dependensi dan lebih baik untuk belajar |
| ORM (GORM, Ent) | SQL sudah ada dan sederhana; ORM menyembunyikan apa yang terjadi dan menambah belajar (G-008) |
| `templ` / SPA (React) | Cukup `html/template`; tampilan sudah Bootstrap + JS vanilla (G-011) |
| Socket.io (Go port) | Protokol rumit, tidak ada port resmi; WebSocket polos cukup (G-005) |
| Redis / message broker | Satu proses, satu laptop (D-001); state di MySQL dan memori |
| Docker | Tidak ada di mesin ini/sekolah; binary + systemd lebih sederhana |

## 5. Alat bantu yang dipasang user
| Alat | Fungsi | Cara |
|------|--------|------|
| Go toolchain (≥1.25, disarankan terbaru) | Kompilasi & tes | `sudo dnf install golang` (periksa versi dengan `go version`) atau tarball dari go.dev/dl |
| make, gcc | `make` menjalankan perintah di Makefile; gcc syarat `go test -race` (cgo) | `sudo dnf install make gcc` |
| MySQL 8.x | Database | Sudah ada (8.4.10) |
| `gopls` + ekstensi Go di VS Code | Editor: autocompletion, error langsung | Ekstensi "Go" resmi |
| `staticcheck` (opsional) | Linter tambahan | `go install honnef.co/go/tools/cmd/staticcheck@latest` |

## 6. Cara belajar sambil mengerjakan
- Resmi & gratis: **A Tour of Go** (go.dev/tour), **Effective Go**, **Go by Example**, dan dokumentasi paket stdlib di pkg.go.dev.
- Setiap item TODO punya baris **"Konsep Go"**; baca bagian itu di Tour/Go by Example sebelum membaca kodenya.
- Jalankan `go doc net/http.ServeMux` (atau `go doc <paket>.<simbol>`) untuk membaca dokumentasi langsung dari terminal.
