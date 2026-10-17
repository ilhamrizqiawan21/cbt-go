// Package analisis menghitung statistik butir soal sederhana per ujian (G-028a):
// persentase benar dan sebaran pilihan jawaban. Hanya siswa yang sudah memiliki nilai
// (sesi selesai) yang dihitung, sehingga jawaban sesi yang masih berjalan tidak mengubah angka.
package analisis

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Butir adalah statistik satu soal.
type Butir struct {
	No          int
	SoalID      int
	Tipe        string
	Teks        string
	Poin        int
	Kunci       string // hanya PG
	Menjawab    int    // jumlah peserta yang punya jawaban untuk soal ini
	Benar       int    // PG/menjodohkan
	Kosong      int    // peserta - menjawab
	PersenBenar int    // 0–100, dari seluruh peserta (kosong dihitung salah); -1 untuk essay
	PilihanA    int
	PilihanB    int
	PilihanC    int
	PilihanD    int
	RataSkor    float64 // essay: rata-rata skor dari nilai_essay
	AdaSkor     bool    // essay: sudah ada nilai
	Sulit       bool    // PersenBenar < 30
	Mudah       bool    // PersenBenar > 90
}

// Hasil adalah statistik satu ujian.
type Hasil struct {
	Peserta   int
	RataNilai float64
	Butir     []Butir
}

// Hitung mengambil statistik seluruh soal ujian dengan satu query teragregasi (tanpa N+1).
func Hitung(ctx context.Context, db *sql.DB, ujianID int) (*Hasil, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}
	h := &Hasil{Butir: []Butir{}}

	var rata sql.NullFloat64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*), AVG(nilai) FROM nilai_ujian WHERE ujian_id = ?`, ujianID).Scan(&h.Peserta, &rata); err != nil {
		return nil, fmt.Errorf("hitung peserta: %w", err)
	}
	h.RataNilai = rata.Float64

	rows, err := db.QueryContext(ctx, `
		SELECT s.id, s.tipe_soal, s.teks_soal, s.poin, COALESCE(s.jawaban_benar, ''),
		       COUNT(js.id),
		       COALESCE(SUM(CASE WHEN s.tipe_soal = 'pg' THEN js.jawaban_dipilih = s.jawaban_benar
		                         WHEN s.tipe_soal = 'menjodohkan' THEN js.is_benar = 1 ELSE 0 END), 0),
		       COALESCE(SUM(js.jawaban_dipilih = 'A'), 0), COALESCE(SUM(js.jawaban_dipilih = 'B'), 0),
		       COALESCE(SUM(js.jawaban_dipilih = 'C'), 0), COALESCE(SUM(js.jawaban_dipilih = 'D'), 0),
		       (SELECT AVG(ne.skor) FROM nilai_essay ne WHERE ne.soal_id = s.id AND ne.ujian_id = s.ujian_id)
		FROM soal s
		LEFT JOIN jawaban_siswa js ON js.soal_id = s.id AND js.siswa_id IN (
			SELECT nu.siswa_id FROM nilai_ujian nu WHERE nu.ujian_id = s.ujian_id)
		WHERE s.ujian_id = ?
		GROUP BY s.id, s.tipe_soal, s.teks_soal, s.poin, s.jawaban_benar
		ORDER BY s.id ASC
	`, ujianID)
	if err != nil {
		return nil, fmt.Errorf("hitung butir soal: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var b Butir
		var avgSkor sql.NullFloat64
		if err := rows.Scan(&b.SoalID, &b.Tipe, &b.Teks, &b.Poin, &b.Kunci, &b.Menjawab, &b.Benar,
			&b.PilihanA, &b.PilihanB, &b.PilihanC, &b.PilihanD, &avgSkor); err != nil {
			return nil, fmt.Errorf("scan butir soal: %w", err)
		}
		b.No = len(h.Butir) + 1
		b.Kosong = max(h.Peserta-b.Menjawab, 0)
		b.PersenBenar = -1
		switch b.Tipe {
		case "essay":
			b.RataSkor, b.AdaSkor = avgSkor.Float64, avgSkor.Valid
		default:
			if h.Peserta > 0 {
				b.PersenBenar = b.Benar * 100 / h.Peserta
				b.Sulit = b.PersenBenar < 30
				b.Mudah = b.PersenBenar > 90
			}
		}
		if b.Tipe != "pg" {
			b.Kunci = ""
		}
		h.Butir = append(h.Butir, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return h, nil
}
