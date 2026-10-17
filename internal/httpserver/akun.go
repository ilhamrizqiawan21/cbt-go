package httpserver

import (
	"database/sql"
	"net/http"

	"cbt-go/internal/analisis"
	"cbt-go/internal/auth"
)

// registerAkunRoutes mendaftarkan halaman dan aksi ganti password mandiri admin dan guru (G-027).
// Percobaan dibatasi memakai pembatas laju admin/guru yang sama dengan login.
func registerAkunRoutes(mux *http.ServeMux, db *sql.DB, d Deps) {
	var izin func(string) bool
	if d.RateLimiterAdmin != nil {
		izin = d.RateLimiterAdmin.Allow
	}
	admin := auth.AkunConfig{DB: db, SM: d.SessionManager, Peran: "admin", Izin: izin, Logger: d.Logger}
	guru := auth.AkunConfig{DB: db, SM: d.SessionManager, Peran: "guru", Izin: izin, Logger: d.Logger}

	requireAdmin, requireGuru := auth.RequireAdmin(d.SessionManager), auth.RequireGuru(d.SessionManager)
	mux.Handle("GET /admin/akun", requireAdmin(auth.AkunPageHandler(admin)))
	mux.Handle("POST /admin/akun/password", requireAdmin(auth.GantiPasswordHandler(admin)))
	mux.Handle("GET /guru/akun", requireGuru(auth.AkunPageHandler(guru)))
	mux.Handle("POST /guru/akun/password", requireGuru(auth.GantiPasswordHandler(guru)))

	// Analisis butir soal (G-028a)
	mux.Handle("GET /admin/analisis", requireAdmin(analisis.Handler{DB: db, SM: d.SessionManager, Peran: "admin", Daftar: analisis.DaftarSemua(db)}))
	mux.Handle("GET /guru/analisis", requireGuru(analisis.Handler{DB: db, SM: d.SessionManager, Peran: "guru", Daftar: analisis.DaftarGuru(db, d.SessionManager)}))
}
