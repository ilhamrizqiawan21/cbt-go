package admin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/alexedwards/scs/v2"
)

// RegisterAdminUjianRoutes mendaftarkan seluruh rute CRUD ujian admin dan endpoint API pendukungnya.
func RegisterAdminUjianRoutes(mux *http.ServeMux, db *sql.DB, sm *scs.SessionManager, requireAdmin func(http.Handler) http.Handler, requireAdminAPI func(http.Handler) http.Handler) {
	mux.Handle("GET /admin/ujian", requireAdmin(UjianListHandler(db, sm)))
	mux.Handle("POST /admin/ujian/tambah", requireAdmin(UjianTambahHandler(db, sm)))
	mux.Handle("GET /admin/ujian/edit/{id}", requireAdmin(UjianGetHandler(db, sm)))
	mux.Handle("POST /admin/ujian/edit/{id}", requireAdmin(UjianEditHandler(db, sm)))
	mux.Handle("POST /admin/ujian/hapus/{id}", requireAdmin(UjianHapusHandler(db, sm)))

	// API endpoint untuk mengambil daftar ujian per mata pelajaran
	mux.Handle("GET /admin/api/ujian-by-mapel/{mapelId}", requireAdminAPI(UjianByMapelAPIHandler(db)))
}

// UjianListHandler menampilkan daftar ujian atau mengembalikan data JSON.
func UjianListHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := ListUjian(r.Context(), db)
		if err != nil {
			serverError(w, r, err)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, list)
			return
		}

		pengajaranList, err := ListPengajaran(r.Context(), db)
		if err != nil {
			serverError(w, r, err)
			return
		}
		flashError := getFlash(sm, r, "error")
		flashSuccess := getFlash(sm, r, "success")

		render(w, r, "admin_ujian.gohtml", map[string]any{
			"UjianList":      list,
			"PengajaranList": pengajaranList,
			"Error":          flashError,
			"Success":        flashSuccess,
			"Username":       sm.GetString(r.Context(), "username"),
			"ActiveMenu":     "ujian",
		})
	}
}

// UjianGetHandler mengembalikan detail satu ujian (untuk form edit atau inspeksi API).
func UjianGetHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		u, err := GetUjianByID(r.Context(), db, id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				http.Error(w, "Ujian tidak ditemukan", http.StatusNotFound)
				return
			}
			serverError(w, r, err)
			return
		}

		writeJSON(w, http.StatusOK, u)
	}
}

// UjianTambahHandler memproses pembuatan ujian baru dengan validasi ketat.
func UjianTambahHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var u Ujian
		if isJSONBody(r) {
			if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON body tidak valid"})
				return
			}
		} else {
			_ = r.ParseForm()
			pengajaranID, _ := strconv.Atoi(r.FormValue("pengajaran_id"))
			durasi, _ := strconv.Atoi(r.FormValue("durasi"))
			batasPelanggaran, _ := strconv.Atoi(r.FormValue("batas_pelanggaran"))
			tMulai, errMulai := ParseDateTime(r.FormValue("tanggal_mulai"))
			tSelesai, errSelesai := ParseDateTime(r.FormValue("tanggal_selesai"))

			if errMulai != nil || errSelesai != nil {
				msg := "Format tanggal mulai atau tanggal selesai tidak valid"
				if wantsJSON(r) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
					return
				}
				setFlash(sm, r, "error", msg)
				http.Redirect(w, r, "/admin/ujian", http.StatusFound)
				return
			}

			acakSoal := r.FormValue("acak_soal") == "1" || r.FormValue("acak_soal") == "true" || r.FormValue("acak_soal") == "on"
			acakPilihan := r.FormValue("acak_pilihan") == "1" || r.FormValue("acak_pilihan") == "true" || r.FormValue("acak_pilihan") == "on"

			token := strings.TrimSpace(r.FormValue("token_ujian"))
			var tokenPtr *string
			if token != "" {
				tokenPtr = &token
			}

			u = Ujian{
				PengajaranID:     pengajaranID,
				NamaUjian:        r.FormValue("nama_ujian"),
				Durasi:           durasi,
				TanggalMulai:     tMulai,
				TanggalSelesai:   tSelesai,
				AcakSoal:         acakSoal,
				AcakPilihan:      acakPilihan,
				BatasPelanggaran: batasPelanggaran,
				TokenUjian:       tokenPtr,
			}
		}

		id, err := TambahUjian(r.Context(), db, u)
		if err != nil {
			if wantsJSON(r) || isJSONBody(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/ujian", http.StatusFound)
			return
		}

		if wantsJSON(r) || isJSONBody(r) {
			writeJSON(w, http.StatusCreated, map[string]any{"id": id, "message": "Ujian berhasil dibuat"})
			return
		}
		setFlash(sm, r, "success", "Ujian berhasil dibuat")
		http.Redirect(w, r, "/admin/ujian", http.StatusFound)
	}
}

// UjianEditHandler memproses pembaruan data ujian.
func UjianEditHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		var u Ujian
		if isJSONBody(r) {
			if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON body tidak valid"})
				return
			}
		} else {
			_ = r.ParseForm()
			pengajaranID, _ := strconv.Atoi(r.FormValue("pengajaran_id"))
			durasi, _ := strconv.Atoi(r.FormValue("durasi"))
			batasPelanggaran, _ := strconv.Atoi(r.FormValue("batas_pelanggaran"))
			tMulai, errMulai := ParseDateTime(r.FormValue("tanggal_mulai"))
			tSelesai, errSelesai := ParseDateTime(r.FormValue("tanggal_selesai"))

			if errMulai != nil || errSelesai != nil {
				msg := "Format tanggal mulai atau tanggal selesai tidak valid"
				if wantsJSON(r) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
					return
				}
				setFlash(sm, r, "error", msg)
				http.Redirect(w, r, "/admin/ujian", http.StatusFound)
				return
			}

			acakSoal := r.FormValue("acak_soal") == "1" || r.FormValue("acak_soal") == "true" || r.FormValue("acak_soal") == "on"
			acakPilihan := r.FormValue("acak_pilihan") == "1" || r.FormValue("acak_pilihan") == "true" || r.FormValue("acak_pilihan") == "on"

			token := strings.TrimSpace(r.FormValue("token_ujian"))
			var tokenPtr *string
			if token != "" {
				tokenPtr = &token
			}

			u = Ujian{
				PengajaranID:     pengajaranID,
				NamaUjian:        r.FormValue("nama_ujian"),
				Durasi:           durasi,
				TanggalMulai:     tMulai,
				TanggalSelesai:   tSelesai,
				AcakSoal:         acakSoal,
				AcakPilihan:      acakPilihan,
				BatasPelanggaran: batasPelanggaran,
				TokenUjian:       tokenPtr,
			}
		}
		u.ID = id

		err = EditUjian(r.Context(), db, u)
		if err != nil {
			if wantsJSON(r) || isJSONBody(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/ujian", http.StatusFound)
			return
		}

		if wantsJSON(r) || isJSONBody(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Ujian berhasil diperbarui"})
			return
		}
		setFlash(sm, r, "success", "Ujian berhasil diperbarui")
		http.Redirect(w, r, "/admin/ujian", http.StatusFound)
	}
}

// UjianHapusHandler menghapus data ujian.
func UjianHapusHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		err = HapusUjian(r.Context(), db, id)
		if err != nil {
			if wantsJSON(r) {
				serverError(w, r, err)
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/ujian", http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Ujian berhasil dihapus"})
			return
		}
		setFlash(sm, r, "success", "Ujian berhasil dihapus")
		http.Redirect(w, r, "/admin/ujian", http.StatusFound)
	}
}

// UjianByMapelAPIHandler menyajikan daftar ujian berdasarkan mapel ID via API JSON.
func UjianByMapelAPIHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mapelStr := r.PathValue("mapelId")
		mapelID, err := strconv.Atoi(mapelStr)
		if err != nil || mapelID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Mapel ID tidak valid"})
			return
		}

		list, err := ListUjianByMapel(r.Context(), db, mapelID)
		if err != nil {
			serverError(w, r, err)
			return
		}

		writeJSON(w, http.StatusOK, list)
	}
}

func isJSONBody(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return strings.Contains(ct, "application/json")
}
