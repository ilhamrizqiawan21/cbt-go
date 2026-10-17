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

			// Validasi status sesi dan device_token di database jika db tersedia
			if db != nil {
				clientToken := r.Header.Get("X-Device-Token")
				if clientToken == "" {
					clientToken = sessionToken
				}

				var dbToken sql.NullString
				var status string
				err := db.QueryRowContext(r.Context(), `
					SELECT device_token, status
					FROM sesi_ujian
					WHERE siswa_id = ? AND ujian_id = ?
				`, siswaID, ujianID).Scan(&dbToken, &status)

				if err == nil {
					if status == "keluar_paksa" {
						// Field status membedakan "terkunci" dari 403 lain, sehingga ujian.js menampilkan
						// layar terkunci dan tidak menghapus antrean jawaban lokal.
						w.Header().Set("Content-Type", "application/json; charset=utf-8")
						w.WriteHeader(http.StatusForbidden)
						_ = json.NewEncoder(w).Encode(map[string]any{
							"error":  "Akses ujian Anda telah dicabut karena pelanggaran",
							"code":   http.StatusForbidden,
							"status": "keluar_paksa",
						})
						return
					}
					if status == "selesai" {
						writeJSONError(w, http.StatusForbidden, "Ujian sudah selesai")
						return
					}
					if dbToken.Valid && clientToken != "" && dbToken.String != clientToken {
						writeJSONError(w, http.StatusConflict, "Sesi Anda telah diambil alih di perangkat lain")
						return
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin memastikan request halaman HTML berasal dari admin.
// Jika tidak login atau bukan admin, dialihkan ke /login-admin.
func RequireAdmin(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sm == nil {
				http.Redirect(w, r, "/login-admin", http.StatusFound)
				return
			}
			adminID := sm.GetInt(r.Context(), "adminId")
			peran := sm.GetString(r.Context(), "peran")
			if adminID == 0 || peran != "admin" {
				http.Redirect(w, r, "/login-admin", http.StatusFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdminAPI memastikan request API berasal dari admin yang memiliki sesi aktif.
// Mengembalikan 401 Unauthorized jika tidak login, atau 403 Forbidden jika bukan admin.
func RequireAdminAPI(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sm == nil {
				writeJSONError(w, http.StatusUnauthorized, "Sesi tidak ditemukan atau telah berakhir")
				return
			}
			adminID := sm.GetInt(r.Context(), "adminId")
			peran := sm.GetString(r.Context(), "peran")

			if adminID == 0 && peran == "" {
				writeJSONError(w, http.StatusUnauthorized, "Sesi tidak ditemukan atau telah berakhir")
				return
			}
			if adminID == 0 || peran != "admin" {
				writeJSONError(w, http.StatusForbidden, "Akses ditolak: hanya admin")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireGuru memastikan request halaman HTML berasal dari guru yang memiliki sesi aktif.
// Jika tidak login atau bukan guru, dialihkan ke /login-guru.
func RequireGuru(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sm == nil {
				http.Redirect(w, r, "/login-guru", http.StatusFound)
				return
			}
			guruID := sm.GetInt(r.Context(), "guruId")
			peran := sm.GetString(r.Context(), "peran")
			if guruID == 0 || peran != "guru" {
				http.Redirect(w, r, "/login-guru", http.StatusFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireGuruAPI memastikan request API berasal dari guru yang memiliki sesi aktif.
// Mengembalikan 401 Unauthorized jika tidak login, atau 403 Forbidden jika bukan guru.
func RequireGuruAPI(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sm == nil {
				writeJSONError(w, http.StatusUnauthorized, "Sesi tidak ditemukan atau telah berakhir")
				return
			}
			guruID := sm.GetInt(r.Context(), "guruId")
			peran := sm.GetString(r.Context(), "peran")

			if guruID == 0 && peran == "" {
				writeJSONError(w, http.StatusUnauthorized, "Sesi tidak ditemukan atau telah berakhir")
				return
			}
			if guruID == 0 || peran != "guru" {
				writeJSONError(w, http.StatusForbidden, "Akses ditolak: hanya guru")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg, "code": code})
}
