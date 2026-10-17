package httpserver

import (
	"log/slog"
	"net/http"
	"time"
)

// statusRecorder mencatat status code yang ditulis handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap membuat http.ResponseController (Flush, Hijack, dsb.) tetap bisa
// menjangkau writer asli. Dibutuhkan nanti oleh WebSocket.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// accessLog mencatat satu baris log per request.
func accessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			logger.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"dur_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

// recoverer mengubah panic di handler menjadi respons 500 agar satu request
// yang bermasalah tidak mematikan seluruh server.
func recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				// http.ErrAbortHandler sengaja di-panic oleh net/http; jangan ditelan.
				if rec := recover(); rec != nil && rec != http.ErrAbortHandler {
					logger.Error("panic di handler", "panic", rec, "path", r.URL.Path)
					http.Error(w, "terjadi kesalahan pada server", http.StatusInternalServerError)
				} else if rec == http.ErrAbortHandler {
					panic(rec)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// securityHeaders menyematkan header keamanan standar pada setiap respons HTTP.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com data:; img-src 'self' data:; connect-src 'self' ws: wss:;")
		next.ServeHTTP(w, r)
	})
}
