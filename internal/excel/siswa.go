// Package excel mengelola pembuatan berkas spreadsheet dan pemrosesan impor data menggunakan excelize.
package excel

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

// SiswaImportItem menampung satu baris data siswa hasil pemindaian berkas spreadsheet.
type SiswaImportItem struct {
	Baris int
	NIS   string
	Nama  string
	Kelas string
	PIN   string
}

// ImportError merekam baris spreadsheet yang tidak memenuhi syarat validasi.
type ImportError struct {
	Baris int    `json:"baris"`
	Pesan string `json:"pesan"`
}

// HasilImportSiswa adalah hasil verifikasi pemrosesan berkas spreadsheet.
type HasilImportSiswa struct {
	Valid  []SiswaImportItem `json:"valid"`
	Errors []ImportError     `json:"errors"`
}

// BuatTemplateSiswa membuat berkas template Excel (.xlsx) dengan kolom NIS, Nama, Kelas, dan PIN.
func BuatTemplateSiswa() (*bytes.Buffer, error) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := f.GetSheetName(0)

	// Header kolom
	headers := []string{"NIS", "Nama", "Kelas", "PIN"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}

	// Contoh data baris 2 & 3
	_ = f.SetCellValue(sheet, "A2", "1001")
	_ = f.SetCellValue(sheet, "B2", "Contoh Siswa 1")
	_ = f.SetCellValue(sheet, "C2", "VII-A")
	_ = f.SetCellValue(sheet, "D2", "1234")

	_ = f.SetCellValue(sheet, "A3", "1002")
	_ = f.SetCellValue(sheet, "B3", "Contoh Siswa 2")
	_ = f.SetCellValue(sheet, "C3", "VII-B")
	_ = f.SetCellValue(sheet, "D3", "1234")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("write template excel: %w", err)
	}
	return &buf, nil
}

// BacaImportSiswa membaca stream berkas .xlsx dari memori dan memvalidasi setiap barisnya.
func BacaImportSiswa(r io.Reader) (*HasilImportSiswa, error) {
	if r == nil {
		return nil, errors.New("stream pembaca excel kosong")
	}

	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, fmt.Errorf("membuka berkas excel: %w", err)
	}
	defer f.Close()

	sheetName := f.GetSheetName(0)
	if sheetName == "" {
		return nil, errors.New("lembar kerja excel tidak ditemukan")
	}

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("membaca baris excel: %w", err)
	}

	res := &HasilImportSiswa{
		Valid:  make([]SiswaImportItem, 0),
		Errors: make([]ImportError, 0),
	}

	if len(rows) <= 1 {
		return res, nil // hanya header atau kosong
	}

	seenNIS := make(map[string]int)

	for i := 1; i < len(rows); i++ {
		nomorBaris := i + 1
		row := rows[i]

		// Lewati baris kosong
		if len(row) == 0 {
			continue
		}

		var nis, nama, kelas, pin string
		if len(row) > 0 {
			nis = strings.TrimSpace(row[0])
		}
		if len(row) > 1 {
			nama = strings.TrimSpace(row[1])
		}
		if len(row) > 2 {
			kelas = strings.TrimSpace(row[2])
		}
		if len(row) > 3 {
			pin = strings.TrimSpace(row[3])
		}

		// Jika seluruh kolom pada baris ini kosong, lewati
		if nis == "" && nama == "" && kelas == "" && pin == "" {
			continue
		}

		if nis == "" {
			res.Errors = append(res.Errors, ImportError{Baris: nomorBaris, Pesan: "NIS tidak boleh kosong"})
			continue
		}
		if nama == "" {
			res.Errors = append(res.Errors, ImportError{Baris: nomorBaris, Pesan: "Nama siswa tidak boleh kosong"})
			continue
		}
		if kelas == "" {
			res.Errors = append(res.Errors, ImportError{Baris: nomorBaris, Pesan: "Kelas tidak boleh kosong"})
			continue
		}
		if pin == "" {
			res.Errors = append(res.Errors, ImportError{Baris: nomorBaris, Pesan: "PIN tidak boleh kosong"})
			continue
		}

		// Cek duplikasi NIS dalam berkas yang sama
		if barisSebelumnya, exists := seenNIS[nis]; exists {
			res.Errors = append(res.Errors, ImportError{
				Baris: nomorBaris,
				Pesan: fmt.Sprintf("NIS %s duplikat dengan baris %d", nis, barisSebelumnya),
			})
			continue
		}
		seenNIS[nis] = nomorBaris

		res.Valid = append(res.Valid, SiswaImportItem{
			Baris: nomorBaris,
			NIS:   nis,
			Nama:  nama,
			Kelas: kelas,
			PIN:   pin,
		})
	}

	return res, nil
}
