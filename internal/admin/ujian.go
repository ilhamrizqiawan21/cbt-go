package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"cbt-go/internal/soal"
)

var (
	ErrInvalidWaktu      = errors.New("tanggal selesai harus lebih besar dari tanggal mulai")
	ErrInvalidDurasi     = errors.New("durasi ujian harus lebih besar dari 0 menit")
	ErrInvalidPengajaran = errors.New("pengajaran harus dipilih")
)

// Ujian merepresentasikan data entitas ujian.
type Ujian struct {
	ID               int        `json:"id"`
	PengajaranID     int        `json:"pengajaran_id"`
	NamaUjian        string     `json:"nama_ujian"`
	Durasi           int        `json:"durasi"` // menit
	TanggalMulai     time.Time  `json:"tanggal_mulai"`
	TanggalSelesai   time.Time  `json:"tanggal_selesai"`
	AcakSoal         bool       `json:"acak_soal"`
	AcakPilihan      bool       `json:"acak_pilihan"`
	BatasPelanggaran int        `json:"batas_pelanggaran"`
	TokenUjian       *string    `json:"token_ujian,omitempty"`
	CreatedAt        *time.Time `json:"created_at,omitempty"`

	// Relasi pengajaran (JOIN)
	NamaGuru  string `json:"nama_guru,omitempty"`
	NamaMapel string `json:"nama_mapel,omitempty"`
	NamaKelas string `json:"nama_kelas,omitempty"`
	MapelID   int    `json:"mapel_id,omitempty"`
}

// ParseDateTime mengonversi string waktu dari form HTML atau API ke time.Time.
func ParseDateTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("waktu kosong")
	}

	formats := []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		time.RFC3339,
	}

	for _, layout := range formats {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("format waktu tidak dikenali: %s", s)
}

// ListUjian mengambil seluruh data ujian beserta informasi pengajarannya.
func ListUjian(ctx context.Context, db *sql.DB) ([]Ujian, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}

	query := `
		SELECT u.id, u.pengajaran_id, u.nama_ujian, u.durasi,
		       u.tanggal_mulai, u.tanggal_selesai, u.acak_soal, u.acak_pilihan,
		       u.batas_pelanggaran, COALESCE(u.token_ujian, ''),
		       g.nama, m.nama_mapel, k.nama_kelas, p.mapel_id
		FROM ujian u
		JOIN pengajaran p ON u.pengajaran_id = p.id
		JOIN guru g ON p.guru_id = g.id
		JOIN mata_pelajaran m ON p.mapel_id = m.id
		JOIN kelas k ON p.kelas_id = k.id
		ORDER BY u.tanggal_mulai DESC, u.id DESC
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list ujian: %w", err)
	}
	defer rows.Close()

	var list []Ujian
	for rows.Next() {
		var u Ujian
		var tokenStr string
		var acakSoal, acakPilihan int
		err := rows.Scan(
			&u.ID, &u.PengajaranID, &u.NamaUjian, &u.Durasi,
			&u.TanggalMulai, &u.TanggalSelesai, &acakSoal, &acakPilihan,
			&u.BatasPelanggaran, &tokenStr,
			&u.NamaGuru, &u.NamaMapel, &u.NamaKelas, &u.MapelID,
		)
		if err != nil {
			return nil, fmt.Errorf("scan ujian: %w", err)
		}
		u.AcakSoal = acakSoal == 1
		u.AcakPilihan = acakPilihan == 1
		if tokenStr != "" {
			u.TokenUjian = &tokenStr
		}
		list = append(list, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if list == nil {
		list = []Ujian{}
	}
	return list, nil
}

// ListUjianByMapel mengambil daftar ujian berdasarkan ID mata pelajaran.
func ListUjianByMapel(ctx context.Context, db *sql.DB, mapelID int) ([]Ujian, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}

	query := `
		SELECT u.id, u.pengajaran_id, u.nama_ujian, u.durasi,
		       u.tanggal_mulai, u.tanggal_selesai, u.acak_soal, u.acak_pilihan,
		       u.batas_pelanggaran, COALESCE(u.token_ujian, ''),
		       g.nama, m.nama_mapel, k.nama_kelas, p.mapel_id
		FROM ujian u
		JOIN pengajaran p ON u.pengajaran_id = p.id
		JOIN guru g ON p.guru_id = g.id
		JOIN mata_pelajaran m ON p.mapel_id = m.id
		JOIN kelas k ON p.kelas_id = k.id
		WHERE p.mapel_id = ?
		ORDER BY u.tanggal_mulai DESC, u.id DESC
	`

	rows, err := db.QueryContext(ctx, query, mapelID)
	if err != nil {
		return nil, fmt.Errorf("list ujian by mapel: %w", err)
	}
	defer rows.Close()

	var list []Ujian
	for rows.Next() {
		var u Ujian
		var tokenStr string
		var acakSoal, acakPilihan int
		err := rows.Scan(
			&u.ID, &u.PengajaranID, &u.NamaUjian, &u.Durasi,
			&u.TanggalMulai, &u.TanggalSelesai, &acakSoal, &acakPilihan,
			&u.BatasPelanggaran, &tokenStr,
			&u.NamaGuru, &u.NamaMapel, &u.NamaKelas, &u.MapelID,
		)
		if err != nil {
			return nil, fmt.Errorf("scan ujian by mapel: %w", err)
		}
		u.AcakSoal = acakSoal == 1
		u.AcakPilihan = acakPilihan == 1
		if tokenStr != "" {
			u.TokenUjian = &tokenStr
		}
		list = append(list, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if list == nil {
		list = []Ujian{}
	}
	return list, nil
}

// GetUjianByID mengambil data satu ujian berdasarkan ID.
func GetUjianByID(ctx context.Context, db *sql.DB, id int) (*Ujian, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}

	query := `
		SELECT u.id, u.pengajaran_id, u.nama_ujian, u.durasi,
		       u.tanggal_mulai, u.tanggal_selesai, u.acak_soal, u.acak_pilihan,
		       u.batas_pelanggaran, COALESCE(u.token_ujian, ''),
		       g.nama, m.nama_mapel, k.nama_kelas, p.mapel_id
		FROM ujian u
		JOIN pengajaran p ON u.pengajaran_id = p.id
		JOIN guru g ON p.guru_id = g.id
		JOIN mata_pelajaran m ON p.mapel_id = m.id
		JOIN kelas k ON p.kelas_id = k.id
		WHERE u.id = ?
	`

	var u Ujian
	var tokenStr string
	var acakSoal, acakPilihan int
	err := db.QueryRowContext(ctx, query, id).Scan(
		&u.ID, &u.PengajaranID, &u.NamaUjian, &u.Durasi,
		&u.TanggalMulai, &u.TanggalSelesai, &acakSoal, &acakPilihan,
		&u.BatasPelanggaran, &tokenStr,
		&u.NamaGuru, &u.NamaMapel, &u.NamaKelas, &u.MapelID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get ujian by id: %w", err)
	}

	u.AcakSoal = acakSoal == 1
	u.AcakPilihan = acakPilihan == 1
	if tokenStr != "" {
		u.TokenUjian = &tokenStr
	}
	return &u, nil
}

// TambahUjian menambahkan ujian baru setelah memvalidasi durasi dan rentang tanggal.
func TambahUjian(ctx context.Context, db *sql.DB, u Ujian) (int, error) {
	if strings.TrimSpace(u.NamaUjian) == "" {
		return 0, ErrInvalidInput
	}
	if u.PengajaranID <= 0 {
		return 0, ErrInvalidPengajaran
	}
	if u.Durasi <= 0 {
		return 0, ErrInvalidDurasi
	}
	if !u.TanggalSelesai.After(u.TanggalMulai) {
		return 0, ErrInvalidWaktu
	}
	if u.BatasPelanggaran <= 0 {
		u.BatasPelanggaran = 3
	}
	if db == nil {
		return 0, errors.New("database tidak tersedia")
	}

	var tokenVal any = nil
	if u.TokenUjian != nil && strings.TrimSpace(*u.TokenUjian) != "" {
		tokenVal = strings.TrimSpace(*u.TokenUjian)
	}

	res, err := db.ExecContext(ctx, `
		INSERT INTO ujian (
			pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai,
			acak_soal, acak_pilihan, batas_pelanggaran, token_ujian
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, u.PengajaranID, strings.TrimSpace(u.NamaUjian), u.Durasi, u.TanggalMulai, u.TanggalSelesai,
		u.AcakSoal, u.AcakPilihan, u.BatasPelanggaran, tokenVal)
	if err != nil {
		return 0, fmt.Errorf("insert ujian: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// EditUjian memperbarui data ujian setelah memvalidasi durasi dan tanggal.
func EditUjian(ctx context.Context, db *sql.DB, u Ujian) error {
	if u.ID <= 0 || strings.TrimSpace(u.NamaUjian) == "" {
		return ErrInvalidInput
	}
	if u.PengajaranID <= 0 {
		return ErrInvalidPengajaran
	}
	if u.Durasi <= 0 {
		return ErrInvalidDurasi
	}
	if !u.TanggalSelesai.After(u.TanggalMulai) {
		return ErrInvalidWaktu
	}
	if u.BatasPelanggaran <= 0 {
		u.BatasPelanggaran = 3
	}
	if db == nil {
		return errors.New("database tidak tersedia")
	}

	var tokenVal any = nil
	if u.TokenUjian != nil && strings.TrimSpace(*u.TokenUjian) != "" {
		tokenVal = strings.TrimSpace(*u.TokenUjian)
	}

	res, err := db.ExecContext(ctx, `
		UPDATE ujian
		SET pengajaran_id = ?, nama_ujian = ?, durasi = ?,
		    tanggal_mulai = ?, tanggal_selesai = ?,
		    acak_soal = ?, acak_pilihan = ?, batas_pelanggaran = ?, token_ujian = ?
		WHERE id = ?
	`, u.PengajaranID, strings.TrimSpace(u.NamaUjian), u.Durasi, u.TanggalMulai, u.TanggalSelesai,
		u.AcakSoal, u.AcakPilihan, u.BatasPelanggaran, tokenVal, u.ID)
	if err != nil {
		return fmt.Errorf("update ujian: %w", err)
	}

	affected, err := res.RowsAffected()
	if err == nil && affected == 0 {
		return ErrNotFound
	}
	return nil
}

// HapusUjian menghapus data ujian beserta data terkait via kaskade foreign key.
func HapusUjian(ctx context.Context, db *sql.DB, id int) error {
	if db == nil {
		return errors.New("database tidak tersedia")
	}
	res, err := db.ExecContext(ctx, "DELETE FROM ujian WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete ujian: %w", err)
	}
	affected, err := res.RowsAffected()
	if err == nil && affected == 0 {
		return ErrNotFound
	}
	return nil
}

// DuplikatUjian membuat ujian baru dari data u lalu menyalin soal dari ujian sumber.
// Bila penyalinan gagal, ujian baru dibatalkan (dihapus) agar tidak tersisa ujian kosong.
func DuplikatUjian(ctx context.Context, db *sql.DB, sumberID int, u Ujian) (baruID, jumlahSoal int, err error) {
	baruID, err = TambahUjian(ctx, db, u)
	if err != nil {
		return 0, 0, err
	}
	jumlahSoal, err = soal.SalinSoal(ctx, db, sumberID, baruID)
	if err != nil {
		if _, delErr := db.ExecContext(ctx, "DELETE FROM ujian WHERE id = ?", baruID); delErr != nil {
			return 0, 0, fmt.Errorf("batalkan ujian duplikat: %w (penyebab: %v)", delErr, err)
		}
		return 0, 0, err
	}
	return baruID, jumlahSoal, nil
}
