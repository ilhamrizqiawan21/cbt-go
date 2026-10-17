package soal

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// UnggahGambarHandler menerima multipart/form-data dengan field berkas "gambar" dan membalas
// {"gambar": "<nama>"}. izin dipanggil lebih dulu (otorisasi per peran); nil berarti diizinkan.
// Nama yang dikembalikan lalu dikirim bersama form soal (field "gambar") untuk disimpan.
func UnggahGambarHandler(izin func(r *http.Request) (bool, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if izin != nil {
			boleh, err := izin(r)
			if err != nil {
				slog.Error("otorisasi unggah gambar", "err", err)
				jsonGambar(w, http.StatusInternalServerError, map[string]string{"error": "terjadi kesalahan pada server"})
				return
			}
			if !boleh {
				jsonGambar(w, http.StatusForbidden, map[string]string{"error": "akses ditolak"})
				return
			}
		}

		// Sisihkan 64 KB untuk overhead multipart di atas batas isi gambar.
		r.Body = http.MaxBytesReader(w, r.Body, MaksUkuranGambar+64<<10)
		if err := r.ParseMultipartForm(MaksUkuranGambar + 64<<10); err != nil {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				jsonGambar(w, http.StatusRequestEntityTooLarge, map[string]string{"error": ErrGambarTerlaluBesar.Error()})
				return
			}
			jsonGambar(w, http.StatusBadRequest, map[string]string{"error": "form unggahan tidak valid"})
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}

		f, _, err := r.FormFile("gambar")
		if err != nil {
			jsonGambar(w, http.StatusBadRequest, map[string]string{"error": "berkas gambar wajib diisi"})
			return
		}
		defer f.Close()

		nama, err := SimpanGambar(f)
		switch {
		case err == nil:
			jsonGambar(w, http.StatusCreated, map[string]string{"gambar": nama})
		case errors.Is(err, ErrGambarKosong), errors.Is(err, ErrGambarTipe), errors.Is(err, ErrGambarRusak):
			jsonGambar(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		case errors.Is(err, ErrGambarTerlaluBesar):
			jsonGambar(w, http.StatusRequestEntityTooLarge, map[string]string{"error": err.Error()})
		default:
			slog.Error("simpan gambar soal", "err", err)
			jsonGambar(w, http.StatusInternalServerError, map[string]string{"error": "terjadi kesalahan pada server"})
		}
	}
}

func jsonGambar(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
