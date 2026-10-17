package soal

import (
	"errors"
	"testing"
)

func sp(s string) *string { return &s }

func TestValidasiDanNormalisasi(t *testing.T) {
	cases := []struct {
		name      string
		in        SoalKelola
		wantErr   error
		wantKunci string // "" = harus nil
		wantOpsi  string // "" = harus nil
	}{
		{"pg lengkap, kunci huruf kecil dinormalkan", SoalKelola{TipeSoal: "pg", PilihanA: sp("1"), PilihanB: sp("2"), JawabanBenar: sp(" b "), OpsiTambahan: sp("{}")}, nil, "B", ""},
		{"pg satu pilihan", SoalKelola{TipeSoal: "pg", PilihanA: sp("1"), JawabanBenar: sp("A")}, ErrPilihanPGKurang, "", ""},
		{"pg tanpa kunci", SoalKelola{TipeSoal: "pg", PilihanA: sp("1"), PilihanB: sp("2")}, ErrKunciPGInvalid, "", ""},
		{"pg kunci ke pilihan kosong", SoalKelola{TipeSoal: "pg", PilihanA: sp("1"), PilihanB: sp("2"), JawabanBenar: sp("C")}, ErrKunciPGInvalid, "", ""},
		{"pg kunci bukan huruf", SoalKelola{TipeSoal: "pg", PilihanA: sp("1"), PilihanB: sp("2"), JawabanBenar: sp("E")}, ErrKunciPGInvalid, "", ""},
		{"menjodohkan dari opsi + pengecoh, kunci diturunkan", SoalKelola{TipeSoal: "menjodohkan", PilihanA: sp("x"), JawabanBenar: sp("lama"),
			OpsiTambahan: sp(`{"pasangan":[{"kiri":" a ","kanan":"1"},{"kiri":"b","kanan":"2"},{"kiri":"","kanan":""}],"pengecoh":[" 3 ",""]}`)},
			nil, `[{"kiri":"a","kanan":"1"},{"kiri":"b","kanan":"2"}]`, `{"pasangan":[{"kiri":"a","kanan":"1"},{"kiri":"b","kanan":"2"}],"pengecoh":["3"]}`},
		{"menjodohkan hanya jawaban_benar (klien API lama)", SoalKelola{TipeSoal: "menjodohkan", JawabanBenar: sp(`[{"kiri":"a","kanan":"1"},{"kiri":"b","kanan":"2"}]`)},
			nil, `[{"kiri":"a","kanan":"1"},{"kiri":"b","kanan":"2"}]`, `{"pasangan":[{"kiri":"a","kanan":"1"},{"kiri":"b","kanan":"2"}],"pengecoh":[]}`},
		{"menjodohkan satu pasangan", SoalKelola{TipeSoal: "menjodohkan", OpsiTambahan: sp(`{"pasangan":[{"kiri":"a","kanan":"1"}]}`)}, ErrPasanganKurang, "", ""},
		{"menjodohkan sisi kosong", SoalKelola{TipeSoal: "menjodohkan", OpsiTambahan: sp(`{"pasangan":[{"kiri":"a","kanan":"1"},{"kiri":"b","kanan":""}]}`)}, ErrPasanganKurang, "", ""},
		{"menjodohkan kiri ganda", SoalKelola{TipeSoal: "menjodohkan", OpsiTambahan: sp(`{"pasangan":[{"kiri":"a","kanan":"1"},{"kiri":"a","kanan":"2"}]}`)}, ErrPasanganKiriGanda, "", ""},
		{"menjodohkan JSON rusak", SoalKelola{TipeSoal: "menjodohkan", OpsiTambahan: sp(`{rusak`)}, ErrOpsiMenjodohkanRusak, "", ""},
		{"menjodohkan kosong", SoalKelola{TipeSoal: "menjodohkan"}, ErrPasanganKurang, "", ""},
		{"essay membersihkan sisa field", SoalKelola{TipeSoal: "essay", PilihanA: sp("x"), JawabanBenar: sp("A"), OpsiTambahan: sp("{}")}, nil, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.in
			err := ValidasiDanNormalisasi(&s)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			cek := func(label string, got *string, want string) {
				t.Helper()
				if want == "" && got != nil {
					t.Errorf("%s = %q, want nil", label, *got)
				}
				if want != "" && (got == nil || *got != want) {
					t.Errorf("%s = %v, want %q", label, got, want)
				}
			}
			cek("jawaban_benar", s.JawabanBenar, tc.wantKunci)
			cek("opsi_tambahan", s.OpsiTambahan, tc.wantOpsi)
			if s.TipeSoal != "pg" && (s.PilihanA != nil || s.PilihanB != nil) {
				t.Errorf("pilihan A–D harus dikosongkan untuk tipe %s", s.TipeSoal)
			}
		})
	}
}
