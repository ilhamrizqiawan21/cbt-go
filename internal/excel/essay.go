package excel

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// EssayExportRow adalah data satu jawaban essay siswa untuk diekspor ke format koreksi Excel.
type EssayExportRow struct {
	SiswaID      int    `json:"siswa_id"`
	NIS          string `json:"nis"`
	NamaSiswa    string `json:"nama_siswa"`
	Kelas        string `json:"kelas"`
	SoalID       int    `json:"soal_id"`
	TeksSoal     string `json:"teks_soal"`
	JawabanSiswa string `json:"jawaban_siswa"`
	PoinMaks     int    `json:"poin_maks"`
	Skor         *int   `json:"skor"`
}

// EssayImportItem menampung skor essay hasil pembacaan baris Excel.
type EssayImportItem struct {
	Baris   int `json:"baris"`
	SiswaID int `json:"siswa_id"`
	SoalID  int `json:"soal_id"`
	Skor    int `json:"skor"`
}

// BuatTemplateEssay menghasilkan lembar kerja Excel berisi daftar jawaban essay siswa yang perlu dinilai.
func BuatTemplateEssay(namaUjian string, rows []EssayExportRow) (*bytes.Buffer, error) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := f.GetSheetName(0)

	_ = f.SetCellValue(sheet, "A1", "FORMULIR PENILAIAN ESSAY GURU")
	_ = f.SetCellValue(sheet, "A2", fmt.Sprintf("Ujian: %s", namaUjian))
	_ = f.SetCellValue(sheet, "A3", "Petunjuk: Isi nilai skor pada kolom 'Skor (0 - Poin Maks)'. Jangan ubah kolom Siswa ID dan Soal ID.")

	headers := []string{
		"No", "Siswa ID", "NIS", "Nama Siswa", "Kelas",
		"Soal ID", "Soal", "Jawaban Siswa", "Poin Maks", "Skor (0 - Poin Maks)",
	}

	for colIdx, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(colIdx+1, 5)
		_ = f.SetCellValue(sheet, cell, h)
	}

	for i, r := range rows {
		rowNum := 6 + i
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", rowNum), i+1)
		_ = f.SetCellValue(sheet, fmt.Sprintf("B%d", rowNum), r.SiswaID)
		_ = f.SetCellValue(sheet, fmt.Sprintf("C%d", rowNum), r.NIS)
		_ = f.SetCellValue(sheet, fmt.Sprintf("D%d", rowNum), r.NamaSiswa)
		_ = f.SetCellValue(sheet, fmt.Sprintf("E%d", rowNum), r.Kelas)
		_ = f.SetCellValue(sheet, fmt.Sprintf("F%d", rowNum), r.SoalID)
		_ = f.SetCellValue(sheet, fmt.Sprintf("G%d", rowNum), r.TeksSoal)
		_ = f.SetCellValue(sheet, fmt.Sprintf("H%d", rowNum), r.JawabanSiswa)
		_ = f.SetCellValue(sheet, fmt.Sprintf("I%d", rowNum), r.PoinMaks)

		if r.Skor != nil {
			_ = f.SetCellValue(sheet, fmt.Sprintf("J%d", rowNum), *r.Skor)
		} else {
			_ = f.SetCellValue(sheet, fmt.Sprintf("J%d", rowNum), "")
		}
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("tulis workbook template essay: %w", err)
	}

	return &buf, nil
}

// BacaImportEssay membaca data masukan skor essay dari stream file Excel (.xlsx).
func BacaImportEssay(r io.Reader) ([]EssayImportItem, []ImportError, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("buka berkas excel: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, nil, fmt.Errorf("baca baris sheet: %w", err)
	}

	var items []EssayImportItem
	var errs []ImportError

	// Baris data dimulai dari baris ke-6 (indeks 5)
	for i := 5; i < len(rows); i++ {
		row := rows[i]
		lineNum := i + 1

		// Lewati baris kosong
		if len(row) < 2 || strings.TrimSpace(row[0]) == "" && strings.TrimSpace(row[1]) == "" {
			continue
		}

		// Kolom B (indeks 1) = Siswa ID
		siswaIDStr := strings.TrimSpace(row[1])
		siswaID, err := strconv.Atoi(siswaIDStr)
		if err != nil || siswaID <= 0 {
			errs = append(errs, ImportError{
				Baris: lineNum,
				Pesan: fmt.Sprintf("Siswa ID tidak valid: '%s'", siswaIDStr),
			})
			continue
		}

		// Kolom F (indeks 5) = Soal ID
		if len(row) <= 5 {
			errs = append(errs, ImportError{
				Baris: lineNum,
				Pesan: "Kolom Soal ID tidak ditemukan",
			})
			continue
		}
		soalIDStr := strings.TrimSpace(row[5])
		soalID, err := strconv.Atoi(soalIDStr)
		if err != nil || soalID <= 0 {
			errs = append(errs, ImportError{
				Baris: lineNum,
				Pesan: fmt.Sprintf("Soal ID tidak valid: '%s'", soalIDStr),
			})
			continue
		}

		// Kolom J (indeks 9) = Skor
		if len(row) <= 9 {
			// Belum ada skor pada baris ini, lewati
			continue
		}
		skorStr := strings.TrimSpace(row[9])
		if skorStr == "" {
			continue
		}

		skor, err := strconv.Atoi(skorStr)
		if err != nil {
			errs = append(errs, ImportError{
				Baris: lineNum,
				Pesan: fmt.Sprintf("Nilai skor harus berupa angka integer: '%s'", skorStr),
			})
			continue
		}

		items = append(items, EssayImportItem{
			Baris:   lineNum,
			SiswaID: siswaID,
			SoalID:  soalID,
			Skor:    skor,
		})
	}

	return items, errs, nil
}
