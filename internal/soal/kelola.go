package soal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrForbiddenGuru   = errors.New("akses ditolak: guru tidak memiliki hak akses atas ujian atau soal ini")
	ErrTeksSoalKosong  = errors.New("teks soal tidak boleh kosong")
	ErrTipeSoalInvalid = errors.New("tipe soal harus pg, menjodohkan, atau essay")
)

// SoalKelola merepresentasikan entitas lengkap soal untuk antarmuka kelola (admin & guru).
// Mengandung kunci jawaban dan opsi tambahan yang hanya boleh diakses staf pengajar/admin.
type SoalKelola struct {
	ID           int     `json:"id"`
	UjianID      int     `json:"ujian_id"`
	TipeSoal     string  `json:"tipe_soal"` // pg | menjodohkan | essay
	TeksSoal     string  `json:"teks_soal"`
	Gambar       *string `json:"gambar,omitempty"`
	Poin         int     `json:"poin"`
	PilihanA     *string `json:"pilihan_a,omitempty"`
	PilihanB     *string `json:"pilihan_b,omitempty"`
	PilihanC     *string `json:"pilihan_c,omitempty"`
	PilihanD     *string `json:"pilihan_d,omitempty"`
	JawabanBenar *string `json:"jawaban_benar,omitempty"`
	OpsiTambahan *string `json:"opsi_tambahan,omitempty"`
}

// UjianGuruItem merangkum data ujian yang diampu oleh seorang guru.
type UjianGuruItem struct {
	ID             int    `json:"id"`
	NamaUjian      string `json:"nama_ujian"`
	Durasi         int    `json:"durasi"`
	NamaMapel      string `json:"nama_mapel"`
	NamaKelas      string `json:"nama_kelas"`
	TanggalMulai   string `json:"tanggal_mulai"`
	TanggalSelesai string `json:"tanggal_selesai"`
}

// DefaultPoin mengembalikan nilai poin default berdasarkan tipe soal jika tidak ditentukan.
// PG = 2, Menjodohkan = 2, Essay = 4.
func DefaultPoin(tipeSoal string, poinInput int) int {
	if poinInput > 0 {
		return poinInput
	}
	switch strings.ToLower(strings.TrimSpace(tipeSoal)) {
	case "essay":
		return 4
	case "menjodohkan":
		return 2
	case "pg":
		return 2
	default:
		return 2
	}
}

// CekOtorisasiUjianGuru memeriksa apakah guru dengan ID tertentu berhak mengelola ujian_id
// berdasarkan relasi pada tabel pengajaran.
func CekOtorisasiUjianGuru(ctx context.Context, db *sql.DB, guruID, ujianID int) (bool, error) {
	if db == nil {
		return false, errors.New("database tidak tersedia")
	}
	if guruID <= 0 || ujianID <= 0 {
		return false, nil
	}

	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM ujian u
		JOIN pengajaran p ON u.pengajaran_id = p.id
		WHERE u.id = ? AND p.guru_id = ?
	`, ujianID, guruID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("cek otorisasi ujian guru: %w", err)
	}
	return count > 0, nil
}

// CekOtorisasiSoalGuru memeriksa apakah guru dengan ID tertentu berhak mengelola soal_id
// berdasarkan relasi ujian -> pengajaran -> guru.
func CekOtorisasiSoalGuru(ctx context.Context, db *sql.DB, guruID, soalID int) (bool, error) {
	if db == nil {
		return false, errors.New("database tidak tersedia")
	}
	if guruID <= 0 || soalID <= 0 {
		return false, nil
	}

	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM soal s
		JOIN ujian u ON s.ujian_id = u.id
		JOIN pengajaran p ON u.pengajaran_id = p.id
		WHERE s.id = ? AND p.guru_id = ?
	`, soalID, guruID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("cek otorisasi soal guru: %w", err)
	}
	return count > 0, nil
}

// ListUjianByGuru mengambil seluruh ujian yang diampu oleh seorang guru.
func ListUjianByGuru(ctx context.Context, db *sql.DB, guruID int) ([]UjianGuruItem, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}

	query := `
		SELECT u.id, u.nama_ujian, u.durasi, m.nama_mapel, k.nama_kelas,
		       DATE_FORMAT(u.tanggal_mulai, '%Y-%m-%d %H:%i') as t_mulai,
		       DATE_FORMAT(u.tanggal_selesai, '%Y-%m-%d %H:%i') as t_selesai
		FROM ujian u
		JOIN pengajaran p ON u.pengajaran_id = p.id
		JOIN mata_pelajaran m ON p.mapel_id = m.id
		JOIN kelas k ON p.kelas_id = k.id
		WHERE p.guru_id = ?
		ORDER BY u.tanggal_mulai DESC, u.id DESC
	`

	rows, err := db.QueryContext(ctx, query, guruID)
	if err != nil {
		return nil, fmt.Errorf("list ujian by guru: %w", err)
	}
	defer rows.Close()

	var list []UjianGuruItem
	for rows.Next() {
		var it UjianGuruItem
		if err := rows.Scan(&it.ID, &it.NamaUjian, &it.Durasi, &it.NamaMapel, &it.NamaKelas, &it.TanggalMulai, &it.TanggalSelesai); err != nil {
			return nil, fmt.Errorf("scan ujian guru: %w", err)
		}
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if list == nil {
		list = []UjianGuruItem{}
	}
	return list, nil
}

// ListSoalByUjian mengambil daftar soal lengkap untuk antarmuka kelola.
func ListSoalByUjian(ctx context.Context, db *sql.DB, ujianID int) ([]SoalKelola, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}

	query := `
		SELECT id, ujian_id, tipe_soal, teks_soal, gambar, poin,
		       pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar, opsi_tambahan
		FROM soal
		WHERE ujian_id = ?
		ORDER BY id ASC
	`

	rows, err := db.QueryContext(ctx, query, ujianID)
	if err != nil {
		return nil, fmt.Errorf("list soal by ujian: %w", err)
	}
	defer rows.Close()

	var list []SoalKelola
	for rows.Next() {
		var s SoalKelola
		var g, pa, pb, pc, pd, jb, ot sql.NullString
		err := rows.Scan(
			&s.ID, &s.UjianID, &s.TipeSoal, &s.TeksSoal, &g, &s.Poin,
			&pa, &pb, &pc, &pd, &jb, &ot,
		)
		if err != nil {
			return nil, fmt.Errorf("scan soal kelola: %w", err)
		}
		if g.Valid {
			s.Gambar = &g.String
		}
		if pa.Valid {
			s.PilihanA = &pa.String
		}
		if pb.Valid {
			s.PilihanB = &pb.String
		}
		if pc.Valid {
			s.PilihanC = &pc.String
		}
		if pd.Valid {
			s.PilihanD = &pd.String
		}
		if jb.Valid {
			s.JawabanBenar = &jb.String
		}
		if ot.Valid {
			s.OpsiTambahan = &ot.String
		}
		list = append(list, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if list == nil {
		list = []SoalKelola{}
	}
	return list, nil
}

// GetSoalByID mengambil satu soal lengkap berdasarkan ID.
func GetSoalByID(ctx context.Context, db *sql.DB, id int) (*SoalKelola, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}

	query := `
		SELECT id, ujian_id, tipe_soal, teks_soal, gambar, poin,
		       pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar, opsi_tambahan
		FROM soal
		WHERE id = ?
	`

	var s SoalKelola
	var g, pa, pb, pc, pd, jb, ot sql.NullString
	err := db.QueryRowContext(ctx, query, id).Scan(
		&s.ID, &s.UjianID, &s.TipeSoal, &s.TeksSoal, &g, &s.Poin,
		&pa, &pb, &pc, &pd, &jb, &ot,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSoalNotFound
		}
		return nil, fmt.Errorf("get soal by id: %w", err)
	}
	if g.Valid {
		s.Gambar = &g.String
	}
	if pa.Valid {
		s.PilihanA = &pa.String
	}
	if pb.Valid {
		s.PilihanB = &pb.String
	}
	if pc.Valid {
		s.PilihanC = &pc.String
	}
	if pd.Valid {
		s.PilihanD = &pd.String
	}
	if jb.Valid {
		s.JawabanBenar = &jb.String
	}
	if ot.Valid {
		s.OpsiTambahan = &ot.String
	}
	return &s, nil
}

// TambahSoal menyimpan satu soal baru dengan validasi dan pengaturan poin default.
func TambahSoal(ctx context.Context, db *sql.DB, s SoalKelola) (int, error) {
	if db == nil {
		return 0, errors.New("database tidak tersedia")
	}
	if s.UjianID <= 0 {
		return 0, errors.New("ujian_id tidak valid")
	}
	if strings.TrimSpace(s.TeksSoal) == "" {
		return 0, ErrTeksSoalKosong
	}
	tipe := strings.ToLower(strings.TrimSpace(s.TipeSoal))
	if tipe != "pg" && tipe != "menjodohkan" && tipe != "essay" {
		return 0, ErrTipeSoalInvalid
	}

	s.TipeSoal = tipe
	s.Poin = DefaultPoin(tipe, s.Poin)

	res, err := db.ExecContext(ctx, `
		INSERT INTO soal (
			ujian_id, tipe_soal, teks_soal, gambar, poin,
			pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar, opsi_tambahan
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.UjianID, s.TipeSoal, s.TeksSoal, s.Gambar, s.Poin,
		s.PilihanA, s.PilihanB, s.PilihanC, s.PilihanD, s.JawabanBenar, s.OpsiTambahan)
	if err != nil {
		return 0, fmt.Errorf("insert soal: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// EditSoal memperbarui data satu soal yang sudah ada.
func EditSoal(ctx context.Context, db *sql.DB, s SoalKelola) error {
	if db == nil {
		return errors.New("database tidak tersedia")
	}
	if s.ID <= 0 {
		return errors.New("id soal tidak valid")
	}
	if strings.TrimSpace(s.TeksSoal) == "" {
		return ErrTeksSoalKosong
	}
	tipe := strings.ToLower(strings.TrimSpace(s.TipeSoal))
	if tipe != "pg" && tipe != "menjodohkan" && tipe != "essay" {
		return ErrTipeSoalInvalid
	}

	s.TipeSoal = tipe
	s.Poin = DefaultPoin(tipe, s.Poin)

	res, err := db.ExecContext(ctx, `
		UPDATE soal
		SET tipe_soal = ?, teks_soal = ?, gambar = ?, poin = ?,
		    pilihan_a = ?, pilihan_b = ?, pilihan_c = ?, pilihan_d = ?,
		    jawaban_benar = ?, opsi_tambahan = ?
		WHERE id = ?
	`, s.TipeSoal, s.TeksSoal, s.Gambar, s.Poin,
		s.PilihanA, s.PilihanB, s.PilihanC, s.PilihanD, s.JawabanBenar, s.OpsiTambahan, s.ID)
	if err != nil {
		return fmt.Errorf("update soal: %w", err)
	}

	affected, err := res.RowsAffected()
	if err == nil && affected == 0 {
		return ErrSoalNotFound
	}
	return nil
}

// HapusSoal menghapus satu soal berdasarkan ID.
func HapusSoal(ctx context.Context, db *sql.DB, id int) error {
	if db == nil {
		return errors.New("database tidak tersedia")
	}
	res, err := db.ExecContext(ctx, "DELETE FROM soal WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete soal: %w", err)
	}
	affected, err := res.RowsAffected()
	if err == nil && affected == 0 {
		return ErrSoalNotFound
	}
	return nil
}

// BatchTambahSoal menambahkan sekumpulan soal ke dalam ujian secara massal.
func BatchTambahSoal(ctx context.Context, db *sql.DB, ujianID int, daftarSoal []SoalKelola) (int, error) {
	if db == nil {
		return 0, errors.New("database tidak tersedia")
	}
	if ujianID <= 0 {
		return 0, errors.New("ujian_id tidak valid")
	}
	if len(daftarSoal) == 0 {
		return 0, nil
	}

	var sukses int
	for _, s := range daftarSoal {
		s.UjianID = ujianID
		_, err := TambahSoal(ctx, db, s)
		if err != nil {
			return sukses, fmt.Errorf("batch tambah soal baris %d: %w", sukses+1, err)
		}
		sukses++
	}
	return sukses, nil
}
