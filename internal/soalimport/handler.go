// Package soalimport menyediakan handler HTTP impor soal dari Excel yang dipakai bersama oleh
// admin dan guru; perbedaannya hanya otorisasi dan alamat kembali (G-024).
package soalimport

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/excel"
	"cbt-go/internal/soal"
)

const maksUnggah = 2 << 20 // 2 MB

// Opsi mengikat handler ke peran pemakainya.
type Opsi struct {
	SM *scs.SessionManager
	DB *sql.DB
	// Izin memeriksa hak atas ujian (admin: selalu true; guru: pengampu ujian).
	Izin func(r *http.Request, ujianID int) (bool, error)
	// Kembali menghasilkan alamat halaman bank soal untuk ujian tertentu.
	Kembali func(ujianID int) string
}

// TemplateHandler mengunduh template .xlsx.
func TemplateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		buf, err := excel.BuatTemplateSoal()
		if err != nil {
			slog.Error("template soal", "err", err)
			http.Error(w, "terjadi kesalahan pada server", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", `attachment; filename="template_soal.xlsx"`)
		_, _ = w.Write(buf.Bytes())
	}
}

// ImportHandler memproses POST multipart: field ujian_id dan berkas "file" (.xlsx, maks 2 MB).
// Semua-atau-tidak-sama-sekali: satu baris salah berarti tidak ada soal yang disimpan.
func ImportHandler(o Opsi) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wantJSON := strings.Contains(r.Header.Get("Accept"), "application/json")
		gagal := func(code int, ujianID int, pesan string, detail []excel.ImportError) {
			if wantJSON {
				writeJSON(w, code, map[string]any{"error": pesan, "errors": detail})
				return
			}
			o.SM.Put(r.Context(), "flash_error", ringkas(pesan, detail))
			http.Redirect(w, r, o.Kembali(ujianID), http.StatusFound)
		}

		r.Body = http.MaxBytesReader(w, r.Body, maksUnggah)
		if err := r.ParseMultipartForm(maksUnggah); err != nil {
			gagal(http.StatusBadRequest, 0, "Ukuran berkas melebihi batas 2 MB atau form tidak valid", nil)
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		ujianID, _ := strconv.Atoi(r.FormValue("ujian_id"))
		if ujianID <= 0 {
			gagal(http.StatusBadRequest, 0, "Ujian belum dipilih", nil)
			return
		}
		boleh, err := o.Izin(r, ujianID)
		if err != nil {
			slog.Error("otorisasi impor soal", "err", err)
			gagal(http.StatusInternalServerError, ujianID, "Terjadi kesalahan pada server", nil)
			return
		}
		if !boleh {
			gagal(http.StatusForbidden, 0, "Akses ditolak: Anda tidak memiliki hak atas ujian ini", nil)
			return
		}

		file, hdr, err := r.FormFile("file")
		if err != nil {
			gagal(http.StatusBadRequest, ujianID, "Berkas Excel tidak ditemukan dalam permintaan", nil)
			return
		}
		defer file.Close()
		if strings.ToLower(filepath.Ext(hdr.Filename)) != ".xlsx" {
			gagal(http.StatusBadRequest, ujianID, "Hanya berkas .xlsx yang diizinkan", nil)
			return
		}

		hasil, err := excel.BacaImportSoal(file)
		if err != nil {
			gagal(http.StatusBadRequest, ujianID, fmt.Sprintf("Gagal membaca berkas Excel: %v", err), nil)
			return
		}
		if len(hasil.Errors) > 0 {
			gagal(http.StatusUnprocessableEntity, ujianID, "Impor dibatalkan, tidak ada soal yang disimpan", hasil.Errors)
			return
		}
		if len(hasil.Items) == 0 {
			gagal(http.StatusBadRequest, ujianID, "Berkas tidak berisi soal", nil)
			return
		}

		daftar := make([]soal.SoalKelola, len(hasil.Items))
		for i, it := range hasil.Items {
			daftar[i] = it.Soal
		}
		n, err := soal.ImporSoalAtomik(r.Context(), o.DB, ujianID, daftar)
		if err != nil {
			slog.Error("impor soal", "err", err)
			gagal(http.StatusInternalServerError, ujianID, "Gagal menyimpan soal; tidak ada yang tersimpan", nil)
			return
		}
		if wantJSON {
			writeJSON(w, http.StatusCreated, map[string]any{"message": "Impor soal berhasil", "total": n})
			return
		}
		o.SM.Put(r.Context(), "flash_success", fmt.Sprintf("Berhasil mengimpor %d soal", n))
		http.Redirect(w, r, o.Kembali(ujianID), http.StatusFound)
	}
}

// ringkas membentuk satu pesan flash: pesan utama + maksimal 5 galat pertama.
func ringkas(pesan string, detail []excel.ImportError) string {
	if len(detail) == 0 {
		return pesan
	}
	var b strings.Builder
	b.WriteString(pesan + ". ")
	for i, e := range detail {
		if i == 5 {
			fmt.Fprintf(&b, "… dan %d galat lain.", len(detail)-5)
			break
		}
		fmt.Fprintf(&b, "Baris %d: %s. ", e.Baris, e.Pesan)
	}
	return strings.TrimSpace(b.String())
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// SalinHandler memproses POST: ujian_id (tujuan) dan dari_ujian_id (sumber). Izin diperiksa
// untuk KEDUA ujian, sehingga guru hanya dapat menyalin antar ujian yang diampunya.
func SalinHandler(o Opsi) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wantJSON := strings.Contains(r.Header.Get("Accept"), "application/json")
		selesai := func(code, ujianID int, kunci, pesan string) {
			if wantJSON {
				if code >= 400 {
					writeJSON(w, code, map[string]string{"error": pesan})
				} else {
					writeJSON(w, code, map[string]string{"message": pesan})
				}
				return
			}
			o.SM.Put(r.Context(), "flash_"+kunci, pesan)
			http.Redirect(w, r, o.Kembali(ujianID), http.StatusFound)
		}

		_ = r.ParseForm()
		tujuan, _ := strconv.Atoi(r.FormValue("ujian_id"))
		sumber, _ := strconv.Atoi(r.FormValue("dari_ujian_id"))
		if tujuan <= 0 || sumber <= 0 {
			selesai(http.StatusBadRequest, tujuan, "error", "Pilih ujian sumber dan tujuan")
			return
		}
		for _, id := range []int{tujuan, sumber} {
			boleh, err := o.Izin(r, id)
			if err != nil {
				slog.Error("otorisasi salin soal", "err", err)
				selesai(http.StatusInternalServerError, tujuan, "error", "Terjadi kesalahan pada server")
				return
			}
			if !boleh {
				selesai(http.StatusForbidden, 0, "error", "Akses ditolak: Anda tidak memiliki hak atas ujian ini")
				return
			}
		}

		n, err := soal.SalinSoal(r.Context(), o.DB, sumber, tujuan)
		switch {
		case err == nil:
			selesai(http.StatusCreated, tujuan, "success", fmt.Sprintf("Berhasil menyalin %d soal", n))
		case errors.Is(err, soal.ErrSalinUjianSama), errors.Is(err, soal.ErrSumberKosong):
			selesai(http.StatusBadRequest, tujuan, "error", err.Error())
		default:
			slog.Error("salin soal", "err", err)
			selesai(http.StatusInternalServerError, tujuan, "error", "Gagal menyalin soal; tidak ada yang tersimpan")
		}
	}
}
