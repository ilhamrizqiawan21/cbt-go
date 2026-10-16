# PANDUAN OPERASIONAL HARI-H — CBT Sekolah (Go)

Panduan operasional ringkas untuk **Admin / Tim Teknis** MTs Al-Ihsan Batujajar selama pelaksanaan ujian berbasis komputer menggunakan CBT Sekolah versi Go.

---

## 1. Persiapan H-1 (Sebelum Hari Pelaksanaan)

- [ ] **Kelistrikan Laptop Server**:
  - Charger terhubung ke sumber daya stabil (gunakan stabilizer / UPS jika ada).
  - Nonaktifkan mode *Sleep / Suspend / Hibernate* saat layar laptop ditutup (`lid close` settings di OS).
- [ ] **Jaringan & Router WiFi**:
  - Router WiFi ujian aktif, DHCP server siap mengalokasikan minimal 450 IP address.
  - Laptop server terhubung via **kabel LAN** ke router utama (jangan menggunakan WiFi untuk server).
  - Pastikan IP server statis / fixed (misal `192.168.1.100`).
- [ ] **Verifikasi Database & Master Data**:
  - Data siswa, kelas, mata pelajaran, dan jadwal ujian telah terisi dan diverifikasi.
  - Soal ujian telah diinput oleh guru/admin dan kunci jawaban terverifikasi.
  - Jalankan backup database pra-ujian:
    ```bash
    ./scripts/backup.sh
    ```
- [ ] **Kesiapan Service**:
  - Pastikan service systemd berjalan normal:
    ```bash
    sudo systemctl status cbt-sekolah.service
    curl -s http://localhost:3000/healthz
    ```

---

## 2. Prosedur Jam Pelaksanaan (Hari-H)

### 30 Menit Sebelum Ujian Dimulai
1. **Rilis Token Ujian**:
   - Masuk ke dashboard Admin (`/admin/ujian`).
   - Salin Token Ujian yang berlaku hari ini dan umumkan kepada pengawas ruangan.
2. **Aktifkan Pemantauan Realtime**:
   - Buka halaman `/admin/monitor/<ujianId>` pada browser laptop proktor.
   - Pastikan indikator koneksi realtime hijau (terhubung ke WebSocket).
3. **Aktifkan Cron Backup Otomatis**:
   - Pastikan cron backup per 10 menit sudah aktif (lihat [BACKUP_RESTORE.md](BACKUP_RESTORE.md)).

### Saat Ujian Berlangsung
- **Siswa Ganti Perangkat (HP Rusak / Baterai Habis)**:
  - Siswa cukup login kembali pada perangkat baru dengan NIS, Kelas, PIN, dan Token yang sama.
  - Sistem Go akan secara otomatis melakukan **Takeover Sesi**: menerbitkan token perangkat baru, membatalkan token perangkat lama (409 Conflict), dan melanjutkan sisa waktu serta jawaban yang telah tersimpan.
- **Siswa Terkunci karena Pelanggaran (`keluar_paksa`)**:
  - Jika siswa keluar tab atau fullscreen melebihi toleransi (3 kali pada HP, 5 kali pada laptop), status menjadi `keluar_paksa`.
  - Admin/Proktor dapat membuka kunci pada monitor ujian dengan mengklik tombol **"Buka Kunci"**. Sesi siswa akan kembali menjadi `sedang_ujian` tanpa kehilangan data jawaban.
- **Perpanjangan Waktu Khusus**:
  - Klik tombol **"Tambah Waktu"** pada baris siswa di monitor admin untuk menambahkan waktu pengerjaan (misal 10 menit). Server secara instan mengirimkan sinyal websocket ke browser siswa untuk memperbarui hitung mundur.

---

## 3. Prosedur Pasca Ujian (Setelah Sesi Berakhir)

1. **Auto-Finalize Verifikasi**:
   - Background worker otomatis memfinalisasi sesi siswa yang telah habis batas waktunya ditambah masa tenggang grace period 120 detik.
   - Periksa halaman monitor bahwa semua siswa telah berstatus `selesai`. Jika masih ada siswa offline yang belum final, proktor dapat menekan tombol **"Paksa Selesai"**.
2. **Koreksi Nilai Essay**:
   - Guru pengampu mata pelajaran login ke `/login-guru`.
   - Masuk ke menu **Essay** (`/guru/essay/<ujianId>`).
   - Berikan nilai per butir essay (0 s.d. poin maksimal) langsung di web atau unduh template Excel, isi skor, lalu unggah kembali.
   - Status koreksi otomatis berubah menjadi `selesai` dan nilai total akhir terhitung secara akurat.
3. **Ekspor Rekapitulasi Nilai**:
   - Admin atau Guru mengunduh rekapitulasi nilai akhir dalam format spreadsheet Excel (`.xlsx`) via menu **Hasil Ujian** (`/admin/hasil/export?ujian_id=...` atau `/guru/hasil-siswa/export?ujian_id=...`).
4. **Pencadangan Final**:
   - Lakukan backup database pasca ujian dan salin ke flashdisk eksternal:
     ```bash
     ./scripts/backup.sh
     ```

