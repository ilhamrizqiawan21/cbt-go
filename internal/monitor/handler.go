package monitor

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// GetMonitorHandler menangani permintaan HTTP GET /admin/api/monitor/{ujianId}.
func GetMonitorHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			http.Error(w, `{"error":"Layanan monitor tidak tersedia"}`, http.StatusInternalServerError)
			return
		}

		ujianIDStr := r.PathValue("ujianId")
		ujianID, err := strconv.Atoi(strings.TrimSpace(ujianIDStr))
		if err != nil || ujianID <= 0 {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": "ID ujian tidak valid",
				"code":  http.StatusBadRequest,
			})
			return
		}

		snap, err := svc.Snapshot(r.Context(), ujianID)
		if err != nil {
			// Detail error hanya ke log server; klien menerima pesan generik.
			code, msg := http.StatusNotFound, "Ujian tidak ditemukan"
			if !errors.Is(err, ErrUjianNotFound) {
				slog.ErrorContext(r.Context(), "snapshot monitor gagal", "ujian_id", ujianID, "err", err)
				code, msg = http.StatusInternalServerError, "Gagal memuat data monitor"
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": msg,
				"code":  code,
			})
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(snap)
	}
}
