package excel

import (
	"bytes"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

// LogKecuranganItem merepresentasikan satu baris log kecurangan siswa untuk diekspor ke Excel.
type LogKecuranganItem struct {
	NIS             string
	NamaSiswa       string
	Kelas           string
	NamaUjian       string
	JenisKecurangan string
	DeviceType      string
	Timestamp       time.Time
}

// BuatEksporLogKecurangan menghasilkan berkas spreadsheet (.xlsx) rekapan log kecurangan.
func BuatEksporLogKecurangan(namaUjian string, items []LogKecuranganItem) (*bytes.Buffer, error) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := f.GetSheetName(0)

	_ = f.SetCellValue(sheet, "A1", "LAPORAN LOG PELANGGARAN UJIAN")
	_ = f.SetCellValue(sheet, "A2", fmt.Sprintf("Ujian: %s", namaUjian))
	_ = f.SetCellValue(sheet, "A3", fmt.Sprintf("Waktu Unduh: %s", time.Now().Format("02/01/2006 15:04")))

	headers := []string{"No", "Waktu", "NIS", "Nama Siswa", "Kelas", "Ujian", "Jenis Pelanggaran", "Perangkat"}

	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 5)
		_ = f.SetCellValue(sheet, cell, h)
	}

	for i, it := range items {
		row := 6 + i
		waktuStr := "-"
		if !it.Timestamp.IsZero() {
			waktuStr = it.Timestamp.Format("02/01/2006 15:04:05")
		}

		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), i+1)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", row), waktuStr)
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", row), it.NIS)
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", row), it.NamaSiswa)
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", row), it.Kelas)
		_ = f.SetCellValue(sheet, fmt.Sprintf("F%d", row), it.NamaUjian)
		_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", row), it.JenisKecurangan)
		_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", row), it.DeviceType)
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("tulis workbook log kecurangan: %w", err)
	}

	return &buf, nil
}
