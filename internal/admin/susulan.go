package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrSusulanSiswaBukanKelas = errors.New("siswa bukan peserta kelas ujian ini")
	ErrSusulanSudahSelesai    = errors.New("siswa sudah memiliki nilai pada ujian ini; gunakan Reset ujian bila perlu mengulang")
	ErrSusulanWaktuLampau     = errors.New("batas izin susulan harus di masa depan")
	ErrSusulanTidakDitemukan  = errors.New("izin susulan tidak ditemukan")
	ErrSusulanSiswaUjian      = errors.New("siswa atau ujian tidak ditemukan")
)

// SusulanRow adalah satu siswa pada kelas ujian beserta status sesi dan izin susulannya.
type SusulanRow struct {
	SiswaID    int
	NIS        string
	Nama       string
	StatusSesi string // "" = belum masuk | sedang_ujian | selesai | keluar_paksa
	SudahNilai bool
	IzinID     int // 0 bila tidak ada izin
	IzinSampai *time.Time
	Alasan     string
}

// ListSusulan mengambil semua siswa pada kelas ujian beserta status sesi dan izin susulan.
func ListSusulan(ctx context.Context, db *sql.DB, ujianID int) ([]SusulanRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.id, s.nis, s.nama,
		       COALESCE(su.status, ''), (nu.id IS NOT NULL),
		       COALESCE(iz.id, 0), iz.tanggal_selesai, COALESCE(iz.alasan, '')
		FROM ujian u
		JOIN pengajaran p ON p.id = u.pengajaran_id
		JOIN kelas k ON k.id = p.kelas_id
		JOIN siswa s ON s.kelas = k.nama_kelas
		LEFT JOIN sesi_ujian su ON su.siswa_id = s.id AND su.ujian_id = u.id
		LEFT JOIN nilai_ujian nu ON nu.siswa_id = s.id AND nu.ujian_id = u.id
		LEFT JOIN izin_susulan iz ON iz.siswa_id = s.id AND iz.ujian_id = u.id
		WHERE u.id = ?
		ORDER BY s.nama ASC, s.id ASC
	`, ujianID)
	if err != nil {
		return nil, fmt.Errorf("list susulan: %w", err)
	}
	defer rows.Close()

	var list []SusulanRow
	for rows.Next() {
		var r SusulanRow
		var sampai sql.NullTime
		if err := rows.Scan(&r.SiswaID, &r.NIS, &r.Nama, &r.StatusSesi, &r.SudahNilai, &r.IzinID, &sampai, &r.Alasan); err != nil {
			return nil, fmt.Errorf("scan susulan: %w", err)
		}
		if sampai.Valid {
			t := sampai.Time
			r.IzinSampai = &t
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// BeriIzinSusulan memberi (atau memperbarui) izin siswa mengerjakan ujian sampai `sampai`.
// Bila sampai nol, dipakai sekarang + durasi ujian + 30 menit. Waktu dinilai dengan jam database
// (konsisten dengan pemeriksaan login, DESIGN §3.2). Siswa yang sudah bernilai ditolak.
func BeriIzinSusulan(ctx context.Context, db *sql.DB, adminID, ujianID, siswaID int, sampai time.Time, alasan string) error {
	alasan = strings.TrimSpace(alasan)
	if len(alasan) > 255 {
		alasan = alasan[:255]
	}

	var kelasUjian, kelasSiswa string
	var durasi int
	err := db.QueryRowContext(ctx, `
		SELECT k.nama_kelas, u.durasi, s.kelas
		FROM ujian u
		JOIN pengajaran p ON p.id = u.pengajaran_id
		JOIN kelas k ON k.id = p.kelas_id
		JOIN siswa s ON s.id = ?
		WHERE u.id = ?`, siswaID, ujianID).Scan(&kelasUjian, &durasi, &kelasSiswa)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSusulanSiswaUjian
	}
	if err != nil {
		return fmt.Errorf("baca ujian/siswa susulan: %w", err)
	}
	if kelasUjian != kelasSiswa {
		return ErrSusulanSiswaBukanKelas
	}

	var sudahNilai int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM nilai_ujian WHERE siswa_id = ? AND ujian_id = ?`, siswaID, ujianID).Scan(&sudahNilai); err != nil {
		return fmt.Errorf("cek nilai susulan: %w", err)
	}
	if sudahNilai > 0 {
		return ErrSusulanSudahSelesai
	}

	var adminVal any
	if adminID > 0 {
		adminVal = adminID
	}
	var alasanVal any
	if alasan != "" {
		alasanVal = alasan
	}

	if sampai.IsZero() {
		_, err = db.ExecContext(ctx, `
			INSERT INTO izin_susulan (siswa_id, ujian_id, tanggal_selesai, alasan, dibuat_oleh)
			VALUES (?, ?, DATE_ADD(NOW(), INTERVAL ? MINUTE), ?, ?)
			ON DUPLICATE KEY UPDATE tanggal_selesai = VALUES(tanggal_selesai), alasan = VALUES(alasan), dibuat_oleh = VALUES(dibuat_oleh)
		`, siswaID, ujianID, durasi+30, alasanVal, adminVal)
	} else {
		var masaDepan bool
		if err := db.QueryRowContext(ctx, `SELECT ? > NOW()`, sampai).Scan(&masaDepan); err != nil {
			return fmt.Errorf("cek waktu susulan: %w", err)
		}
		if !masaDepan {
			return ErrSusulanWaktuLampau
		}
		_, err = db.ExecContext(ctx, `
			INSERT INTO izin_susulan (siswa_id, ujian_id, tanggal_selesai, alasan, dibuat_oleh)
			VALUES (?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE tanggal_selesai = VALUES(tanggal_selesai), alasan = VALUES(alasan), dibuat_oleh = VALUES(dibuat_oleh)
		`, siswaID, ujianID, sampai, alasanVal, adminVal)
	}
	if err != nil {
		return fmt.Errorf("simpan izin susulan: %w", err)
	}
	return nil
}

// CabutIzinSusulan menghapus izin. Sesi yang sedang berjalan tidak dihentikan (batas_waktu
// sudah tersimpan di sesi_ujian); siswa tidak dapat login baru bila jendela ujian sudah lewat.
func CabutIzinSusulan(ctx context.Context, db *sql.DB, izinID int) (ujianID int, err error) {
	err = db.QueryRowContext(ctx, `SELECT ujian_id FROM izin_susulan WHERE id = ?`, izinID).Scan(&ujianID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrSusulanTidakDitemukan
	}
	if err != nil {
		return 0, fmt.Errorf("baca izin susulan: %w", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM izin_susulan WHERE id = ?`, izinID); err != nil {
		return 0, fmt.Errorf("hapus izin susulan: %w", err)
	}
	return ujianID, nil
}
