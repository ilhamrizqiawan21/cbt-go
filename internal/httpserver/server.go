// Package httpserver merakit router HTTP dan middleware dasar.
package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexedwards/scs/mysqlstore"
	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/admin"
	"cbt-go/internal/auth"
	"cbt-go/internal/config"
	"cbt-go/internal/guru"
	"cbt-go/internal/monitor"
	"cbt-go/internal/ratelimit"
	"cbt-go/internal/realtime"
)

const healthTimeout = 2 * time.Second

// Pinger dipenuhi oleh *sql.DB. Interface kecil ini ada supaya handler
// healthz bisa diuji tanpa database sungguhan.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// Deps adalah dependensi yang dibutuhkan router.
type Deps struct {
	DB               Pinger
	Logger           *slog.Logger
	Config           config.Config
	SessionManager   *scs.SessionManager
	RateLimiterNIS   *ratelimit.KeyLimiter
	RateLimiterIP    *ratelimit.KeyLimiter
	RateLimiterAdmin *ratelimit.KeyLimiter
	RealtimeHub      *realtime.Hub
	MonitorService   *monitor.Service
	CustomHandler    http.Handler // opsional, untuk rute kustom atau pengujian
}

// NewSessionManager membangun scs.SessionManager dengan backend MySQL (tabel sessions).
func NewSessionManager(db *sql.DB, cfg config.Config) *scs.SessionManager {
	sm := scs.New()
	if db != nil {
		sm.Store = mysqlstore.New(db)
	}
	sm.Lifetime = 8 * time.Hour
	sm.Cookie.Name = "cbt_session"
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = cfg.IsProduction() || cfg.TrustProxy
	return sm
}

// New membangun http.Handler utama: router + middleware.
func New(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz(d.DB, d.Logger))
	registerPublicRoutes(mux, d.DB, d.SessionManager)
	registerAuthRoutes(mux, d)
	registerAPISiswaRoutes(mux, d)

	sqlDB, _ := d.DB.(*sql.DB)
	monitorSvc := d.MonitorService
	if monitorSvc == nil && sqlDB != nil {
		monitorSvc = monitor.NewService(sqlDB, d.RealtimeHub)
	}

	if monitorSvc != nil {
		requireAdminAPI := auth.RequireAdminAPI(d.SessionManager)
		mux.Handle("GET /admin/api/monitor/{ujianId}", requireAdminAPI(monitor.GetMonitorHandler(monitorSvc)))
	}

	if sqlDB != nil {
		admin.RegisterAdminMasterRoutes(mux, sqlDB, d.SessionManager, auth.RequireAdmin(d.SessionManager))
		admin.RegisterAdminSiswaRoutes(mux, sqlDB, d.SessionManager, auth.RequireAdmin(d.SessionManager))
		admin.RegisterAdminUjianRoutes(mux, sqlDB, d.SessionManager, auth.RequireAdmin(d.SessionManager), auth.RequireAdminAPI(d.SessionManager))
		admin.RegisterAdminSoalRoutes(mux, sqlDB, d.SessionManager, auth.RequireAdmin(d.SessionManager), auth.RequireAdminAPI(d.SessionManager))
		admin.RegisterAdminHasilRoutes(mux, sqlDB, d.SessionManager, auth.RequireAdmin(d.SessionManager))
		admin.RegisterAdminLogResetRoutes(mux, sqlDB, d.SessionManager, auth.RequireAdmin(d.SessionManager))
		admin.RegisterAdminMonitorRoutes(mux, sqlDB, d.SessionManager, monitorSvc, d.RealtimeHub, auth.RequireAdmin(d.SessionManager), auth.RequireAdminAPI(d.SessionManager))
		mux.Handle("GET /admin/dashboard", auth.RequireAdmin(d.SessionManager)(admin.DashboardHandler(sqlDB, d.SessionManager)))
		guru.RegisterGuruRoutes(mux, sqlDB, d.SessionManager, auth.RequireGuru(d.SessionManager), auth.RequireGuruAPI(d.SessionManager))
		guru.RegisterGuruHasilRoutes(mux, sqlDB, d.SessionManager, auth.RequireGuru(d.SessionManager))
		guru.RegisterGuruEssayRoutes(mux, sqlDB, d.SessionManager, auth.RequireGuru(d.SessionManager))
		guru.RegisterGuruLogRoutes(mux, sqlDB, d.SessionManager, auth.RequireGuru(d.SessionManager))
		mux.Handle("GET /guru/dashboard", auth.RequireGuru(d.SessionManager)(guru.GuruDashboardHandler(sqlDB, d.SessionManager)))
	}

	if d.RealtimeHub != nil {
		var onNotify realtime.OnMonitorNotifyFunc
		if monitorSvc != nil {
			onNotify = func(ujianID int, immediateEvent string, immediateData any) {
				_ = monitorSvc.SiarkanUpdate(context.Background(), ujianID)
			}
		}

		if sqlDB != nil {
			realtime.RegisterSiswaEvents(d.RealtimeHub, sqlDB, onNotify)
		}
		mux.Handle("GET /ws", d.RealtimeHub.ServeSiswaWS())
		mux.Handle("GET /ws/admin", d.RealtimeHub.ServeAdminWS())
	}

	var baseHandler http.Handler = mux
	if d.CustomHandler != nil {
		baseHandler = d.CustomHandler
	}

	var h http.Handler = baseHandler

	if d.SessionManager != nil {
		h = d.SessionManager.LoadAndSave(h)
	}

	// Proteksi CSRF: memvalidasi Sec-Fetch-Site dan Origin header (stdlib Go >= 1.25)
	cop := http.NewCrossOriginProtection()
	h = cop.Handler(h)

	// Header keamanan HTTP standar
	h = securityHeaders(h)

	// Logging & Panic Recovery
	if d.Logger != nil {
		h = recoverer(d.Logger)(h)
		h = accessLog(d.Logger)(h)
	}

	return h
}

func healthz(db Pinger, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthTimeout)
		defer cancel()

		status, code := "ok", http.StatusOK
		if db != nil {
			if err := db.PingContext(ctx); err != nil {
				// Detail error hanya ke log server, tidak dibocorkan ke klien.
				if logger != nil {
					logger.Error("healthz: database tidak merespons", "err", err)
				}
				status, code = "degraded", http.StatusServiceUnavailable
			}
		}
		writeJSON(w, code, map[string]string{"status": status})
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v) // klien sudah putus bila gagal; tidak ada yang bisa dilakukan
}
