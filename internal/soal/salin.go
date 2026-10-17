package soal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrSalinUjianSama = errors.New("ujian sumber dan tujuan tidak boleh sama")
	ErrSumberKosong   = errors.New("ujian sumber belum memiliki soal")
)

// SalinSoal menggandakan semua soal dari ujian sumber ke ujian tujuan (G-025) dalam satu
// transaksi. Sumber tidak diubah. Berkas gambar disalin menjadi berkas baru supaya tiap soal
// memiliki berkasnya sendiri. Bila gagal, berkas salinan yang sudah dibuat dibersihkan.
func SalinSoal(ctx context.Context, db *sql.DB, sumberID, tujuanID int) (int, error) {
	if db == nil {
		return 0, errors.New("database tidak tersedia")
	}
	if sumberID <= 0 || tujuanID <= 0 {
		return 0, errors.New("ujian_id tidak valid")
	}
	if sumberID == tujuanID {
		return 0, ErrSalinUjianSama
	}

	daftar, err := ListSoalByUjian(ctx, db, sumberID)
	if err != nil {
		return 0, err
	}
	if len(daftar) == 0 {
		return 0, ErrSumberKosong
	}

	var baru []string // berkas salinan, untuk dibersihkan bila transaksi gagal
	bersihkan := func() {
		for _, n := range baru {
			HapusBerkasGambar(n)
		}
	}

	for i := range daftar {
		g := daftar[i].Gambar
		if g == nil || *g == "" {
			daftar[i].Gambar = nil
			continue
		}
		salinan, err := SalinBerkasGambar(*g)
		if err != nil {
			bersihkan()
			return 0, fmt.Errorf("salin gambar soal %d: %w", daftar[i].ID, err)
		}
		baru = append(baru, salinan)
		daftar[i].Gambar = &salinan
	}

	n, err := ImporSoalAtomik(ctx, db, tujuanID, daftar)
	if err != nil {
		bersihkan()
		return 0, err
	}
	return n, nil
}
