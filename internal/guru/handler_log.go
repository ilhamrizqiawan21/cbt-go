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

// RegisterGuruLogRoutes mendaftarkan rute log pelanggaran siswa dan ekspor spreadsheet untuk guru.
func RegisterGuruLogRoutes(mux *http.ServeMux, db *sql.DB, sm *scs.SessionManager, requireGuru func(http.Handler) http.Handler) {
	mux.Handle("GET /guru/log-kecurangan", requireGuru(GuruLogListHandler(db, sm)))
	mux.Handle("GET /guru/log-kecurangan/export", requireGuru(GuruLogExportHandler(db, sm)))
}

// GuruLogListHandler menampilkan log pelanggaran siswa pada ujian yang diampu oleh guru aktif.
func GuruLogListHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
		ujianID, _ := strconv.Atoi(r.URL.Query().Get("ujian_id"))
		jenisFilter := r.URL.Query().Get("jenis")

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
		}

		list, err := admin.GetLogKecuranganByGuru(r.Context(), db, guruID, ujianID, jenisFilter)
		if err != nil {
			serverError(w, r, err)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, list)
			return
		}

		ujianList, err := soal.ListUjianByGuru(r.Context(), db, guruID)
		if err != nil {
			serverError(w, r, err)
			return
		}
		data := dataHalaman(sm, r, "log")
		data["LogList"] = list
		data["UjianList"] = ujianList
		data["UjianID"] = ujianID
		data["Filter"] = jenisFilter
		render(w, r, "guru_log.gohtml", data)
	}
}

// GuruLogExportHandler mengekspor data log pelanggaran ke berkas Excel (.xlsx).
func GuruLogExportHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
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

		u, err := admin.GetUjianByID(r.Context(), db, ujianID)
		if err != nil {
			http.Error(w, "Ujian tidak ditemukan", http.StatusNotFound)
			return
		}

		list, err := admin.GetLogKecuranganByGuru(r.Context(), db, guruID, ujianID, "")
		if err != nil {
			serverError(w, r, err)
			return
		}

		items := admin.ConvertToExcelLogItems(list)
		buf, err := excel.BuatEksporLogKecurangan(u.NamaUjian, items)
		if err != nil {
			serverError(w, r, fmt.Errorf("buat excel log guru: %w", err))
			return
		}

		filename := fmt.Sprintf("log_pelanggaran_ujian_%d.xlsx", ujianID)
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}
}
