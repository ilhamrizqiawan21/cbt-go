package admin

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
)

// RingkasanDashboard berisi hitungan data master dan aktivitas ujian saat ini.
type RingkasanDashboard struct {
	Siswa      int
	Guru       int
	Kelas      int
	Mapel      int
	Ujian      int
	UjianAktif int
	SesiAktif  int
}

// UjianDashboard adalah ujian yang sedang/akan berlangsung beserta jumlah peserta.
type UjianDashboard struct {
	ID             int
	NamaUjian      string
	NamaMapel      string
	NamaKelas      string
	TanggalMulai   time.Time
	TanggalSelesai time.Time
	Berlangsung    bool // dihitung dengan jam database (NOW()), bukan jam aplikasi
	SedangUjian    int
	Selesai        int
}

// AmbilRingkasanDashboard menghitung seluruh angka dashboard dalam satu round-trip.
func AmbilRingkasanDashboard(ctx context.Context, db *sql.DB) (*RingkasanDashboard, error) {
	var r RingkasanDashboard
	err := db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM siswa),
			(SELECT COUNT(*) FROM guru),
			(SELECT COUNT(*) FROM kelas),
			(SELECT COUNT(*) FROM mata_pelajaran),
			(SELECT COUNT(*) FROM ujian),
			(SELECT COUNT(*) FROM ujian WHERE NOW() BETWEEN tanggal_mulai AND tanggal_selesai),
			(SELECT COUNT(*) FROM sesi_ujian WHERE status = 'sedang_ujian')
	`).Scan(&r.Siswa, &r.Guru, &r.Kelas, &r.Mapel, &r.Ujian, &r.UjianAktif, &r.SesiAktif)
	if err != nil {
		return nil, fmt.Errorf("ringkasan dashboard: %w", err)
	}
	return &r, nil
}

// ListUjianMendatang mengambil ujian yang belum berakhir, terdekat lebih dulu.
func ListUjianMendatang(ctx context.Context, db *sql.DB, limit int) ([]UjianDashboard, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT u.id, u.nama_ujian, m.nama_mapel, k.nama_kelas, u.tanggal_mulai, u.tanggal_selesai,
		       NOW() >= u.tanggal_mulai AS berlangsung,
		       COALESCE(SUM(su.status = 'sedang_ujian'), 0),
		       COALESCE(SUM(su.status = 'selesai'), 0)
		FROM ujian u
		JOIN pengajaran p ON p.id = u.pengajaran_id
		JOIN mata_pelajaran m ON m.id = p.mapel_id
		JOIN kelas k ON k.id = p.kelas_id
		LEFT JOIN sesi_ujian su ON su.ujian_id = u.id
		WHERE u.tanggal_selesai >= NOW()
		GROUP BY u.id, u.nama_ujian, m.nama_mapel, k.nama_kelas, u.tanggal_mulai, u.tanggal_selesai
		ORDER BY u.tanggal_mulai ASC, u.id ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list ujian mendatang: %w", err)
	}
	defer rows.Close()

	list := []UjianDashboard{}
	for rows.Next() {
		var u UjianDashboard
		if err := rows.Scan(&u.ID, &u.NamaUjian, &u.NamaMapel, &u.NamaKelas, &u.TanggalMulai, &u.TanggalSelesai,
			&u.Berlangsung, &u.SedangUjian, &u.Selesai); err != nil {
			return nil, fmt.Errorf("scan ujian mendatang: %w", err)
		}
		list = append(list, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterasi ujian mendatang: %w", err)
	}
	return list, nil
}

// DashboardHandler menampilkan halaman ringkasan admin (GET /admin/dashboard).
func DashboardHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ringkasan, err := AmbilRingkasanDashboard(r.Context(), db)
		if err != nil {
			serverError(w, r, err)
			return
		}
		ujian, err := ListUjianMendatang(r.Context(), db, 10)
		if err != nil {
			serverError(w, r, err)
			return
		}
		render(w, r, "admin_dashboard.gohtml", map[string]any{
			"Ringkasan":  ringkasan,
			"UjianList":  ujian,
			"Error":      getFlash(sm, r, "error"),
			"Success":    getFlash(sm, r, "success"),
			"Username":   sm.GetString(r.Context(), "username"),
			"ActiveMenu": "dashboard",
		})
	}
}
