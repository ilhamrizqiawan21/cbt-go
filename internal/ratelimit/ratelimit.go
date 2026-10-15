// Package ratelimit menyediakan pembatas laju permintaan HTTP berbasis peta kunci (NIS, IP).
package ratelimit

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"cbt-go/web"
)

type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// KeyLimiter mengatur rate limiter independen untuk setiap kunci yang berbeda (mis. per NIS atau per IP).
type KeyLimiter struct {
	mu      sync.Mutex
	entries map[string]*entry
	limit   rate.Limit
	burst   int
	ttl     time.Duration
}

// New membuat KeyLimiter baru dengan pembersihan berkala default (1 menit).
func New(ctx context.Context, burst int, window time.Duration, ttl time.Duration) *KeyLimiter {
	return NewWithInterval(ctx, burst, window, 1*time.Minute, ttl)
}

// NewWithInterval membuat KeyLimiter dengan interval pembersih kustom (berguna untuk pengujian).
func NewWithInterval(ctx context.Context, burst int, window time.Duration, cleanupInterval time.Duration, ttl time.Duration) *KeyLimiter {
	if burst < 1 {
		burst = 1
	}
	limit := rate.Every(window / time.Duration(burst))

	kl := &KeyLimiter{
		entries: make(map[string]*entry),
		limit:   limit,
		burst:   burst,
		ttl:     ttl,
	}

	go func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				kl.cleanup()
			}
		}
	}()

	return kl
}

func (kl *KeyLimiter) cleanup() {
	kl.mu.Lock()
	defer kl.mu.Unlock()

	now := time.Now()
	for k, e := range kl.entries {
		if now.Sub(e.lastSeen) > kl.ttl {
			delete(kl.entries, k)
		}
	}
}

// Allow memeriksa apakah request untuk kunci tertentu masih dalam batas yang diizinkan.
func (kl *KeyLimiter) Allow(key string) bool {
	if key == "" {
		return true
	}

	kl.mu.Lock()
	defer kl.mu.Unlock()

	now := time.Now()
	e, ok := kl.entries[key]
	if !ok {
		e = &entry{
			limiter:  rate.NewLimiter(kl.limit, kl.burst),
			lastSeen: now,
		}
		kl.entries[key] = e
	} else {
		e.lastSeen = now
	}

	return e.limiter.Allow()
}

// Count mengembalikan jumlah kunci aktif yang sedang dicatat.
func (kl *KeyLimiter) Count() int {
	kl.mu.Lock()
	defer kl.mu.Unlock()
	return len(kl.entries)
}

// ClientIP mengekstrak IP klien dari request secara aman.
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if idx := strings.Index(xff, ","); idx != -1 {
				xff = xff[:idx]
			}
			if ip := strings.TrimSpace(xff); ip != "" {
				return ip
			}
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

// Middleware menghasilkan middleware HTTP yang menolak request berlebih dengan 429 Too Many Requests.
func (kl *KeyLimiter) Middleware(keyFunc func(*http.Request) string, message string, templateName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			if !kl.Allow(key) {
				w.Header().Set("Retry-After", strconv.Itoa(int(kl.ttl.Seconds())))

				if strings.Contains(r.Header.Get("Accept"), "text/html") && templateName != "" {
					w.WriteHeader(http.StatusTooManyRequests)
					_ = web.Render(w, templateName, map[string]any{"Error": message})
					return
				}

				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
