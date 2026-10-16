package excel_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"cbt-go/internal/excel"
)

func TestBuatTemplateSiswaExcel(t *testing.T) {
	buf, err := excel.BuatTemplateSiswa()
	if err != nil {
		t.Fatalf("BuatTemplateSiswa gagal: %v", err)
	}
	if buf == nil || buf.Len() == 0 {
		t.Fatal("hasil buffer template kosong")
	}

	f, err := excelize.OpenReader(buf)
	if err != nil {
		t.Fatalf("baca buffer template dengan excelize: %v", err)
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(rows) < 1 {
		t.Fatal("template harus memiliki minimal baris header")
	}

	header := rows[0]
	if len(header) < 4 {
		t.Fatalf("panjang kolom header = %d, want >= 4", len(header))
	}
	if header[0] != "NIS" || header[1] != "Nama" || header[2] != "Kelas" || header[3] != "PIN" {
		t.Errorf("header tidak sesuai: %+v", header)
	}
}

func TestBacaImportSiswaExcel(t *testing.T) {
	// Buat file excel uji dengan 3 baris valid dan 1 baris NIS kosong
	f := excelize.NewFile()
	defer f.Close()

	sheet := f.GetSheetName(0)
	_ = f.SetCellValue(sheet, "A1", "NIS")
	_ = f.SetCellValue(sheet, "B1", "Nama")
	_ = f.SetCellValue(sheet, "C1", "Kelas")
	_ = f.SetCellValue(sheet, "D1", "PIN")

	// Baris 2 (valid)
	_ = f.SetCellValue(sheet, "A2", "1001")
	_ = f.SetCellValue(sheet, "B2", "Siswa Satu")
	_ = f.SetCellValue(sheet, "C2", "VII-A")
	_ = f.SetCellValue(sheet, "D2", "1234")

	// Baris 3 (valid)
	_ = f.SetCellValue(sheet, "A3", "1002")
	_ = f.SetCellValue(sheet, "B3", "Siswa Dua")
	_ = f.SetCellValue(sheet, "C3", "VII-A")
	_ = f.SetCellValue(sheet, "D3", "5678")

	// Baris 4 (valid)
	_ = f.SetCellValue(sheet, "A4", "1003")
	_ = f.SetCellValue(sheet, "B4", "Siswa Tiga")
	_ = f.SetCellValue(sheet, "C4", "VII-B")
	_ = f.SetCellValue(sheet, "D4", "9999")

	// Baris 5 (tidak valid: NIS kosong)
	_ = f.SetCellValue(sheet, "A5", "")
	_ = f.SetCellValue(sheet, "B5", "Siswa Tanpa NIS")
	_ = f.SetCellValue(sheet, "C5", "VII-B")
	_ = f.SetCellValue(sheet, "D5", "1111")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("tulis file excel: %v", err)
	}

	res, err := excel.BacaImportSiswa(&buf)
	if err != nil {
		t.Fatalf("BacaImportSiswa error: %v", err)
	}

	if len(res.Valid) != 3 {
		t.Errorf("jumlah baris valid = %d, want 3", len(res.Valid))
	}
	if len(res.Errors) != 1 {
		t.Errorf("jumlah baris error = %d, want 1", len(res.Errors))
	}
	if !strings.Contains(res.Errors[0].Pesan, "NIS") {
		t.Errorf("pesan error baris 5 = %q, want mention NIS", res.Errors[0].Pesan)
	}
}
