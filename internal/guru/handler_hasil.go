package guru

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/admin"
	"cbt-go/internal/excel"
	"cbt-go/internal/soal"
	"cbt-go/web"
)

// RegisterGuruHasilRoutes mendaftarkan rute laporan hasil siswa untuk guru yang mengampu ujian.
func RegisterGuruHasilRoutes(mux *http.ServeMux, db *sql.DB, sm *scs.SessionManager, requireGuru func(http.Handler) http.Handler) {
	mux.Handle("GET /guru/hasil-siswa", requireGuru(GuruHasilListHandler(db, sm)))
	mux.Handle("GET /guru/hasil-siswa/export", requireGuru(GuruHasilExportHandler(db, sm)))
}

// GuruHasilListHandler menampilkan rekap nilai siswa pada ujian yang diampu oleh guru aktif.
func GuruHasilListHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
		ujianStr := r.URL.Query().Get("ujian_id")
		ujianID, _ := strconv.Atoi(ujianStr)
		kelasFilter := r.URL.Query().Get("kelas")

		var list []admin.HasilUjianRow
		var err error

		if ujianID > 0 {
			boleh, cErr := soal.CekOtorisasiUjianGuru(r.Context(), db, guruID, ujianID)
			if cErr != nil || !boleh {
				if wantsJSON(r) {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "Akses ditolak: Anda bukan pengampu ujian ini"})
					return
				}
				http.Error(w, "Akses ditolak: Anda bukan pengampu ujian ini", http.StatusForbidden)
				return
			}

			list, err = admin.GetHasilByUjian(r.Context(), db, ujianID, kelasFilter)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, list)
			return
		}

		listUjianGuru, _ := soal.ListUjianByGuru(r.Context(), db, guruID)
		flashError := getFlash(sm, r, "error")
		flashSuccess := getFlash(sm, r, "success")

		_ = web.Render(w, "guru_hasil.gohtml", map[string]any{
			"HasilList": list,
			"UjianList": listUjianGuru,
			"UjianID":   ujianID,
			"Filter":    kelasFilter,
			"Error":     flashError,
			"Success":   flashSuccess,
			"Username":  sm.GetString(r.Context(), "username"),
		})
	}
}

// GuruHasilExportHandler mengekspor data hasil siswa ke Excel (.xlsx) dengan otorisasi pengajaran guru.
func GuruHasilExportHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
		ujianStr := r.URL.Query().Get("ujian_id")
		ujianID, err := strconv.Atoi(ujianStr)
		if err != nil || ujianID <= 0 {
			http.Error(w, "Parameter ujian_id tidak valid", http.StatusBadRequest)
			return
		}

		boleh, cErr := soal.CekOtorisasiUjianGuru(r.Context(), db, guruID, ujianID)
		if cErr != nil || !boleh {
			if wantsJSON(r) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "Akses ditolak: Anda bukan pengampu ujian ini"})
				return
			}
			http.Error(w, "Akses ditolak: Anda bukan pengampu ujian ini", http.StatusForbidden)
			return
		}

		list, err := admin.GetHasilByUjian(r.Context(), db, ujianID, "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		u, err := admin.GetUjianByID(r.Context(), db, ujianID)
		if err != nil {
			http.Error(w, "Ujian tidak ditemukan", http.StatusNotFound)
			return
		}

		items := admin.ConvertToExcelItems(list)
		buf, err := excel.BuatEksporHasilUjian(u.NamaUjian, u.NamaMapel, items)
		if err != nil {
			http.Error(w, "Gagal membuat berkas Excel: "+err.Error(), http.StatusInternalServerError)
			return
		}

		filename := fmt.Sprintf("hasil_siswa_guru_%d.xlsx", ujianID)
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}
}
