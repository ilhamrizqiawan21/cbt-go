package monitor

import (
	"encoding/json"
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
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": err.Error(),
				"code":  http.StatusNotFound,
			})
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(snap)
	}
}
