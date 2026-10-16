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

	// POST /login-admin dengan proteksi rate limit per IP+username
	var loginAdminH http.Handler = auth.LoginAdminHandler(sqlDB, d.SessionManager, d.Logger)
	if d.RateLimiterAdmin != nil {
		loginAdminH = d.RateLimiterAdmin.Middleware(func(r *http.Request) string {
			ip := ratelimit.ClientIP(r, d.Config.TrustProxy)
			username := strings.TrimSpace(r.FormValue("username"))
			return ip + ":" + username
		}, "Terlalu banyak percobaan login admin. Silakan tunggu 15 menit.", "login-admin.gohtml")(loginAdminH)
	}
	mux.Handle("POST /login-admin", loginAdminH)

	// POST /login-guru dengan proteksi rate limit per IP+username
	var loginGuruH http.Handler = auth.LoginGuruHandler(sqlDB, d.SessionManager, d.Logger)
	if d.RateLimiterAdmin != nil {
		loginGuruH = d.RateLimiterAdmin.Middleware(func(r *http.Request) string {
			ip := ratelimit.ClientIP(r, d.Config.TrustProxy)
			username := strings.TrimSpace(r.FormValue("username"))
			return ip + ":" + username
		}, "Terlalu banyak percobaan login guru. Silakan tunggu 15 menit.", "login-guru.gohtml")(loginGuruH)
	}
	mux.Handle("POST /login-guru", loginGuruH)

	// GET /ujian dilindungi RequireSiswa
	mux.Handle("GET /ujian", auth.RequireSiswa(d.SessionManager)(auth.UjianHandler(sqlDB, d.SessionManager)))

	// Logout siswa, admin, dan guru
	mux.Handle("GET /logout", auth.LogoutHandler(d.SessionManager))
	mux.Handle("GET /logout-admin", auth.LogoutAdminHandler(d.SessionManager))
	mux.Handle("GET /admin/logout", auth.LogoutAdminHandler(d.SessionManager))
	mux.Handle("GET /logout-guru", auth.LogoutGuruHandler(d.SessionManager))
	mux.Handle("GET /guru/logout", auth.LogoutGuruHandler(d.SessionManager))
}
