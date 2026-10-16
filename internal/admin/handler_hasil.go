package admin

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/excel"
	"cbt-go/web"
)

// RegisterAdminHasilRoutes mendaftarkan seluruh rute laporan hasil ujian, ekspor Excel, dan cetak untuk admin.
func RegisterAdminHasilRoutes(mux *http.ServeMux, db *sql.DB, sm *scs.SessionManager, requireAdmin func(http.Handler) http.Handler) {
	mux.Handle("GET /admin/hasil", requireAdmin(HasilListHandler(db, sm)))
	mux.Handle("GET /admin/hasil/export", requireAdmin(HasilExportHandler(db)))
	mux.Handle("GET /admin/hasil/cetak/{ujianId}", requireAdmin(HasilCetakHandler(db)))
}

// HasilListHandler menampilkan daftar hasil ujian siswa atau mengembalikan data JSON.
func HasilListHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ujianStr := r.URL.Query().Get("ujian_id")
		ujianID, _ := strconv.Atoi(ujianStr)
		kelasFilter := r.URL.Query().Get("kelas")

		var list []HasilUjianRow
		var err error
		if ujianID > 0 {
			list, err = GetHasilByUjian(r.Context(), db, ujianID, kelasFilter)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, list)
			return
		}

		ujianList, _ := ListUjian(r.Context(), db)
		flashError := getFlash(sm, r, "error")
		flashSuccess := getFlash(sm, r, "success")

		_ = web.Render(w, "admin_hasil.gohtml", map[string]any{
			"HasilList": list,
			"UjianList": ujianList,
			"UjianID":   ujianID,
			"Filter":    kelasFilter,
			"Error":     flashError,
			"Success":   flashSuccess,
			"Username":  sm.GetString(r.Context(), "username"),
		})
	}
}

// HasilExportHandler menghasilkan berkas Excel (.xlsx) dari rekap nilai suatu ujian.
func HasilExportHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ujianStr := r.URL.Query().Get("ujian_id")
		ujianID, err := strconv.Atoi(ujianStr)
		if err != nil || ujianID <= 0 {
			http.Error(w, "Parameter ujian_id wajib diisi", http.StatusBadRequest)
			return
		}

		list, err := GetHasilByUjian(r.Context(), db, ujianID, "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		u, err := GetUjianByID(r.Context(), db, ujianID)
		if err != nil {
			http.Error(w, "Ujian tidak ditemukan", http.StatusNotFound)
			return
		}

		items := ConvertToExcelItems(list)
		buf, err := excel.BuatEksporHasilUjian(u.NamaUjian, u.NamaMapel, items)
		if err != nil {
			http.Error(w, "Gagal membuat berkas Excel: "+err.Error(), http.StatusInternalServerError)
			return
		}

		filename := fmt.Sprintf("hasil_ujian_%d.xlsx", ujianID)
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}
}

// HasilCetakHandler menampilkan halaman cetak nilai ujian siap cetak (HTML print-friendly).
func HasilCetakHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ujianStr := r.PathValue("ujianId")
		ujianID, err := strconv.Atoi(ujianStr)
		if err != nil || ujianID <= 0 {
			http.Error(w, "ID ujian tidak valid", http.StatusBadRequest)
			return
		}

		u, err := GetUjianByID(r.Context(), db, ujianID)
		if err != nil {
			http.Error(w, "Ujian tidak ditemukan", http.StatusNotFound)
			return
		}

		list, err := GetHasilByUjian(r.Context(), db, ujianID, "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_ = web.Render(w, "admin_hasil_cetak.gohtml", map[string]any{
			"Ujian":        u,
			"HasilList":    list,
			"TanggalCetak": time.Now().Format("02 January 2006"),
		})
	}
}
