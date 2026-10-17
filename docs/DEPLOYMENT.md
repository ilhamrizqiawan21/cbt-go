# PANDUAN DEPLOYMENT — CBT Sekolah (Go)

Dokumen ini menjelaskan prosedur instalasi, konfigurasi, dan pemeliharaan server aplikasi **CBT Sekolah** berbasis Go pada lingkungan server laptop sekolah MTs Al-Ihsan Batujajar (maupun server Linux standar).

---

## 1. Arsitektur Deployment

- **Aplikasi**: Binary tunggal Go (`bin/server`) yang meng-embed seluruh template HTML dan aset statis via `embed.FS`.
- **Database**: MySQL 8.0 / MariaDB 10.5+ lokal (akses via UNIX domain socket `/var/lib/mysql/mysql.sock` atau TCP `127.0.0.1:3306`).
- **Process Manager**: `systemd` dengan konfigurasi `Restart=always` untuk auto-restart saat terjadi kegagalan atau boot server.
- **Logging**: Standar `log/slog` yang langsung ditangkap oleh `systemd-journald` (`journalctl -u cbt-sekolah`).

---

## 2. Kompilasi & Build Binary

Jalankan kompilasi produksi di lingkungan pengembangan atau server target:

```bash
# Bersihkan dan buat binary release
make build

# Verifikasi binary hasil kompilasi
./bin/server --help 2>&1 | head -n 5 || ls -lh bin/server
```

Binary yang dihasilkan berada di `bin/server` dan sepenuhnya mandiri (self-contained).

---

## 3. Konfigurasi Lingkungan (`.env`)

Salin berkas konfigurasi template:

```bash
cp .env.example .env
chmod 600 .env
```

Contoh konfigurasi produksi standar (`.env`):

```ini
APP_ENV=production
PORT=3000
APP_TIMEZONE=Asia/Jakarta
TRUST_PROXY=false
TAMPILKAN_NILAI_SISWA=false
LOG_LEVEL=info

# Konfigurasi Database (Gunakan Socket untuk Latensi Rendah di Server Lokal)
DB_SOCKET=/var/lib/mysql/mysql.sock
DB_USER=cbt_app
DB_PASSWORD=PasswordKuatDb123!
DB_NAME=cbt_sekolah_go
DB_MAX_OPEN_CONNS=50
```

> [!IMPORTANT]
> - Pastikan zona waktu `APP_TIMEZONE` selaras dengan zona waktu MySQL (`Asia/Jakarta`).
> - Cookie sesi otomatis mengaktifkan `HttpOnly=true` dan `SameSite=Lax`. Flag `Secure=true` aktif otomatis pada mode production atau bila `TRUST_PROXY=true` (tunnel HTTPS). Bila server diakses langsung via HTTP IP lokal sekolah (`http://192.168.x.x:3000`), atur `COOKIE_SECURE=false` di `.env` agar peramban siswa tidak menolak cookie sesi.

---

## 4. Migrasi Database Awal

Sebelum menjalankan service, terapkan seluruh migrasi tabel goose:

```bash
./bin/migrate up
```

---

## 5. Konfigurasi Service Systemd

Buat berkas unit systemd di `/etc/systemd/system/cbt-sekolah.service`:

```ini
[Unit]
Description=CBT Sekolah - MTs Al-Ihsan Batujajar (Go)
After=network.target mariadb.service mysql.service
Wants=mariadb.service mysql.service

[Service]
Type=simple
User=ilham-rizqiawan
Group=ilham-rizqiawan
WorkingDirectory=/home/ilham-rizqiawan/Projects/cbt-go
ExecStart=/home/ilham-rizqiawan/Projects/cbt-go/bin/server
Restart=always
RestartSec=3s

# Batas file descriptor untuk menangani 500+ koneksi konkuren
LimitNOFILE=65535

# Isolasi Keamanan Dasar
PrivateTmp=true
ProtectSystem=full
NoNewPrivileges=true

# Environment & Logging
EnvironmentFile=/home/ilham-rizqiawan/Projects/cbt-go/.env
StandardOutput=journal
StandardError=journal
SyslogIdentifier=cbt-sekolah

[Install]
WantedBy=multi-user.target
```

### Mengaktifkan dan Menjalankan Service

```bash
# Reload daemon systemd
sudo systemctl daemon-reload

# Aktifkan auto-start saat booting laptop/server
sudo systemctl enable cbt-sekolah.service

# Jalankan service
sudo systemctl start cbt-sekolah.service

# Periksa status
sudo systemctl status cbt-sekolah.service
```

### Memeriksa Log Aplikasi

```bash
# Pantau log secara realtime (streaming)
sudo journalctl -u cbt-sekolah -f

# Lihat log error terakhir
sudo journalctl -u cbt-sekolah -p err -n 50
```

---

## 6. Verifikasi Operasional (`Health Check`)

Lakukan tes kesehatan server:

```bash
curl -i http://localhost:3000/healthz
```

Respon yang diharapkan:
```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8

{"status":"ok"}
```

---

## 7. Prosedur Pembaruan (Deployment Update)

Untuk merilis pembaruan kode baru tanpa mengganggu integritas sesi:

```bash
# 1. Tarik pembaruan kode terbaru
git pull origin main

# 2. Kompilasi binary baru
make build

# 3. Jalankan migrasi basis data (bila ada tabel baru)
./bin/migrate up

# 4. Restart service systemd
sudo systemctl restart cbt-sekolah.service

# 5. Verifikasi status
sudo systemctl status cbt-sekolah.service
curl -s http://localhost:3000/healthz
```
Sesi siswa tidak akan hilang karena state sesi tersimpan secara persisten pada tabel `sessions` dan database `sesi_ujian`.

---

## 8. Manajemen Akun Admin & Guru Produksi (Ganti Password — F-7)

Akun admin dan guru default dihasilkan melalui `cmd/seed` (`admin / admin123`, `guru / guru123`). Pada server produksi, password wajib diganti menggunakan password kuat.

### Cara 1: Mengganti Password via CLI MySQL/MariaDB

Password disimpan dalam bentuk hash `bcrypt` (cost 10). Anda dapat membuat hash baru menggunakan one-liner Go, lalu memperbarui tabel basis data:

1. Buat hash password baru di terminal server:
```bash
go run -e 'package main; import ("fmt"; "golang.org/x/crypto/bcrypt"); func main() { h, _ := bcrypt.GenerateFromPassword([]byte("GantiDenganPasswordKuatAnda!"), 10); fmt.Println(string(h)) }'
```

2. Jalankan query SQL di MySQL/MariaDB:
```sql
-- Untuk akun admin:
UPDATE admin SET password = '<hasil_hash_bcrypt_dari_langkah_1>' WHERE username = 'admin';

-- Untuk akun guru:
UPDATE guru SET password = '<hasil_hash_bcrypt_dari_langkah_1>' WHERE username = 'guru';
```

### Cara 2: Menambah Akun Admin Baru

```sql
INSERT INTO admin (username, password, nama)
VALUES ('proktor_utama', '<hasil_hash_bcrypt>', 'Proktor Ujian MTs');
```


