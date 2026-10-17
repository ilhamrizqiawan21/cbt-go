package admin

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// HadirRow adalah satu baris daftar hadir ujian.
type HadirRow struct {
	NIS         string
	Nama        string
	Status      string // "" = tidak hadir | sedang_ujian | selesai | keluar_paksa
	Mulai       *time.Time
	Selesai     *time.Time
	Pelanggaran int // pindah tab / keluar layar penuh / copy-paste
	AmbilAlih   int // jumlah perpindahan perangkat
}

// RekapHadir meringkas daftar hadir untuk berita acara.
type RekapHadir struct {
	Total, Hadir, TidakHadir, Selesai, Terkunci, BelumSelesai int
	TotalPelanggaran                                          int
}

// GetDaftarHadir mengambil semua siswa kelas ujian (termasuk yang tidak masuk) dengan satu
// query teragregasi. Jenis log "buka_kunci" (penanda aksi pengawas) tidak dihitung pelanggaran.
func GetDaftarHadir(ctx context.Context, db *sql.DB, ujianID int) ([]HadirRow, RekapHadir, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.nis, s.nama, COALESCE(su.status, ''), su.waktu_mulai, su.selesai_pada,
		       COALESCE(lg.pelanggaran, 0), COALESCE(lg.ambil_alih, 0)
		FROM ujian u
		JOIN pengajaran p ON p.id = u.pengajaran_id
		JOIN kelas k ON k.id = p.kelas_id
		JOIN siswa s ON s.kelas = k.nama_kelas
		LEFT JOIN sesi_ujian su ON su.siswa_id = s.id AND su.ujian_id = u.id
		LEFT JOIN (
			SELECT siswa_id, ujian_id,
			       SUM(jenis_kecurangan NOT IN ('buka_kunci', 'ambil_alih_sesi')) AS pelanggaran,
			       SUM(jenis_kecurangan = 'ambil_alih_sesi') AS ambil_alih
			FROM log_kecurangan WHERE ujian_id = ? GROUP BY siswa_id, ujian_id
		) lg ON lg.siswa_id = s.id AND lg.ujian_id = u.id
		WHERE u.id = ?
		ORDER BY s.nama ASC, s.id ASC
	`, ujianID, ujianID)
	if err != nil {
		return nil, RekapHadir{}, fmt.Errorf("daftar hadir: %w", err)
	}
	defer rows.Close()

	var list []HadirRow
	var rk RekapHadir
	for rows.Next() {
		var h HadirRow
		var mulai, selesai sql.NullTime
		if err := rows.Scan(&h.NIS, &h.Nama, &h.Status, &mulai, &selesai, &h.Pelanggaran, &h.AmbilAlih); err != nil {
			return nil, RekapHadir{}, fmt.Errorf("scan daftar hadir: %w", err)
		}
		if mulai.Valid {
			t := mulai.Time
			h.Mulai = &t
		}
		if selesai.Valid {
			t := selesai.Time
			h.Selesai = &t
		}
		rk.Total++
		rk.TotalPelanggaran += h.Pelanggaran
		switch h.Status {
		case "":
			rk.TidakHadir++
		case "selesai":
			rk.Hadir++
			rk.Selesai++
		case "keluar_paksa":
			rk.Hadir++
			rk.Terkunci++
		default:
			rk.Hadir++
			rk.BelumSelesai++
		}
		list = append(list, h)
	}
	return list, rk, rows.Err()
}
