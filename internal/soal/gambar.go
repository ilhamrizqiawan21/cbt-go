package soal

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // mendaftarkan decoder untuk image.DecodeConfig
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

// MaksUkuranGambar adalah batas unggahan gambar soal (G-023): 1 MB demi kuota siswa (NFR-03).
const MaksUkuranGambar = 1 << 20

// maksSisiGambar membatasi dimensi png/jpg agar tidak membebani memori peramban siswa.
const maksSisiGambar = 4000

// DirGambar adalah folder penyimpanan gambar soal. Diatur sekali saat startup (httpserver);
// nilai awal cocok untuk menjalankan server dari root proyek.
var DirGambar = filepath.Join("uploads", "soal")

var (
	ErrGambarKosong       = errors.New("berkas gambar kosong")
	ErrGambarTerlaluBesar = errors.New("ukuran gambar maksimal 1 MB")
	ErrGambarTipe         = errors.New("gambar harus berformat PNG, JPG, atau WEBP")
	ErrGambarRusak        = errors.New("berkas gambar tidak dapat dibaca atau dimensinya terlalu besar")
	ErrNamaGambarInvalid  = errors.New("nama gambar tidak valid")
)

// namaGambarRE: 32 heksadesimal acak + ekstensi yang diizinkan. Satu-satunya bentuk nama
// yang boleh tersimpan di soal.gambar atau dilayani rute gambar (mencegah path traversal).
var namaGambarRE = regexp.MustCompile(`^[0-9a-f]{32}\.(png|jpg|webp)$`)

// NamaGambarValid melaporkan apakah nama sesuai format yang dibuat SimpanGambar.
func NamaGambarValid(nama string) bool { return namaGambarRE.MatchString(nama) }

// SimpanGambar membaca maksimal MaksUkuranGambar byte dari r, memeriksa tipe dari ISI berkas
// (bukan ekstensi atau header klien), lalu menyimpannya dengan nama acak. SVG ditolak karena
// dapat memuat skrip. Mengembalikan nama berkas (bukan jalur).
func SimpanGambar(r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaksUkuranGambar+1))
	if err != nil {
		return "", fmt.Errorf("baca gambar: %w", err)
	}
	if len(data) == 0 {
		return "", ErrGambarKosong
	}
	if len(data) > MaksUkuranGambar {
		return "", ErrGambarTerlaluBesar
	}

	var ext string
	switch http.DetectContentType(data) {
	case "image/png":
		ext = "png"
	case "image/jpeg":
		ext = "jpg"
	case "image/webp":
		ext = "webp"
	default:
		return "", ErrGambarTipe
	}

	// png/jpg: periksa header dan dimensi tanpa mendekode seluruh piksel.
	if ext != "webp" {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maksSisiGambar || cfg.Height > maksSisiGambar {
			return "", ErrGambarRusak
		}
	}

	if err := os.MkdirAll(DirGambar, 0o755); err != nil {
		return "", fmt.Errorf("buat folder gambar: %w", err)
	}
	acak := make([]byte, 16)
	if _, err := rand.Read(acak); err != nil {
		return "", fmt.Errorf("acak nama gambar: %w", err)
	}
	nama := hex.EncodeToString(acak) + "." + ext

	f, err := os.OpenFile(filepath.Join(DirGambar, nama), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("buat berkas gambar: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", fmt.Errorf("tulis gambar: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("tutup gambar: %w", err)
	}
	return nama, nil
}

// JalurGambar mengembalikan jalur berkas di disk untuk nama yang valid.
func JalurGambar(nama string) (string, error) {
	if !NamaGambarValid(nama) {
		return "", ErrNamaGambarInvalid
	}
	return filepath.Join(DirGambar, nama), nil
}

// HapusBerkasGambar menghapus berkas gambar (diam bila tidak ada). Nama tidak valid diabaikan.
func HapusBerkasGambar(nama string) {
	if p, err := JalurGambar(nama); err == nil {
		_ = os.Remove(p)
	}
}

// SalinBerkasGambar menggandakan berkas gambar ke nama acak baru (dipakai salin soal, T6.3)
// agar tiap soal memiliki berkasnya sendiri dan penghapusan satu soal tidak merusak yang lain.
func SalinBerkasGambar(nama string) (string, error) {
	p, err := JalurGambar(nama)
	if err != nil {
		return "", err
	}
	f, err := os.Open(p)
	if err != nil {
		return "", fmt.Errorf("buka gambar sumber: %w", err)
	}
	defer f.Close()
	return SimpanGambar(f)
}
