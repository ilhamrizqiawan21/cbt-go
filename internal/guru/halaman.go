package guru

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/alexedwards/scs/v2"

	"cbt-go/web"
)

// render menampilkan template; kegagalan eksekusi template dicatat dan diganti halaman 500
// (bukan halaman kosong tanpa jejak).
func render(w http.ResponseWriter, r *http.Request, name string, data any) {
	if err := web.Render(w, name, data); err != nil {
		slog.ErrorContext(r.Context(), "render template guru gagal", "template", name, "err", err)
		web.RenderError(w, http.StatusInternalServerError, "Terjadi kesalahan saat menampilkan halaman.")
	}
}

// serverError mencatat detail error di log dan mengirim pesan generik ke klien.
func serverError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "kesalahan server guru", "method", r.Method, "path", r.URL.Path, "err", err)
	const msg = "Terjadi kesalahan pada server. Silakan coba lagi."
	if wantsJSON(r) || isJSONBody(r) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg})
		return
	}
	web.RenderError(w, http.StatusInternalServerError, msg)
}

// forbidden menolak akses ke ujian/soal milik guru lain.
func forbidden(w http.ResponseWriter, r *http.Request) {
	const msg = "Akses ditolak: Anda bukan pengampu ujian ini"
	if wantsJSON(r) || isJSONBody(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": msg})
		return
	}
	web.RenderError(w, http.StatusForbidden, msg)
}

// dataHalaman berisi field yang dipakai navbar dan flash di setiap halaman guru.
func dataHalaman(sm *scs.SessionManager, r *http.Request, menu string) map[string]any {
	return map[string]any{
		"NamaGuru":   sm.GetString(r.Context(), "namaGuru"),
		"Username":   sm.GetString(r.Context(), "username"),
		"ActiveMenu": menu,
		"Error":      getFlash(sm, r, "error"),
		"Success":    getFlash(sm, r, "success"),
	}
}

// kelolaSoalURL membangun alamat kembali ke daftar soal satu ujian.
func kelolaSoalURL(ujianID int) string {
	if ujianID > 0 {
		return "/guru/kelola-soal?ujian_id=" + strconv.Itoa(ujianID)
	}
	return "/guru/kelola-soal"
}

// UjianRingkasan adalah ujian milik guru beserta angka progres untuk dashboard dan daftar essay.
type UjianRingkasan struct {
	ID             int
	NamaUjian      string
	NamaMapel      string
	NamaKelas      string
	TanggalMulai   time.Time
	TanggalSelesai time.Time
	Berlangsung    bool // dihitung dengan jam database
	Berakhir       bool
	JumlahSoal     int
	JumlahEssay    int
	Selesai        int // siswa yang sudah punya nilai
	MenungguEssay  int // nilai berstatus menunggu_essay
}

// ListRingkasanUjianGuru mengambil ujian yang diampu guru beserta hitungannya dalam satu query.
// Subquery berkorelasi dievaluasi per ujian milik satu guru (jumlahnya kecil), tetap satu round-trip.
func ListRingkasanUjianGuru(ctx context.Context, db *sql.DB, guruID int) ([]UjianRingkasan, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT u.id, u.nama_ujian, m.nama_mapel, k.nama_kelas, u.tanggal_mulai, u.tanggal_selesai,
		       NOW() BETWEEN u.tanggal_mulai AND u.tanggal_selesai,
		       NOW() > u.tanggal_selesai,
		       (SELECT COUNT(*) FROM soal so WHERE so.ujian_id = u.id),
		       (SELECT COUNT(*) FROM soal so WHERE so.ujian_id = u.id AND so.tipe_soal = 'essay'),
		       (SELECT COUNT(*) FROM nilai_ujian nu WHERE nu.ujian_id = u.id),
		       (SELECT COUNT(*) FROM nilai_ujian nu WHERE nu.ujian_id = u.id AND nu.status_koreksi = 'menunggu_essay')
		FROM ujian u
		JOIN pengajaran p ON p.id = u.pengajaran_id
		JOIN mata_pelajaran m ON m.id = p.mapel_id
		JOIN kelas k ON k.id = p.kelas_id
		WHERE p.guru_id = ?
		ORDER BY u.tanggal_mulai DESC, u.id DESC
	`, guruID)
	if err != nil {
		return nil, fmt.Errorf("ringkasan ujian guru: %w", err)
	}
	defer rows.Close()

	list := []UjianRingkasan{}
	for rows.Next() {
		var u UjianRingkasan
		if err := rows.Scan(&u.ID, &u.NamaUjian, &u.NamaMapel, &u.NamaKelas, &u.TanggalMulai, &u.TanggalSelesai,
			&u.Berlangsung, &u.Berakhir, &u.JumlahSoal, &u.JumlahEssay, &u.Selesai, &u.MenungguEssay); err != nil {
			return nil, fmt.Errorf("scan ringkasan ujian guru: %w", err)
		}
		list = append(list, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterasi ringkasan ujian guru: %w", err)
	}
	return list, nil
}

// GuruDashboardHandler menampilkan ringkasan ujian yang diampu guru (GET /guru/dashboard).
func GuruDashboardHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := ListRingkasanUjianGuru(r.Context(), db, sm.GetInt(r.Context(), "guruId"))
		if err != nil {
			serverError(w, r, err)
			return
		}
		var menunggu int
		for _, u := range list {
			menunggu += u.MenungguEssay
		}
		data := dataHalaman(sm, r, "dashboard")
		data["UjianList"] = list
		data["TotalMenunggu"] = menunggu
		render(w, r, "guru_dashboard.gohtml", data)
	}
}
