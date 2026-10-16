package sesi_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"cbt-go/internal/sesi"
	"cbt-go/internal/testdb"
)

func setupSinkronData(t *testing.T, db *sql.DB) (int, int, int, int, int) {
	t.Helper()
	ctx := context.Background()

	res, err := db.ExecContext(ctx, "INSERT INTO guru (nip, nama, username, password) VALUES ('19800101', 'Guru Sinkron', 'guru_sinkron', 'hash')")
	if err != nil {
		t.Fatalf("insert guru: %v", err)
	}
	guruID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO kelas (nama_kelas) VALUES ('VIII-A')")
	if err != nil {
		t.Fatalf("insert kelas: %v", err)
	}
	kelasID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO mata_pelajaran (nama_mapel) VALUES ('IPA')")
	if err != nil {
		t.Fatalf("insert mapel: %v", err)
	}
	mapelID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO pengajaran (guru_id, kelas_id, mapel_id) VALUES (?, ?, ?)", guruID, kelasID, mapelID)
	if err != nil {
		t.Fatalf("insert pengajaran: %v", err)
	}
	pengajaranID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('T0010', 'Siswa Sinkron', 'VIII-A', 'hash')")
	if err != nil {
		t.Fatalf("insert siswa: %v", err)
	}
	siswaID, _ := res.LastInsertId()

	// Ujian 1 (Ujian Aktif)
	res, err = db.ExecContext(ctx, `
		INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai, acak_soal, acak_pilihan, batas_pelanggaran)
		VALUES (?, 'Ujian IPA 1', 60, NOW() - INTERVAL 10 MINUTE, NOW() + INTERVAL 2 HOUR, 1, 1, 3)
	`, pengajaranID)
	if err != nil {
		t.Fatalf("insert ujian 1: %v", err)
	}
	ujianID, _ := res.LastInsertId()

	// Ujian 2 (Ujian Lain untuk tes penolakan soal ujian lain)
	res, err = db.ExecContext(ctx, `
		INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai, acak_soal, acak_pilihan, batas_pelanggaran)
		VALUES (?, 'Ujian IPA 2', 60, NOW() - INTERVAL 10 MINUTE, NOW() + INTERVAL 2 HOUR, 1, 1, 3)
	`, pengajaranID)
	if err != nil {
		t.Fatalf("insert ujian 2: %v", err)
	}
	ujianLainID, _ := res.LastInsertId()

	// Soal 1 (PG, kunci B)
	resSoal1, err := db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
		VALUES (?, 'pg', 'Soal 1 IPA', 2, 'A', 'B', 'C', 'D', 'B')
	`, ujianID)
	if err != nil {
		t.Fatalf("insert soal 1: %v", err)
	}
	soal1ID, _ := resSoal1.LastInsertId()

	// Soal 2 (Menjodohkan)
	pasanganKunci := `[{"kiri":"Padat","kanan":"Batu"},{"kiri":"Cair","kanan":"Air"}]`
	opsiJSON := `{"pasangan":[{"kiri":"Padat","kanan":"Batu"},{"kiri":"Cair","kanan":"Air"}],"pengecoh":["Gas"]}`
	resSoal2, err := db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, opsi_tambahan, jawaban_benar)
		VALUES (?, 'menjodohkan', 'Jodohkan wujud zat:', 2, ?, ?)
	`, ujianID, opsiJSON, pasanganKunci)
	if err != nil {
		t.Fatalf("insert soal 2: %v", err)
	}
	soal2ID, _ := resSoal2.LastInsertId()

	// Soal Ujian Lain (milik ujianLainID)
	resSoalLain, err := db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
		VALUES (?, 'pg', 'Soal Milik Ujian Lain', 2, 'A', 'B', 'C', 'D', 'A')
	`, ujianLainID)
	if err != nil {
		t.Fatalf("insert soal ujian lain: %v", err)
	}
	soalLainID, _ := resSoalLain.LastInsertId()

	// Buat sesi aktif
	_, err = sesi.MulaiAtauLanjut(ctx, db, int(siswaID), int(ujianID), "laptop")
	if err != nil {
		t.Fatalf("MulaiAtauLanjut: %v", err)
	}

	return int(siswaID), int(ujianID), int(soal1ID), int(soal2ID), int(soalLainID)
}

func TestSinkronJawabanIdempoten(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)
	ctx := context.Background()

	siswaID, ujianID, soal1ID, soal2ID, _ := setupSinkronData(t, db)

	// Batch 2 jawaban: Soal 1 (B = benar), Soal 2 (menjodohkan benar)
	pasanganJawab := `[{"kiri":"Padat","kanan":"Batu"},{"kiri":"Cair","kanan":"Air"}]`
	now := time.Now().UTC().Truncate(time.Second)

	items := []sesi.ItemJawaban{
		{SoalID: soal1ID, Jawaban: "B", ClientTS: now},
		{SoalID: soal2ID, Jawaban: pasanganJawab, ClientTS: now},
	}

	// 1. Pengiriman pertama
	hasil1, err := sesi.SinkronJawaban(ctx, db, siswaID, ujianID, items)
	if err != nil {
		t.Fatalf("SinkronJawaban 1 gagal: %v", err)
	}
	if hasil1.Tersinkron != 2 || hasil1.Diabaikan != 0 {
		t.Errorf("hasil 1 salah: %+v, want tersinkron=2, diabaikan=0", hasil1)
	}

	// Periksa baris di database
	rows1, err := db.QueryContext(ctx, `
		SELECT soal_id, jawaban_dipilih, is_benar
		FROM jawaban_siswa
		WHERE siswa_id = ? AND ujian_id = ?
		ORDER BY soal_id ASC
	`, siswaID, ujianID)
	if err != nil {
		t.Fatalf("query jawaban 1: %v", err)
	}
	defer rows1.Close()

	type JRow struct {
		SoalID  int
		Jawaban string
		IsBenar int
	}
	var data1 []JRow
	for rows1.Next() {
		var r JRow
		if err := rows1.Scan(&r.SoalID, &r.Jawaban, &r.IsBenar); err != nil {
			t.Fatalf("scan 1: %v", err)
		}
		data1 = append(data1, r)
	}
	if len(data1) != 2 {
		t.Fatalf("jumlah jawaban di DB = %d, want 2", len(data1))
	}
	if data1[0].IsBenar != 1 || data1[1].IsBenar != 1 {
		t.Errorf("jawaban seharusnya bernilai is_benar=1: %+v", data1)
	}

	// 2. Pengiriman kedua (identik) -> idempoten, tidak duplikasi
	hasil2, err := sesi.SinkronJawaban(ctx, db, siswaID, ujianID, items)
	if err != nil {
		t.Fatalf("SinkronJawaban 2 gagal: %v", err)
	}
	if hasil2.Tersinkron != 2 || hasil2.Diabaikan != 0 {
		t.Errorf("hasil 2 salah: %+v, want tersinkron=2, diabaikan=0", hasil2)
	}

	rows2, err := db.QueryContext(ctx, `
		SELECT soal_id, jawaban_dipilih, is_benar
		FROM jawaban_siswa
		WHERE siswa_id = ? AND ujian_id = ?
		ORDER BY soal_id ASC
	`, siswaID, ujianID)
	if err != nil {
		t.Fatalf("query jawaban 2: %v", err)
	}
	defer rows2.Close()

	var data2 []JRow
	for rows2.Next() {
		var r JRow
		if err := rows2.Scan(&r.SoalID, &r.Jawaban, &r.IsBenar); err != nil {
			t.Fatalf("scan 2: %v", err)
		}
		data2 = append(data2, r)
	}
	if len(data2) != 2 {
		t.Fatalf("jumlah jawaban di DB setelah kirim kedua = %d, want 2 (tidak boleh duplikat)", len(data2))
	}
	for i := range data1 {
		if data1[i] != data2[i] {
			t.Errorf("data tidak identik pada indeks %d: %+v vs %+v", i, data1[i], data2[i])
		}
	}
}

func TestSinkronJawabanClientTsLamaDitolak(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)
	ctx := context.Background()

	siswaID, ujianID, soal1ID, _, _ := setupSinkronData(t, db)

	// Simpan jawaban baru terlebih dahulu: jawaban 'B' pada t_baru
	tBaru := time.Now().UTC().Add(-5 * time.Second).Truncate(time.Second)
	itemsBaru := []sesi.ItemJawaban{
		{SoalID: soal1ID, Jawaban: "B", ClientTS: tBaru},
	}
	hasilBaru, err := sesi.SinkronJawaban(ctx, db, siswaID, ujianID, itemsBaru)
	if err != nil {
		t.Fatalf("simpan jawaban baru gagal: %v", err)
	}
	if hasilBaru.Tersinkron != 1 {
		t.Fatalf("tersinkron baru = %d, want 1", hasilBaru.Tersinkron)
	}

	// Sekarang kirim jawaban 'A' dengan timestamp lebih lampau (15 detik lalu)
	tLama := time.Now().UTC().Add(-15 * time.Second).Truncate(time.Second)
	itemsLama := []sesi.ItemJawaban{
		{SoalID: soal1ID, Jawaban: "A", ClientTS: tLama},
	}
	hasilLama, err := sesi.SinkronJawaban(ctx, db, siswaID, ujianID, itemsLama)
	if err != nil {
		t.Fatalf("simpan jawaban lama gagal: %v", err)
	}
	if hasilLama.Diabaikan != 1 || hasilLama.Tersinkron != 0 {
		t.Errorf("jawaban usang harus diabaikan: %+v", hasilLama)
	}

	// Jawaban di DB harus TETAP 'B'
	var jawabanDB string
	err = db.QueryRowContext(ctx, "SELECT jawaban_dipilih FROM jawaban_siswa WHERE siswa_id = ? AND soal_id = ?", siswaID, soal1ID).Scan(&jawabanDB)
	if err != nil {
		t.Fatalf("query jawaban: %v", err)
	}
	if jawabanDB != "B" {
		t.Errorf("jawaban di DB = %q, want 'B' (jawaban lama tidak boleh menimpa)", jawabanDB)
	}
}

func TestSinkronJawabanClientTsMasaDepanDibatasi(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)
	ctx := context.Background()

	siswaID, ujianID, soal1ID, _, _ := setupSinkronData(t, db)

	// Kirim dengan client_ts 1 hari di masa depan
	tDepan := time.Now().UTC().Add(24 * time.Hour)
	items1 := []sesi.ItemJawaban{
		{SoalID: soal1ID, Jawaban: "A", ClientTS: tDepan},
	}
	_, err := sesi.SinkronJawaban(ctx, db, siswaID, ujianID, items1)
	if err != nil {
		t.Fatalf("kirim masa depan gagal: %v", err)
	}

	// Kirim lagi beberapa detik kemudian dengan jawaban 'B'
	tNormal := time.Now().UTC().Add(1 * time.Second)
	items2 := []sesi.ItemJawaban{
		{SoalID: soal1ID, Jawaban: "B", ClientTS: tNormal},
	}
	hasil2, err := sesi.SinkronJawaban(ctx, db, siswaID, ujianID, items2)
	if err != nil {
		t.Fatalf("kirim kedua gagal: %v", err)
	}
	if hasil2.Tersinkron != 1 {
		t.Errorf("jawaban kedua seharusnya tersinkron karena waktu masa depan dibatasi: %+v", hasil2)
	}

	var jawabanDB string
	err = db.QueryRowContext(ctx, "SELECT jawaban_dipilih FROM jawaban_siswa WHERE siswa_id = ? AND soal_id = ?", siswaID, soal1ID).Scan(&jawabanDB)
	if err != nil {
		t.Fatalf("query jawaban: %v", err)
	}
	if jawabanDB != "B" {
		t.Errorf("jawaban di DB = %q, want 'B'", jawabanDB)
	}
}

func TestSinkronJawabanSoalUjianLainDitolak(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)
	ctx := context.Background()

	siswaID, ujianID, soal1ID, _, soalLainID := setupSinkronData(t, db)

	// Batch memuat soal1ID (milik ujian) dan soalLainID (bukan milik ujian)
	items := []sesi.ItemJawaban{
		{SoalID: soal1ID, Jawaban: "B"},
		{SoalID: soalLainID, Jawaban: "A"},
	}

	_, err := sesi.SinkronJawaban(ctx, db, siswaID, ujianID, items)
	if err == nil {
		t.Fatal("diharapkan error ketika mengirim soal milik ujian lain, dapat nil")
	}
	if !errors.Is(err, sesi.ErrSoalBukanMilikUjian) {
		t.Errorf("err = %v, want ErrSoalBukanMilikUjian", err)
	}

	// Pastikan transaksi atomic: tidak ada jawaban yang tersimpan
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM jawaban_siswa WHERE siswa_id = ?", siswaID).Scan(&count)
	if err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 0 {
		t.Errorf("count jawaban di DB = %d, want 0 (rollback)", count)
	}
}

func TestSinkronJawabanLewatBatasWaktuDanGraceDitolak(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)
	ctx := context.Background()

	siswaID, ujianID, soal1ID, _, _ := setupSinkronData(t, db)

	// Ubah batas_waktu sesi ke 10 menit yang lalu (melebihi grace 120 detik)
	_, err := db.ExecContext(ctx, `
		UPDATE sesi_ujian
		SET batas_waktu = NOW() - INTERVAL 10 MINUTE
		WHERE siswa_id = ? AND ujian_id = ?
	`, siswaID, ujianID)
	if err != nil {
		t.Fatalf("update batas_waktu: %v", err)
	}

	items := []sesi.ItemJawaban{
		{SoalID: soal1ID, Jawaban: "B"},
	}

	_, err = sesi.SinkronJawaban(ctx, db, siswaID, ujianID, items)
	if err == nil {
		t.Fatal("diharapkan error waktu habis, dapat nil")
	}
	if !errors.Is(err, sesi.ErrWaktuUjianHabis) {
		t.Errorf("err = %v, want ErrWaktuUjianHabis", err)
	}
}
