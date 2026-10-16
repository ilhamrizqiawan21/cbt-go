package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"cbt-go/internal/excel"
)

// HasilUjianRow merepresentasikan baris rekapitulasi nilai ujian seorang siswa.
type HasilUjianRow struct {
	ID            int       `json:"id"`
	NIS           string    `json:"nis"`
	NamaSiswa     string    `json:"nama_siswa"`
	Kelas         string    `json:"kelas"`
	PoinOtomatis  int       `json:"poin_otomatis"`
	PoinEssay     int       `json:"poin_essay"`
	PoinMaks      int       `json:"poin_maks"`
	NilaiAkhir    int       `json:"nilai"`
	Benar         int       `json:"benar"`
	Salah         int       `json:"salah"`
	Kosong        int       `json:"kosong"`
	StatusKoreksi string    `json:"status_koreksi"`
	SelesaiPada   time.Time `json:"selesai_pada"`

	UjianID   int    `json:"ujian_id"`
	NamaUjian string `json:"nama_ujian"`
	NamaMapel string `json:"nama_mapel"`
	NamaGuru  string `json:"nama_guru"`
	NamaKelas string `json:"nama_kelas"`
}

// GetHasilByUjian mengambil data rekap nilai seluruh siswa untuk ujian_id tertentu (akses admin).
func GetHasilByUjian(ctx context.Context, db *sql.DB, ujianID int, kelasFilter string) ([]HasilUjianRow, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}

	query := `
		SELECT nu.id, s.nis, s.nama, s.kelas,
		       nu.poin_otomatis, nu.poin_essay, nu.poin_maks, nu.nilai,
		       nu.benar, nu.salah, nu.kosong, nu.status_koreksi, nu.selesai_pada,
		       u.id, u.nama_ujian, m.nama_mapel, g.nama, k.nama_kelas
		FROM nilai_ujian nu
		JOIN siswa s ON nu.siswa_id = s.id
		JOIN ujian u ON nu.ujian_id = u.id
		JOIN pengajaran p ON u.pengajaran_id = p.id
		JOIN mata_pelajaran m ON p.mapel_id = m.id
		JOIN kelas k ON p.kelas_id = k.id
		JOIN guru g ON p.guru_id = g.id
		WHERE u.id = ?
	`
	var args []any
	args = append(args, ujianID)

	if strings.TrimSpace(kelasFilter) != "" {
		query += " AND s.kelas = ?"
		args = append(args, strings.TrimSpace(kelasFilter))
	}
	query += " ORDER BY s.kelas ASC, s.nama ASC"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query hasil ujian: %w", err)
	}
	defer rows.Close()

	var list []HasilUjianRow
	for rows.Next() {
		var h HasilUjianRow
		var selesai sql.NullTime
		err := rows.Scan(
			&h.ID, &h.NIS, &h.NamaSiswa, &h.Kelas,
			&h.PoinOtomatis, &h.PoinEssay, &h.PoinMaks, &h.NilaiAkhir,
			&h.Benar, &h.Salah, &h.Kosong, &h.StatusKoreksi, &selesai,
			&h.UjianID, &h.NamaUjian, &h.NamaMapel, &h.NamaGuru, &h.NamaKelas,
		)
		if err != nil {
			return nil, fmt.Errorf("scan hasil ujian: %w", err)
		}
		if selesai.Valid {
			h.SelesaiPada = selesai.Time
		}
		list = append(list, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if list == nil {
		list = []HasilUjianRow{}
	}
	return list, nil
}

// ConvertToExcelItems mengonversi list HasilUjianRow ke slice excel.HasilUjianItem untuk file spreadsheet.
func ConvertToExcelItems(list []HasilUjianRow) []excel.HasilUjianItem {
	items := make([]excel.HasilUjianItem, len(list))
	for i, it := range list {
		items[i] = excel.HasilUjianItem{
			NIS:           it.NIS,
			NamaSiswa:     it.NamaSiswa,
			Kelas:         it.Kelas,
			Benar:         it.Benar,
			Salah:         it.Salah,
			Kosong:        it.Kosong,
			PoinOtomatis:  it.PoinOtomatis,
			PoinEssay:     it.PoinEssay,
			PoinMaks:      it.PoinMaks,
			NilaiAkhir:    it.NilaiAkhir,
			StatusKoreksi: it.StatusKoreksi,
			SelesaiPada:   it.SelesaiPada,
		}
	}
	return items
}
