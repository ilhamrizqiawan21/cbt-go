package guru

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/admin"
	"cbt-go/internal/excel"
	"cbt-go/internal/soal"
)

// RegisterGuruEssayRoutes mendaftarkan seluruh rute penilaian essay guru.
func RegisterGuruEssayRoutes(mux *http.ServeMux, db *sql.DB, sm *scs.SessionManager, requireGuru func(http.Handler) http.Handler) {
	mux.Handle("GET /guru/essay", requireGuru(GuruEssayListPageHandler(db, sm)))
	mux.Handle("GET /guru/essay/{ujianId}", requireGuru(GuruEssayDetailHandler(db, sm)))
	mux.Handle("POST /guru/essay/{ujianId}", requireGuru(GuruEssayNilaiHandler(db, sm)))
	mux.Handle("GET /guru/essay/{ujianId}/export", requireGuru(GuruEssayExportHandler(db, sm)))
	mux.Handle("POST /guru/essay/{ujianId}/import", requireGuru(GuruEssayImportHandler(db, sm)))
}

// GuruEssayListPageHandler menampilkan ujian milik guru beserta progres koreksi essay.
func GuruEssayListPageHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
		if wantsJSON(r) {
			listUjian, err := soal.ListUjianByGuru(r.Context(), db, guruID)
			if err != nil {
				serverError(w, r, err)
				return
			}
			writeJSON(w, http.StatusOK, listUjian)
			return
		}

		list, err := ListRingkasanUjianGuru(r.Context(), db, guruID)
		if err != nil {
			serverError(w, r, err)
			return
		}
		data := dataHalaman(sm, r, "essay")
		data["UjianList"] = list
		render(w, r, "guru_essay_list.gohtml", data)
	}
}

// SoalEssayRingkas dipakai halaman koreksi untuk filter dan judul per soal.
type SoalEssayRingkas struct {
	ID       int
	Nomor    int
	TeksSoal string
	PoinMaks int
}

// GuruEssayDetailHandler menampilkan lembar koreksi essay satu ujian (siswa yang sudah selesai).
func GuruEssayDetailHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
		ujianID, err := strconv.Atoi(r.PathValue("ujianId"))
		if err != nil || ujianID <= 0 {
			http.Error(w, "ID ujian tidak valid", http.StatusBadRequest)
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

		list, err := GetJawabanEssayUjian(r.Context(), db, ujianID)
		if err != nil {
			serverError(w, r, err)
			return
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, list)
			return
		}

		u, err := admin.GetUjianByID(r.Context(), db, ujianID)
		if err != nil {
			serverError(w, r, err)
			return
		}

		// Daftar soal essay unik (urutan sesuai id) dan hitungan yang belum dinilai.
		var daftarSoal []SoalEssayRingkas
		dilihat := map[int]bool{}
		siswa := map[int]bool{}
		belum := 0
		for _, row := range list {
			if !dilihat[row.SoalID] {
				dilihat[row.SoalID] = true
				daftarSoal = append(daftarSoal, SoalEssayRingkas{ID: row.SoalID, Nomor: len(daftarSoal) + 1, TeksSoal: row.TeksSoal, PoinMaks: row.PoinMaks})
			}
			siswa[row.SiswaID] = true
			if row.Skor == nil {
				belum++
			}
		}

		data := dataHalaman(sm, r, "essay")
		data["Ujian"] = u
		data["UjianID"] = ujianID
		data["JawabanList"] = list
		data["SoalList"] = daftarSoal
		data["JumlahSiswa"] = len(siswa)
		data["BelumDinilai"] = belum
		render(w, r, "guru_essay.gohtml", data)
	}
}

// GuruEssayNilaiHandler menyimpan nilai skor essay untuk seorang siswa dan butir soal tertentu.
func GuruEssayNilaiHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
		ujianStr := r.PathValue("ujianId")
		ujianID, err := strconv.Atoi(ujianStr)
		if err != nil || ujianID <= 0 {
			http.Error(w, "ID ujian tidak valid", http.StatusBadRequest)
			return
		}

		var payload struct {
			SiswaID int `json:"siswa_id"`
			SoalID  int `json:"soal_id"`
			Skor    int `json:"skor"`
		}

		if isJSONBody(r) {
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Payload JSON tidak valid"})
				return
			}
		} else {
			_ = r.ParseForm()
			sID, _ := strconv.Atoi(r.FormValue("siswa_id"))
			soalID, _ := strconv.Atoi(r.FormValue("soal_id"))
			skor, _ := strconv.Atoi(r.FormValue("skor"))
			payload.SiswaID = sID
			payload.SoalID = soalID
			payload.Skor = skor
		}

		hasil, err := SimpanNilaiEssay(r.Context(), db, guruID, ujianID, payload.SiswaID, payload.SoalID, payload.Skor)
		if err != nil {
			var status int
			switch {
			case errors.Is(err, soal.ErrForbiddenGuru):
				status = http.StatusForbidden
			case errors.Is(err, ErrSkorInvalid), errors.Is(err, ErrSesiBelumSelesai), errors.Is(err, ErrSoalEssayTidakAda):
				status = http.StatusBadRequest
			default:
				serverError(w, r, err)
				return
			}
			if wantsJSON(r) || isJSONBody(r) {
				writeJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
			setFlash(sm, r, "error", err.Error())
			http.Redirect(w, r, fmt.Sprintf("/guru/essay/%d", ujianID), http.StatusFound)
			return
		}

		resp := map[string]any{
			"message":        "Nilai essay berhasil disimpan",
			"status_koreksi": hasil.StatusKoreksi,
			"nilai_akhir":    hasil.Nilai,
			"poin_essay":     hasil.PoinEssay,
		}

		if wantsJSON(r) || isJSONBody(r) {
			writeJSON(w, http.StatusOK, resp)
			return
		}
		setFlash(sm, r, "success", "Nilai essay berhasil disimpan")
		http.Redirect(w, r, fmt.Sprintf("/guru/essay/%d", ujianID), http.StatusFound)
	}
}

// GuruEssayExportHandler mengunduh formulir penilaian essay dalam bentuk Excel.
func GuruEssayExportHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
		ujianStr := r.PathValue("ujianId")
		ujianID, err := strconv.Atoi(ujianStr)
		if err != nil || ujianID <= 0 {
			http.Error(w, "ID ujian tidak valid", http.StatusBadRequest)
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

		rows, err := GetJawabanEssayUjian(r.Context(), db, ujianID)
		if err != nil {
			serverError(w, r, err)
			return
		}

		buf, err := excel.BuatTemplateEssay(u.NamaUjian, rows)
		if err != nil {
			serverError(w, r, fmt.Errorf("buat template essay: %w", err))
			return
		}

		filename := fmt.Sprintf("koreksi_essay_ujian_%d.xlsx", ujianID)
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}
}

// GuruEssayImportHandler mengimpor nilai skor essay dari berkas Excel.
func GuruEssayImportHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	const maxUploadSize = 2 << 20

	return func(w http.ResponseWriter, r *http.Request) {
		guruID := sm.GetInt(r.Context(), "guruId")
		ujianStr := r.PathValue("ujianId")
		ujianID, err := strconv.Atoi(ujianStr)
		if err != nil || ujianID <= 0 {
			http.Error(w, "ID ujian tidak valid", http.StatusBadRequest)
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

		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			msg := "Ukuran file melebihi batas 2 MB"
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
				return
			}
			setFlash(sm, r, "error", msg)
			http.Redirect(w, r, fmt.Sprintf("/guru/essay/%d", ujianID), http.StatusFound)
			return
		}

		file, handler, err := r.FormFile("file")
		if err != nil {
			file, handler, err = r.FormFile("excel")
		}
		if err != nil {
			msg := "Berkas Excel tidak ditemukan"
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
				return
			}
			setFlash(sm, r, "error", msg)
			http.Redirect(w, r, fmt.Sprintf("/guru/essay/%d", ujianID), http.StatusFound)
			return
		}
		defer file.Close()

		if strings.ToLower(filepath.Ext(handler.Filename)) != ".xlsx" {
			msg := "Hanya berkas .xlsx yang diizinkan"
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
				return
			}
			setFlash(sm, r, "error", msg)
			http.Redirect(w, r, fmt.Sprintf("/guru/essay/%d", ujianID), http.StatusFound)
			return
		}

		items, parseErrs, err := excel.BacaImportEssay(file)
		if err != nil {
			msg := fmt.Sprintf("Gagal membaca file Excel: %v", err)
			if wantsJSON(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
				return
			}
			setFlash(sm, r, "error", msg)
			http.Redirect(w, r, fmt.Sprintf("/guru/essay/%d", ujianID), http.StatusFound)
			return
		}

		sukses, dbErrs, err := BatchSimpanNilaiEssay(r.Context(), db, guruID, ujianID, items)
		if err != nil {
			serverError(w, r, fmt.Errorf("simpan batch nilai essay: %w", err))
			return
		}

		semuaErrs := append(parseErrs, dbErrs...)
		resp := map[string]any{
			"sukses": sukses,
			"errors": semuaErrs,
			"pesan":  fmt.Sprintf("Berhasil menilai %d jawaban essay", sukses),
		}

		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, resp)
			return
		}

		if len(semuaErrs) > 0 {
			setFlash(sm, r, "error", ringkasErrorImpor(sukses, semuaErrs))
		} else {
			setFlash(sm, r, "success", fmt.Sprintf("Berhasil mengimpor %d nilai essay.", sukses))
		}
		http.Redirect(w, r, fmt.Sprintf("/guru/essay/%d", ujianID), http.StatusFound)
	}
}

// ringkasErrorImpor menyusun pesan flash: jumlah sukses + maksimal 5 baris bermasalah pertama.
func ringkasErrorImpor(sukses int, errs []excel.ImportError) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Berhasil %d baris; %d baris bermasalah: ", sukses, len(errs))
	for i, e := range errs {
		if i == 5 {
			fmt.Fprintf(&b, " … (+%d lainnya)", len(errs)-5)
			break
		}
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "baris %d: %s", e.Baris, e.Pesan)
	}
	return b.String()
}
