// Command server menjalankan aplikasi CBT Sekolah.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "time/tzdata" // sertakan database zona waktu agar tidak bergantung pada OS server

	"github.com/joho/godotenv"

	"cbt-go/internal/config"
	"cbt-go/internal/db"
	"cbt-go/internal/httpserver"
	"cbt-go/internal/ratelimit"
	"cbt-go/internal/realtime"
	"cbt-go/internal/sesi"
)

const shutdownTimeout = 15 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run dipisah dari main supaya error bisa dikembalikan (bukan os.Exit di tengah
// jalan) dan semua `defer` sempat berjalan.
func run() error {
	// .env bersifat opsional; di produksi variabel bisa datang dari systemd.
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("membaca .env: %w", err)
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)

	// ctx dibatalkan saat menerima Ctrl+C / SIGTERM (mis. systemctl stop).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DB, cfg.Timezone)
	if err != nil {
		return err
	}
	defer pool.Close()

	if beda, pesan, err := db.VerifyTimezone(ctx, pool, cfg.Timezone); err != nil {
		logger.Warn("gagal memeriksa kesesuaian zona waktu database", "err", err)
	} else if beda {
		logger.Warn(pesan)
	}

	sm := httpserver.NewSessionManager(pool, cfg)

	// Rate limiter: Siswa 10 req/15 mnt per NIS, 600 req/15 mnt per IP (atau RATE_LIMIT_LOGIN_IP)
	// agar mampu melayani 410-500 siswa serentak dari 1 IP NAT sekolah (paritas Node rateLimiter.js).
	rateLimitIP := 600
	if v, err := strconv.Atoi(os.Getenv("RATE_LIMIT_LOGIN_IP")); err == nil && v > 0 {
		rateLimitIP = v
	}
	limiterNIS := ratelimit.New(ctx, 10, 15*time.Minute, 15*time.Minute)
	limiterIP := ratelimit.New(ctx, rateLimitIP, 15*time.Minute, 15*time.Minute)

	// Auto-finalize background job: periksa dan tutup sesi kadaluarsa setiap 30 detik (T1.7, D-009)
	autoFinalizer := sesi.StartAutoFinalizeJob(ctx, pool, 30*time.Second, sesi.DefaultGraceDetik, logger)
	defer autoFinalizer.Stop()

	// Hub WebSocket untuk koneksi siswa (/ws) dan pemantauan admin (/ws/admin) (T2.1)
	wsHub := realtime.NewHub(sm, realtime.Config{})
	defer wsHub.Close()

	srv := &http.Server{
		Addr: net.JoinHostPort("", strconv.Itoa(cfg.Port)),
		Handler: httpserver.New(httpserver.Deps{
			DB:             pool,
			Logger:         logger,
			Config:         cfg,
			SessionManager: sm,
			RateLimiterNIS: limiterNIS,
			RateLimiterIP:  limiterIP,
			RealtimeHub:    wsHub,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("server berjalan", "addr", srv.Addr, "env", cfg.Env)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("server berhenti: %w", err)
	case <-ctx.Done():
		logger.Info("menerima sinyal berhenti, mematikan server dengan tertib")
	}

	// Beri request yang sedang berjalan waktu untuk selesai.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

func newLogger(cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}
