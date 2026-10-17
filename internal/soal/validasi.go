package soal

import (
	"encoding/json"
	"errors"
	"strings"
)

var (
	ErrPilihanPGKurang      = errors.New("soal pilihan ganda membutuhkan minimal dua pilihan jawaban")
	ErrKunciPGInvalid       = errors.New("kunci jawaban pilihan ganda harus A, B, C, atau D dan pilihannya tidak boleh kosong")
	ErrPasanganKurang       = errors.New("soal menjodohkan membutuhkan minimal dua pasangan dengan sisi kiri dan kanan terisi")
	ErrPasanganKiriGanda    = errors.New("sisi kiri pasangan menjodohkan tidak boleh sama")
	ErrOpsiMenjodohkanRusak = errors.New("format pasangan menjodohkan tidak valid")
)

type pasanganSoal struct {
	Kiri  string `json:"kiri"`
	Kanan string `json:"kanan"`
}

// ValidasiDanNormalisasi memeriksa kelengkapan soal sesuai tipenya dan merapikan field
// agar tidak ada sisa data tipe lain. Fungsi murni (tanpa DB), dipanggil TambahSoal/EditSoal.
//   - pg: ≥2 pilihan terisi, kunci A–D (huruf besar) yang pilihannya terisi; opsi_tambahan dikosongkan.
//   - menjodohkan: ≥2 pasangan lengkap dengan kiri unik. Sumber: opsi_tambahan {"pasangan","pengecoh"};
//     bila kosong, jawaban_benar berupa array pasangan diterima. jawaban_benar selalu diturunkan
//     ulang dari pasangan (format yang dipakai penilaian) agar keduanya tidak berselisih.
//   - essay: pilihan, kunci, dan opsi dikosongkan (essay dinilai guru, D-006).
func ValidasiDanNormalisasi(s *SoalKelola) error {
	// Gambar hanya boleh berupa nama berkas buatan SimpanGambar (G-023); jalur lain ditolak
	// agar rute penyaji gambar tidak dapat diarahkan ke berkas sembarang.
	if s.Gambar != nil {
		if g := strings.TrimSpace(*s.Gambar); g == "" {
			s.Gambar = nil
		} else if !NamaGambarValid(g) {
			return ErrNamaGambarInvalid
		} else {
			s.Gambar = &g
		}
	}
	switch s.TipeSoal {
	case "pg":
		pilihan := map[string]*string{"A": s.PilihanA, "B": s.PilihanB, "C": s.PilihanC, "D": s.PilihanD}
		terisi := 0
		for _, p := range pilihan {
			if p != nil && strings.TrimSpace(*p) != "" {
				terisi++
			}
		}
		if terisi < 2 {
			return ErrPilihanPGKurang
		}
		if s.JawabanBenar == nil {
			return ErrKunciPGInvalid
		}
		kunci := strings.ToUpper(strings.TrimSpace(*s.JawabanBenar))
		p, ok := pilihan[kunci]
		if !ok || p == nil || strings.TrimSpace(*p) == "" {
			return ErrKunciPGInvalid
		}
		s.JawabanBenar = &kunci
		s.OpsiTambahan = nil

	case "menjodohkan":
		var opsi struct {
			Pasangan []pasanganSoal `json:"pasangan"`
			Pengecoh []string       `json:"pengecoh"`
		}
		switch {
		case s.OpsiTambahan != nil && strings.TrimSpace(*s.OpsiTambahan) != "":
			if err := json.Unmarshal([]byte(*s.OpsiTambahan), &opsi); err != nil {
				return ErrOpsiMenjodohkanRusak
			}
		case s.JawabanBenar != nil && strings.TrimSpace(*s.JawabanBenar) != "":
			if err := json.Unmarshal([]byte(*s.JawabanBenar), &opsi.Pasangan); err != nil {
				return ErrOpsiMenjodohkanRusak
			}
		}

		bersih := make([]pasanganSoal, 0, len(opsi.Pasangan))
		kiriDipakai := map[string]bool{}
		for _, p := range opsi.Pasangan {
			p.Kiri, p.Kanan = strings.TrimSpace(p.Kiri), strings.TrimSpace(p.Kanan)
			if p.Kiri == "" && p.Kanan == "" {
				continue
			}
			if p.Kiri == "" || p.Kanan == "" {
				return ErrPasanganKurang
			}
			if kiriDipakai[p.Kiri] {
				return ErrPasanganKiriGanda
			}
			kiriDipakai[p.Kiri] = true
			bersih = append(bersih, p)
		}
		if len(bersih) < 2 {
			return ErrPasanganKurang
		}
		pengecoh := make([]string, 0, len(opsi.Pengecoh))
		for _, x := range opsi.Pengecoh {
			if x = strings.TrimSpace(x); x != "" {
				pengecoh = append(pengecoh, x)
			}
		}

		kunciJSON, err := json.Marshal(bersih)
		if err != nil {
			return err
		}
		opsiJSON, err := json.Marshal(map[string]any{"pasangan": bersih, "pengecoh": pengecoh})
		if err != nil {
			return err
		}
		k, o := string(kunciJSON), string(opsiJSON)
		s.JawabanBenar, s.OpsiTambahan = &k, &o
		s.PilihanA, s.PilihanB, s.PilihanC, s.PilihanD = nil, nil, nil, nil

	case "essay":
		s.PilihanA, s.PilihanB, s.PilihanC, s.PilihanD = nil, nil, nil, nil
		s.JawabanBenar, s.OpsiTambahan = nil, nil
	}
	return nil
}
