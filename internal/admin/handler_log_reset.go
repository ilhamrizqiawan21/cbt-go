package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/alexedwards/scs/v2"
)

// RegisterAdminLogResetRoutes mendaftarkan rute pemantauan log pelanggaran dan aksi reset ujian untuk admin.
func RegisterAdminLogResetRoutes(mux *http.ServeMux, db *sql.DB, sm *scs.SessionManager, requireAdmin func(http.Handler) http.Handler) {
	mux.Handle("GET /admin/log-kecurangan", requireAdmin(LogKecuranganListHandler(db, sm)))
	mux.Handle("POST /admin/reset-ujian", requireAdmin(ResetUjianHandler(db, sm)))
	mux.Handle("POST /admin/reset-ujian-semua", requireAdmin(ResetUjianSemuaHandler(db, sm)))
}

// LogKecuranganListHandler menampilkan riwayat log pelanggaran seluruh siswa.
func LogKecuranganListHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ujianStr := r.URL.Query().Get("ujian_id")
		ujianID, _ := strconv.Atoi(ujianStr)
		jenisFilter := r.URL.Query().Get("jenis")

		list, err := GetLogKecurangan(r.Context(), db, ujianID, jenisFilter)
		if err != nil {
			serverError(w, r, err)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, list)
			return
		}

		ujianList, err := ListUjian(r.Context(), db)
		if err != nil {
			serverError(w, r, err)
			return
		}
		flashError := getFlash(sm, r, "error")
		flashSuccess := getFlash(sm, r, "success")

		render(w, r, "admin_log.gohtml", map[string]any{
			"LogList":    list,
			"UjianList":  ujianList,
			"UjianID":    ujianID,
			"Filter":     jenisFilter,
			"Error":      flashError,
			"Success":    flashSuccess,
			"Username":   sm.GetString(r.Context(), "username"),
			"ActiveMenu": "log",
		})
	}
}

// ResetUjianHandler mereset sesi dan jawaban satu siswa pada ujian tertentu (khusus admin).
func ResetUjianHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminID := sm.GetInt(r.Context(), "adminId")

		var payload struct {
			SiswaID int `json:"siswa_id"`
			UjianID int `json:"ujian_id"`
		}

		if isJSONBody(r) {
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON body tidak valid"})
				return
			}
		} else {
			_ = r.ParseForm()
			sID, _ := strconv.Atoi(r.FormValue("siswa_id"))
			uID, _ := strconv.Atoi(r.FormValue("ujian_id"))
			payload.SiswaID = sID
			payload.UjianID = uID
		}

		err := ResetUjianSiswa(r.Context(), db, adminID, payload.SiswaID, payload.UjianID)
		if err != nil {
			if wantsJSON(r) || isJSONBody(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, kembaliKe(r, "/admin/log-kecurangan"), http.StatusFound)
			return
		}

		if wantsJSON(r) || isJSONBody(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Ujian siswa berhasil direset"})
			return
		}
		setFlash(sm, r, "success", "Ujian siswa berhasil direset")
		http.Redirect(w, r, kembaliKe(r, "/admin/log-kecurangan"), http.StatusFound)
	}
}

// ResetUjianSemuaHandler mereset seluruh sesi dan data nilai ujian untuk satu ujian tertentu.
func ResetUjianSemuaHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminID := sm.GetInt(r.Context(), "adminId")

		var payload struct {
			UjianID int `json:"ujian_id"`
		}

		if isJSONBody(r) {
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON body tidak valid"})
				return
			}
		} else {
			_ = r.ParseForm()
			uID, _ := strconv.Atoi(r.FormValue("ujian_id"))
			payload.UjianID = uID
		}

		err := ResetUjianSemua(r.Context(), db, adminID, payload.UjianID)
		if err != nil {
			if wantsJSON(r) || isJSONBody(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, kembaliKe(r, "/admin/log-kecurangan"), http.StatusFound)
			return
		}

		if wantsJSON(r) || isJSONBody(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Seluruh peserta ujian berhasil direset"})
			return
		}
		setFlash(sm, r, "success", "Seluruh peserta ujian berhasil direset")
		http.Redirect(w, r, kembaliKe(r, "/admin/log-kecurangan"), http.StatusFound)
	}
}

// kembaliKe membaca field form "kembali" agar admin diarahkan ke halaman asal
// (log atau hasil). Hanya path internal /admin/ yang diterima untuk mencegah open redirect.
func kembaliKe(r *http.Request, fallback string) string {
	p := r.FormValue("kembali")
	if strings.HasPrefix(p, "/admin/") && !strings.Contains(p, "//") && !strings.Contains(p, "\\") {
		return p
	}
	return fallback
}
