package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/alexedwards/scs/v2"
)

// RegisterAdminSusulanRoutes mendaftarkan rute izin ujian susulan per siswa (G-026).
func RegisterAdminSusulanRoutes(mux *http.ServeMux, db *sql.DB, sm *scs.SessionManager, requireAdmin func(http.Handler) http.Handler) {
	mux.Handle("GET /admin/susulan", requireAdmin(SusulanListHandler(db, sm)))
	mux.Handle("POST /admin/susulan/tambah", requireAdmin(SusulanTambahHandler(db, sm)))
	mux.Handle("POST /admin/susulan/hapus/{id}", requireAdmin(SusulanHapusHandler(db, sm)))
}

func susulanURL(ujianID int) string {
	if ujianID > 0 {
		return "/admin/susulan?ujian_id=" + strconv.Itoa(ujianID)
	}
	return "/admin/susulan"
}

// SusulanListHandler menampilkan siswa kelas ujian terpilih beserta izin susulannya.
func SusulanListHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ujianList, err := ListUjian(r.Context(), db)
		if err != nil {
			serverError(w, r, err)
			return
		}
		ujianID, _ := strconv.Atoi(r.URL.Query().Get("ujian_id"))
		if ujianID <= 0 && len(ujianList) > 0 {
			ujianID = ujianList[0].ID
		}
		var rows []SusulanRow
		var terpilih *Ujian
		for i := range ujianList {
			if ujianList[i].ID == ujianID {
				terpilih = &ujianList[i]
			}
		}
		if terpilih != nil {
			rows, err = ListSusulan(r.Context(), db, ujianID)
			if err != nil {
				serverError(w, r, err)
				return
			}
		}
		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, rows)
			return
		}
		render(w, r, "admin_susulan.gohtml", map[string]any{
			"UjianList":  ujianList,
			"UjianID":    ujianID,
			"Ujian":      terpilih,
			"Baris":      rows,
			"Error":      getFlash(sm, r, "error"),
			"Success":    getFlash(sm, r, "success"),
			"Username":   sm.GetString(r.Context(), "username"),
			"ActiveMenu": "ujian",
		})
	}
}

// SusulanTambahHandler memberi izin: ujian_id, siswa_id, [sampai], [alasan].
func SusulanTambahHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		ujianID, _ := strconv.Atoi(r.FormValue("ujian_id"))
		siswaID, _ := strconv.Atoi(r.FormValue("siswa_id"))

		selesai := func(code int, kunci, pesan string) {
			if wantsJSON(r) {
				if code >= 400 {
					writeJSON(w, code, map[string]string{"error": pesan})
				} else {
					writeJSON(w, code, map[string]string{"message": pesan})
				}
				return
			}
			setFlash(sm, r, kunci, pesan)
			http.Redirect(w, r, susulanURL(ujianID), http.StatusFound)
		}

		if ujianID <= 0 || siswaID <= 0 {
			selesai(http.StatusBadRequest, "error", "Ujian dan siswa wajib dipilih")
			return
		}
		var sampai time.Time
		if v := r.FormValue("sampai"); v != "" {
			t, err := ParseDateTime(v)
			if err != nil {
				selesai(http.StatusBadRequest, "error", "Format batas waktu tidak valid")
				return
			}
			sampai = t
		}
		err := BeriIzinSusulan(r.Context(), db, sm.GetInt(r.Context(), "adminId"), ujianID, siswaID, sampai, r.FormValue("alasan"))
		switch {
		case err == nil:
			selesai(http.StatusCreated, "success", "Izin ujian susulan diberikan")
		case errors.Is(err, ErrSusulanSiswaBukanKelas), errors.Is(err, ErrSusulanSudahSelesai),
			errors.Is(err, ErrSusulanWaktuLampau), errors.Is(err, ErrSusulanSiswaUjian):
			selesai(http.StatusBadRequest, "error", err.Error())
		default:
			serverError(w, r, err)
		}
	}
}

// SusulanHapusHandler mencabut izin.
func SusulanHapusHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}
		ujianID, err := CabutIzinSusulan(r.Context(), db, id)
		if err != nil {
			if errors.Is(err, ErrSusulanTidakDitemukan) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			serverError(w, r, err)
			return
		}
		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Izin susulan dicabut"})
			return
		}
		setFlash(sm, r, "success", "Izin susulan dicabut")
		http.Redirect(w, r, susulanURL(ujianID), http.StatusFound)
	}
}
