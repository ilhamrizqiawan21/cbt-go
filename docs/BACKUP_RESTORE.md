# PANDUAN BACKUP & RESTORE — CBT Sekolah (Go)

Dokumen ini memuat prosedur operasional standar (SOP) untuk pencadangan (backup) dan pemulihan bencana (disaster recovery / restore) basis data **CBT Sekolah**.

---

## 1. Lokasi & Skrip Terkait

- **Skrip Backup**: `scripts/backup.sh` (dump database MySQL/MariaDB dengan opsi kompresi gzip & rotasi otomatis).
- **Skrip Restore**: `scripts/restore.sh` (dekompresi dan import berkas dump ke database target).
- **Direktori Default**: `./backups/` (atau dikustomisasi via opsi `--out-dir`).
- **Pemberian Izin Eksekusi**:
  ```bash
  chmod +x scripts/backup.sh scripts/restore.sh
  ```

---

## 2. Prosedur Pencadangan (Backup)

### A. Eksekusi Manual

Untuk membuat snapshot database saat ini:

```bash
# Backup database default (cbt_sekolah_go) ke folder ./backups/
./scripts/backup.sh

# Atau tentukan parameter spesifik:
./scripts/backup.sh --db=cbt_sekolah_go --socket=/var/lib/mysql/mysql.sock --keep=30
```

Hasil backup berupa berkas terkompresi berformat:
`./backups/cbt_<nama_db>_YYYYMMDD_HHMMSS.sql.gz`

### B. Otomatisasi via Cron Job (Hari Ujian)

Saat ujian berlangsung, disarankan melakukan backup berkala **setiap 10 menit**:

Buka editor cron:
```bash
crontab -e
```

Tambahkan baris berikut:
```cron
# Backup database CBT Sekolah setiap 10 menit selama jam operasional (07:00 - 15:00)
*/10 7-15 * * * /home/ilham-rizqiawan/Projects/cbt-go/scripts/backup.sh >> /home/ilham-rizqiawan/Projects/cbt-go/backups/backup.log 2>&1
```

### C. Rotasi Berkas Backup

Skrip `scripts/backup.sh` otomatis mempertahankan **30 snapshot terakhir** (dapat diubah via opsi `--keep=<jumlah>`). Berkas backup tertua yang melebihi kuota akan dihapus secara otomatis untuk menghemat ruang disk laptop sekolah.

---

## 3. Prosedur Pemulihan (Restore)

> [!WARNING]
> Operasi restore akan menimpa data pada database tujuan. Pastikan database target sudah benar sebelum melanjutkan.

### A. Menentukan Berkas Backup yang Akan Dipulihkan

Lihat daftar cadangan yang tersedia, urutkan berdasarkan waktu terbaru:
```bash
ls -lhrt backups/*.sql.gz
```

### B. Menjalankan Restore

Jalankan perintah restore menggunakan `scripts/restore.sh`:

```bash
./scripts/restore.sh --file=backups/cbt_cbt_sekolah_go_20261016_211130.sql.gz --yes
```

Atau jika menggunakan UNIX socket lokal:
```bash
./scripts/restore.sh \
  --file=backups/cbt_cbt_sekolah_go_20261016_211130.sql.gz \
  --db=cbt_sekolah_go \
  --socket=/var/lib/mysql/mysql.sock \
  --yes
```

### C. Verifikasi Pasca Restore

1. Periksa jumlah rekaman pada tabel utama:
   ```bash
   mysql -u root -S /var/lib/mysql/mysql.sock -e "SELECT count(*) AS total_siswa FROM cbt_sekolah_go.siswa; SELECT count(*) AS total_jawaban FROM cbt_sekolah_go.jawaban_siswa;"
   ```
2. Pastikan service CBT berjalan normal:
   ```bash
   sudo systemctl restart cbt-sekolah.service
   curl -s http://localhost:3000/healthz
   ```

---

## 4. Disaster Recovery & Salinan Eksternal (Flashdisk)

Untuk mengantisipasi kerusakan fisik pada laptop sekolah saat hari-H:

1. **Siapkan Flashdisk USB**: Hubungkan flashdisk ke laptop sekolah.
2. **Sinkronisasi Berkala**:
   ```bash
   # Salin berkas backup terbaru ke flashdisk (misal di-mount pada /media/usb)
   rsync -av --update ./backups/ /media/usb/cbt_backups/
   ```
3. Jika laptop utama mati mendadak, flashdisk dapat langsung dipasang ke laptop cadangan dan dipulihkan dalam waktu kurang dari 2 menit menggunakan `scripts/restore.sh`.

