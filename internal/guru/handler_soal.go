package guru

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

// RegisterGuruRoutes mendaftarkan seluruh rute kelola soal khusus guru dengan otorisasi mapel/pengajaran ketat.
func RegisterGuruRoutes(mux *http.ServeMux, db *sql.DB, sm *scs.SessionManager, requireGuru func(http.Handler) http.Handler, requireGuruAPI func(http.Handler) http.Handler) {
	mux.Handle("GET /guru/kelola-soal", requireGuru(KelolaSoalPageHandler(db, sm)))
	mux.Handle("GET /guru/kelola-soal/tambah-batch", requireGuru(KelolaSoalBatchPageHandler(db, sm)))

	// API Guru
	mux.Handle("GET /guru/api/ujian-by-guru", requireGuruAPI(UjianByGuruAPIHandler(db, sm)))
	mux.Handle("GET /guru/api/soal/{ujianId}", requireGuruAPI(SoalByUjianAPIHandler(db, sm)))

	// Aksi Soal Guru (terproteksi otorisasi kepemilikan pengajaran)
	mux.Handle("POST /guru/soal/tambah", requireGuru(GuruSoalTambahHandler(db, sm)))
	mux.Handle("GET /guru/soal/edit/{id}", requireGuru(GuruSoalGetHandler(db, sm)))
	mux.Handle("POST /guru/soal/edit/{id}", requireGuru(GuruSoalEditHandler(db, sm)))
	mux.Handle("POST /guru/soal/hapus/{id}", requireGuru(GuruSoalHapusHandler(db, sm)))
	mux.Handle("POST /guru/soal/batch-tambah", requireGuru(GuruSoalBatchTambahHandler(db, sm)))
}

// KelolaSoalPageHandler menampilkan bank soal satu ujian milik guru aktif (?ujian_id=, default ujian terbaru).
func KelolaSoalPageHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
		listUjian, err := soal.ListUjianByGuru(r.Context(), db, guruID)
		if err != nil {
			serverError(w, r, err)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, listUjian)
			return
		}

		ujianID, _ := strconv.Atoi(r.URL.Query().Get("ujian_id"))
		if ujianID <= 0 && len(listUjian) > 0 {
			ujianID = listUjian[0].ID
		}

		var listSoal []soal.SoalKelola
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
			listSoal, err = soal.ListSoalByUjian(r.Context(), db, ujianID)
			if err != nil {
				serverError(w, r, err)
				return
			}
		}

		data := dataHalaman(sm, r, "soal")
		data["UjianList"] = listUjian
		data["UjianID"] = ujianID
		data["SoalList"] = listSoal
		// Partial soal_bank dipakai bersama admin; rute guru /guru/soal/* memeriksa kepemilikan.
		data["SoalBase"] = "/guru/soal"
		data["SoalPage"] = "/guru/kelola-soal"
		render(w, r, "guru_soal.gohtml", data)
	}
}

// KelolaSoalBatchPageHandler: rute Node dipertahankan; form tambah soal ada di halaman Kelola Soal (modal).
func KelolaSoalBatchPageHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ujianID, _ := strconv.Atoi(r.URL.Query().Get("ujian_id"))
		http.Redirect(w, r, kelolaSoalURL(ujianID), http.StatusFound)
	}
}

// UjianByGuruAPIHandler mengembalikan daftar ujian yang diampu oleh guru aktif dalam format JSON.
func UjianByGuruAPIHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
		list, err := soal.ListUjianByGuru(r.Context(), db, guruID)
		if err != nil {
			serverError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// SoalByUjianAPIHandler mengembalikan seluruh soal pada ujian tertentu jika guru memiliki hak pengajaran.
func SoalByUjianAPIHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
		ujianStr := r.PathValue("ujianId")
		ujianID, err := strconv.Atoi(ujianStr)
		if err != nil || ujianID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ID ujian tidak valid"})
			return
		}

		// Validasi Otorisasi Guru atas Ujian
		boleh, err := soal.CekOtorisasiUjianGuru(r.Context(), db, guruID, ujianID)
		if err != nil {
			serverError(w, r, err)
			return
		}
		if !boleh {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Akses ditolak: Anda bukan pengampu ujian ini"})
			return
		}

		list, err := soal.ListSoalByUjian(r.Context(), db, ujianID)
		if err != nil {
			serverError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// GuruSoalTambahHandler memproses penambahan soal tunggal oleh guru.
func GuruSoalTambahHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
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

		// Validasi Otorisasi Guru
		boleh, err := soal.CekOtorisasiUjianGuru(r.Context(), db, guruID, s.UjianID)
		if err != nil || !boleh {
			msg := "Akses ditolak: Anda tidak memiliki hak akses atas ujian ini"
			if wantsJSON(r) || isJSONBody(r) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": msg})
				return
			}
			setFlash(sm, r, "error", msg)
			http.Redirect(w, r, kelolaSoalURL(0), http.StatusFound)
			return
		}

		id, err := soal.TambahSoal(r.Context(), db, s)
		if err != nil {
			if wantsJSON(r) || isJSONBody(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, kelolaSoalURL(s.UjianID), http.StatusFound)
			return
		}

		if wantsJSON(r) || isJSONBody(r) {
			writeJSON(w, http.StatusCreated, map[string]any{"id": id, "message": "Soal berhasil ditambahkan"})
			return
		}
		setFlash(sm, r, "success", "Soal berhasil ditambahkan")
		http.Redirect(w, r, kelolaSoalURL(s.UjianID), http.StatusFound)
	}
}

// GuruSoalGetHandler mengambil detail satu soal dengan validasi kepemilikan guru.
func GuruSoalGetHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		boleh, err := soal.CekOtorisasiSoalGuru(r.Context(), db, guruID, id)
		if err != nil || !boleh {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Akses ditolak: Anda bukan pengampu soal ini"})
			return
		}

		s, err := soal.GetSoalByID(r.Context(), db, id)
		if errors.Is(err, soal.ErrSoalNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Soal tidak ditemukan"})
			return
		}
		if err != nil {
			serverError(w, r, err)
			return
		}

		writeJSON(w, http.StatusOK, s)
	}
}

// GuruSoalEditHandler memperbarui satu soal setelah verifikasi otorisasi guru.
func GuruSoalEditHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		boleh, err := soal.CekOtorisasiSoalGuru(r.Context(), db, guruID, id)
		if err != nil || !boleh {
			msg := "Akses ditolak: Anda tidak memiliki hak untuk mengedit soal ini"
			if wantsJSON(r) || isJSONBody(r) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": msg})
				return
			}
			setFlash(sm, r, "error", msg)
			http.Redirect(w, r, kelolaSoalURL(0), http.StatusFound)
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
			http.Redirect(w, r, kelolaSoalURL(formInt(r, "ujian_id")), http.StatusFound)
			return
		}

		if wantsJSON(r) || isJSONBody(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Soal berhasil diperbarui"})
			return
		}
		setFlash(sm, r, "success", "Soal berhasil diperbarui")
		http.Redirect(w, r, kelolaSoalURL(formInt(r, "ujian_id")), http.StatusFound)
	}
}

// GuruSoalHapusHandler menghapus satu soal setelah verifikasi otorisasi guru.
func GuruSoalHapusHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		boleh, err := soal.CekOtorisasiSoalGuru(r.Context(), db, guruID, id)
		if err != nil || !boleh {
			msg := "Akses ditolak: Anda tidak memiliki hak untuk menghapus soal ini"
			if wantsJSON(r) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": msg})
				return
			}
			setFlash(sm, r, "error", msg)
			http.Redirect(w, r, kelolaSoalURL(0), http.StatusFound)
			return
		}

		err = soal.HapusSoal(r.Context(), db, id)
		if err != nil {
			if wantsJSON(r) {
				serverError(w, r, err)
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, kelolaSoalURL(formInt(r, "ujian_id")), http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Soal berhasil dihapus"})
			return
		}
		setFlash(sm, r, "success", "Soal berhasil dihapus")
		http.Redirect(w, r, kelolaSoalURL(formInt(r, "ujian_id")), http.StatusFound)
	}
}

// GuruSoalBatchTambahHandler memproses penambahan batch soal oleh guru.
func GuruSoalBatchTambahHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
		var payload struct {
			UjianID int               `json:"ujian_id"`
			Soal    []soal.SoalKelola `json:"soal"`
		}

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
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Format soal_json tidak valid"})
				return
			}
			payload.UjianID = ujianID
		}

		// Validasi Otorisasi Ujian Guru
		boleh, err := soal.CekOtorisasiUjianGuru(r.Context(), db, guruID, payload.UjianID)
		if err != nil || !boleh {
			msg := "Akses ditolak: Anda tidak memiliki hak atas ujian ini"
			if wantsJSON(r) || isJSONBody(r) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": msg})
				return
			}
			setFlash(sm, r, "error", msg)
			http.Redirect(w, r, kelolaSoalURL(0), http.StatusFound)
			return
		}

		total, err := soal.BatchTambahSoal(r.Context(), db, payload.UjianID, payload.Soal)
		if err != nil {
			if wantsJSON(r) || isJSONBody(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, kelolaSoalURL(payload.UjianID), http.StatusFound)
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
		http.Redirect(w, r, kelolaSoalURL(payload.UjianID), http.StatusFound)
	}
}

// Helpers
func wantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json")
}

func isJSONBody(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	return strings.Contains(ct, "application/json")
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
	return sm.PopString(r.Context(), "flash_"+key)
}

// formInt membaca field form bilangan bulat; nilai tidak valid menjadi 0.
func formInt(r *http.Request, key string) int {
	n, _ := strconv.Atoi(r.FormValue(key))
	return n
}

func stringPtr(s string) *string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
