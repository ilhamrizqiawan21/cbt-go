package excel_test

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"

	"cbt-go/internal/excel"
)

func TestBuatDanBacaImportEssay(t *testing.T) {
	skorVal := 4
	rows := []excel.EssayExportRow{
		{
			SiswaID:      1,
			NIS:          "1001",
			NamaSiswa:    "Budi Santoso",
			Kelas:        "VII-A",
			SoalID:       10,
			TeksSoal:     "Jelaskan fotosintesis!",
			JawabanSiswa: "Proses pembuatan makanan pada tumbuhan",
			PoinMaks:     4,
			Skor:         &skorVal,
		},
		{
			SiswaID:      2,
			NIS:          "1002",
			NamaSiswa:    "Siti Aminah",
			Kelas:        "VII-A",
			SoalID:       10,
			TeksSoal:     "Jelaskan fotosintesis!",
			JawabanSiswa: "Fotosintesis membutuhkan cahaya matahari",
			PoinMaks:     4,
			Skor:         nil, // belum dinilai
		},
	}

	buf, err := excel.BuatTemplateEssay("Ujian IPA", rows)
	if err != nil {
		t.Fatalf("BuatTemplateEssay gagal: %v", err)
	}

	// Baca kembali template
	items, errs, err := excel.BacaImportEssay(buf)
	if err != nil {
		t.Fatalf("BacaImportEssay gagal: %v", err)
	}

	if len(errs) != 0 {
		t.Errorf("tidak boleh ada error validasi baris, dapat: %+v", errs)
	}

	if len(items) != 1 {
		t.Fatalf("harus ada 1 item yang memiliki skor, dapat %d", len(items))
	}

	if items[0].SiswaID != 1 || items[0].SoalID != 10 || items[0].Skor != 4 {
		t.Errorf("data item essay tidak sesuai: %+v", items[0])
	}
}

func TestBacaImportEssay_ValidasiError(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)

	// Buat baris header
	for colIdx, h := range []string{"No", "Siswa ID", "NIS", "Nama", "Kelas", "Soal ID", "Soal", "Jawaban", "Poin Maks", "Skor"} {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 5)
		_ = f.SetCellValue(sheet, cell, h)
	}

	// Baris 6: Siswa ID bukan angka
	_ = f.SetCellValue(sheet, "B6", "BUKAN_ANGKA")
	_ = f.SetCellValue(sheet, "F6", "10")
	_ = f.SetCellValue(sheet, "J6", "4")

	// Baris 7: Skor bukan angka
	_ = f.SetCellValue(sheet, "B7", "2")
	_ = f.SetCellValue(sheet, "F7", "10")
	_ = f.SetCellValue(sheet, "J7", "NILAI_HURUF")

	var buf bytes.Buffer
	_ = f.Write(&buf)

	items, errs, err := excel.BacaImportEssay(&buf)
	if err != nil {
		t.Fatalf("BacaImportEssay error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("tidak boleh ada item valid, dapat %d", len(items))
	}
	if len(errs) != 2 {
		t.Fatalf("harus ada 2 laporan error validasi baris, dapat %d", len(errs))
	}
}
