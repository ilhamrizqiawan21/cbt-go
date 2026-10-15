package penilaian

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
)

// TipeSoal merepresentasikan jenis soal pada CBT.
type TipeSoal string

const (
	TipePG          TipeSoal = "pg"
	TipeMenjodohkan TipeSoal = "menjodohkan"
	TipeEssay       TipeSoal = "essay"
)

// StatusKoreksi menandakan apakah seluruh soal (termasuk essay) sudah selesai dinilai.
type StatusKoreksi string

const (
	StatusSelesai       StatusKoreksi = "selesai"
	StatusMenungguEssay StatusKoreksi = "menunggu_essay"
)

// Soal memuat data soal yang dibutuhkan untuk proses penilaian murni.
type Soal struct {
	ID           int      `json:"id"`
	TipeSoal     TipeSoal `json:"tipe_soal"`
	Poin         int      `json:"poin"`
	JawabanBenar string   `json:"jawaban_benar"` // Kunci jawaban (PG atau JSON string untuk menjodohkan)
}

// Pasangan merepresentasikan satu pasangan kunci/jawaban soal menjodohkan.
type Pasangan struct {
	Kiri  string `json:"kiri"`
	Kanan string `json:"kanan"`
}

// DetailSoal mencatat rincian penilaian per butir soal.
type DetailSoal struct {
	SoalID      int      `json:"soal_id"`
	TipeSoal    TipeSoal `json:"tipe_soal"`
	IsBenar     int      `json:"is_benar"`     // 1 = benar, 0 = salah/kosong
	PoinDidapat int      `json:"poin_didapat"` // Poin yang diperoleh siswa
	Status      string   `json:"status"`       // "benar", "salah", "kosong", "menunggu_koreksi", "dinilai"
}

// Hasil adalah keluaran dari fungsi murni Hitung.
type Hasil struct {
	PoinOtomatis  int           `json:"poin_otomatis"`
	PoinEssay     int           `json:"poin_essay"`
	PoinMaks      int           `json:"poin_maks"`
	Nilai         int           `json:"nilai"` // Skala 0-100 dengan pembulatan half-up
	Benar         int           `json:"benar"`
	Salah         int           `json:"salah"`
	Kosong        int           `json:"kosong"`
	StatusKoreksi StatusKoreksi `json:"status_koreksi"`
	DetailPerSoal []DetailSoal  `json:"detail_per_soal,omitempty"`
}

// CekJawabanPG memeriksa apakah jawaban pilihan ganda siswa cocok dengan kunci (case-insensitive & trim).
func CekJawabanPG(jawabanUser, jawabanBenar string) bool {
	jUser := strings.TrimSpace(jawabanUser)
	jBenar := strings.TrimSpace(jawabanBenar)
	if jUser == "" || jBenar == "" {
		return false
	}
	return strings.EqualFold(jUser, jBenar)
}

// CekJawabanMenjodohkan memeriksa apakah seluruh pasangan cocok (all-or-nothing, D-006, PRD §5).
// Urutan pasangan tidak mempengaruhi hasil penilaian.
func CekJawabanMenjodohkan(jawabanUserJSON, jawabanBenarJSON string) bool {
	var userList []Pasangan
	var benarList []Pasangan

	if err := json.Unmarshal([]byte(jawabanUserJSON), &userList); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(jawabanBenarJSON), &benarList); err != nil {
		return false
	}

	if len(userList) == 0 || len(userList) != len(benarList) {
		return false
	}

	normUser := normalisasiPasangan(userList)
	normBenar := normalisasiPasangan(benarList)

	for i := range normUser {
		if normUser[i] != normBenar[i] {
			return false
		}
	}

	return true
}

func normalisasiPasangan(list []Pasangan) []string {
	norm := make([]string, len(list))
	for i, p := range list {
		// Menggabungkan string kiri dan kanan yang telah di-trim dengan pembatas null byte
		norm[i] = strings.TrimSpace(p.Kiri) + "\x00" + strings.TrimSpace(p.Kanan)
	}
	sort.Strings(norm)
	return norm
}

// Hitung menjalankan penilaian secara murni tanpa efek samping I/O atau database.
func Hitung(soalList []Soal, jawabanMap map[int]string, skorEssay map[int]int) Hasil {
	var poinOtomatis, poinEssay, poinMaks int
	var benar, salah, kosong int
	var adaEssayBelumDinilai bool

	detailPerSoal := make([]DetailSoal, 0, len(soalList))

	for _, s := range soalList {
		maxPoin := s.Poin
		if maxPoin <= 0 {
			if s.TipeSoal == TipeEssay {
				maxPoin = 4
			} else {
				maxPoin = 2
			}
		}
		poinMaks += maxPoin

		jUser := ""
		if jawabanMap != nil {
			jUser = jawabanMap[s.ID]
		}

		switch s.TipeSoal {
		case TipePG:
			trimmed := strings.TrimSpace(jUser)
			if trimmed == "" {
				kosong++
				detailPerSoal = append(detailPerSoal, DetailSoal{
					SoalID:      s.ID,
					TipeSoal:    s.TipeSoal,
					IsBenar:     0,
					PoinDidapat: 0,
					Status:      "kosong",
				})
			} else if CekJawabanPG(trimmed, s.JawabanBenar) {
				benar++
				poinOtomatis += maxPoin
				detailPerSoal = append(detailPerSoal, DetailSoal{
					SoalID:      s.ID,
					TipeSoal:    s.TipeSoal,
					IsBenar:     1,
					PoinDidapat: maxPoin,
					Status:      "benar",
				})
			} else {
				salah++
				detailPerSoal = append(detailPerSoal, DetailSoal{
					SoalID:      s.ID,
					TipeSoal:    s.TipeSoal,
					IsBenar:     0,
					PoinDidapat: 0,
					Status:      "salah",
				})
			}

		case TipeMenjodohkan:
			trimmed := strings.TrimSpace(jUser)
			if trimmed == "" || trimmed == "[]" {
				kosong++
				detailPerSoal = append(detailPerSoal, DetailSoal{
					SoalID:      s.ID,
					TipeSoal:    s.TipeSoal,
					IsBenar:     0,
					PoinDidapat: 0,
					Status:      "kosong",
				})
			} else if CekJawabanMenjodohkan(trimmed, s.JawabanBenar) {
				benar++
				poinOtomatis += maxPoin
				detailPerSoal = append(detailPerSoal, DetailSoal{
					SoalID:      s.ID,
					TipeSoal:    s.TipeSoal,
					IsBenar:     1,
					PoinDidapat: maxPoin,
					Status:      "benar",
				})
			} else {
				salah++
				detailPerSoal = append(detailPerSoal, DetailSoal{
					SoalID:      s.ID,
					TipeSoal:    s.TipeSoal,
					IsBenar:     0,
					PoinDidapat: 0,
					Status:      "salah",
				})
			}

		case TipeEssay:
			var skor int
			var sudahDinilai bool
			if skorEssay != nil {
				skor, sudahDinilai = skorEssay[s.ID]
			}

			if !sudahDinilai {
				adaEssayBelumDinilai = true
				detailPerSoal = append(detailPerSoal, DetailSoal{
					SoalID:      s.ID,
					TipeSoal:    s.TipeSoal,
					IsBenar:     0,
					PoinDidapat: 0,
					Status:      "menunggu_koreksi",
				})
			} else {
				// Clamping skor antara 0 dan maxPoin
				if skor < 0 {
					skor = 0
				} else if skor > maxPoin {
					skor = maxPoin
				}
				poinEssay += skor
				detailPerSoal = append(detailPerSoal, DetailSoal{
					SoalID:      s.ID,
					TipeSoal:    s.TipeSoal,
					IsBenar:     0, // Essay tidak otomatis dihitung benar/salah biner
					PoinDidapat: skor,
					Status:      "dinilai",
				})
			}
		}
	}

	totalDidapat := poinOtomatis + poinEssay
	nilaiAkhir := 0
	if poinMaks > 0 {
		nilaiAkhir = int(math.Round((float64(totalDidapat) / float64(poinMaks)) * 100))
	}

	statusKoreksi := StatusSelesai
	if adaEssayBelumDinilai {
		statusKoreksi = StatusMenungguEssay
	}

	return Hasil{
		PoinOtomatis:  poinOtomatis,
		PoinEssay:     poinEssay,
		PoinMaks:      poinMaks,
		Nilai:         nilaiAkhir,
		Benar:         benar,
		Salah:         salah,
		Kosong:        kosong,
		StatusKoreksi: statusKoreksi,
		DetailPerSoal: detailPerSoal,
	}
}
