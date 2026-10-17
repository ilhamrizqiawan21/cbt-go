package admin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/soal"
)

// RegisterAdminSoalRoutes mendaftarkan seluruh rute pengelolaan soal untuk admin.
func RegisterAdminSoalRoutes(mux *http.ServeMux, db *sql.DB, sm *scs.SessionManager, requireAdmin func(http.Handler) http.Handler, requireAdminAPI func(http.Handler) http.Handler) {
	mux.Handle("GET /admin/soal", requireAdmin(SoalListHandler(db, sm)))
	mux.Handle("POST /admin/soal/tambah", requireAdmin(SoalTambahHandler(db, sm)))
	mux.Handle("GET /admin/soal/edit/{id}", requireAdmin(SoalGetHandler(db, sm)))
	mux.Handle("POST /admin/soal/edit/{id}", requireAdmin(SoalEditHandler(db, sm)))
	mux.Handle("POST /admin/soal/hapus/{id}", requireAdmin(SoalHapusHandler(db, sm)))
	mux.Handle("GET /admin/soal/tambah-batch", requireAdmin(SoalBatchPageHandler(db, sm)))
	mux.Handle("POST /admin/soal/batch-tambah", requireAdmin(SoalBatchTambahHandler(db, sm)))
}

// SoalListHandler menampilkan daftar soal per ujian atau mengembalikan data JSON.
func SoalListHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ujianStr := r.URL.Query().Get("ujian_id")
		ujianID, _ := strconv.Atoi(ujianStr)

		var list []soal.SoalKelola
		var err error
		if ujianID > 0 {
			list, err = soal.ListSoalByUjian(r.Context(), db, ujianID)
			if err != nil {
				serverError(w, r, err)
				return
			}
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
		if ujianID <= 0 && len(ujianList) > 0 {
			ujianID = ujianList[0].ID
			list, err = soal.ListSoalByUjian(r.Context(), db, ujianID)
			if err != nil {
				serverError(w, r, err)
				return
			}
		}

		flashError := getFlash(sm, r, "error")
		flashSuccess := getFlash(sm, r, "success")
		render(w, r, "admin_soal.gohtml", map[string]any{
			"SoalList":   list,
			"UjianList":  ujianList,
			"UjianID":    ujianID,
			"SoalBase":   "/admin/soal",
			"SoalPage":   "/admin/soal",
			"Error":      flashError,
			"Success":    flashSuccess,
			"Username":   sm.GetString(r.Context(), "username"),
			"ActiveMenu": "soal",
		})
	}
}

// SoalGetHandler mengembalikan detail satu soal untuk formulir edit atau inspeksi.
func SoalGetHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		s, err := soal.GetSoalByID(r.Context(), db, id)
		if err != nil {
			if errors.Is(err, soal.ErrSoalNotFound) {
				http.Error(w, "Soal tidak ditemukan", http.StatusNotFound)
				return
			}
			serverError(w, r, err)
			return
		}

		writeJSON(w, http.StatusOK, s)
	}
}

// SoalTambahHandler menambahkan soal tunggal.
func SoalTambahHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var s soal.SoalKelola
		if isJSONBody(r) {
			if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON body tidak valid"})
				return
			}
		} else {
			_ = r.ParseForm()
			ujianID, _ := strconv.Atoi(r.FormValue("ujian_id"))
			poin, _ := strconv.Atoi(r.FormValue("poin"))
			s = soal.SoalKelola{
				UjianID:      ujianID,
				TipeSoal:     r.FormValue("tipe_soal"),
				TeksSoal:     r.FormValue("teks_soal"),
				Poin:         poin,
				PilihanA:     stringPtr(r.FormValue("pilihan_a")),
				PilihanB:     stringPtr(r.FormValue("pilihan_b")),
				PilihanC:     stringPtr(r.FormValue("pilihan_c")),
				PilihanD:     stringPtr(r.FormValue("pilihan_d")),
				JawabanBenar: stringPtr(r.FormValue("jawaban_benar")),
				OpsiTambahan: stringPtr(r.FormValue("opsi_tambahan")),
			}
		}

		id, err := soal.TambahSoal(r.Context(), db, s)
		if err != nil {
			if wantsJSON(r) || isJSONBody(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/soal?ujian_id="+strconv.Itoa(s.UjianID), http.StatusFound)
			return
		}

		if wantsJSON(r) || isJSONBody(r) {
			writeJSON(w, http.StatusCreated, map[string]any{"id": id, "message": "Soal berhasil ditambahkan"})
			return
		}
		setFlash(sm, r, "success", "Soal berhasil ditambahkan")
		http.Redirect(w, r, "/admin/soal?ujian_id="+strconv.Itoa(s.UjianID), http.StatusFound)
	}
}

// SoalEditHandler memperbarui data soal tunggal.
func SoalEditHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		var s soal.SoalKelola
		if isJSONBody(r) {
			if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON body tidak valid"})
				return
			}
		} else {
			_ = r.ParseForm()
			poin, _ := strconv.Atoi(r.FormValue("poin"))
			s = soal.SoalKelola{
				TipeSoal:     r.FormValue("tipe_soal"),
				TeksSoal:     r.FormValue("teks_soal"),
				Poin:         poin,
				PilihanA:     stringPtr(r.FormValue("pilihan_a")),
				PilihanB:     stringPtr(r.FormValue("pilihan_b")),
				PilihanC:     stringPtr(r.FormValue("pilihan_c")),
				PilihanD:     stringPtr(r.FormValue("pilihan_d")),
				JawabanBenar: stringPtr(r.FormValue("jawaban_benar")),
				OpsiTambahan: stringPtr(r.FormValue("opsi_tambahan")),
			}
		}
		s.ID = id

		err = soal.EditSoal(r.Context(), db, s)
		if err != nil {
			if wantsJSON(r) || isJSONBody(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, soalURL(r.FormValue("ujian_id")), http.StatusFound)
			return
		}

		if wantsJSON(r) || isJSONBody(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Soal berhasil diperbarui"})
			return
		}
		setFlash(sm, r, "success", "Soal berhasil diperbarui")
		http.Redirect(w, r, soalURL(r.FormValue("ujian_id")), http.StatusFound)
	}
}

// SoalHapusHandler menghapus satu soal.
func SoalHapusHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		err = soal.HapusSoal(r.Context(), db, id)
		if err != nil {
			if wantsJSON(r) {
				serverError(w, r, err)
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, soalURL(r.FormValue("ujian_id")), http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Soal berhasil dihapus"})
			return
		}
		setFlash(sm, r, "success", "Soal berhasil dihapus")
		http.Redirect(w, r, soalURL(r.FormValue("ujian_id")), http.StatusFound)
	}
}

// SoalBatchPageHandler: rute Node dipertahankan, tetapi form tambah soal kini ada di halaman
// Bank Soal (modal), sehingga diarahkan ke sana. Batch via API tetap di POST /admin/soal/batch-tambah.
func SoalBatchPageHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, soalURL(r.URL.Query().Get("ujian_id")), http.StatusFound)
	}
}

// BatchPayload struktur payload untuk batch tambah soal.
type BatchPayload struct {
	UjianID int               `json:"ujian_id"`
	Soal    []soal.SoalKelola `json:"soal"`
}

// SoalBatchTambahHandler memproses penambahan batch soal.
func SoalBatchTambahHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload BatchPayload
		if isJSONBody(r) {
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Payload JSON tidak valid"})
				return
			}
		} else {
			_ = r.ParseForm()
			ujianID, _ := strconv.Atoi(r.FormValue("ujian_id"))
			soalJSON := r.FormValue("soal_json")
			if err := json.Unmarshal([]byte(soalJSON), &payload.Soal); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Format data soal_json tidak valid"})
				return
			}
			payload.UjianID = ujianID
		}

		total, err := soal.BatchTambahSoal(r.Context(), db, payload.UjianID, payload.Soal)
		if err != nil {
			if wantsJSON(r) || isJSONBody(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/soal?ujian_id="+strconv.Itoa(payload.UjianID), http.StatusFound)
			return
		}

		if wantsJSON(r) || isJSONBody(r) {
			writeJSON(w, http.StatusCreated, map[string]any{
				"message": "Berhasil menambahkan batch soal",
				"total":   total,
			})
			return
		}
		setFlash(sm, r, "success", "Berhasil menambahkan "+strconv.Itoa(total)+" soal")
		http.Redirect(w, r, "/admin/soal?ujian_id="+strconv.Itoa(payload.UjianID), http.StatusFound)
	}
}

func stringPtr(s string) *string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// soalURL membangun alamat kembali ke daftar soal satu ujian; id tidak valid → daftar umum.
func soalURL(ujianID string) string {
	if id, err := strconv.Atoi(ujianID); err == nil && id > 0 {
		return "/admin/soal?ujian_id=" + strconv.Itoa(id)
	}
	return "/admin/soal"
}
