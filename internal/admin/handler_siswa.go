package admin

import (
	"database/sql"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/excel"
	"cbt-go/web"
)

// RegisterAdminSiswaRoutes mendaftarkan seluruh rute CRUD siswa dan impor/template Excel.
func RegisterAdminSiswaRoutes(mux *http.ServeMux, db *sql.DB, sm *scs.SessionManager, requireAdmin func(http.Handler) http.Handler) {
	mux.Handle("GET /admin/siswa", requireAdmin(SiswaListHandler(db, sm)))
	mux.Handle("POST /admin/siswa/tambah", requireAdmin(SiswaTambahHandler(db, sm)))
	mux.Handle("POST /admin/siswa/edit/{id}", requireAdmin(SiswaEditHandler(db, sm)))
	mux.Handle("POST /admin/siswa/hapus/{id}", requireAdmin(SiswaHapusHandler(db, sm)))
	mux.Handle("GET /admin/siswa/template", requireAdmin(SiswaTemplateHandler()))
	mux.Handle("POST /admin/siswa/import", requireAdmin(SiswaImportHandler(db, sm)))
	mux.Handle("GET /admin/siswa/kartu", requireAdmin(SiswaKartuCetakHandler(db, sm)))
	mux.Handle("POST /admin/siswa/generate-pin", requireAdmin(SiswaResetPINMassalHandler(db, sm)))
}

// SiswaListHandler menampilkan daftar siswa atau mengembalikan JSON.
func SiswaListHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kelasFilter := r.URL.Query().Get("kelas")
		list, err := ListSiswa(r.Context(), db, kelasFilter)
		if err != nil {
			serverError(w, r, err)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, list)
			return
		}

		kelasList, err := ListKelas(r.Context(), db)
		if err != nil {
			serverError(w, r, err)
			return
		}
		flashError := getFlash(sm, r, "error")
		flashSuccess := getFlash(sm, r, "success")

		render(w, r, "admin_siswa.gohtml", map[string]any{
			"SiswaList":  list,
			"KelasList":  kelasList,
			"Filter":     kelasFilter,
			"Error":      flashError,
			"Success":    flashSuccess,
			"Username":   sm.GetString(r.Context(), "username"),
			"ActiveMenu": "siswa",
		})
	}
}

// SiswaTambahHandler menambah siswa baru.
func SiswaTambahHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		nis := strings.TrimSpace(r.FormValue("nis"))
		nama := strings.TrimSpace(r.FormValue("nama"))
		kelas := strings.TrimSpace(r.FormValue("kelas"))
		pin := strings.TrimSpace(r.FormValue("pin"))
		if pin == "" {
			pin = strings.TrimSpace(r.FormValue("pin_ujian"))
		}

		id, err := TambahSiswa(r.Context(), db, nis, nama, kelas, pin)
		if err != nil {
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/siswa", http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusCreated, map[string]any{"id": id, "message": "Siswa berhasil ditambahkan"})
			return
		}
		setFlash(sm, r, "success", "Siswa berhasil ditambahkan")
		http.Redirect(w, r, "/admin/siswa", http.StatusFound)
	}
}

// SiswaEditHandler mengubah data siswa.
func SiswaEditHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		_ = r.ParseForm()
		nis := strings.TrimSpace(r.FormValue("nis"))
		nama := strings.TrimSpace(r.FormValue("nama"))
		kelas := strings.TrimSpace(r.FormValue("kelas"))
		pin := strings.TrimSpace(r.FormValue("pin"))
		if pin == "" {
			pin = strings.TrimSpace(r.FormValue("pin_ujian"))
		}

		err = EditSiswa(r.Context(), db, id, nis, nama, kelas, pin)
		if err != nil {
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/siswa", http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Siswa berhasil diperbarui"})
			return
		}
		setFlash(sm, r, "success", "Siswa berhasil diperbarui")
		http.Redirect(w, r, "/admin/siswa", http.StatusFound)
	}
}

// SiswaHapusHandler menghapus data siswa.
func SiswaHapusHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.Error(w, "ID tidak valid", http.StatusBadRequest)
			return
		}

		err = HapusSiswa(r.Context(), db, id)
		if err != nil {
			if wantsJSON(r) {
				serverError(w, r, err)
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, "/admin/siswa", http.StatusFound)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, map[string]string{"message": "Siswa berhasil dihapus"})
			return
		}
		setFlash(sm, r, "success", "Siswa berhasil dihapus")
		http.Redirect(w, r, "/admin/siswa", http.StatusFound)
	}
}

// SiswaTemplateHandler mengunduh berkas template Excel kosong untuk impor siswa.
func SiswaTemplateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := excel.BuatTemplateSiswa()
		if err != nil {
			serverError(w, r, fmt.Errorf("buat template excel siswa: %w", err))
			return
		}

		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", "attachment; filename=\"template_siswa.xlsx\"")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data.Bytes())
	}
}

// SiswaImportHandler mengunggah berkas Excel dan mengimpor siswa secara batch.
func SiswaImportHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	const maxUploadSize = 2 << 20 // 2 MB

	return func(w http.ResponseWriter, r *http.Request) {
		// Batasi pembacaan body maksimum 2 MB
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			msg := "Ukuran file melebihi batas 2 MB"
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
				return
			}
			setFlash(sm, r, "error", msg)
			http.Redirect(w, r, "/admin/siswa", http.StatusFound)
			return
		}

		file, handler, err := r.FormFile("file")
		if err != nil {
			file, handler, err = r.FormFile("excel")
		}
		if err != nil {
			msg := "File Excel tidak ditemukan dalam request"
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
				return
			}
			setFlash(sm, r, "error", msg)
			http.Redirect(w, r, "/admin/siswa", http.StatusFound)
			return
		}
		defer file.Close()

		// Hanya menerima berkas .xlsx
		ext := strings.ToLower(filepath.Ext(handler.Filename))
		if ext != ".xlsx" {
			msg := "Format file tidak valid, hanya berkas .xlsx yang diizinkan"
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
				return
			}
			setFlash(sm, r, "error", msg)
			http.Redirect(w, r, "/admin/siswa", http.StatusFound)
			return
		}

		// Baca dan validasi baris dari Excel di memori
		parsed, err := excel.BacaImportSiswa(file)
		if err != nil {
			msg := fmt.Sprintf("Gagal membaca berkas Excel: %v", err)
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
				return
			}
			setFlash(sm, r, "error", msg)
			http.Redirect(w, r, "/admin/siswa", http.StatusFound)
			return
		}

		berhasil, dbErrors, err := ImportSiswaBatch(r.Context(), db, parsed.Valid)
		if err != nil {
			msg := fmt.Sprintf("Gagal memproses batch siswa: %v", err)
			if wantsJSON(r) {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg})
				return
			}
			setFlash(sm, r, "error", msg)
			http.Redirect(w, r, "/admin/siswa", http.StatusFound)
			return
		}

		laporanGagal := append(parsed.Errors, dbErrors...)

		resp := map[string]any{
			"berhasil": berhasil,
			"gagal":    laporanGagal,
			"pesan":    fmt.Sprintf("Berhasil mengimpor %d siswa, gagal %d baris", berhasil, len(laporanGagal)),
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, resp)
			return
		}

		var pesanFlash string
		if len(laporanGagal) > 0 {
			var errMsgs []string
			for _, g := range laporanGagal {
				errMsgs = append(errMsgs, fmt.Sprintf("Baris %d: %s", g.Baris, g.Pesan))
			}
			pesanFlash = fmt.Sprintf("Berhasil impor %d siswa. Gagal %d baris: %s", berhasil, len(laporanGagal), strings.Join(errMsgs, "; "))
			setFlash(sm, r, "error", pesanFlash)
		} else {
			pesanFlash = fmt.Sprintf("Berhasil mengimpor seluruh %d siswa", berhasil)
			setFlash(sm, r, "success", pesanFlash)
		}

		http.Redirect(w, r, "/admin/siswa", http.StatusFound)
	}
}

// SiswaKartuCetakHandler menampilkan halaman kartu peserta ujian yang siap dicetak/PDF.
func SiswaKartuCetakHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kelasFilter := r.URL.Query().Get("kelas")
		list, err := ListSiswa(r.Context(), db, kelasFilter)
		if err != nil {
			serverError(w, r, err)
			return
		}

		_ = web.Render(w, "admin_siswa_kartu.gohtml", map[string]any{
			"SiswaList": list,
			"Kelas":     kelasFilter,
			"Username":  sm.GetString(r.Context(), "username"),
		})
	}
}

// SiswaResetPINMassalHandler menghasilkan PIN baru massal untuk kelas tertentu atau seluruh siswa,
// menyimpannya ke database, dan mengembalikan file Excel (.xlsx) untuk diunduh panitia.
func SiswaResetPINMassalHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		kelasFilter := strings.TrimSpace(r.FormValue("kelas"))

		daftar, err := ResetPINMassal(r.Context(), db, kelasFilter)
		if err != nil {
			serverError(w, r, err)
			return
		}

		items := make([]excel.SiswaPINItem, len(daftar))
		for i, s := range daftar {
			items[i] = excel.SiswaPINItem{
				NIS:   s.NIS,
				Nama:  s.Nama,
				Kelas: s.Kelas,
				PIN:   s.PIN,
			}
		}

		buf, err := excel.EksporDaftarPIN(items)
		if err != nil {
			serverError(w, r, err)
			return
		}

		filename := "Daftar_PIN_Siswa.xlsx"
		if kelasFilter != "" {
			filename = fmt.Sprintf("Daftar_PIN_Siswa_Kelas_%s.xlsx", kelasFilter)
		}

		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
		_, _ = w.Write(buf.Bytes())
	}
}
