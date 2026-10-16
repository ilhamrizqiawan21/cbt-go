package excel

import (
	"bytes"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

// HasilUjianItem memuat data baris nilai siswa untuk diekspor ke Excel.
type HasilUjianItem struct {
	NIS           string
	NamaSiswa     string
	Kelas         string
	Benar         int
	Salah         int
	Kosong        int
	PoinOtomatis  int
	PoinEssay     int
	PoinMaks      int
	NilaiAkhir    int
	StatusKoreksi string
	SelesaiPada   time.Time
}

// BuatEksporHasilUjian membuat berkas .xlsx rekapan nilai siswa untuk satu ujian.
func BuatEksporHasilUjian(namaUjian, namaMapel string, items []HasilUjianItem) (*bytes.Buffer, error) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := f.GetSheetName(0)

	// Judul Informasi
	_ = f.SetCellValue(sheet, "A1", "LAPORAN HASIL UJIAN")
	_ = f.SetCellValue(sheet, "A2", fmt.Sprintf("Ujian: %s | Mata Pelajaran: %s", namaUjian, namaMapel))
	_ = f.SetCellValue(sheet, "A3", fmt.Sprintf("Waktu Unduh: %s", time.Now().Format("02/01/2006 15:04")))

	// Header Kolom pada baris ke-5
	headers := []string{
		"No", "NIS", "Nama Siswa", "Kelas", "Benar", "Salah", "Kosong",
		"Poin PG & Menjodohkan", "Poin Essay", "Total Poin", "Nilai Akhir (0-100)",
		"Status Koreksi", "Waktu Selesai",
	}

	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 5)
		_ = f.SetCellValue(sheet, cell, h)
	}

	// Baris Data
	for i, it := range items {
		row := 6 + i
		totalPoin := it.PoinOtomatis + it.PoinEssay

		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), i+1)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), it.NIS)
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), it.NamaSiswa)
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", row), it.Kelas)
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", row), it.Benar)
		_ = f.SetCellValue(sheet, fmt.Sprintf("F%d", row), it.Salah)
		_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", row), it.Kosong)
		_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", row), it.PoinOtomatis)
		_ = f.SetCellValue(sheet, fmt.Sprintf("I%d", row), it.PoinEssay)
		_ = f.SetCellValue(sheet, fmt.Sprintf("J%d", row), totalPoin)
		_ = f.SetCellValue(sheet, fmt.Sprintf("K%d", row), it.NilaiAkhir)
		_ = f.SetCellValue(sheet, fmt.Sprintf("L%d", row), it.StatusKoreksi)

		waktuStr := "-"
		if !it.SelesaiPada.IsZero() {
			waktuStr = it.SelesaiPada.Format("02/01/2006 15:04")
		}
		_ = f.SetCellValue(sheet, fmt.Sprintf("M%d", row), waktuStr)
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("tulis workbook ekspor hasil: %w", err)
	}

	return &buf, nil
}
