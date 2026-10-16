package excel_test

import (
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"cbt-go/internal/excel"
)

func TestBuatEksporHasilUjian(t *testing.T) {
	items := []excel.HasilUjianItem{
		{
			NIS:           "1001",
			NamaSiswa:     "Budi Santoso",
			Kelas:         "VII-A",
			Benar:         8,
			Salah:         2,
			Kosong:        0,
			PoinOtomatis:  16,
			PoinEssay:     14,
			PoinMaks:      40,
			NilaiAkhir:    75,
			StatusKoreksi: "selesai",
			SelesaiPada:   time.Now(),
		},
		{
			NIS:           "1002",
			NamaSiswa:     "Siti Aminah",
			Kelas:         "VII-A",
			Benar:         10,
			Salah:         0,
			Kosong:        0,
			PoinOtomatis:  20,
			PoinEssay:     20,
			PoinMaks:      40,
			NilaiAkhir:    100,
			StatusKoreksi: "selesai",
			SelesaiPada:   time.Now(),
		},
	}

	buf, err := excel.BuatEksporHasilUjian("PTS Matematika", "Matematika", items)
	if err != nil {
		t.Fatalf("BuatEksporHasilUjian gagal: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("hasil buffer ekspor kosong")
	}

	// Baca balik dengan excelize untuk verifikasi validitas file
	f, err := excelize.OpenReader(buf)
	if err != nil {
		t.Fatalf("file excel yang dihasilkan tidak dapat dibaca kembali: %v", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)

	// Verifikasi Header baris 5
	h1, _ := f.GetCellValue(sheet, "B5")
	if h1 != "NIS" {
		t.Errorf("header B5 salah: got %s, want NIS", h1)
	}

	// Verifikasi Data baris 6 (Budi Santoso)
	nis, _ := f.GetCellValue(sheet, "B6")
	if nis != "1001" {
		t.Errorf("NIS baris 6 salah: got %s, want 1001", nis)
	}
	nama, _ := f.GetCellValue(sheet, "C6")
	if nama != "Budi Santoso" {
		t.Errorf("Nama baris 6 salah: got %s, want Budi Santoso", nama)
	}
	nilai, _ := f.GetCellValue(sheet, "K6")
	if nilai != "75" {
		t.Errorf("Nilai baris 6 salah: got %s, want 75", nilai)
	}
}
