# ERD — CBT Sekolah (versi Go)

Dokumen terkait: [DESIGN](DESIGN.md), [PRD](PRD.md). **Sumber kebenaran skema: [migrations/](../migrations/)** (baseline `00001_baseline.sql`, G-007). ERD ini harus diperbarui bersama setiap migrasi baru.

Skema ini identik dengan skema akhir proyek Node (baseline + migrasi 001–003), ditambah tabel `sessions` untuk library session Go dan, sejak Fase 6, tabel `izin_susulan` (migrasi `00002`). Verifikasi baseline: dijalankan pada MySQL 8.4.10 → 15 tabel, 20 foreign key, `Down` bersih.

## 1. Diagram
```mermaid
erDiagram
    ADMIN { int id PK
            varchar username UK
            varchar password "bcrypt" }
    GURU { int id PK
           varchar nip
           varchar nama
           varchar username UK
           varchar password "bcrypt" }
    KELAS { int id PK
            varchar nama_kelas UK }
    MATA_PELAJARAN { int id PK
                     varchar nama_mapel }
    SISWA { int id PK
            varchar nis UK
            varchar nama
            varchar kelas "NAMA kelas, bukan FK (G-016)"
            varchar pin_ujian "bcrypt" }
    PENGAJARAN { int id PK
                 int guru_id FK
                 int mapel_id FK
                 int kelas_id FK }
    UJIAN { int id PK
            int pengajaran_id FK
            varchar nama_ujian
            int durasi "menit"
            datetime tanggal_mulai
            datetime tanggal_selesai
            tinyint acak_soal
            tinyint acak_pilihan
            int batas_pelanggaran
            varchar token_ujian "nullable" }
    SOAL { int id PK
           int ujian_id FK
           varchar tipe_soal "pg|menjodohkan|essay"
           text teks_soal
           varchar gambar
           int poin "pg=2 menjodohkan=2 essay=4"
           text pilihan_a_d
           text jawaban_benar "JANGAN kirim ke klien"
           text opsi_tambahan "JSON" }
    SESI_UJIAN { int id PK
                 int siswa_id FK
                 int ujian_id FK
                 varchar socket_id
                 datetime waktu_mulai
                 datetime batas_waktu
                 int tambahan_menit
                 int seed
                 char device_token
                 varchar device_type
                 datetime last_seen
                 datetime selesai_pada
                 enum status }
    JAWABAN_SISWA { int id PK
                    int siswa_id FK
                    int ujian_id FK
                    int soal_id FK
                    text jawaban_dipilih
                    tinyint is_benar
                    datetime client_ts
                    datetime diperbarui_pada }
    NILAI_ESSAY { int id PK
                  int siswa_id FK
                  int ujian_id FK
                  int soal_id FK
                  int skor "0..poin"
                  int dinilai_oleh FK
                  datetime dinilai_pada }
    NILAI_UJIAN { int id PK
                  int siswa_id FK
                  int ujian_id FK
                  int poin_otomatis
                  int poin_essay
                  int poin_maks
                  int nilai "0..100"
                  int benar
                  int salah
                  int kosong
                  enum status_koreksi
                  datetime selesai_pada }
    LOG_KECURANGAN { int id PK
                     int siswa_id FK
                     int ujian_id FK
                     varchar jenis_kecurangan
                     varchar device_type
                     timestamp timestamp }
    AUDIT_ADMIN { int id PK
                  int admin_id FK
                  int sesi_id FK
                  varchar aksi
                  text detail
                  timestamp dibuat_pada }
    IZIN_SUSULAN { int id PK
                   int siswa_id FK
                   int ujian_id FK
                   datetime tanggal_selesai "izin berlaku sampai"
                   varchar alasan
                   int dibuat_oleh FK "admin, SET NULL" }
    SESSIONS { char token PK "scs, 43 char"
               blob data
               timestamp expiry }

    GURU ||--o{ PENGAJARAN : mengampu
    MATA_PELAJARAN ||--o{ PENGAJARAN : dalam
    KELAS ||--o{ PENGAJARAN : untuk
    PENGAJARAN ||--o{ UJIAN : memiliki
    UJIAN ||--o{ SOAL : berisi
    SISWA ||--o{ SESI_UJIAN : memulai
    UJIAN ||--o{ SESI_UJIAN : dikerjakan
    SISWA ||--o{ JAWABAN_SISWA : menjawab
    SOAL ||--o{ JAWABAN_SISWA : dijawab
    SISWA ||--o{ NILAI_ESSAY : dinilai
    SOAL ||--o{ NILAI_ESSAY : dinilai_pada
    GURU ||--o{ NILAI_ESSAY : menilai
    SISWA ||--o{ NILAI_UJIAN : memperoleh
    UJIAN ||--o{ NILAI_UJIAN : menghasilkan
    SISWA ||--o{ LOG_KECURANGAN : melanggar
    UJIAN ||--o{ LOG_KECURANGAN : tercatat
    SISWA ||--o{ IZIN_SUSULAN : diberi
    UJIAN ||--o{ IZIN_SUSULAN : untuk
    ADMIN ||--o{ IZIN_SUSULAN : memberi
    ADMIN ||--o{ AUDIT_ADMIN : melakukan
    SESI_UJIAN ||--o{ AUDIT_ADMIN : dikenai
```

## 2. Aturan integritas
- `UNIQUE (siswa_id, ujian_id)` pada `sesi_ujian` dan `nilai_ujian`.
- `UNIQUE (siswa_id, ujian_id, soal_id)` pada `jawaban_siswa` dan `nilai_essay`.
- Seluruh FK `ON DELETE CASCADE`, kecuali `nilai_essay.dinilai_oleh` dan `audit_admin.admin_id` → `SET NULL`.
- `sesi_ujian.status`: `sedang_ujian | selesai | keluar_paksa`. Online/offline **diturunkan** dari `last_seen` (online bila < 45 detik), bukan disimpan.
- `nilai_ujian.status_koreksi`: `menunggu_essay | selesai`.
- `soal.tipe_soal`: `pg | menjodohkan | essay`.
- `jawaban_siswa.jawaban_dipilih`: PG = huruf `A`–`D`; menjodohkan = JSON array pasangan; essay = tidak dipakai.
- `nilai_essay.skor` harus `0 ≤ skor ≤ soal.poin` (divalidasi aplikasi).
- `UNIQUE (siswa_id, ujian_id)` pada `izin_susulan` (migrasi `00002`, G-026). Izin hanya memperpanjang batas akhir bagi siswa itu: login/paket soal/realtime menerima siswa bila jadwal ujian terbuka ATAU izin masih berlaku (`sesi.KondisiJendelaSiswa`), dan `batas_waktu` dibatasi oleh yang lebih akhir antara `ujian.tanggal_selesai` dan izin (`sesi.SQLAkhirEfektif`). Jejak audit (`dibuat_oleh`, `alasan`, `created_at`) ada di tabel ini karena `audit_admin.sesi_id` wajib terisi sedangkan sesi belum ada saat izin diberikan.
- `soal.gambar` hanya berisi nama berkas `<32 hex>.<png|jpg|webp>` (G-023); berkas ada di `uploads/soal/`, bukan di database.
- Indeks tambahan khusus Go: `idx_sesi_status_batas (status, batas_waktu)` untuk job penutupan otomatis, `idx_sesi_ujian_status (ujian_id, status)` untuk snapshot monitor.

## 3. Waktu dan zona waktu
- Semua perhitungan sisa waktu memakai jam **database** (`NOW()`), konsisten dengan DESIGN §3.2.
- Server MySQL laptop sekolah berzona `SYSTEM` = WIB. `APP_TIMEZONE` (default `Asia/Jakarta`) harus **sama** dengan zona server MySQL; T0.4 menambahkan pemeriksaan saat startup.
- Kolom `DATETIME` tanpa zona; driver membaca dengan `parseTime=true` dan `loc` = `APP_TIMEZONE`.

## 4. Tabel `sessions` (scs)
Struktur mengikuti dokumentasi `github.com/alexedwards/scs/mysqlstore`: `token CHAR(43) PK`, `data BLOB`, `expiry TIMESTAMP(6)` + indeks `expiry`. **Berbeda** dari `express-mysql-session`, sehingga sesi login Node dan Go tidak saling kompatibel (memang tidak perlu: database terpisah). T0.6 wajib memverifikasi struktur ini ke dokumentasi scs terbaru.

## 5. Catatan data
- Data siswa asli tidak boleh masuk repo, dump, atau log. Gunakan data sintetis (NIS `T0001`…, PIN `1234` hanya di DB uji).
- `*.sql` diabaikan git kecuali `migrations/*.sql`.
- Migrasi baru: nama `NNNNN_deskripsi.sql`, format goose (`-- +goose Up/Down`), tanpa menghapus data, idempoten bila memungkinkan.
