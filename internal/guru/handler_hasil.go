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
		ujianID, _ := strconv.Atoi(r.URL.Query().Get("ujian_id"))
		kelasFilter := r.URL.Query().Get("kelas")

		listUjianGuru, err := soal.ListUjianByGuru(r.Context(), db, guruID)
		if err != nil {
			serverError(w, r, err)
			return
		}
		// Halaman HTML langsung menampilkan ujian terbaru bila belum dipilih.
		if ujianID <= 0 && !wantsJSON(r) && len(listUjianGuru) > 0 {
			ujianID = listUjianGuru[0].ID
		}

		var list []admin.HasilUjianRow
		if ujianID > 0 {
			boleh, err := soal.CekOtorisasiUjianGuru(r.Context(), db, guruID, ujianID)
			if err != nil {
				serverError(w, r, err)
				return
			}
			if !boleh {
				forbidden(w, r)
				return
			}
			list, err = admin.GetHasilByUjian(r.Context(), db, ujianID, kelasFilter)
			if err != nil {
				serverError(w, r, err)
				return
			}
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, list)
			return
		}

		data := dataHalaman(sm, r, "hasil")
		data["HasilList"] = list
		data["UjianList"] = listUjianGuru
		data["UjianID"] = ujianID
		data["Filter"] = kelasFilter
		render(w, r, "guru_hasil.gohtml", data)
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

		boleh, err := soal.CekOtorisasiUjianGuru(r.Context(), db, guruID, ujianID)
		if err != nil {
			serverError(w, r, err)
			return
		}
		if !boleh {
			forbidden(w, r)
			return
		}

		list, err := admin.GetHasilByUjian(r.Context(), db, ujianID, "")
		if err != nil {
			serverError(w, r, err)
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
			serverError(w, r, fmt.Errorf("buat excel hasil guru: %w", err))
			return
		}

		filename := fmt.Sprintf("hasil_siswa_guru_%d.xlsx", ujianID)
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}
}
