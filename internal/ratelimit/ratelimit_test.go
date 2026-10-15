package ratelimit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cbt-go/internal/ratelimit"
)

func TestRateLimiterPerKey(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 10 request per 15 menit
	limiter := ratelimit.New(ctx, 10, 15*time.Minute, 30*time.Minute)

	nisA := "1001"
	nisB := "1002"

	// 10 request pertama untuk nisA harus lolos
	for i := 1; i <= 10; i++ {
		if !limiter.Allow(nisA) {
			t.Fatalf("request ke-%d untuk NIS %s harus lolos", i, nisA)
		}
	}

	// Request ke-11 untuk nisA harus ditolak (429)
	if limiter.Allow(nisA) {
		t.Fatalf("request ke-11 untuk NIS %s seharusnya ditolak", nisA)
	}

	// NIS lain (nisB) tetap harus lolos
	if !limiter.Allow(nisB) {
		t.Fatalf("request pertama untuk NIS %s harus lolos meski nisA terblokir", nisB)
	}
}

func TestRateLimiterCleanupAndNoLeak(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Limiter dengan TTL sangat singkat untuk pengujian pembersihan (10ms)
	// dan interval pembersih 20ms
	limiter := ratelimit.NewWithInterval(ctx, 5, 1*time.Minute, 20*time.Millisecond, 10*time.Millisecond)

	limiter.Allow("kunci-1")
	limiter.Allow("kunci-2")

	if count := limiter.Count(); count != 2 {
		t.Fatalf("jumlah entri harus 2, dapat %d", count)
	}

	// Tunggu pembersih berjalan setelah TTL lewat
	time.Sleep(50 * time.Millisecond)

	if count := limiter.Count(); count != 0 {
		t.Fatalf("entri kedaluwarsa harus dibersihkan, sisa: %d", count)
	}

	// Batalkan context untuk menghentikan goroutine pembersih
	cancel()
	time.Sleep(30 * time.Millisecond)

	// Pastikan goroutine selesai rapi tanpa panic
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		trustProxy bool
		wantIP     string
	}{
		{
			name:       "remote addr standard dengan port",
			remoteAddr: "192.168.1.50:54321",
			xff:        "",
			trustProxy: false,
			wantIP:     "192.168.1.50",
		},
		{
			name:       "remote addr tanpa port",
			remoteAddr: "192.168.1.50",
			xff:        "",
			trustProxy: false,
			wantIP:     "192.168.1.50",
		},
		{
			name:       "XFF diabaikan bila trustProxy false",
			remoteAddr: "10.0.0.1:1234",
			xff:        "203.0.113.195",
			trustProxy: false,
			wantIP:     "10.0.0.1",
		},
		{
			name:       "XFF dihormati bila trustProxy true",
			remoteAddr: "10.0.0.1:1234",
			xff:        "203.0.113.195, 10.0.0.1",
			trustProxy: true,
			wantIP:     "203.0.113.195",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}

			got := ratelimit.ClientIP(req, tc.trustProxy)
			if got != tc.wantIP {
				t.Errorf("ClientIP = %q; want %q", got, tc.wantIP)
			}
		})
	}
}

func TestMiddlewareRateLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	limiter := ratelimit.New(ctx, 2, 1*time.Minute, 5*time.Minute)
	pesan := "Terlalu banyak percobaan login. Silakan coba lagi setelah 15 menit."

	handler := limiter.Middleware(
		func(r *http.Request) string { return r.Header.Get("X-User-Key") },
		pesan,
		"login.gohtml",
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	// Request 1: lolos
	req1 := httptest.NewRequest(http.MethodPost, "/login", nil)
	req1.Header.Set("X-User-Key", "user1")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("request 1 harus 200, dapat %d", rec1.Code)
	}

	// Request 2: lolos
	req2 := httptest.NewRequest(http.MethodPost, "/login", nil)
	req2.Header.Set("X-User-Key", "user1")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("request 2 harus 200, dapat %d", rec2.Code)
	}

	// Request 3 (API/JSON): ditolak 429
	req3 := httptest.NewRequest(http.MethodPost, "/login", nil)
	req3.Header.Set("X-User-Key", "user1")
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("request 3 harus 429, dapat %d", rec3.Code)
	}
	if rec3.Header().Get("Retry-After") == "" {
		t.Errorf("respons 429 harus memuat header Retry-After")
	}
	if !strings.Contains(rec3.Body.String(), pesan) {
		t.Errorf("respons 429 harus memuat pesan error dalam Bahasa Indonesia")
	}
}
