// Package admin mengelola fitur dan data operasional untuk peran administrator.
package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/monitor"
	"cbt-go/internal/sesi"
)

// RealtimeNotifier adalah interface minimal untuk mengirim event langsung ke siswa.
type RealtimeNotifier interface {
	SendToSiswa(siswaID, ujianID int, event string, data any) bool
}

// MonitorUpdater adalah interface minimal untuk snapshot dan penyiaran pembaruan monitor.
type MonitorUpdater interface {
	Snapshot(ctx context.Context, ujianID int) (*monitor.SnapshotData, error)
	SiarkanUpdate(ctx context.Context, ujianID int) error
}

// RegisterAdminMonitorRoutes mendaftarkan rute halaman pemantauan dan endpoint aksi sesi siswa oleh admin.
func RegisterAdminMonitorRoutes(
	mux *http.ServeMux,
	db *sql.DB,
	sm *scs.SessionManager,
	monitorSvc MonitorUpdater,
	notifier RealtimeNotifier,
	requireAdmin func(http.Handler) http.Handler,
	requireAdminAPI func(http.Handler) http.Handler,
) {
	// Halaman UI Monitor
	mux.Handle("GET /admin/monitor", requireAdmin(AdminMonitorPageHandler(db, sm, monitorSvc)))
	mux.Handle("GET /admin/monitor/{ujianId}", requireAdmin(AdminMonitorPageHandler(db, sm, monitorSvc)))

	// API Aksi Sesi Siswa (Buka Kunci, Tambah Waktu, Paksa Selesai)
	mux.Handle("POST /admin/api/sesi/{id}/buka-kunci", requireAdminAPI(AdminBukaKunciHandler(db, sm, monitorSvc, notifier)))
	mux.Handle("POST /admin/api/sesi/{id}/tambah-waktu", requireAdminAPI(AdminTambahWaktuHandler(db, sm, monitorSvc, notifier)))
	mux.Handle("POST /admin/api/sesi/{id}/paksa-selesai", requireAdminAPI(AdminPaksaSelesaiHandler(db, sm, monitorSvc, notifier)))
}

// AdminMonitorPageHandler menampilkan halaman monitor pemantauan realtime ujian.
func AdminMonitorPageHandler(db *sql.DB, sm *scs.SessionManager, monitorSvc MonitorUpdater) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ujianList, err := ListUjian(r.Context(), db)
		if err != nil {
			serverError(w, r, err)
			return
		}

		ujianIDStr := r.PathValue("ujianId")
		if ujianIDStr == "" {
			ujianIDStr = r.URL.Query().Get("ujian_id")
		}

		var selectedUjianID int
		if ujianIDStr != "" {
			selectedUjianID, _ = strconv.Atoi(ujianIDStr)
		} else if len(ujianList) > 0 {
			// Default ke ujian pertama jika ada
			selectedUjianID = ujianList[0].ID
		}

		var snap *monitor.SnapshotData
		if selectedUjianID > 0 && monitorSvc != nil {
			snap, err = monitorSvc.Snapshot(r.Context(), selectedUjianID)
			if err != nil && !errors.Is(err, monitor.ErrUjianNotFound) {
				// Halaman tetap tampil; klien akan mencoba lagi lewat WebSocket/API.
				slog.ErrorContext(r.Context(), "snapshot awal monitor gagal", "ujian_id", selectedUjianID, "err", err)
			}
		}

		render(w, r, "admin_monitor.gohtml", map[string]any{
			"UjianList":  ujianList,
			"UjianID":    selectedUjianID,
			"Snapshot":   snap,
			"ActiveMenu": "monitor",
			"Username":   sm.GetString(r.Context(), "username"),
			"Error":      getFlash(sm, r, "error"),
			"Success":    getFlash(sm, r, "success"),
		})
	}
}

// sesiIDDariPath membaca {id} dari path; menulis 400 JSON bila tidak valid.
func sesiIDDariPath(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(strings.TrimSpace(r.PathValue("id")))
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ID sesi tidak valid"})
		return 0, false
	}
	return id, true
}

// tulisErrorAksi memetakan error bisnis aksi sesi ke status HTTP; error lain menjadi 500 generik.
func tulisErrorAksi(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrSesiTidakDitemukan), errors.Is(err, sesi.ErrSesiNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "Sesi ujian tidak ditemukan"})
	case errors.Is(err, ErrSesiTidakTerkunci):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Sesi tidak sedang terkunci"})
	case errors.Is(err, ErrSesiSudahSelesai):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Tidak dapat menambah waktu untuk sesi yang sudah selesai"})
	default:
		serverError(w, r, err)
	}
}

// siarkanMonitor memperbarui dashboard admin lain; kegagalan hanya dicatat karena aksi sudah tersimpan.
func siarkanMonitor(r *http.Request, monitorSvc MonitorUpdater, ujianID int) {
	if monitorSvc == nil {
		return
	}
	if err := monitorSvc.SiarkanUpdate(r.Context(), ujianID); err != nil {
		slog.ErrorContext(r.Context(), "siarkan monitor setelah aksi admin gagal", "ujian_id", ujianID, "err", err)
	}
}

// AdminBukaKunciHandler membuka status terkunci (keluar_paksa) pada sesi siswa tanpa menghapus jawaban.
func AdminBukaKunciHandler(db *sql.DB, sm *scs.SessionManager, monitorSvc MonitorUpdater, notifier RealtimeNotifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sesiID, ok := sesiIDDariPath(w, r)
		if !ok {
			return
		}

		s, err := BukaKunciSesi(r.Context(), db, sm.GetInt(r.Context(), "adminId"), sesiID)
		if err != nil {
			tulisErrorAksi(w, r, err)
			return
		}

		siarkanMonitor(r, monitorSvc, s.UjianID)
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"message": "Kunci sesi berhasil dibuka",
		})
	}
}

// TambahWaktuPayload adalah data JSON untuk tambah waktu.
type TambahWaktuPayload struct {
	Menit int `json:"menit"`
}

// AdminTambahWaktuHandler menambahkan batas waktu pengerjaan ujian bagi siswa terkait.
func AdminTambahWaktuHandler(db *sql.DB, sm *scs.SessionManager, monitorSvc MonitorUpdater, notifier RealtimeNotifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sesiID, ok := sesiIDDariPath(w, r)
		if !ok {
			return
		}

		// Tanpa nilai dari klien dipakai 10 menit (default tombol di halaman monitor).
		menit := 10
		if isJSONBody(r) {
			var payload TambahWaktuPayload
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&payload); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "JSON body tidak valid"})
				return
			}
			if payload.Menit != 0 {
				menit = payload.Menit
			}
		} else if v := strings.TrimSpace(r.FormValue("menit")); v != "" {
			m, err := strconv.Atoi(v)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Jumlah menit tidak valid"})
				return
			}
			menit = m
		}
		if menit < MinTambahMenit || menit > MaxTambahMenit {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": fmt.Sprintf("Jumlah menit harus %d–%d", MinTambahMenit, MaxTambahMenit),
			})
			return
		}

		s, err := TambahWaktuSesi(r.Context(), db, sm.GetInt(r.Context(), "adminId"), sesiID, menit)
		if err != nil {
			tulisErrorAksi(w, r, err)
			return
		}

		batas := s.BatasWaktu.Format(time.RFC3339)
		// Klien ujian (ujian.js) menampilkan `pesan` lalu resinkron timer dari server.
		if notifier != nil {
			notifier.SendToSiswa(s.SiswaID, s.UjianID, "tambah-waktu", map[string]any{
				"menit":       menit,
				"batas_waktu": batas,
				"pesan":       fmt.Sprintf("⏰ Pengawas menambah waktu %d menit.", menit),
			})
		}
		siarkanMonitor(r, monitorSvc, s.UjianID)

		writeJSON(w, http.StatusOK, map[string]any{
			"success":     true,
			"message":     fmt.Sprintf("Waktu berhasil ditambahkan %d menit", menit),
			"batas_waktu": batas,
		})
	}
}

// AdminPaksaSelesaiHandler menyelesaikan sesi siswa secara paksa dan menghitung skor akhir.
func AdminPaksaSelesaiHandler(db *sql.DB, sm *scs.SessionManager, monitorSvc MonitorUpdater, notifier RealtimeNotifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sesiID, ok := sesiIDDariPath(w, r)
		if !ok {
			return
		}

		s, err := AmbilSesi(r.Context(), db, sesiID)
		if err != nil {
			tulisErrorAksi(w, r, err)
			return
		}

		// Finalize idempoten dan memakai transaksinya sendiri (DESIGN §3.4).
		hasil, err := sesi.Finalize(r.Context(), db, s.SiswaID, s.UjianID)
		if err != nil {
			tulisErrorAksi(w, r, err)
			return
		}
		if err := CatatAuditPaksaSelesai(r.Context(), db, sm.GetInt(r.Context(), "adminId"), sesiID); err != nil {
			// Nilai sudah final; audit yang gagal dicatat di log agar bisa ditelusuri.
			slog.ErrorContext(r.Context(), "audit paksa-selesai gagal", "sesi_id", sesiID, "err", err)
		}

		// ujian.js menangani 'paksa-submit' dengan mengirim jawaban terakhir lalu keluar.
		if notifier != nil {
			notifier.SendToSiswa(s.SiswaID, s.UjianID, "paksa-submit", map[string]any{
				"message": "Ujian Anda telah diselesaikan oleh pengawas.",
			})
		}
		siarkanMonitor(r, monitorSvc, s.UjianID)

		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"message": "Sesi berhasil diselesaikan",
			"nilai":   hasil.Nilai,
		})
	}
}
