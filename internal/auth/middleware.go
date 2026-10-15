package auth

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/alexedwards/scs/v2"
)

// RequireSiswa memastikan request halaman HTML berasal dari siswa yang memiliki sesi aktif.
// Jika tidak, request diarahkan (redirect) ke /login.
func RequireSiswa(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sm == nil {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}
			siswaID := sm.GetInt(r.Context(), "siswaId")
			ujianID := sm.GetInt(r.Context(), "ujianId")
			peran := sm.GetString(r.Context(), "peran")

			if siswaID == 0 || ujianID == 0 || peran != "siswa" {
				http.Redirect(w, r, "/login", http.StatusFound)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireSiswaAPI memastikan request API berasal dari siswa yang memiliki sesi aktif.
// Mengembalikan 401 Unauthorized jika tidak login, atau 409 Conflict jika device_token mismatch (sudah diambil alih).
func RequireSiswaAPI(sm *scs.SessionManager, db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sm == nil {
				writeJSONError(w, http.StatusUnauthorized, "Sesi tidak ditemukan atau telah berakhir")
				return
			}

			siswaID := sm.GetInt(r.Context(), "siswaId")
			ujianID := sm.GetInt(r.Context(), "ujianId")
			peran := sm.GetString(r.Context(), "peran")
			sessionToken := sm.GetString(r.Context(), "deviceToken")

			if siswaID == 0 || ujianID == 0 || peran != "siswa" {
				writeJSONError(w, http.StatusUnauthorized, "Sesi tidak ditemukan atau telah berakhir")
				return
			}

			// Validasi device_token di database jika db tersedia
			if db != nil && sessionToken != "" {
				var dbToken sql.NullString
				var status string
				err := db.QueryRowContext(r.Context(), `
					SELECT device_token, status
					FROM sesi_ujian
					WHERE siswa_id = ? AND ujian_id = ?
				`, siswaID, ujianID).Scan(&dbToken, &status)

				if err == nil {
					if status == "keluar_paksa" {
						writeJSONError(w, http.StatusForbidden, "Akses ujian Anda telah dicabut karena pelanggaran")
						return
					}
					if status == "selesai" {
						writeJSONError(w, http.StatusForbidden, "Ujian sudah selesai")
						return
					}
					if dbToken.Valid && dbToken.String != sessionToken {
						writeJSONError(w, http.StatusConflict, "Sesi Anda telah diambil alih di perangkat lain")
						return
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
