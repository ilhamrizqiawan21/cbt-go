// Package soal mengelola paket soal ujian untuk siswa, pengacakan deterministik,
// serta sanitasi kunci jawaban agar tidak pernah bocor ke klien.
package soal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"

	"cbt-go/internal/sesi"
)

var (
	ErrUjianWaktuInvalid = errors.New("soal: ujian sudah berakhir atau belum dimulai")
	ErrSoalNotFound      = errors.New("soal: soal tidak ditemukan")
	ErrUjianNotFound     = errors.New("soal: ujian tidak ditemukan")
)

// PilihanKlien adalah opsi pilihan ganda yang aman dikirim ke klien (tanpa kunci).
type PilihanKlien struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// SoalKlien adalah representasi paket soal untuk dikirim ke browser siswa (GET /api/soal/:ujianId).
// Field jawaban_benar TIDAK ADA di struct ini demi mencegah kebocoran kunci jawaban (NFR-04).
type SoalKlien struct {
	ID        int            `json:"id"`
	Tipe      string         `json:"tipe"`
	TeksSoal  string         `json:"teks_soal"`
	Poin      int            `json:"poin"`
	Gambar    *string        `json:"gambar"`
	Pilihan   []PilihanKlien `json:"pilihan,omitempty"`
	Kiri      []string       `json:"kiri,omitempty"`
	OpsiKanan []string       `json:"opsi_kanan,omitempty"`
}

// OpsiTambahan merupakan struktur opsi ekstra untuk soal menjodohkan.
type OpsiTambahan struct {
	Pasangan []struct {
		Kiri  string `json:"kiri"`
		Kanan string `json:"kanan"`
	} `json:"pasangan"`
	Pengecoh []string `json:"pengecoh"`
}

type soalRaw struct {
	id           int
	tipeSoal     string
	teksSoal     string
	gambar       sql.NullString
	poin         int
	pilihanA     sql.NullString
	pilihanB     sql.NullString
	pilihanC     sql.NullString
	pilihanD     sql.NullString
	opsiTambahan sql.NullString
}

// AcakSoal mengacak urutan daftar soal secara deterministik berdasarkan seed siswa.
func AcakSoal(daftar []SoalKlien, seed int) []SoalKlien {
	if len(daftar) <= 1 {
		return daftar
	}
	hasil := make([]SoalKlien, len(daftar))
	copy(hasil, daftar)

	rng := rand.New(rand.NewPCG(uint64(seed), 1))
	rng.Shuffle(len(hasil), func(i, j int) {
		hasil[i], hasil[j] = hasil[j], hasil[i]
	})
	return hasil
}

// AcakPilihan mengacak urutan pilihan ganda secara deterministik berdasarkan kombinasi seed dan ID soal.
func AcakPilihan(pilihan []PilihanKlien, seed int, soalID int) []PilihanKlien {
	if len(pilihan) <= 1 {
		return pilihan
	}
	hasil := make([]PilihanKlien, len(pilihan))
	copy(hasil, pilihan)

	rng := rand.New(rand.NewPCG(uint64(seed)^uint64(soalID), 2))
	rng.Shuffle(len(hasil), func(i, j int) {
		hasil[i], hasil[j] = hasil[j], hasil[i]
	})
	return hasil
}

// AcakKanan mengacak urutan opsi jawaban kanan pada soal menjodohkan secara deterministik.
func AcakKanan(kanan []string, seed int, soalID int) []string {
	if len(kanan) <= 1 {
		return kanan
	}
	hasil := make([]string, len(kanan))
	copy(hasil, kanan)

	rng := rand.New(rand.NewPCG(uint64(seed)^uint64(soalID), 3))
	rng.Shuffle(len(hasil), func(i, j int) {
		hasil[i], hasil[j] = hasil[j], hasil[i]
	})
	return hasil
}

// FormatMenjodohkan mem-parsing opsi_tambahan JSON dan mengacak opsi_kanan agar kunci pasangan tidak terbaca langsung.
func FormatMenjodohkan(opsiTambahanJSON string, seed int, soalID int) (kiri []string, opsiKanan []string, err error) {
	kiri = make([]string, 0)
	opsiKanan = make([]string, 0)

	if opsiTambahanJSON == "" {
		return kiri, opsiKanan, nil
	}

	var opsi OpsiTambahan
	if err := json.Unmarshal([]byte(opsiTambahanJSON), &opsi); err != nil {
		return kiri, opsiKanan, nil
	}

	seen := make(map[string]bool)
	var rawKanan []string

	for _, p := range opsi.Pasangan {
		kiri = append(kiri, p.Kiri)
		if !seen[p.Kanan] {
			seen[p.Kanan] = true
			rawKanan = append(rawKanan, p.Kanan)
		}
	}

	for _, pengecoh := range opsi.Pengecoh {
		if !seen[pengecoh] {
			seen[pengecoh] = true
			rawKanan = append(rawKanan, pengecoh)
		}
	}

	opsiKanan = AcakKanan(rawKanan, seed, soalID)
	return kiri, opsiKanan, nil
}

// CekWaktuUjian memeriksa apakah saat ini ujian dapat diakses siswa: di antara tanggal_mulai dan
// tanggal_selesai, atau siswa memiliki izin susulan yang masih berlaku (G-026).
func CekWaktuUjian(ctx context.Context, db *sql.DB, siswaID, ujianID int) (bool, error) {
	var valid bool
	err := db.QueryRowContext(ctx, `
		SELECT `+sesi.KondisiJendelaSiswa("u")+`
		FROM ujian u
		WHERE u.id = ?
	`, siswaID, ujianID).Scan(&valid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrUjianNotFound
		}
		return false, fmt.Errorf("cek waktu ujian: %w", err)
	}
	return valid, nil
}

// AmbilPaketSoal mengambil seluruh butir soal ujian untuk siswa, menerapkan acak deterministik (bila aktif),
// dan memformatnya tanpa kunci jawaban.
func AmbilPaketSoal(ctx context.Context, db *sql.DB, siswaID, ujianID int) ([]SoalKlien, error) {
	// 1. Validasi jendela waktu ujian
	valid, err := CekWaktuUjian(ctx, db, siswaID, ujianID)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, ErrUjianWaktuInvalid
	}

	// 2. Ambil pengaturan acak ujian
	var acakSoal, acakPilihan int
	err = db.QueryRowContext(ctx, `
		SELECT acak_soal, acak_pilihan
		FROM ujian
		WHERE id = ?
	`, ujianID).Scan(&acakSoal, &acakPilihan)
	if err != nil {
		return nil, fmt.Errorf("ambil konfigurasi acak ujian: %w", err)
	}

	// 3. Ambil atau inisialisasi seed acak persisten siswa
	var seed int
	err = db.QueryRowContext(ctx, `
		SELECT seed
		FROM sesi_ujian
		WHERE siswa_id = ? AND ujian_id = ?
	`, siswaID, ujianID).Scan(&seed)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("ambil seed sesi: %w", err)
	}

	if seed == 0 {
		var genErr error
		seed, genErr = sesi.GenerateSeed()
		if genErr != nil {
			return nil, genErr
		}
		_, _ = db.ExecContext(ctx, `
			UPDATE sesi_ujian SET seed = ? WHERE siswa_id = ? AND ujian_id = ?
		`, seed, siswaID, ujianID)
	}

	// 4. Query butir soal (tanpa kolom jawaban_benar!)
	rows, err := db.QueryContext(ctx, `
		SELECT id, tipe_soal, teks_soal, gambar, poin,
		       pilihan_a, pilihan_b, pilihan_c, pilihan_d,
		       opsi_tambahan
		FROM soal
		WHERE ujian_id = ?
		ORDER BY id ASC
	`, ujianID)
	if err != nil {
		return nil, fmt.Errorf("query butir soal: %w", err)
	}
	defer rows.Close()

	var daftarRaw []soalRaw
	for rows.Next() {
		var r soalRaw
		err := rows.Scan(
			&r.id, &r.tipeSoal, &r.teksSoal, &r.gambar, &r.poin,
			&r.pilihanA, &r.pilihanB, &r.pilihanC, &r.pilihanD,
			&r.opsiTambahan,
		)
		if err != nil {
			return nil, fmt.Errorf("scan soal: %w", err)
		}
		daftarRaw = append(daftarRaw, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterasi soal: %w", err)
	}

	if len(daftarRaw) == 0 {
		return nil, ErrSoalNotFound
	}

	// 5. Susun format SoalKlien
	hasil := make([]SoalKlien, 0, len(daftarRaw))
	for _, s := range daftarRaw {
		var gambarPtr *string
		if s.gambar.Valid && s.gambar.String != "" {
			g := s.gambar.String
			gambarPtr = &g
		}

		klien := SoalKlien{
			ID:       s.id,
			Tipe:     s.tipeSoal,
			TeksSoal: s.teksSoal,
			Poin:     s.poin,
			Gambar:   gambarPtr,
		}

		switch s.tipeSoal {
		case "pg":
			pilihan := []PilihanKlien{
				{Key: "A", Text: s.pilihanA.String},
				{Key: "B", Text: s.pilihanB.String},
				{Key: "C", Text: s.pilihanC.String},
				{Key: "D", Text: s.pilihanD.String},
			}
			if acakPilihan == 1 {
				pilihan = AcakPilihan(pilihan, seed, s.id)
			}
			klien.Pilihan = pilihan

		case "menjodohkan":
			kiri, kanan, _ := FormatMenjodohkan(s.opsiTambahan.String, seed, s.id)
			klien.Kiri = kiri
			klien.OpsiKanan = kanan

		case "essay":
			// Essay dijawab di kertas (D-006, FR-09), tidak memerlukan opsi
		}

		hasil = append(hasil, klien)
	}

	// 6. Terapkan acak soal bila aktif pada ujian
	if acakSoal == 1 {
		hasil = AcakSoal(hasil, seed)
	}

	return hasil, nil
}
