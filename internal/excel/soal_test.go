package excel_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"cbt-go/internal/excel"
)

func xlsxSoal(t *testing.T, baris [][]any) *bytes.Buffer {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	sh := f.GetSheetName(0)
	_ = f.SetSheetRow(sh, "A1", &[]any{"Tipe", "Soal", "Poin", "A", "B", "C", "D", "Kunci", "Pasangan", "Pengecoh"})
	for i, b := range baris {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		_ = f.SetSheetRow(sh, cell, &b)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return &buf
}

func TestBacaImportSoalValid(t *testing.T) {
	buf := xlsxSoal(t, [][]any{
		{"pg", "Ibu kota?", "", "Bandung", "Jakarta", "Medan", "", "b", "", ""},
		{"MENJODOHKAN", "Jodohkan", 3, "", "", "", "", "", "Jepang=Tokyo; Prancis=Paris\nMesir=Kairo", "Roma; Oslo"},
		{"essay", "Jelaskan!", "", "", "", "", "", "", "", ""},
		{},
		{"essay", "Uraikan", 5},
	})
	res, err := excel.BacaImportSoal(buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 || len(res.Items) != 4 {
		t.Fatalf("items=%d errors=%v", len(res.Items), res.Errors)
	}
	pg := res.Items[0].Soal
	if pg.Poin != 2 || pg.JawabanBenar == nil || *pg.JawabanBenar != "B" {
		t.Errorf("pg: poin=%d kunci=%v (kunci harus dinormalisasi huruf besar, poin default 2)", pg.Poin, pg.JawabanBenar)
	}
	jd := res.Items[1].Soal
	if jd.TipeSoal != "menjodohkan" || jd.Poin != 3 || jd.OpsiTambahan == nil ||
		!strings.Contains(*jd.OpsiTambahan, `"Mesir"`) || !strings.Contains(*jd.OpsiTambahan, `"Oslo"`) {
		t.Errorf("menjodohkan salah: %+v", jd)
	}
	if res.Items[2].Soal.Poin != 4 || res.Items[3].Soal.Poin != 5 {
		t.Errorf("poin essay: %d, %d", res.Items[2].Soal.Poin, res.Items[3].Soal.Poin)
	}
	if res.Items[3].Baris != 6 {
		t.Errorf("nomor baris = %d, want 6", res.Items[3].Baris)
	}
}

func TestBacaImportSoalGalatPerBaris(t *testing.T) {
	buf := xlsxSoal(t, [][]any{
		{"pg", "Satu pilihan saja", "", "x", "", "", "", "A", "", ""},  // baris 2: pilihan kurang
		{"hitung", "Tipe asing", "", "", "", "", "", "", "", ""},       // baris 3
		{"pg", "Kunci kosong", "", "a", "b", "", "", "C", "", ""},      // baris 4
		{"menjodohkan", "Rusak", "", "", "", "", "", "", "a; b=c", ""}, // baris 5
		{"essay", "", "", "", "", "", "", "", "", ""},                  // baris 6: teks kosong
		{"essay", "Poin salah", "abc", "", "", "", "", "", "", ""},     // baris 7
		{"pg", "Valid", "", "a", "b", "", "", "A", "", ""},             // baris 8 valid
	})
	res, err := excel.BacaImportSoal(buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].Baris != 8 {
		t.Errorf("hanya baris 8 yang valid: %+v", res.Items)
	}
	var baris []int
	for _, e := range res.Errors {
		baris = append(baris, e.Baris)
		if e.Pesan == "" {
			t.Errorf("pesan kosong untuk baris %d", e.Baris)
		}
	}
	want := []int{2, 3, 4, 5, 6, 7}
	if len(baris) != len(want) {
		t.Fatalf("baris galat = %v, want %v", baris, want)
	}
	for i := range want {
		if baris[i] != want[i] {
			t.Fatalf("baris galat = %v, want %v", baris, want)
		}
	}
}

func TestBacaImportSoalBatasDanTemplate(t *testing.T) {
	baris := make([][]any, excel.MaksBarisImportSoal+1)
	for i := range baris {
		baris[i] = []any{"essay", "x"}
	}
	if _, err := excel.BacaImportSoal(xlsxSoal(t, baris)); err == nil {
		t.Error("lebih dari batas baris harus ditolak")
	}
	if _, err := excel.BacaImportSoal(strings.NewReader("bukan xlsx")); err == nil {
		t.Error("berkas bukan xlsx harus ditolak")
	}

	// Template yang kami unduhkan harus lolos impor tanpa galat.
	tpl, err := excel.BuatTemplateSoal()
	if err != nil {
		t.Fatal(err)
	}
	res, err := excel.BacaImportSoal(tpl)
	if err != nil || len(res.Errors) != 0 || len(res.Items) != 3 {
		t.Errorf("template tidak lolos impor: err=%v errors=%v items=%d", err, res.Errors, len(res.Items))
	}
}
