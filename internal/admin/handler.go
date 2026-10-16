package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/alexedwards/scs/v2"

	"cbt-go/web"
)

// RegisterAdminMasterRoutes mendaftarkan seluruh rute CRUD master data admin (guru, kelas, mapel, pengajaran).
func RegisterAdminMasterRoutes(mux *http.ServeMux, db *sql.DB, sm *scs.SessionManager, requireAdmin func(http.Handler) http.Handler) {
	// ======================== GURU ========================
	mux.Handle("GET /admin/guru", requireAdmin(GuruListHandler(db, sm)))
	mux.Handle("POST /admin/guru/tambah", requireAdmin(GuruTambahHandler(db, sm)))
	mux.Handle("POST /admin/guru/edit/{id}", requireAdmin(GuruEditHandler(db, sm)))
	mux.Handle("POST /admin/guru/hapus/{id}", requireAdmin(GuruHapusHandler(db, sm)))

	// ======================== KELAS ========================
	mux.Handle("GET /admin/kelas", requireAdmin(KelasListHandler(db, sm)))
	mux.Handle("POST /admin/kelas/tambah", requireAdmin(KelasTambahHandler(db, sm)))
	mux.Handle("POST /admin/kelas/edit/{id}", requireAdmin(KelasEditHandler(db, sm)))
	mux.Handle("POST /admin/kelas/hapus/{id}", requireAdmin(KelasHapusHandler(db, sm)))

	// ======================== MATA PELAJARAN ========================
	mux.Handle("GET /admin/mapel", requireAdmin(MapelListHandler(db, sm)))
	mux.Handle("POST /admin/mapel/tambah", requireAdmin(MapelTambahHandler(db, sm)))
	mux.Handle("POST /admin/mapel/edit/{id}", requireAdmin(MapelEditHandler(db, sm)))
	mux.Handle("POST /admin/mapel/hapus/{id}", requireAdmin(MapelHapusHandler(db, sm)))

	// ======================== PENGAJARAN ========================
	mux.Handle("GET /admin/pengajaran", requireAdmin(PengajaranListHandler(db, sm)))
	mux.Handle("POST /admin/pengajaran/tambah", requireAdmin(PengajaranTambahHandler(db, sm)))
	mux.Handle("POST /admin/pengajaran/hapus/{id}", requireAdmin(PengajaranHapusHandler(db, sm)))
}

// ======================== HANDLER GURU ========================

func GuruListHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := ListGuru(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, list)
			return
		}
		flashError := getFlash(sm, r, "error")
		flashSuccess := getFlash(sm, r, "success")
		_ = web.Render(w, "admin_guru.gohtml", map[string]any{
			"GuruList": list,
			"Error":    flashError,
			"Success":  flashSuccess,
			"Username": sm.GetString(r.Context(), "username"),
		})
	}
}

func GuruTambahHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		nip := strings.TrimSpace(r.FormValue("nip"))
		nama := strings.TrimSpace(r.FormValue("nama"))
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")

		id, err := TambahGuru(r.Context(), db, nip, nama, username, password)
		if err != nil {
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/guru", http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusCreated, map[string]any{"id": id, "message": "Guru berhasil ditambahkan"})
			return
		}
		setFlash(sm, r, "success", "Guru berhasil ditambahkan")
		http.Redirect(w, r, "/admin/guru", http.StatusFound)
	}
}

func GuruEditHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		_ = r.ParseForm()
		nip := strings.TrimSpace(r.FormValue("nip"))
		nama := strings.TrimSpace(r.FormValue("nama"))
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")

		err = EditGuru(r.Context(), db, id, nip, nama, username, password)
		if err != nil {
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/guru", http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Data guru berhasil diperbarui"})
			return
		}
		setFlash(sm, r, "success", "Data guru berhasil diperbarui")
		http.Redirect(w, r, "/admin/guru", http.StatusFound)
	}
}

func GuruHapusHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		err = HapusGuru(r.Context(), db, id)
		if err != nil {
			if wantsJSON(r) {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/guru", http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Guru berhasil dihapus"})
			return
		}
		setFlash(sm, r, "success", "Guru berhasil dihapus")
		http.Redirect(w, r, "/admin/guru", http.StatusFound)
	}
}

// ======================== HANDLER KELAS ========================

func KelasListHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := ListKelas(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, list)
			return
		}
		_ = web.Render(w, "admin_kelas.gohtml", map[string]any{
			"KelasList": list,
			"Error":     getFlash(sm, r, "error"),
			"Success":   getFlash(sm, r, "success"),
			"Username":  sm.GetString(r.Context(), "username"),
		})
	}
}

func KelasTambahHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		namaKelas := strings.TrimSpace(r.FormValue("nama_kelas"))

		id, err := TambahKelas(r.Context(), db, namaKelas)
		if err != nil {
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/kelas", http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusCreated, map[string]any{"id": id, "message": "Kelas berhasil ditambahkan"})
			return
		}
		setFlash(sm, r, "success", "Kelas berhasil ditambahkan")
		http.Redirect(w, r, "/admin/kelas", http.StatusFound)
	}
}

func KelasEditHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		_ = r.ParseForm()
		namaKelas := strings.TrimSpace(r.FormValue("nama_kelas"))

		err = EditKelas(r.Context(), db, id, namaKelas)
		if err != nil {
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/kelas", http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Data kelas berhasil diperbarui"})
			return
		}
		setFlash(sm, r, "success", "Data kelas berhasil diperbarui")
		http.Redirect(w, r, "/admin/kelas", http.StatusFound)
	}
}

func KelasHapusHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		err = HapusKelas(r.Context(), db, id)
		if err != nil {
			if wantsJSON(r) {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/kelas", http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Kelas berhasil dihapus"})
			return
		}
		setFlash(sm, r, "success", "Kelas berhasil dihapus")
		http.Redirect(w, r, "/admin/kelas", http.StatusFound)
	}
}

// ======================== HANDLER MAPEL ========================

func MapelListHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := ListMapel(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, list)
			return
		}
		_ = web.Render(w, "admin_mapel.gohtml", map[string]any{
			"MapelList": list,
			"Error":     getFlash(sm, r, "error"),
			"Success":   getFlash(sm, r, "success"),
			"Username":  sm.GetString(r.Context(), "username"),
		})
	}
}

func MapelTambahHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		namaMapel := strings.TrimSpace(r.FormValue("nama_mapel"))

		id, err := TambahMapel(r.Context(), db, namaMapel)
		if err != nil {
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/mapel", http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusCreated, map[string]any{"id": id, "message": "Mata pelajaran berhasil ditambahkan"})
			return
		}
		setFlash(sm, r, "success", "Mata pelajaran berhasil ditambahkan")
		http.Redirect(w, r, "/admin/mapel", http.StatusFound)
	}
}

func MapelEditHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		_ = r.ParseForm()
		namaMapel := strings.TrimSpace(r.FormValue("nama_mapel"))

		err = EditMapel(r.Context(), db, id, namaMapel)
		if err != nil {
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/mapel", http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Mata pelajaran berhasil diperbarui"})
			return
		}
		setFlash(sm, r, "success", "Mata pelajaran berhasil diperbarui")
		http.Redirect(w, r, "/admin/mapel", http.StatusFound)
	}
}

func MapelHapusHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		err = HapusMapel(r.Context(), db, id)
		if err != nil {
			if wantsJSON(r) {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/mapel", http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Mata pelajaran berhasil dihapus"})
			return
		}
		setFlash(sm, r, "success", "Mata pelajaran berhasil dihapus")
		http.Redirect(w, r, "/admin/mapel", http.StatusFound)
	}
}

// ======================== HANDLER PENGAJARAN ========================

func PengajaranListHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := ListPengajaran(r.Context(), db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, list)
			return
		}

		guruList, _ := ListGuru(r.Context(), db)
		kelasList, _ := ListKelas(r.Context(), db)
		mapelList, _ := ListMapel(r.Context(), db)

		_ = web.Render(w, "admin_pengajaran.gohtml", map[string]any{
			"PengajaranList": list,
			"GuruList":       guruList,
			"KelasList":      kelasList,
			"MapelList":      mapelList,
			"Error":          getFlash(sm, r, "error"),
			"Success":        getFlash(sm, r, "success"),
			"Username":       sm.GetString(r.Context(), "username"),
		})
	}
}

func PengajaranTambahHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		guruID, _ := strconv.Atoi(r.FormValue("guru_id"))
		mapelID, _ := strconv.Atoi(r.FormValue("mapel_id"))
		kelasID, _ := strconv.Atoi(r.FormValue("kelas_id"))

		id, err := TambahPengajaran(r.Context(), db, guruID, mapelID, kelasID)
		if err != nil {
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/pengajaran", http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusCreated, map[string]any{"id": id, "message": "Pengajaran berhasil ditambahkan"})
			return
		}
		setFlash(sm, r, "success", "Pengajaran berhasil ditambahkan")
		http.Redirect(w, r, "/admin/pengajaran", http.StatusFound)
	}
}

func PengajaranHapusHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		err = HapusPengajaran(r.Context(), db, id)
		if err != nil {
			if wantsJSON(r) {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/pengajaran", http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Pengajaran berhasil dihapus"})
			return
		}
		setFlash(sm, r, "success", "Pengajaran berhasil dihapus")
		http.Redirect(w, r, "/admin/pengajaran", http.StatusFound)
	}
}

// ======================== HELPER ========================

func wantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func setFlash(sm *scs.SessionManager, r *http.Request, key, val string) {
	if sm != nil {
		sm.Put(r.Context(), "flash_"+key, val)
	}
}

func getFlash(sm *scs.SessionManager, r *http.Request, key string) string {
	if sm == nil {
		return ""
	}
	val := sm.PopString(r.Context(), "flash_"+key)
	return val
}
