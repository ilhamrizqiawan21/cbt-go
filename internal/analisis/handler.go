package analisis

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/soal"
	"cbt-go/web"
)

// UjianPilihan adalah satu entri pemilih ujian di halaman.
type UjianPilihan struct {
	ID        int
	NamaUjian string
	NamaKelas string
}

// Handler membuat halaman analisis butir untuk satu peran.
//   - peran: "admin" atau "guru" (menentukan navbar dan daftar ujian).
//   - daftar: ujian yang boleh dilihat pemanggil (admin: semua; guru: ujian pengampuannya).
type Handler struct {
	DB     *sql.DB
	SM     *scs.SessionManager
	Peran  string
	Daftar func(r *http.Request) ([]UjianPilihan, error)
}

// ServeHTTP menampilkan analisis untuk ?ujian_id= (default: ujian pertama pada daftar).
// Ujian di luar daftar ditolak 403, sehingga guru tidak dapat melihat ujian guru lain.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	daftar, err := h.Daftar(r)
	if err != nil {
		h.galat(w, r, err)
		return
	}
	ujianID, _ := strconv.Atoi(r.URL.Query().Get("ujian_id"))
	if ujianID <= 0 && len(daftar) > 0 {
		ujianID = daftar[0].ID
	}
	var hasil *Hasil
	if ujianID > 0 {
		boleh := false
		for _, u := range daftar {
			if u.ID == ujianID {
				boleh = true
				break
			}
		}
		if !boleh {
			web.RenderError(w, http.StatusForbidden, "Akses ditolak: ujian ini bukan milik Anda")
			return
		}
		hasil, err = Hitung(r.Context(), h.DB, ujianID)
		if err != nil {
			h.galat(w, r, err)
			return
		}
	}
	if err := web.Render(w, "analisis.gohtml", map[string]any{
		"Peran":      h.Peran,
		"Username":   h.SM.GetString(r.Context(), "username"),
		"NamaGuru":   h.SM.GetString(r.Context(), "namaGuru"),
		"ActiveMenu": "analisis",
		"UjianList":  daftar,
		"UjianID":    ujianID,
		"Hasil":      hasil,
	}); err != nil {
		h.galat(w, r, err)
	}
}

func (h Handler) galat(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "analisis butir gagal", "peran", h.Peran, "err", err)
	web.RenderError(w, http.StatusInternalServerError, "Terjadi kesalahan pada server. Silakan coba lagi.")
}

// DaftarGuru mengembalikan ujian pengampuan guru yang sedang login.
func DaftarGuru(db *sql.DB, sm *scs.SessionManager) func(*http.Request) ([]UjianPilihan, error) {
	return func(r *http.Request) ([]UjianPilihan, error) {
		list, err := soal.ListUjianByGuru(r.Context(), db, sm.GetInt(r.Context(), "guruId"))
		if err != nil {
			return nil, err
		}
		out := make([]UjianPilihan, len(list))
		for i, u := range list {
			out[i] = UjianPilihan{ID: u.ID, NamaUjian: u.NamaUjian, NamaKelas: u.NamaKelas}
		}
		return out, nil
	}
}

// DaftarSemua mengembalikan semua ujian (admin), terbaru lebih dulu.
func DaftarSemua(db *sql.DB) func(*http.Request) ([]UjianPilihan, error) {
	return func(r *http.Request) ([]UjianPilihan, error) {
		rows, err := db.QueryContext(r.Context(), `
			SELECT u.id, u.nama_ujian, k.nama_kelas
			FROM ujian u
			JOIN pengajaran p ON p.id = u.pengajaran_id
			JOIN kelas k ON k.id = p.kelas_id
			ORDER BY u.tanggal_mulai DESC, u.id DESC`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []UjianPilihan
		for rows.Next() {
			var u UjianPilihan
			if err := rows.Scan(&u.ID, &u.NamaUjian, &u.NamaKelas); err != nil {
				return nil, err
			}
			out = append(out, u)
		}
		return out, rows.Err()
	}
}
