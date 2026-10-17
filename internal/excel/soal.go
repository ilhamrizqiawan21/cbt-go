package excel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"cbt-go/internal/soal"
)

// MaksBarisImportSoal membatasi satu berkas impor agar satu transaksi tetap kecil.
const MaksBarisImportSoal = 500

// SoalImportItem adalah satu baris Excel yang sudah divalidasi dan dinormalisasi.
type SoalImportItem struct {
	Baris int
	Soal  soal.SoalKelola
}

// HasilImportSoal: bila Errors tidak kosong, pemanggil TIDAK boleh menyimpan apa pun (G-024).
type HasilImportSoal struct {
	Items  []SoalImportItem
	Errors []ImportError
}

var headerSoal = []string{"Tipe", "Soal", "Poin", "Pilihan A", "Pilihan B", "Pilihan C", "Pilihan D", "Kunci", "Pasangan", "Pengecoh"}

// BuatTemplateSoal membuat template .xlsx dengan contoh tiap tipe soal.
// Pasangan menjodohkan ditulis "kiri=kanan" dipisah titik koma atau baris baru.
func BuatTemplateSoal() (*bytes.Buffer, error) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)

	for i, h := range headerSoal {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	contoh := [][]any{
		{"pg", "Ibu kota Indonesia adalah ...", 2, "Bandung", "Jakarta", "Surabaya", "Medan", "B", "", ""},
		{"menjodohkan", "Jodohkan negara dengan ibu kotanya", 2, "", "", "", "", "", "Jepang=Tokyo; Prancis=Paris; Mesir=Kairo", "Roma"},
		{"essay", "Jelaskan hikmah belajar bahasa Arab!", 4, "", "", "", "", "", "", ""},
	}
	for r, baris := range contoh {
		for c, v := range baris {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}
	_ = f.SetColWidth(sheet, "B", "B", 50)
	_ = f.SetColWidth(sheet, "I", "I", 40)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("tulis template soal: %w", err)
	}
	return &buf, nil
}

// BacaImportSoal membaca lembar pertama. Baris 1 = header, dilewati. Setiap baris diperiksa
// dengan soal.ValidasiDanNormalisasi (aturan sama dengan input manual, G-018).
func BacaImportSoal(r io.Reader) (*HasilImportSoal, error) {
	if r == nil {
		return nil, errors.New("stream pembaca excel kosong")
	}
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, fmt.Errorf("membuka berkas excel: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	if sheet == "" {
		return nil, errors.New("lembar kerja excel tidak ditemukan")
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("membaca baris excel: %w", err)
	}

	res := &HasilImportSoal{}
	if len(rows)-1 > MaksBarisImportSoal {
		return nil, fmt.Errorf("maksimal %d soal per berkas", MaksBarisImportSoal)
	}

	for i := 1; i < len(rows); i++ {
		nomor := i + 1
		kol := func(n int) string {
			if n < len(rows[i]) {
				return strings.TrimSpace(rows[i][n])
			}
			return ""
		}
		kosong := true
		for n := range headerSoal {
			if kol(n) != "" {
				kosong = false
				break
			}
		}
		if kosong {
			continue
		}
		salah := func(pesan string) {
			res.Errors = append(res.Errors, ImportError{Baris: nomor, Pesan: pesan})
		}

		tipe := strings.ToLower(kol(0))
		if tipe != "pg" && tipe != "menjodohkan" && tipe != "essay" {
			salah("Tipe harus pg, menjodohkan, atau essay")
			continue
		}
		if kol(1) == "" {
			salah(soal.ErrTeksSoalKosong.Error())
			continue
		}
		poin := 0
		if p := kol(2); p != "" {
			poin, err = strconv.Atoi(p)
			if err != nil || poin <= 0 || poin > 100 {
				salah("Poin harus bilangan bulat 1–100")
				continue
			}
		}

		s := soal.SoalKelola{TipeSoal: tipe, TeksSoal: kol(1), Poin: soal.DefaultPoin(tipe, poin)}
		opsi := func(n int) *string {
			if v := kol(n); v != "" {
				return &v
			}
			return nil
		}
		switch tipe {
		case "pg":
			s.PilihanA, s.PilihanB, s.PilihanC, s.PilihanD = opsi(3), opsi(4), opsi(5), opsi(6)
			s.JawabanBenar = opsi(7)
		case "menjodohkan":
			pasangan, pesan := parsePasangan(kol(8))
			if pesan != "" {
				salah(pesan)
				continue
			}
			pengecoh := splitDaftar(kol(9))
			opsiJSON, _ := jsonCompact(map[string]any{"pasangan": pasangan, "pengecoh": pengecoh})
			s.OpsiTambahan = &opsiJSON
		}

		if err := soal.ValidasiDanNormalisasi(&s); err != nil {
			salah(err.Error())
			continue
		}
		res.Items = append(res.Items, SoalImportItem{Baris: nomor, Soal: s})
	}
	return res, nil
}

func splitDaftar(s string) []string {
	var out []string
	for _, x := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == '\n' }) {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func parsePasangan(s string) ([]map[string]string, string) {
	var out []map[string]string
	for _, p := range splitDaftar(s) {
		kiri, kanan, ok := strings.Cut(p, "=")
		kiri, kanan = strings.TrimSpace(kiri), strings.TrimSpace(kanan)
		if !ok || kiri == "" || kanan == "" {
			return nil, fmt.Sprintf("Pasangan %q tidak valid; tulis kiri=kanan dipisah titik koma", p)
		}
		out = append(out, map[string]string{"kiri": kiri, "kanan": kanan})
	}
	return out, ""
}

func jsonCompact(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}
