-- +goose Up
-- Izin ujian susulan per siswa (G-026): siswa berizin boleh login di luar jendela ujian
-- sampai tanggal_selesai izin. Jejak audit disimpan di tabel ini (dibuat_oleh, alasan, created_at)
-- karena audit_admin mensyaratkan sesi_ujian yang belum ada saat izin diberikan.
CREATE TABLE izin_susulan (
  id              INT AUTO_INCREMENT PRIMARY KEY,
  siswa_id        INT NOT NULL,
  ujian_id        INT NOT NULL,
  tanggal_selesai DATETIME NOT NULL,
  alasan          VARCHAR(255) NULL,
  dibuat_oleh     INT NULL,
  created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uq_izin_siswa_ujian (siswa_id, ujian_id),
  INDEX idx_izin_ujian (ujian_id),
  FOREIGN KEY (siswa_id)    REFERENCES siswa(id)  ON DELETE CASCADE,
  FOREIGN KEY (ujian_id)    REFERENCES ujian(id)  ON DELETE CASCADE,
  FOREIGN KEY (dibuat_oleh) REFERENCES admin(id)  ON DELETE SET NULL
) ENGINE=InnoDB;

-- +goose Down
DROP TABLE IF EXISTS izin_susulan;
