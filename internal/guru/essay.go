package guru

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"cbt-go/internal/excel"
	"cbt-go/internal/penilaian"
	"cbt-go/internal/sesi"
	"cbt-go/internal/soal"
)

var (
	ErrSkorInvalid = errors.New("skor harus bernilai antara 0 dan poin maksimal soal")
)

// GetJawabanEssayUjian mengambil seluruh data jawaban soal essay siswa untuk keperluan koreksi dan ekspor Excel.
func GetJawabanEssayUjian(ctx context.Context, db *sql.DB, ujianID int) ([]excel.EssayExportRow, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}

	query := `
		SELECT s.id, s.nis, s.nama, s.kelas,
		       so.id, so.teks_soal, COALESCE(js.jawaban_dipilih, ''), so.poin,
		       ne.skor
		FROM soal so
		JOIN ujian u ON so.ujian_id = u.id
		JOIN pengajaran p ON u.pengajaran_id = p.id
		JOIN siswa s ON s.kelas = (SELECT nama_kelas FROM kelas WHERE id = p.kelas_id)
		LEFT JOIN jawaban_siswa js ON js.soal_id = so.id AND js.siswa_id = s.id AND js.ujian_id = u.id
		LEFT JOIN nilai_essay ne ON ne.soal_id = so.id AND ne.siswa_id = s.id AND ne.ujian_id = u.id
		WHERE so.ujian_id = ? AND so.tipe_soal = 'essay'
		ORDER BY s.kelas ASC, s.nama ASC, so.id ASC
	`

	rows, err := db.QueryContext(ctx, query, ujianID)
	if err != nil {
		return nil, fmt.Errorf("query jawaban essay: %w", err)
	}
	defer rows.Close()

	var list []excel.EssayExportRow
	for rows.Next() {
		var r excel.EssayExportRow
		var skorNull sql.NullInt64
		err := rows.Scan(
			&r.SiswaID, &r.NIS, &r.NamaSiswa, &r.Kelas,
			&r.SoalID, &r.TeksSoal, &r.JawabanSiswa, &r.PoinMaks,
			&skorNull,
		)
		if err != nil {
			return nil, fmt.Errorf("scan jawaban essay: %w", err)
		}
		if skorNull.Valid {
			s := int(skorNull.Int64)
			r.Skor = &s
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if list == nil {
		list = []excel.EssayExportRow{}
	}
	return list, nil
}

// SimpanNilaiEssay menyimpan skor essay per siswa dan memperbarui nilai_ujian secara otomatis.
func SimpanNilaiEssay(ctx context.Context, db *sql.DB, guruID, ujianID, siswaID, soalID, skor int) (*penilaian.Hasil, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}

	// 1. Cek otorisasi guru pengampu
	boleh, err := soal.CekOtorisasiUjianGuru(ctx, db, guruID, ujianID)
	if err != nil {
		return nil, fmt.Errorf("cek otorisasi ujian guru: %w", err)
	}
	if !boleh {
		return nil, soal.ErrForbiddenGuru
	}

	// 2. Ambil poin maksimal soal
	var poinMaks int
	var tipeSoal string
	err = db.QueryRowContext(ctx, `
		SELECT poin, tipe_soal
		FROM soal
		WHERE id = ? AND ujian_id = ?
	`, soalID, ujianID).Scan(&poinMaks, &tipeSoal)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("soal tidak ditemukan pada ujian ini")
		}
		return nil, fmt.Errorf("query soal essay: %w", err)
	}

	if tipeSoal != "essay" {
		return nil, errors.New("soal bukan bertipe essay")
	}

	// 3. Validasi skor dalam batas 0 <= skor <= poinMaks
	if skor < 0 || skor > poinMaks {
		return nil, ErrSkorInvalid
	}

	// 4. Simpan skor ke tabel nilai_essay secara idempoten
	_, err = db.ExecContext(ctx, `
		INSERT INTO nilai_essay (siswa_id, ujian_id, soal_id, skor, dinilai_oleh, dinilai_pada)
		VALUES (?, ?, ?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE
			skor = VALUES(skor),
			dinilai_oleh = VALUES(dinilai_oleh),
			dinilai_pada = NOW()
	`, siswaID, ujianID, soalID, skor, guruID)
	if err != nil {
		return nil, fmt.Errorf("insert nilai essay: %w", err)
	}

	// 5. Hitung ulang nilai ujian siswa (jika seluruh essay dinilai -> status_koreksi otomatis 'selesai')
	hasil, err := sesi.HitungUlangNilai(ctx, db, siswaID, ujianID)
	if err != nil {
		return nil, fmt.Errorf("hitung ulang nilai setelah essay: %w", err)
	}

	return hasil, nil
}

// BatchSimpanNilaiEssay memproses pembaruan skor essay massal (misalnya hasil impor Excel).
func BatchSimpanNilaiEssay(ctx context.Context, db *sql.DB, guruID, ujianID int, items []excel.EssayImportItem) (int, []excel.ImportError, error) {
	if db == nil {
		return 0, nil, errors.New("database tidak tersedia")
	}

	boleh, err := soal.CekOtorisasiUjianGuru(ctx, db, guruID, ujianID)
	if err != nil {
		return 0, nil, err
	}
	if !boleh {
		return 0, nil, soal.ErrForbiddenGuru
	}

	var sukses int
	var laporanGagal []excel.ImportError

	for _, it := range items {
		_, err := SimpanNilaiEssay(ctx, db, guruID, ujianID, it.SiswaID, it.SoalID, it.Skor)
		if err != nil {
			laporanGagal = append(laporanGagal, excel.ImportError{
				Baris: it.Baris,
				Pesan: fmt.Sprintf("Gagal menyimpan skor siswa %d soal %d: %v", it.SiswaID, it.SoalID, err),
			})
			continue
		}
		sukses++
	}

	return sukses, nil, nil
}
