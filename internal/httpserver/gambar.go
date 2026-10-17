package httpserver

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/soal"
)

// gambarSoalHandler melayani GET /uploads/soal/{nama} (G-023). Berkas tidak berada di bawah
// folder statis publik: admin dan guru boleh melihat (nama berkas acak 128-bit, dibutuhkan
// untuk pratinjau sebelum soal disimpan); siswa hanya untuk gambar soal pada ujian yang sesinya
// sedang berjalan.
func gambarSoalHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nama := r.PathValue("nama")
		jalur, err := soal.JalurGambar(nama)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		ctx := r.Context()
		boleh := sm.GetInt(ctx, "adminId") > 0 || sm.GetInt(ctx, "guruId") > 0
		if !boleh {
			if siswaID := sm.GetInt(ctx, "siswaId"); siswaID > 0 {
				var ada int
				err := db.QueryRowContext(ctx, `
					SELECT 1 FROM sesi_ujian su
					JOIN soal s ON s.ujian_id = su.ujian_id
					WHERE su.siswa_id = ? AND su.status = 'sedang_ujian' AND s.gambar = ?
					LIMIT 1`, siswaID, nama).Scan(&ada)
				switch {
				case err == nil:
					boleh = true
				case err != sql.ErrNoRows:
					slog.Error("cek akses gambar siswa", "err", err)
					http.Error(w, "terjadi kesalahan pada server", http.StatusInternalServerError)
					return
				}
			}
		}
		if !boleh {
			http.Error(w, "akses ditolak", http.StatusForbidden)
			return
		}

		// Nama berkas unik per unggahan → aman di-cache; "private" agar proxy tidak menyimpannya.
		w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeFile(w, r, jalur)
	}
}
