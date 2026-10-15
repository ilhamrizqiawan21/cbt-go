package httpserver

import (
	"database/sql"
	"net/http"
	"strings"

	"cbt-go/internal/auth"
	"cbt-go/internal/ratelimit"
)

// registerAuthRoutes mendaftarkan rute autentikasi siswa, logout, dan halaman ujian.
func registerAuthRoutes(mux *http.ServeMux, d Deps) {
	sqlDB, _ := d.DB.(*sql.DB)

	// POST /login-siswa dengan proteksi rate limit per IP dan per NIS
	var loginH http.Handler = auth.LoginSiswaHandler(sqlDB, d.SessionManager, d.Config, d.Logger)

	if d.RateLimiterNIS != nil {
		loginH = d.RateLimiterNIS.Middleware(func(r *http.Request) string {
			return strings.TrimSpace(r.FormValue("nis"))
		}, "Terlalu banyak percobaan login untuk NIS ini. Silakan tunggu 15 menit.", "login.gohtml")(loginH)
	}

	if d.RateLimiterIP != nil {
		loginH = d.RateLimiterIP.Middleware(func(r *http.Request) string {
			return ratelimit.ClientIP(r, d.Config.TrustProxy)
		}, "Terlalu banyak percobaan login dari IP ini. Silakan tunggu 15 menit.", "login.gohtml")(loginH)
	}

	mux.Handle("POST /login-siswa", loginH)

	// GET /ujian dilindungi RequireSiswa
	mux.Handle("GET /ujian", auth.RequireSiswa(d.SessionManager)(auth.UjianHandler(sqlDB, d.SessionManager)))

	// GET /logout
	mux.Handle("GET /logout", auth.LogoutHandler(d.SessionManager))
}
