package auth

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"cbt-go/web"
)

// LoginAdminHandler menangani autentikasi administrator sistem (POST /login-admin).
func LoginAdminHandler(db *sql.DB, sm *scs.SessionManager, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			renderAdminLoginError(w, "Format data form tidak valid")
			return
		}

		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")

		if username == "" || password == "" {
			renderAdminLoginError(w, "Username dan password wajib diisi")
			return
		}

		if db == nil {
			renderAdminLoginError(w, "Database tidak tersedia")
			return
		}

		var adminID int
		var dbHash string
		err := db.QueryRowContext(r.Context(), `
			SELECT id, password
			FROM admin
			WHERE username = ?
		`, username).Scan(&adminID, &dbHash)

		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Cegah timing attack
				_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(password))
				renderAdminLoginError(w, "Username atau password salah")
				return
			}
			if logger != nil {
				logger.Error("kueri admin login gagal", "err", err)
			}
			renderAdminLoginError(w, "Terjadi kesalahan sistem")
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(dbHash), []byte(password)); err != nil {
			renderAdminLoginError(w, "Username atau password salah")
			return
		}

		if sm != nil {
			if err := sm.RenewToken(r.Context()); err != nil && logger != nil {
				logger.Warn("gagal memperbarui token session admin", "err", err)
			}
			sm.Put(r.Context(), "adminId", adminID)
			sm.Put(r.Context(), "username", username)
			sm.Put(r.Context(), "peran", "admin")
		}

		http.Redirect(w, r, "/admin/dashboard", http.StatusFound)
	}
}

// LoginGuruHandler menangani autentikasi guru pengampu (POST /login-guru).
func LoginGuruHandler(db *sql.DB, sm *scs.SessionManager, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			renderGuruLoginError(w, "Format data form tidak valid")
			return
		}

		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")

		if username == "" || password == "" {
			renderGuruLoginError(w, "Username dan password wajib diisi")
			return
		}

		if db == nil {
			renderGuruLoginError(w, "Database tidak tersedia")
			return
		}

		var guruID int
		var nip, nama, dbHash string
		err := db.QueryRowContext(r.Context(), `
			SELECT id, nip, nama, password
			FROM guru
			WHERE username = ?
		`, username).Scan(&guruID, &nip, &nama, &dbHash)

		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(password))
				renderGuruLoginError(w, "Username atau password salah")
				return
			}
			if logger != nil {
				logger.Error("kueri guru login gagal", "err", err)
			}
			renderGuruLoginError(w, "Terjadi kesalahan sistem")
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(dbHash), []byte(password)); err != nil {
			renderGuruLoginError(w, "Username atau password salah")
			return
		}

		if sm != nil {
			if err := sm.RenewToken(r.Context()); err != nil && logger != nil {
				logger.Warn("gagal memperbarui token session guru", "err", err)
			}
			sm.Put(r.Context(), "guruId", guruID)
			sm.Put(r.Context(), "username", username)
			sm.Put(r.Context(), "namaGuru", nama)
			sm.Put(r.Context(), "nipGuru", nip)
			sm.Put(r.Context(), "peran", "guru")
		}

		http.Redirect(w, r, "/guru/dashboard", http.StatusFound)
	}
}

// LogoutAdminHandler menghapus sesi aktif admin dan mengarahkan kembali ke halaman login.
func LogoutAdminHandler(sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sm != nil {
			_ = sm.Destroy(r.Context())
		}
		http.Redirect(w, r, "/login-admin", http.StatusFound)
	}
}

// LogoutGuruHandler menghapus sesi aktif guru dan mengarahkan kembali ke halaman login.
func LogoutGuruHandler(sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sm != nil {
			_ = sm.Destroy(r.Context())
		}
		http.Redirect(w, r, "/login-guru", http.StatusFound)
	}
}

func renderAdminLoginError(w http.ResponseWriter, errMsg string) {
	w.WriteHeader(http.StatusOK)
	_ = web.Render(w, "login-admin.gohtml", map[string]any{
		"Error": errMsg,
	})
}

func renderGuruLoginError(w http.ResponseWriter, errMsg string) {
	w.WriteHeader(http.StatusOK)
	_ = web.Render(w, "login-guru.gohtml", map[string]any{
		"Error": errMsg,
	})
}
