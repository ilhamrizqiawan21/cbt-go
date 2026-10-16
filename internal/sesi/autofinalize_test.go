package sesi_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"cbt-go/internal/sesi"
	"cbt-go/internal/testdb"
)

func setupAutoFinalizeData(t *testing.T, db *sql.DB) (int, int, int, int) {
	t.Helper()
	testdb.Reset(t, db)
	ctx := context.Background()

	res, err := db.ExecContext(ctx, "INSERT INTO guru (nip, nama, username, password) VALUES ('19800103', 'Guru Auto', 'guru_auto', 'hash')")
	if err != nil {
		t.Fatalf("insert guru: %v", err)
	}
	guruID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO kelas (nama_kelas) VALUES ('IX-A')")
	if err != nil {
		t.Fatalf("insert kelas: %v", err)
	}
	kelasID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO mata_pelajaran (nama_mapel) VALUES ('Bahasa Inggris')")
	if err != nil {
		t.Fatalf("insert mapel: %v", err)
	}
	mapelID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO pengajaran (guru_id, kelas_id, mapel_id) VALUES (?, ?, ?)", guruID, kelasID, mapelID)
	if err != nil {
		t.Fatalf("insert pengajaran: %v", err)
	}
	pengajaranID, _ := res.LastInsertId()

	// Siswa 1 (akan dibuat kadaluarsa)
	res, err = db.ExecContext(ctx, "INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('T0031', 'Siswa Expired', 'IX-A', 'hash')")
	if err != nil {
		t.Fatalf("insert siswa 1: %v", err)
	}
	siswa1ID, _ := res.LastInsertId()

	// Siswa 2 (sesi aktif normal)
	res, err = db.ExecContext(ctx, "INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('T0032', 'Siswa Aktif', 'IX-A', 'hash')")
	if err != nil {
		t.Fatalf("insert siswa 2: %v", err)
	}
	siswa2ID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, `
		INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai, acak_soal, acak_pilihan, batas_pelanggaran)
		VALUES (?, 'Ujian Bahasa Inggris', 60, NOW() - INTERVAL 2 HOUR, NOW() + INTERVAL 2 HOUR, 1, 1, 3)
	`, pengajaranID)
	if err != nil {
		t.Fatalf("insert ujian: %v", err)
	}
	ujianID, _ := res.LastInsertId()

	// 1 Soal PG (kunci A, poin 2)
	resSoal, err := db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
		VALUES (?, 'pg', 'What is the color of sky?', 2, 'Blue', 'Red', 'Green', 'Yellow', 'A')
	`, ujianID)
	if err != nil {
		t.Fatalf("insert soal: %v", err)
	}
	soalID, _ := resSoal.LastInsertId()

	return int(siswa1ID), int(siswa2ID), int(ujianID), int(soalID)
}

func TestTutupSesiKadaluarsa(t *testing.T) {
	db := testdb.Open(t)
	siswa1ID, siswa2ID, ujianID, soalID := setupAutoFinalizeData(t, db)
	ctx := context.Background()

	// Siswa 1 mulai ujian, lalu mundurkan batas_waktu ke 10 menit yang lalu (lewat batas + grace)
	_, err := sesi.MulaiAtauLanjut(ctx, db, siswa1ID, ujianID, "laptop")
	if err != nil {
		t.Fatalf("mulai siswa 1: %v", err)
	}
	_, err = db.ExecContext(ctx, "UPDATE sesi_ujian SET batas_waktu = NOW() - INTERVAL 10 MINUTE WHERE siswa_id = ? AND ujian_id = ?", siswa1ID, ujianID)
	if err != nil {
		t.Fatalf("update batas_waktu siswa 1: %v", err)
	}

	// Simpan jawaban siswa 1 (benar: A)
	items1 := []sesi.ItemJawabanInput{
		{SoalID: soalID, Jawaban: "A", ClientTS: time.Now().Add(-11 * time.Minute)},
	}
	// Langsung masukkan ke DB karena SinkronJawaban akan menolak waktu habis
	_, err = db.ExecContext(ctx, `
		INSERT INTO jawaban_siswa (siswa_id, ujian_id, soal_id, jawaban_dipilih, is_benar, client_ts)
		VALUES (?, ?, ?, 'A', 1, NOW() - INTERVAL 11 MINUTE)
	`, siswa1ID, ujianID, soalID)
	if err != nil {
		t.Fatalf("insert jawaban siswa 1: %v", err)
	}
	_ = items1

	// Siswa 2 mulai ujian normal (masih aktif)
	_, err = sesi.MulaiAtauLanjut(ctx, db, siswa2ID, ujianID, "hp")
	if err != nil {
		t.Fatalf("mulai siswa 2: %v", err)
	}

	// Jalankan TutupSesiKadaluarsa dengan grace 120 detik
	hasilList, err := sesi.TutupSesiKadaluarsa(ctx, db, 120)
	if err != nil {
		t.Fatalf("TutupSesiKadaluarsa gagal: %v", err)
	}

	// Harus memfinalisasi tepat 1 sesi (Siswa 1)
	if len(hasilList) != 1 {
		t.Fatalf("ekspektasi 1 sesi terfinalisasi, didapat %d", len(hasilList))
	}
	if hasilList[0].Nilai != 100 {
		t.Errorf("nilai siswa 1 harus 100, dapat %d", hasilList[0].Nilai)
	}

	// Periksa status siswa 1 di DB -> selesai
	var status1 string
	err = db.QueryRowContext(ctx, "SELECT status FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?", siswa1ID, ujianID).Scan(&status1)
	if err != nil {
		t.Fatalf("query status siswa 1: %v", err)
	}
	if status1 != "selesai" {
		t.Errorf("status siswa 1 harus selesai, dapat %s", status1)
	}

	// Periksa nilai_ujian siswa 1 terbuat
	var countNilai1 int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM nilai_ujian WHERE siswa_id = ? AND ujian_id = ?", siswa1ID, ujianID).Scan(&countNilai1)
	if err != nil {
		t.Fatalf("count nilai siswa 1: %v", err)
	}
	if countNilai1 != 1 {
		t.Errorf("nilai_ujian siswa 1 harus tepat 1, dapat %d", countNilai1)
	}

	// Periksa status siswa 2 di DB -> tetap sedang_ujian
	var status2 string
	err = db.QueryRowContext(ctx, "SELECT status FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?", siswa2ID, ujianID).Scan(&status2)
	if err != nil {
		t.Fatalf("query status siswa 2: %v", err)
	}
	if status2 != "sedang_ujian" {
		t.Errorf("status siswa 2 harus tetap sedang_ujian, dapat %s", status2)
	}
}

func TestAutoFinalizeJob_TickerDanCancel(t *testing.T) {
	db := testdb.Open(t)
	siswa1ID, _, ujianID, _ := setupAutoFinalizeData(t, db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Buat sesi siswa 1 yang kadaluarsa
	_, err := sesi.MulaiAtauLanjut(ctx, db, siswa1ID, ujianID, "laptop")
	if err != nil {
		t.Fatalf("mulai siswa 1: %v", err)
	}
	_, err = db.ExecContext(ctx, "UPDATE sesi_ujian SET batas_waktu = NOW() - INTERVAL 5 MINUTE WHERE siswa_id = ? AND ujian_id = ?", siswa1ID, ujianID)
	if err != nil {
		t.Fatalf("update batas_waktu: %v", err)
	}

	// Jalankan job dengan interval sangat kecil (25 ms)
	job := sesi.StartAutoFinalizeJob(ctx, db, 25*time.Millisecond, 120, nil)

	// Tunggu job berjalan setidaknya satu atau dua tick
	time.Sleep(100 * time.Millisecond)

	// Pastikan sesi siswa 1 sudah otomatis ditutup menjadi selesai
	var status string
	err = db.QueryRowContext(ctx, "SELECT status FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?", siswa1ID, ujianID).Scan(&status)
	if err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "selesai" {
		t.Errorf("status sesi harus otomatis berubah menjadi selesai via background job, dapat %s", status)
	}

	// Stop job secara tertib
	job.Stop()
}
