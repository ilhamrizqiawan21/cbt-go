package httpserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakePinger struct{ err error }

func (f fakePinger) PingContext(context.Context) error { return f.err }

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHealthz(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
		wantBody   string
	}{
		{"database sehat", nil, http.StatusOK, `"status":"ok"`},
		{"database mati", errors.New("connection refused"), http.StatusServiceUnavailable, `"status":"degraded"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := New(Deps{DB: fakePinger{err: tt.pingErr}, Logger: discardLogger()})
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, ingin %d", rr.Code, tt.wantStatus)
			}
			if !strings.Contains(rr.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, harus memuat %q", rr.Body.String(), tt.wantBody)
			}
			if strings.Contains(rr.Body.String(), "connection refused") {
				t.Error("detail error internal tidak boleh bocor ke klien")
			}
		})
	}
}

func TestHealthz_MethodNotAllowed(t *testing.T) {
	h := New(Deps{DB: fakePinger{}, Logger: discardLogger()})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, ingin 405", rr.Code)
	}
}

func TestRecoverer_PanicBecomes500(t *testing.T) {
	panicky := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	h := recoverer(discardLogger())(panicky)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil)) // tidak boleh ikut panic

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, ingin 500", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "boom") {
		t.Error("pesan panic tidak boleh bocor ke klien")
	}
}
