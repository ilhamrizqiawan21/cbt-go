package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cbt-go/internal/excel"
)

// LogKecuranganRow menampung data satu kejadian pelanggaran ujian siswa.
type LogKecuranganRow struct {
	ID              int       `json:"id"`
	SiswaID         int       `json:"siswa_id"`
	NIS             string    `json:"nis"`
	NamaSiswa       string    `json:"nama_siswa"`
	Kelas           string    `json:"kelas"`
	UjianID         int       `json:"ujian_id"`
	NamaUjian       string    `json:"nama_ujian"`
	JenisKecurangan string    `json:"jenis_kecurangan"`
	DeviceType      string    `json:"device_type"`
	Timestamp       time.Time `json:"timestamp"`
}

// GetLogKecurangan mengambil seluruh rekaman pelanggaran untuk admin.
func GetLogKecurangan(ctx context.Context, db *sql.DB, ujianID int, jenisFilter string) ([]LogKecuranganRow, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}

	query := `
		SELECT lk.id, lk.siswa_id, s.nis, s.nama, COALESCE(s.kelas, ''),
		       lk.ujian_id, u.nama_ujian, lk.jenis_kecurangan,
		       COALESCE(lk.device_type, ''), lk.timestamp
		FROM log_kecurangan lk
		JOIN siswa s ON lk.siswa_id = s.id
		JOIN ujian u ON lk.ujian_id = u.id
		WHERE 1=1
	`
	var args []any
	if ujianID > 0 {
		query += " AND lk.ujian_id = ?"
		args = append(args, ujianID)
	}
	if strings.TrimSpace(jenisFilter) != "" {
		query += " AND lk.jenis_kecurangan = ?"
		args = append(args, strings.TrimSpace(jenisFilter))
	}
	query += " ORDER BY lk.id DESC"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query log kecurangan: %w", err)
	}
	defer rows.Close()

	var list []LogKecuranganRow
	for rows.Next() {
		var l LogKecuranganRow
		err := rows.Scan(
			&l.ID, &l.SiswaID, &l.NIS, &l.NamaSiswa, &l.Kelas,
			&l.UjianID, &l.NamaUjian, &l.JenisKecurangan,
			&l.DeviceType, &l.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("scan log kecurangan: %w", err)
		}
		list = append(list, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if list == nil {
		list = []LogKecuranganRow{}
	}
	return list, nil
}

// GetLogKecuranganByGuru mengambil log pelanggaran pada ujian yang diampu oleh guru pengajar.
func GetLogKecuranganByGuru(ctx context.Context, db *sql.DB, guruID, ujianID int, jenisFilter string) ([]LogKecuranganRow, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}

	query := `
		SELECT lk.id, lk.siswa_id, s.nis, s.nama, COALESCE(s.kelas, ''),
		       lk.ujian_id, u.nama_ujian, lk.jenis_kecurangan,
		       COALESCE(lk.device_type, ''), lk.timestamp
		FROM log_kecurangan lk
		JOIN siswa s ON lk.siswa_id = s.id
		JOIN ujian u ON lk.ujian_id = u.id
		JOIN pengajaran p ON u.pengajaran_id = p.id
		WHERE p.guru_id = ?
	`
	var args []any
	args = append(args, guruID)

	if ujianID > 0 {
		query += " AND lk.ujian_id = ?"
		args = append(args, ujianID)
	}
	if strings.TrimSpace(jenisFilter) != "" {
		query += " AND lk.jenis_kecurangan = ?"
		args = append(args, strings.TrimSpace(jenisFilter))
	}
	query += " ORDER BY lk.id DESC"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query log kecurangan guru: %w", err)
	}
	defer rows.Close()

	var list []LogKecuranganRow
	for rows.Next() {
		var l LogKecuranganRow
		err := rows.Scan(
			&l.ID, &l.SiswaID, &l.NIS, &l.NamaSiswa, &l.Kelas,
			&l.UjianID, &l.NamaUjian, &l.JenisKecurangan,
			&l.DeviceType, &l.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("scan log kecurangan guru: %w", err)
		}
		list = append(list, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if list == nil {
		list = []LogKecuranganRow{}
	}
	return list, nil
}

// ConvertToExcelLogItems mengonversi slice LogKecuranganRow ke slice item untuk generator Excel.
func ConvertToExcelLogItems(list []LogKecuranganRow) []excel.LogKecuranganItem {
	items := make([]excel.LogKecuranganItem, len(list))
	for i, l := range list {
		items[i] = excel.LogKecuranganItem{
			NIS:             l.NIS,
			NamaSiswa:       l.NamaSiswa,
			Kelas:           l.Kelas,
			NamaUjian:       l.NamaUjian,
			JenisKecurangan: l.JenisKecurangan,
			DeviceType:      l.DeviceType,
			Timestamp:       l.Timestamp,
		}
	}
	return items
}

// ResetUjianSiswa mereset hasil ujian dan sesi satu siswa tertentu secara atomik.
// Hanya menyentuh siswa dan ujian yang dimaksud.
func ResetUjianSiswa(ctx context.Context, db *sql.DB, adminID, siswaID, ujianID int) error {
	if db == nil {
		return errors.New("database tidak tersedia")
	}
	if siswaID <= 0 || ujianID <= 0 {
		return errors.New("siswa_id dan ujian_id harus valid")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx reset: %w", err)
	}
	defer tx.Rollback()

	// 1. Hapus jawaban siswa
	_, err = tx.ExecContext(ctx, "DELETE FROM jawaban_siswa WHERE siswa_id = ? AND ujian_id = ?", siswaID, ujianID)
	if err != nil {
		return fmt.Errorf("hapus jawaban siswa: %w", err)
	}

	// 2. Hapus nilai essay
	_, err = tx.ExecContext(ctx, "DELETE FROM nilai_essay WHERE siswa_id = ? AND ujian_id = ?", siswaID, ujianID)
	if err != nil {
		return fmt.Errorf("hapus nilai essay: %w", err)
	}

	// 3. Hapus nilai ujian
	_, err = tx.ExecContext(ctx, "DELETE FROM nilai_ujian WHERE siswa_id = ? AND ujian_id = ?", siswaID, ujianID)
	if err != nil {
		return fmt.Errorf("hapus nilai ujian: %w", err)
	}

	// 4. Hapus log kecurangan
	_, err = tx.ExecContext(ctx, "DELETE FROM log_kecurangan WHERE siswa_id = ? AND ujian_id = ?", siswaID, ujianID)
	if err != nil {
		return fmt.Errorf("hapus log kecurangan: %w", err)
	}

	// 5. Hapus sesi ujian agar siswa dapat memulai sesi fresh
	_, err = tx.ExecContext(ctx, "DELETE FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?", siswaID, ujianID)
	if err != nil {
		return fmt.Errorf("hapus sesi ujian: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx reset: %w", err)
	}

	slog.Info("admin berhasil mereset ujian siswa", "admin_id", adminID, "siswa_id", siswaID, "ujian_id", ujianID)
	return nil
}

// ResetUjianSemua mereset hasil dan sesi seluruh siswa pada satu ujian tertentu secara atomik.
// Hanya menyentuh ujian yang dimaksud dan tidak mempengaruhi ujian lainnya.
func ResetUjianSemua(ctx context.Context, db *sql.DB, adminID, ujianID int) error {
	if db == nil {
		return errors.New("database tidak tersedia")
	}
	if ujianID <= 0 {
		return errors.New("ujian_id harus valid")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx reset semua: %w", err)
	}
	defer tx.Rollback()

	// Hapus jawaban seluruh siswa pada ujian ini
	_, err = tx.ExecContext(ctx, "DELETE FROM jawaban_siswa WHERE ujian_id = ?", ujianID)
	if err != nil {
		return fmt.Errorf("hapus jawaban ujian: %w", err)
	}

	// Hapus nilai essay seluruh siswa pada ujian ini
	_, err = tx.ExecContext(ctx, "DELETE FROM nilai_essay WHERE ujian_id = ?", ujianID)
	if err != nil {
		return fmt.Errorf("hapus nilai essay ujian: %w", err)
	}

	// Hapus nilai ujian seluruh siswa pada ujian ini
	_, err = tx.ExecContext(ctx, "DELETE FROM nilai_ujian WHERE ujian_id = ?", ujianID)
	if err != nil {
		return fmt.Errorf("hapus nilai ujian: %w", err)
	}

	// Hapus log kecurangan seluruh siswa pada ujian ini
	_, err = tx.ExecContext(ctx, "DELETE FROM log_kecurangan WHERE ujian_id = ?", ujianID)
	if err != nil {
		return fmt.Errorf("hapus log kecurangan ujian: %w", err)
	}

	// Hapus seluruh sesi pada ujian ini
	_, err = tx.ExecContext(ctx, "DELETE FROM sesi_ujian WHERE ujian_id = ?", ujianID)
	if err != nil {
		return fmt.Errorf("hapus sesi ujian: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx reset semua: %w", err)
	}

	slog.Info("admin berhasil mereset seluruh sesi pada ujian", "admin_id", adminID, "ujian_id", ujianID)
	return nil
}
