package sesi_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"cbt-go/internal/penilaian"
	"cbt-go/internal/sesi"
	"cbt-go/internal/testdb"
)

func setupFinalizeData(t *testing.T, db *sql.DB) (int, int, int, int, int) {
	t.Helper()
	testdb.Reset(t, db)
	ctx := context.Background()

	res, err := db.ExecContext(ctx, "INSERT INTO guru (nip, nama, username, password) VALUES ('19800102', 'Guru Finalize', 'guru_finalize', 'hash')")
	if err != nil {
		t.Fatalf("insert guru: %v", err)
	}
	guruID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO kelas (nama_kelas) VALUES ('VIII-B')")
	if err != nil {
		t.Fatalf("insert kelas: %v", err)
	}
	kelasID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO mata_pelajaran (nama_mapel) VALUES ('Matematika')")
	if err != nil {
		t.Fatalf("insert mapel: %v", err)
	}
	mapelID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO pengajaran (guru_id, kelas_id, mapel_id) VALUES (?, ?, ?)", guruID, kelasID, mapelID)
	if err != nil {
		t.Fatalf("insert pengajaran: %v", err)
	}
	pengajaranID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('T0020', 'Siswa Finalize', 'VIII-B', 'hash')")
	if err != nil {
		t.Fatalf("insert siswa: %v", err)
	}
	siswaID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, `
		INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai, acak_soal, acak_pilihan, batas_pelanggaran)
		VALUES (?, 'Ujian Matematika Final', 60, NOW() - INTERVAL 10 MINUTE, NOW() + INTERVAL 2 HOUR, 1, 1, 3)
	`, pengajaranID)
	if err != nil {
		t.Fatalf("insert ujian: %v", err)
	}
	ujianID, _ := res.LastInsertId()

	// Soal 1: PG (poin 2, kunci B)
	resSoal1, err := db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
		VALUES (?, 'pg', '1 + 1 = ?', 2, '1', '2', '3', '4', 'B')
	`, ujianID)
	if err != nil {
		t.Fatalf("insert soal 1: %v", err)
	}
	soal1ID, _ := resSoal1.LastInsertId()

	// Soal 2: Menjodohkan (poin 2)
	kunciMenjodohkan := `[{"kiri":"A","kanan":"1"},{"kiri":"B","kanan":"2"}]`
	resSoal2, err := db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, jawaban_benar)
		VALUES (?, 'menjodohkan', 'Jodohkan huruf dan angka', 2, ?)
	`, ujianID, kunciMenjodohkan)
	if err != nil {
		t.Fatalf("insert soal 2: %v", err)
	}
	soal2ID, _ := resSoal2.LastInsertId()

	// Soal 3: Essay (poin 4)
	resSoal3, err := db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin)
		VALUES (?, 'essay', 'Jelaskan teori relativitas secara singkat', 4)
	`, ujianID)
	if err != nil {
		t.Fatalf("insert soal 3: %v", err)
	}
	soal3ID, _ := resSoal3.LastInsertId()

	return int(siswaID), int(ujianID), int(soal1ID), int(soal2ID), int(soal3ID)
}

func TestUpdateHeartbeat(t *testing.T) {
	db := testdb.Open(t)
	siswaID, ujianID, _, _, _ := setupFinalizeData(t, db)
	ctx := context.Background()

	// Buat sesi ujian
	_, err := sesi.MulaiAtauLanjut(ctx, db, siswaID, ujianID, "laptop")
	if err != nil {
		t.Fatalf("mulai sesi: %v", err)
	}

	// Mundurkan last_seen 2 menit
	_, err = db.ExecContext(ctx, "UPDATE sesi_ujian SET last_seen = NOW() - INTERVAL 2 MINUTE WHERE siswa_id = ? AND ujian_id = ?", siswaID, ujianID)
	if err != nil {
		t.Fatalf("update last_seen awal: %v", err)
	}

	var waktuLama time.Time
	err = db.QueryRowContext(ctx, "SELECT last_seen FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?", siswaID, ujianID).Scan(&waktuLama)
	if err != nil {
		t.Fatalf("scan waktu lama: %v", err)
	}

	// Panggil UpdateHeartbeat
	if err := sesi.UpdateHeartbeat(ctx, db, siswaID, ujianID); err != nil {
		t.Fatalf("UpdateHeartbeat gagal: %v", err)
	}

	var waktuBaru time.Time
	err = db.QueryRowContext(ctx, "SELECT last_seen FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?", siswaID, ujianID).Scan(&waktuBaru)
	if err != nil {
		t.Fatalf("scan waktu baru: %v", err)
	}

	if !waktuBaru.After(waktuLama) {
		t.Errorf("waktu baru (%v) harus lebih baru dari waktu lama (%v)", waktuBaru, waktuLama)
	}
}

func TestFinalizeSesiDanStatusKoreksi(t *testing.T) {
	db := testdb.Open(t)
	siswaID, ujianID, soal1ID, soal2ID, _ := setupFinalizeData(t, db)
	ctx := context.Background()

	_, err := sesi.MulaiAtauLanjut(ctx, db, siswaID, ujianID, "laptop")
	if err != nil {
		t.Fatalf("mulai sesi: %v", err)
	}

	// Simpan jawaban siswa: soal 1 benar (B), soal 2 benar (menjodohkan cocok)
	items := []sesi.ItemJawabanInput{
		{SoalID: soal1ID, Jawaban: "B", ClientTS: time.Now()},
		{SoalID: soal2ID, Jawaban: `[{"kiri":"B","kanan":"2"},{"kiri":"A","kanan":"1"}]`, ClientTS: time.Now()},
	}
	_, err = sesi.SinkronJawaban(ctx, db, siswaID, ujianID, items)
	if err != nil {
		t.Fatalf("sinkron jawaban: %v", err)
	}

	// Finalisasi sesi
	hasil, err := sesi.Finalize(ctx, db, siswaID, ujianID)
	if err != nil {
		t.Fatalf("Finalize gagal: %v", err)
	}

	// Total poin: PG=2, Menjodohkan=2, Essay=4 -> Total Poin Maks = 8
	// Didapat: PG=2, Menjodohkan=2, Essay belum dinilai (0) -> Total = 4
	// Nilai = (4/8) * 100 = 50
	if hasil.PoinOtomatis != 4 {
		t.Errorf("ekspektasi poin otomatis 4, dapat %d", hasil.PoinOtomatis)
	}
	if hasil.PoinMaks != 8 {
		t.Errorf("ekspektasi poin maks 8, dapat %d", hasil.PoinMaks)
	}
	if hasil.Nilai != 50 {
		t.Errorf("ekspektasi nilai 50, dapat %d", hasil.Nilai)
	}
	if hasil.StatusKoreksi != penilaian.StatusMenungguEssay {
		t.Errorf("ekspektasi status koreksi menunggu_essay, dapat %s", hasil.StatusKoreksi)
	}

	// Periksa tabel nilai_ujian
	var nuNilai, nuBenar, nuSalah, nuKosong int
	var nuStatus string
	var selesaiPada sql.NullTime
	err = db.QueryRowContext(ctx, `
		SELECT nilai, benar, salah, kosong, status_koreksi, selesai_pada
		FROM nilai_ujian
		WHERE siswa_id = ? AND ujian_id = ?
	`, siswaID, ujianID).Scan(&nuNilai, &nuBenar, &nuSalah, &nuKosong, &nuStatus, &selesaiPada)
	if err != nil {
		t.Fatalf("query nilai_ujian: %v", err)
	}

	if nuNilai != 50 || nuBenar != 2 || nuSalah != 0 || nuKosong != 0 {
		t.Errorf("nilai_ujian mismatch: nilai=%d benar=%d salah=%d kosong=%d", nuNilai, nuBenar, nuSalah, nuKosong)
	}
	if nuStatus != "menunggu_essay" {
		t.Errorf("status nilai_ujian harus menunggu_essay, dapat %s", nuStatus)
	}
	if !selesaiPada.Valid {
		t.Errorf("selesai_pada di nilai_ujian harus terisi")
	}

	// Periksa status sesi_ujian berubah menjadi selesai
	var statusSesi string
	var sesiSelesaiPada sql.NullTime
	err = db.QueryRowContext(ctx, "SELECT status, selesai_pada FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?", siswaID, ujianID).Scan(&statusSesi, &sesiSelesaiPada)
	if err != nil {
		t.Fatalf("query sesi_ujian: %v", err)
	}
	if statusSesi != "selesai" {
		t.Errorf("status sesi harus selesai, dapat %s", statusSesi)
	}
	if !sesiSelesaiPada.Valid {
		t.Errorf("selesai_pada di sesi_ujian harus terisi")
	}

	// Sesi selesai harus menolak sinkron jawaban baru
	_, err = sesi.SinkronJawaban(ctx, db, siswaID, ujianID, items)
	if err == nil {
		t.Fatalf("sinkron jawaban setelah sesi selesai seharusnya ditolak")
	}
	if !errors.Is(err, sesi.ErrSesiSelesai) {
		t.Errorf("ekspektasi ErrSesiSelesai, dapat: %v", err)
	}
}

func TestFinalizeIdempotenDanKonkuren(t *testing.T) {
	db := testdb.Open(t)
	siswaID, ujianID, soal1ID, _, _ := setupFinalizeData(t, db)
	ctx := context.Background()

	_, err := sesi.MulaiAtauLanjut(ctx, db, siswaID, ujianID, "hp")
	if err != nil {
		t.Fatalf("mulai sesi: %v", err)
	}

	// Jawab 1 soal PG benar
	items := []sesi.ItemJawabanInput{
		{SoalID: soal1ID, Jawaban: "B", ClientTS: time.Now()},
	}
	_, err = sesi.SinkronJawaban(ctx, db, siswaID, ujianID, items)
	if err != nil {
		t.Fatalf("sinkron jawaban: %v", err)
	}

	// Panggil Finalize pertama kali
	res1, err := sesi.Finalize(ctx, db, siswaID, ujianID)
	if err != nil {
		t.Fatalf("finalize 1: %v", err)
	}

	// Panggil Finalize kedua kali
	res2, err := sesi.Finalize(ctx, db, siswaID, ujianID)
	if err != nil {
		t.Fatalf("finalize 2: %v", err)
	}

	if res1.Nilai != res2.Nilai || res1.PoinOtomatis != res2.PoinOtomatis {
		t.Errorf("hasil pemanggilan berulang harus sama: %+v vs %+v", res1, res2)
	}

	// Panggil paralel secara konkuren
	var wg sync.WaitGroup
	errCh := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, fErr := sesi.Finalize(ctx, db, siswaID, ujianID)
			if fErr != nil {
				errCh <- fErr
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for fErr := range errCh {
		t.Fatalf("finalize konkuren gagal: %v", fErr)
	}

	// Pastikan tepat ada satu baris di nilai_ujian
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM nilai_ujian WHERE siswa_id = ? AND ujian_id = ?", siswaID, ujianID).Scan(&count)
	if err != nil {
		t.Fatalf("count nilai_ujian: %v", err)
	}
	if count != 1 {
		t.Errorf("ekspektasi tepat 1 baris di nilai_ujian, didapat %d", count)
	}
}

func TestHitungUlangNilaiSetelahEssayDinilai(t *testing.T) {
	db := testdb.Open(t)
	siswaID, ujianID, soal1ID, soal2ID, soal3ID := setupFinalizeData(t, db)
	ctx := context.Background()

	_, err := sesi.MulaiAtauLanjut(ctx, db, siswaID, ujianID, "hp")
	if err != nil {
		t.Fatalf("mulai sesi: %v", err)
	}

	items := []sesi.ItemJawabanInput{
		{SoalID: soal1ID, Jawaban: "B", ClientTS: time.Now()},
		{SoalID: soal2ID, Jawaban: `[{"kiri":"A","kanan":"1"},{"kiri":"B","kanan":"2"}]`, ClientTS: time.Now()},
	}
	_, err = sesi.SinkronJawaban(ctx, db, siswaID, ujianID, items)
	if err != nil {
		t.Fatalf("sinkron jawaban: %v", err)
	}

	// Finalize sebelum essay dinilai
	h1, err := sesi.Finalize(ctx, db, siswaID, ujianID)
	if err != nil {
		t.Fatalf("finalize 1: %v", err)
	}
	if h1.StatusKoreksi != penilaian.StatusMenungguEssay {
		t.Fatalf("seharusnya status menunggu_essay")
	}

	// Guru memberikan skor essay (misal skor 4 dari poin maks 4)
	_, err = db.ExecContext(ctx, `
		INSERT INTO nilai_essay (siswa_id, ujian_id, soal_id, skor, dinilai_pada)
		VALUES (?, ?, ?, 4, NOW())
	`, siswaID, ujianID, soal3ID)
	if err != nil {
		t.Fatalf("insert nilai essay: %v", err)
	}

	// Panggil HitungUlangNilai
	h2, err := sesi.HitungUlangNilai(ctx, db, siswaID, ujianID)
	if err != nil {
		t.Fatalf("HitungUlangNilai gagal: %v", err)
	}

	if h2.StatusKoreksi != penilaian.StatusSelesai {
		t.Errorf("setelah essay dinilai status_koreksi harus selesai, dapat %s", h2.StatusKoreksi)
	}
	if h2.PoinEssay != 4 {
		t.Errorf("poin essay harus 4, dapat %d", h2.PoinEssay)
	}
	if h2.Nilai != 100 {
		t.Errorf("nilai harus 100 ((4+4)/8 * 100), dapat %d", h2.Nilai)
	}

	// Cek tabel nilai_ujian terupdate
	var nuNilai int
	var nuStatus string
	err = db.QueryRowContext(ctx, "SELECT nilai, status_koreksi FROM nilai_ujian WHERE siswa_id = ? AND ujian_id = ?", siswaID, ujianID).Scan(&nuNilai, &nuStatus)
	if err != nil {
		t.Fatalf("query nilai_ujian setelah hitung ulang: %v", err)
	}
	if nuNilai != 100 || nuStatus != "selesai" {
		t.Errorf("nilai_ujian belum terupdate dengan benar: nilai=%d status=%s", nuNilai, nuStatus)
	}
}
