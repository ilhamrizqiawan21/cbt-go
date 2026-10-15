-- +goose Up
-- Baseline skema CBT Sekolah (Go).
-- Gabungan: database/schema.sql + migrasi Node 001-003 + tabel session scs.
-- Sumber kebenaran skema: folder migrations/ ini (lihat docs/ERD.md).
-- Database dibuat manual (lihat README), bukan oleh migrasi:
--   CREATE DATABASE cbt_sekolah_go CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

CREATE TABLE admin (
  id         INT AUTO_INCREMENT PRIMARY KEY,
  username   VARCHAR(50)  NOT NULL UNIQUE,
  password   VARCHAR(100) NOT NULL,            -- hash bcrypt
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB;

CREATE TABLE guru (
  id         INT AUTO_INCREMENT PRIMARY KEY,
  nip        VARCHAR(30)  NOT NULL,
  nama       VARCHAR(100) NOT NULL,
  username   VARCHAR(50)  NOT NULL UNIQUE,
  password   VARCHAR(100) NOT NULL,            -- hash bcrypt
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB;

CREATE TABLE kelas (
  id         INT AUTO_INCREMENT PRIMARY KEY,
  nama_kelas VARCHAR(30) NOT NULL UNIQUE
) ENGINE=InnoDB;

CREATE TABLE mata_pelajaran (
  id         INT AUTO_INCREMENT PRIMARY KEY,
  nama_mapel VARCHAR(100) NOT NULL
) ENGINE=InnoDB;

-- siswa.kelas menyimpan NAMA kelas (bukan id); dicocokkan dengan kelas.nama_kelas di aplikasi.
CREATE TABLE siswa (
  id         INT AUTO_INCREMENT PRIMARY KEY,
  nis        VARCHAR(30)  NOT NULL UNIQUE,
  nama       VARCHAR(100) NOT NULL,
  kelas      VARCHAR(30)  NULL,
  pin_ujian  VARCHAR(100) NOT NULL,            -- hash bcrypt
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_siswa_kelas (kelas)
) ENGINE=InnoDB;

CREATE TABLE pengajaran (
  id       INT AUTO_INCREMENT PRIMARY KEY,
  guru_id  INT NOT NULL,
  mapel_id INT NOT NULL,
  kelas_id INT NOT NULL,
  UNIQUE KEY uq_pengajaran (guru_id, mapel_id, kelas_id),
  FOREIGN KEY (guru_id)  REFERENCES guru(id)           ON DELETE CASCADE,
  FOREIGN KEY (mapel_id) REFERENCES mata_pelajaran(id) ON DELETE CASCADE,
  FOREIGN KEY (kelas_id) REFERENCES kelas(id)          ON DELETE CASCADE
) ENGINE=InnoDB;

CREATE TABLE ujian (
  id                INT AUTO_INCREMENT PRIMARY KEY,
  pengajaran_id     INT NOT NULL,
  nama_ujian        VARCHAR(150) NOT NULL,
  durasi            INT NOT NULL DEFAULT 60,   -- menit
  tanggal_mulai     DATETIME NOT NULL,
  tanggal_selesai   DATETIME NOT NULL,
  acak_soal         TINYINT(1) NOT NULL DEFAULT 0,
  acak_pilihan      TINYINT(1) NOT NULL DEFAULT 0,
  batas_pelanggaran INT NOT NULL DEFAULT 3,
  token_ujian       VARCHAR(50) NULL,          -- opsional (FR-14)
  created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (pengajaran_id) REFERENCES pengajaran(id) ON DELETE CASCADE
) ENGINE=InnoDB;

-- tipe_soal: 'pg' | 'menjodohkan' | 'essay'
-- jawaban_benar: huruf (pg) atau JSON (menjodohkan). TIDAK PERNAH dikirim ke klien.
CREATE TABLE soal (
  id            INT AUTO_INCREMENT PRIMARY KEY,
  ujian_id      INT NOT NULL,
  tipe_soal     VARCHAR(20) NOT NULL DEFAULT 'pg',
  teks_soal     TEXT NOT NULL,
  gambar        VARCHAR(255) NULL,
  poin          INT NOT NULL DEFAULT 2,        -- pg=2, menjodohkan=2, essay=4 (diatur aplikasi)
  pilihan_a     TEXT NULL,
  pilihan_b     TEXT NULL,
  pilihan_c     TEXT NULL,
  pilihan_d     TEXT NULL,
  jawaban_benar TEXT NULL,
  opsi_tambahan TEXT NULL,                     -- JSON
  created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (ujian_id) REFERENCES ujian(id) ON DELETE CASCADE
) ENGINE=InnoDB;

-- UNIQUE (siswa_id, ujian_id) dipakai untuk INSERT ... ON DUPLICATE KEY UPDATE.
-- status: sedang_ujian | selesai | keluar_paksa. Online/offline diturunkan dari last_seen.
CREATE TABLE sesi_ujian (
  id             INT AUTO_INCREMENT PRIMARY KEY,
  siswa_id       INT NOT NULL,
  ujian_id       INT NOT NULL,
  socket_id      VARCHAR(50) NULL,             -- id koneksi WebSocket aktif
  waktu_mulai    DATETIME NULL,
  batas_waktu    DATETIME NULL,
  tambahan_menit INT NOT NULL DEFAULT 0,
  seed           INT NULL,                     -- seed acak soal/pilihan (tetap saat sambung ulang)
  device_token   CHAR(36) NULL,
  device_type    VARCHAR(20) NULL,             -- hp | laptop
  last_seen      DATETIME NULL,
  selesai_pada   DATETIME NULL,
  status         ENUM('sedang_ujian','selesai','keluar_paksa') NOT NULL DEFAULT 'sedang_ujian',
  UNIQUE KEY uq_sesi (siswa_id, ujian_id),
  INDEX idx_sesi_status_batas (status, batas_waktu),   -- job penutupan otomatis
  INDEX idx_sesi_ujian_status (ujian_id, status),      -- snapshot monitor
  FOREIGN KEY (siswa_id) REFERENCES siswa(id)  ON DELETE CASCADE,
  FOREIGN KEY (ujian_id) REFERENCES ujian(id)  ON DELETE CASCADE
) ENGINE=InnoDB;

CREATE TABLE jawaban_siswa (
  id              INT AUTO_INCREMENT PRIMARY KEY,
  siswa_id        INT NOT NULL,
  ujian_id        INT NOT NULL,
  soal_id         INT NOT NULL,
  jawaban_dipilih TEXT NULL,                   -- pg: huruf A-D; menjodohkan: JSON pasangan
  is_benar        TINYINT(1) NULL,
  client_ts       DATETIME NULL,
  diperbarui_pada DATETIME NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uq_jawaban (siswa_id, ujian_id, soal_id),
  FOREIGN KEY (siswa_id) REFERENCES siswa(id) ON DELETE CASCADE,
  FOREIGN KEY (ujian_id) REFERENCES ujian(id) ON DELETE CASCADE,
  FOREIGN KEY (soal_id)  REFERENCES soal(id)  ON DELETE CASCADE
) ENGINE=InnoDB;

-- status_koreksi: menunggu_essay | selesai
CREATE TABLE nilai_ujian (
  id              INT AUTO_INCREMENT PRIMARY KEY,
  siswa_id        INT NOT NULL,
  ujian_id        INT NOT NULL,
  poin_otomatis   INT NOT NULL DEFAULT 0,      -- pg + menjodohkan
  poin_essay      INT NOT NULL DEFAULT 0,
  poin_maks       INT NOT NULL DEFAULT 0,
  nilai           INT NOT NULL DEFAULT 0,      -- akhir 0..100
  benar           INT NOT NULL DEFAULT 0,
  salah           INT NOT NULL DEFAULT 0,
  kosong          INT NOT NULL DEFAULT 0,
  status_koreksi  ENUM('menunggu_essay','selesai') NOT NULL DEFAULT 'selesai',
  selesai_pada    DATETIME NULL,
  UNIQUE KEY uq_nilai (siswa_id, ujian_id),
  FOREIGN KEY (siswa_id) REFERENCES siswa(id) ON DELETE CASCADE,
  FOREIGN KEY (ujian_id) REFERENCES ujian(id) ON DELETE CASCADE
) ENGINE=InnoDB;

CREATE TABLE nilai_essay (
  id           INT AUTO_INCREMENT PRIMARY KEY,
  siswa_id     INT NOT NULL,
  ujian_id     INT NOT NULL,
  soal_id      INT NOT NULL,
  skor         INT NOT NULL DEFAULT 0,         -- 0..soal.poin (divalidasi aplikasi)
  dinilai_oleh INT NULL,
  dinilai_pada DATETIME NULL,
  UNIQUE KEY uq_nilai_essay (siswa_id, ujian_id, soal_id),
  FOREIGN KEY (siswa_id)     REFERENCES siswa(id) ON DELETE CASCADE,
  FOREIGN KEY (ujian_id)     REFERENCES ujian(id) ON DELETE CASCADE,
  FOREIGN KEY (soal_id)      REFERENCES soal(id)  ON DELETE CASCADE,
  FOREIGN KEY (dinilai_oleh) REFERENCES guru(id)  ON DELETE SET NULL
) ENGINE=InnoDB;

-- jenis_kecurangan: 'pindah_tab' | 'keluar_fullscreen' | 'copy_paste'
CREATE TABLE log_kecurangan (
  id               INT AUTO_INCREMENT PRIMARY KEY,
  siswa_id         INT NOT NULL,
  ujian_id         INT NOT NULL,
  jenis_kecurangan VARCHAR(30) NOT NULL,
  device_type      VARCHAR(20) NULL,
  timestamp        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_log_siswa_ujian (siswa_id, ujian_id),
  FOREIGN KEY (siswa_id) REFERENCES siswa(id) ON DELETE CASCADE,
  FOREIGN KEY (ujian_id) REFERENCES ujian(id) ON DELETE CASCADE
) ENGINE=InnoDB;

CREATE TABLE audit_admin (
  id          INT AUTO_INCREMENT PRIMARY KEY,
  admin_id    INT NULL,
  sesi_id     INT NOT NULL,
  aksi        VARCHAR(50) NOT NULL,            -- buka-kunci | tambah-waktu | paksa-selesai | takeover
  detail      TEXT NULL,
  dibuat_pada TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_audit_sesi (sesi_id),
  FOREIGN KEY (admin_id) REFERENCES admin(id)      ON DELETE SET NULL,
  FOREIGN KEY (sesi_id)  REFERENCES sesi_ujian(id) ON DELETE CASCADE
) ENGINE=InnoDB;

-- Tabel session untuk github.com/alexedwards/scs (mysqlstore). Struktur MENGIKUTI
-- dokumentasi scs, bukan express-mysql-session. Verifikasi ulang via docs terbaru (T0.6).
CREATE TABLE sessions (
  token  CHAR(43) PRIMARY KEY,
  data   BLOB NOT NULL,
  expiry TIMESTAMP(6) NOT NULL,
  INDEX sessions_expiry_idx (expiry)
) ENGINE=InnoDB;

-- +goose Down
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS audit_admin;
DROP TABLE IF EXISTS log_kecurangan;
DROP TABLE IF EXISTS nilai_essay;
DROP TABLE IF EXISTS nilai_ujian;
DROP TABLE IF EXISTS jawaban_siswa;
DROP TABLE IF EXISTS sesi_ujian;
DROP TABLE IF EXISTS soal;
DROP TABLE IF EXISTS ujian;
DROP TABLE IF EXISTS pengajaran;
DROP TABLE IF EXISTS siswa;
DROP TABLE IF EXISTS mata_pelajaran;
DROP TABLE IF EXISTS kelas;
DROP TABLE IF EXISTS guru;
DROP TABLE IF EXISTS admin;
