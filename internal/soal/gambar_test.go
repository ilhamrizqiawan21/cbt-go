package soal_test

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cbt-go/internal/soal"
)

func gantiDirGambar(t *testing.T) string {
	t.Helper()
	asli := soal.DirGambar
	dir := filepath.Join(t.TempDir(), "soal")
	soal.DirGambar = dir
	t.Cleanup(func() { soal.DirGambar = asli })
	return dir
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSimpanGambar(t *testing.T) {
	dir := gantiDirGambar(t)

	webp := append([]byte("RIFF\x1a\x00\x00\x00WEBPVP8 "), make([]byte, 20)...)
	tests := []struct {
		nama    string
		data    []byte
		wantErr error
		wantExt string
	}{
		{"png valid", pngBytes(t, 10, 10), nil, ".png"},
		{"webp valid", webp, nil, ".webp"},
		{"kosong", nil, soal.ErrGambarKosong, ""},
		{"svg ditolak", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), soal.ErrGambarTipe, ""},
		{"html menyamar", []byte("<html><script>alert(1)</script></html>"), soal.ErrGambarTipe, ""},
		{"png terpotong", pngBytes(t, 10, 10)[:20], soal.ErrGambarRusak, ""},
		{"dimensi terlalu besar", pngBytes(t, 4001, 1), soal.ErrGambarRusak, ""},
		{"lebih dari 1 MB", append(pngBytes(t, 10, 10), bytes.Repeat([]byte{0}, soal.MaksUkuranGambar)...), soal.ErrGambarTerlaluBesar, ""},
	}
	for _, tc := range tests {
		t.Run(tc.nama, func(t *testing.T) {
			nama, err := soal.SimpanGambar(bytes.NewReader(tc.data))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			if !soal.NamaGambarValid(nama) || filepath.Ext(nama) != tc.wantExt {
				t.Errorf("nama = %q, ekstensi diharapkan %s", nama, tc.wantExt)
			}
			if _, err := os.Stat(filepath.Join(dir, nama)); err != nil {
				t.Errorf("berkas tidak tersimpan: %v", err)
			}
		})
	}

	// Dua unggahan identik tetap menghasilkan nama berbeda (acak).
	a, _ := soal.SimpanGambar(bytes.NewReader(pngBytes(t, 5, 5)))
	b, _ := soal.SimpanGambar(bytes.NewReader(pngBytes(t, 5, 5)))
	if a == b {
		t.Error("nama berkas harus acak")
	}

	// Salin menghasilkan berkas baru; hapus salinan tidak menyentuh aslinya.
	salin, err := soal.SalinBerkasGambar(a)
	if err != nil || salin == a {
		t.Fatalf("SalinBerkasGambar = %q, %v", salin, err)
	}
	soal.HapusBerkasGambar(salin)
	if _, err := os.Stat(filepath.Join(dir, a)); err != nil {
		t.Errorf("berkas asli ikut terhapus: %v", err)
	}
}

func TestNamaGambarValid(t *testing.T) {
	ok := strings.Repeat("a", 32) + ".png"
	for nama, want := range map[string]bool{
		ok:                                true,
		strings.Repeat("0f", 16) + ".jpg": true,
		strings.Repeat("a", 32) + ".webp": true,
		strings.Repeat("a", 32) + ".svg":  false,
		strings.Repeat("A", 32) + ".png":  false,
		"../" + ok:                        false,
		"/etc/passwd":                     false,
		ok + "/..":                        false,
		strings.Repeat("a", 31) + ".png":  false,
		"":                                false,
	} {
		if got := soal.NamaGambarValid(nama); got != want {
			t.Errorf("NamaGambarValid(%q) = %v, want %v", nama, got, want)
		}
	}
}

func TestValidasiGambarSoal(t *testing.T) {
	buat := func(g string) *soal.SoalKelola {
		return &soal.SoalKelola{TipeSoal: "essay", TeksSoal: "x", Gambar: &g}
	}
	jahat := "../../etc/passwd"
	if err := soal.ValidasiDanNormalisasi(buat(jahat)); !errors.Is(err, soal.ErrNamaGambarInvalid) {
		t.Errorf("jalur traversal harus ditolak, err = %v", err)
	}
	s := buat("   ")
	if err := soal.ValidasiDanNormalisasi(s); err != nil || s.Gambar != nil {
		t.Errorf("gambar kosong harus menjadi nil: err=%v gambar=%v", err, s.Gambar)
	}
	s = buat(strings.Repeat("b", 32) + ".jpg")
	if err := soal.ValidasiDanNormalisasi(s); err != nil || s.Gambar == nil {
		t.Errorf("nama valid harus lolos: %v", err)
	}
}
