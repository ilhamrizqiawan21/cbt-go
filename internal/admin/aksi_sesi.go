package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrSesiTidakDitemukan = errors.New("sesi ujian tidak ditemukan")
	ErrSesiTidakTerkunci  = errors.New("sesi tidak sedang terkunci")
	ErrSesiSudahSelesai   = errors.New("sesi ujian sudah selesai")
)

// Batas menit yang boleh ditambahkan admin dalam satu aksi.
const (
	MinTambahMenit = 1
	MaxTambahMenit = 180
)

// SesiAksi adalah ringkasan sesi yang dibutuhkan handler setelah aksi admin
// (untuk notifikasi realtime ke siswa dan siaran ulang monitor).
type SesiAksi struct {
	SesiID     int
	SiswaID    int
	UjianID    int
	Status     string
	DeviceType string
	BatasWaktu *time.Time
}

// ambilSesiForUpdate mengunci baris sesi di dalam transaksi agar dua admin yang
// menekan tombol bersamaan tidak saling menimpa.
func ambilSesiForUpdate(ctx context.Context, tx *sql.Tx, sesiID int) (*SesiAksi, error) {
	s := SesiAksi{SesiID: sesiID}
	var device sql.NullString
	var batas sql.NullTime
	err := tx.QueryRowContext(ctx, `
		SELECT siswa_id, ujian_id, status, device_type, batas_waktu
		FROM sesi_ujian
		WHERE id = ?
		FOR UPDATE
	`, sesiID).Scan(&s.SiswaID, &s.UjianID, &s.Status, &device, &batas)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSesiTidakDitemukan
	}
	if err != nil {
		return nil, fmt.Errorf("ambil sesi %d: %w", sesiID, err)
	}
	s.DeviceType = device.String
	if batas.Valid {
		t := batas.Time
		s.BatasWaktu = &t
	}
	return &s, nil
}

// execer dipenuhi *sql.DB maupun *sql.Tx, sehingga audit bisa ikut transaksi atau berdiri sendiri.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// catatAudit menulis jejak aksi admin. adminID 0 disimpan sebagai NULL (FK SET NULL).
func catatAudit(ctx context.Context, tx execer, adminID, sesiID int, aksi, detail string) error {
	var admin any
	if adminID > 0 {
		admin = adminID
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO audit_admin (admin_id, sesi_id, aksi, detail)
		VALUES (?, ?, ?, ?)
	`, admin, sesiID, aksi, detail)
	if err != nil {
		return fmt.Errorf("catat audit %s: %w", aksi, err)
	}
	return nil
}

// BukaKunciSesi mengembalikan sesi keluar_paksa menjadi sedang_ujian tanpa menyentuh jawaban.
// Hitungan pelanggaran "direset" dengan menyisipkan penanda 'buka_kunci' di log_kecurangan;
// sesi.HitungPelanggaran dan monitor hanya menghitung pelanggaran setelah penanda terakhir.
func BukaKunciSesi(ctx context.Context, db *sql.DB, adminID, sesiID int) (*SesiAksi, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("buka kunci begin tx: %w", err)
	}
	defer tx.Rollback() // tidak berefek bila Commit sudah berhasil

	s, err := ambilSesiForUpdate(ctx, tx, sesiID)
	if err != nil {
		return nil, err
	}
	if s.Status != "keluar_paksa" {
		return nil, ErrSesiTidakTerkunci
	}

	if _, err := tx.ExecContext(ctx, `UPDATE sesi_ujian SET status = 'sedang_ujian' WHERE id = ?`, sesiID); err != nil {
		return nil, fmt.Errorf("buka kunci update sesi: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO log_kecurangan (siswa_id, ujian_id, jenis_kecurangan, device_type)
		VALUES (?, ?, 'buka_kunci', ?)
	`, s.SiswaID, s.UjianID, s.DeviceType); err != nil {
		return nil, fmt.Errorf("buka kunci penanda log: %w", err)
	}
	if err := catatAudit(ctx, tx, adminID, sesiID, "buka-kunci", "Buka kunci sesi siswa"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("buka kunci commit: %w", err)
	}
	s.Status = "sedang_ujian"
	return s, nil
}

// TambahWaktuSesi memajukan batas_waktu dan mencatat tambahan_menit (D-007: admin boleh
// melewati tanggal_selesai ujian). Menit di luar rentang ditolak oleh pemanggil.
func TambahWaktuSesi(ctx context.Context, db *sql.DB, adminID, sesiID, menit int) (*SesiAksi, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("tambah waktu begin tx: %w", err)
	}
	defer tx.Rollback()

	s, err := ambilSesiForUpdate(ctx, tx, sesiID)
	if err != nil {
		return nil, err
	}
	if s.Status == "selesai" {
		return nil, ErrSesiSudahSelesai
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE sesi_ujian
		SET tambahan_menit = tambahan_menit + ?,
		    batas_waktu = DATE_ADD(COALESCE(batas_waktu, NOW()), INTERVAL ? MINUTE)
		WHERE id = ?
	`, menit, menit, sesiID); err != nil {
		return nil, fmt.Errorf("tambah waktu update sesi: %w", err)
	}
	var batas time.Time
	if err := tx.QueryRowContext(ctx, `SELECT batas_waktu FROM sesi_ujian WHERE id = ?`, sesiID).Scan(&batas); err != nil {
		return nil, fmt.Errorf("tambah waktu baca batas: %w", err)
	}
	if err := catatAudit(ctx, tx, adminID, sesiID, "tambah-waktu", fmt.Sprintf("Tambah waktu %d menit", menit)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("tambah waktu commit: %w", err)
	}
	s.BatasWaktu = &batas
	return s, nil
}

// AmbilSesi membaca ringkasan sesi tanpa penguncian (dipakai sebelum Finalize).
func AmbilSesi(ctx context.Context, db *sql.DB, sesiID int) (*SesiAksi, error) {
	s := SesiAksi{SesiID: sesiID}
	var device sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT siswa_id, ujian_id, status, device_type FROM sesi_ujian WHERE id = ?
	`, sesiID).Scan(&s.SiswaID, &s.UjianID, &s.Status, &device)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSesiTidakDitemukan
	}
	if err != nil {
		return nil, fmt.Errorf("ambil sesi %d: %w", sesiID, err)
	}
	s.DeviceType = device.String
	return &s, nil
}

// CatatAuditPaksaSelesai mencatat audit setelah sesi.Finalize (yang memakai transaksinya sendiri).
func CatatAuditPaksaSelesai(ctx context.Context, db *sql.DB, adminID, sesiID int) error {
	return catatAudit(ctx, db, adminID, sesiID, "paksa-selesai", "Sesi diselesaikan paksa oleh admin")
}
