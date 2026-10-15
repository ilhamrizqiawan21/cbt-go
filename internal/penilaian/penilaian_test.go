package penilaian_test

import (
	"testing"

	"cbt-go/internal/penilaian"
)

func TestPenilaianHitung(t *testing.T) {
	t.Run("1. Skenario PRD: 40 soal PG/menjodohkan semua benar (80 poin) + 5 essay nilai 4 (20 poin) -> nilai 100", func(t *testing.T) {
		var soalList []penilaian.Soal
		jawabanMap := make(map[int]string)
		skorEssay := make(map[int]int)

		// 35 PG
		for i := 1; i <= 35; i++ {
			soalList = append(soalList, penilaian.Soal{
				ID:           i,
				TipeSoal:     penilaian.TipePG,
				Poin:         2,
				JawabanBenar: "B",
			})
			jawabanMap[i] = "B"
		}

		// 5 Menjodohkan
		pasanganJSON := `[{"kiri":"1","kanan":"A"},{"kiri":"2","kanan":"B"}]`
		for i := 36; i <= 40; i++ {
			soalList = append(soalList, penilaian.Soal{
				ID:           i,
				TipeSoal:     penilaian.TipeMenjodohkan,
				Poin:         2,
				JawabanBenar: pasanganJSON,
			})
			jawabanMap[i] = pasanganJSON
		}

		// 5 Essay
		for i := 41; i <= 45; i++ {
			soalList = append(soalList, penilaian.Soal{
				ID:       i,
				TipeSoal: penilaian.TipeEssay,
				Poin:     4,
			})
			skorEssay[i] = 4
		}

		hasil := penilaian.Hitung(soalList, jawabanMap, skorEssay)

		if hasil.PoinOtomatis != 80 {
			t.Errorf("PoinOtomatis = %d; want 80", hasil.PoinOtomatis)
		}
		if hasil.PoinEssay != 20 {
			t.Errorf("PoinEssay = %d; want 20", hasil.PoinEssay)
		}
		if hasil.PoinMaks != 100 {
			t.Errorf("PoinMaks = %d; want 100", hasil.PoinMaks)
		}
		if hasil.Nilai != 100 {
			t.Errorf("Nilai = %d; want 100", hasil.Nilai)
		}
		if hasil.Benar != 40 {
			t.Errorf("Benar = %d; want 40", hasil.Benar)
		}
		if hasil.Salah != 0 {
			t.Errorf("Salah = %d; want 0", hasil.Salah)
		}
		if hasil.Kosong != 0 {
			t.Errorf("Kosong = %d; want 0", hasil.Kosong)
		}
		if hasil.StatusKoreksi != penilaian.StatusSelesai {
			t.Errorf("StatusKoreksi = %q; want %q", hasil.StatusKoreksi, penilaian.StatusSelesai)
		}
	})

	t.Run("2. Skenario PRD: 30 benar dari 40 PG (60 poin) + essay 12 dari 20 -> (60+12)/100 = 72", func(t *testing.T) {
		var soalList []penilaian.Soal
		jawabanMap := make(map[int]string)
		skorEssay := make(map[int]int)

		for i := 1; i <= 40; i++ {
			soalList = append(soalList, penilaian.Soal{
				ID:           i,
				TipeSoal:     penilaian.TipePG,
				Poin:         2,
				JawabanBenar: "A",
			})
			if i <= 30 {
				jawabanMap[i] = "A" // benar
			} else {
				jawabanMap[i] = "C" // salah
			}
		}

		// 5 Essay total 12
		soalList = append(soalList,
			penilaian.Soal{ID: 41, TipeSoal: penilaian.TipeEssay, Poin: 4},
			penilaian.Soal{ID: 42, TipeSoal: penilaian.TipeEssay, Poin: 4},
			penilaian.Soal{ID: 43, TipeSoal: penilaian.TipeEssay, Poin: 4},
			penilaian.Soal{ID: 44, TipeSoal: penilaian.TipeEssay, Poin: 4},
			penilaian.Soal{ID: 45, TipeSoal: penilaian.TipeEssay, Poin: 4},
		)
		skorEssay[41] = 3
		skorEssay[42] = 3
		skorEssay[43] = 2
		skorEssay[44] = 2
		skorEssay[45] = 2

		hasil := penilaian.Hitung(soalList, jawabanMap, skorEssay)

		if hasil.PoinOtomatis != 60 {
			t.Errorf("PoinOtomatis = %d; want 60", hasil.PoinOtomatis)
		}
		if hasil.PoinEssay != 12 {
			t.Errorf("PoinEssay = %d; want 12", hasil.PoinEssay)
		}
		if hasil.PoinMaks != 100 {
			t.Errorf("PoinMaks = %d; want 100", hasil.PoinMaks)
		}
		if hasil.Nilai != 72 {
			t.Errorf("Nilai = %d; want 72", hasil.Nilai)
		}
		if hasil.Benar != 30 {
			t.Errorf("Benar = %d; want 30", hasil.Benar)
		}
		if hasil.Salah != 10 {
			t.Errorf("Salah = %d; want 10", hasil.Salah)
		}
		if hasil.Kosong != 0 {
			t.Errorf("Kosong = %d; want 0", hasil.Kosong)
		}
		if hasil.StatusKoreksi != penilaian.StatusSelesai {
			t.Errorf("StatusKoreksi = %q; want %q", hasil.StatusKoreksi, penilaian.StatusSelesai)
		}
	})

	t.Run("3. Ada essay belum dinilai -> status_koreksi = menunggu_essay", func(t *testing.T) {
		soalList := []penilaian.Soal{
			{ID: 1, TipeSoal: penilaian.TipePG, Poin: 2, JawabanBenar: "A"},
			{ID: 2, TipeSoal: penilaian.TipeEssay, Poin: 4},
		}
		jawabanMap := map[int]string{1: "A"}
		skorEssay := map[int]int{} // essay id 2 belum dinilai

		hasil := penilaian.Hitung(soalList, jawabanMap, skorEssay)

		if hasil.PoinOtomatis != 2 {
			t.Errorf("PoinOtomatis = %d; want 2", hasil.PoinOtomatis)
		}
		if hasil.PoinEssay != 0 {
			t.Errorf("PoinEssay = %d; want 0", hasil.PoinEssay)
		}
		if hasil.PoinMaks != 6 {
			t.Errorf("PoinMaks = %d; want 6", hasil.PoinMaks)
		}
		if hasil.StatusKoreksi != penilaian.StatusMenungguEssay {
			t.Errorf("StatusKoreksi = %q; want %q", hasil.StatusKoreksi, penilaian.StatusMenungguEssay)
		}
	})

	t.Run("4. Menjodohkan: aturan all-or-nothing", func(t *testing.T) {
		pasangan := `[{"kiri":"Presiden RI ke-1","kanan":"Soekarno"},{"kiri":"Presiden RI ke-2","kanan":"Soeharto"}]`
		soalList := []penilaian.Soal{
			{ID: 1, TipeSoal: penilaian.TipeMenjodohkan, Poin: 2, JawabanBenar: pasangan},
		}

		// Skenario a: satu benar satu salah
		satuSalah := `[{"kiri":"Presiden RI ke-1","kanan":"Soekarno"},{"kiri":"Presiden RI ke-2","kanan":"Habibie"}]`
		hasilSalah := penilaian.Hitung(soalList, map[int]string{1: satuSalah}, nil)
		if hasilSalah.PoinOtomatis != 0 || hasilSalah.Benar != 0 || hasilSalah.Salah != 1 {
			t.Errorf("satu pasangan salah harus 0 poin: %+v", hasilSalah)
		}

		// Skenario b: semua benar dengan urutan pasangan terbalik
		terbalik := `[{"kiri":"Presiden RI ke-2","kanan":"Soeharto"},{"kiri":"Presiden RI ke-1","kanan":"Soekarno"}]`
		hasilBenar := penilaian.Hitung(soalList, map[int]string{1: terbalik}, nil)
		if hasilBenar.PoinOtomatis != 2 || hasilBenar.Benar != 1 || hasilBenar.Salah != 0 {
			t.Errorf("semua pasangan benar harus dapat 2 poin: %+v", hasilBenar)
		}

		// Skenario c: pasangan tidak lengkap (hanya jawab 1 dari 2)
		parsial := `[{"kiri":"Presiden RI ke-1","kanan":"Soekarno"}]`
		hasilParsial := penilaian.Hitung(soalList, map[int]string{1: parsial}, nil)
		if hasilParsial.PoinOtomatis != 0 || hasilParsial.Benar != 0 || hasilParsial.Salah != 1 {
			t.Errorf("pasangan tidak lengkap harus 0 poin: %+v", hasilParsial)
		}
	})

	t.Run("5. Soal tidak dijawab dihitung kosong dan bernilai 0", func(t *testing.T) {
		soalList := []penilaian.Soal{
			{ID: 1, TipeSoal: penilaian.TipePG, Poin: 2, JawabanBenar: "A"},
			{ID: 2, TipeSoal: penilaian.TipePG, Poin: 2, JawabanBenar: "B"},
			{ID: 3, TipeSoal: penilaian.TipeMenjodohkan, Poin: 2, JawabanBenar: `[{"kiri":"1","kanan":"A"}]`},
		}
		jawabanMap := map[int]string{
			1: "A",
			2: "",   // kosong
			3: "[]", // kosong
		}

		hasil := penilaian.Hitung(soalList, jawabanMap, nil)
		if hasil.Benar != 1 {
			t.Errorf("Benar = %d; want 1", hasil.Benar)
		}
		if hasil.Salah != 0 {
			t.Errorf("Salah = %d; want 0", hasil.Salah)
		}
		if hasil.Kosong != 2 {
			t.Errorf("Kosong = %d; want 2", hasil.Kosong)
		}
		if hasil.PoinOtomatis != 2 {
			t.Errorf("PoinOtomatis = %d; want 2", hasil.PoinOtomatis)
		}
		if hasil.PoinMaks != 6 {
			t.Errorf("PoinMaks = %d; want 6", hasil.PoinMaks)
		}
		// round(2/6 * 100) = round(33.333...) = 33
		if hasil.Nilai != 33 {
			t.Errorf("Nilai = %d; want 33", hasil.Nilai)
		}
	})

	t.Run("6. Pembulatan nilai 0.5 ke atas", func(t *testing.T) {
		// 1 benar dari 2 soal poin 1 -> 1/2 * 100 = 50
		// 1 benar dari 6 poin -> 1/6 * 100 = 16.666 -> 17
		// 3 benar dari 8 poin -> 3/8 * 100 = 37.5 -> 38
		soalList := []penilaian.Soal{
			{ID: 1, TipeSoal: penilaian.TipePG, Poin: 3, JawabanBenar: "A"},
			{ID: 2, TipeSoal: penilaian.TipePG, Poin: 5, JawabanBenar: "B"},
		}
		// Jawab benar soal 1 (3 poin), soal 2 salah -> 3/8 * 100 = 37.5 -> round ke 38
		jawabanMap := map[int]string{1: "A", 2: "C"}

		hasil := penilaian.Hitung(soalList, jawabanMap, nil)
		if hasil.Nilai != 38 {
			t.Errorf("Nilai = %d; want 38 (pembulatan 37.5 ke atas)", hasil.Nilai)
		}
	})

	t.Run("7. Kasus ekstrem: poin_maks 0 tidak memicu pembagian nol", func(t *testing.T) {
		hasil := penilaian.Hitung([]penilaian.Soal{}, map[int]string{}, nil)
		if hasil.Nilai != 0 {
			t.Errorf("Nilai = %d; want 0", hasil.Nilai)
		}
		if hasil.PoinMaks != 0 {
			t.Errorf("PoinMaks = %d; want 0", hasil.PoinMaks)
		}
	})

	t.Run("8. PG case-insensitive", func(t *testing.T) {
		if !penilaian.CekJawabanPG("b", "B") {
			t.Error("jawaban 'b' harus cocok dengan kunci 'B'")
		}
		if !penilaian.CekJawabanPG(" B ", "b") {
			t.Error("jawaban ' B ' dengan spasi harus cocok")
		}
		if penilaian.CekJawabanPG("", "A") {
			t.Error("jawaban kosong tidak boleh benar")
		}
	})
}
