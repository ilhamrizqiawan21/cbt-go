package sesi_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cbt-go/internal/sesi"
	"cbt-go/internal/testdb"
)

type harnessSinkron struct {
	db      *sql.DB
	siswaID int
	ujianID int
	soal1ID int
	soal2ID int
	sesiID  int
}

func setupHarnessSinkron(t *testing.T) *harnessSinkron {
	t.Helper()
	db := testdb.Open(t)
	testdb.Reset(t, db)

	// Buat kelas, mapel, guru, pengajaran
	resKelas, err := db.Exec("INSERT INTO kelas (nama_kelas) VALUES ('VII-A')")
	if err != nil {
		t.Fatalf("insert kelas: %v", err)
	}
	kelasID, _ := resKelas.LastInsertId()

	resMapel, err := db.Exec("INSERT INTO mata_pelajaran (nama_mapel) VALUES ('Matematika')")
	if err != nil {
		t.Fatalf("insert mapel: %v", err)
	}
	mapelID, _ := resMapel.LastInsertId()

	resGuru, err := db.Exec("INSERT INTO guru (nip, nama, username, password) VALUES ('19800101', 'Guru 1', 'guru1', 'hash')")
	if err != nil {
		t.Fatalf("insert guru: %v", err)
	}
	guruID, _ := resGuru.LastInsertId()

	resPengajaran, err := db.Exec("INSERT INTO pengajaran (guru_id, mapel_id, kelas_id) VALUES (?, ?, ?)", guruID, mapelID, kelasID)
	if err != nil {
		t.Fatalf("insert pengajaran: %v", err)
	}
	pengajaranID, _ := resPengajaran.LastInsertId()

	// Buat siswa
	resSiswa, err := db.Exec("INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('T0001', 'Siswa 1', 'VII-A', 'hash')")
	if err != nil {
		t.Fatalf("insert siswa: %v", err)
	}
	siswaID, _ := resSiswa.LastInsertId()

	// Buat ujian
	resUjian, err := db.Exec(`
		INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai)
		VALUES (?, 'Ujian Harian', 60, NOW() - INTERVAL 10 MINUTE, NOW() + INTERVAL 50 MINUTE)
	`, pengajaranID)
	if err != nil {
		t.Fatalf("insert ujian: %v", err)
	}
	ujianID, _ := resUjian.LastInsertId()

	// Buat soal 1 (PG kunci B)
	resSoal1, err := db.Exec(`
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
		VALUES (?, 'pg', '1 + 1 = ?', 2, '1', '2', '3', '4', 'B')
	`, ujianID)
	if err != nil {
		t.Fatalf("insert soal 1: %v", err)
	}
	soal1ID, _ := resSoal1.LastInsertId()

	// Buat soal 2 (PG kunci B)
	resSoal2, err := db.Exec(`
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
		VALUES (?, 'pg', 'Ibu kota Indonesia?', 2, 'Bandung', 'Jakarta', 'Surabaya', 'Medan', 'B')
	`, ujianID)
	if err != nil {
		t.Fatalf("insert soal 2: %v", err)
	}
	soal2ID, _ := resSoal2.LastInsertId()

	// Buat sesi ujian aktif
	hasilMulai, err := sesi.MulaiAtauLanjut(context.Background(), db, int(siswaID), int(ujianID), "laptop")
	if err != nil {
		t.Fatalf("mulai sesi: %v", err)
	}

	return &harnessSinkron{
		db:      db,
		siswaID: int(siswaID),
		ujianID: int(ujianID),
		soal1ID: int(soal1ID),
		soal2ID: int(soal2ID),
		sesiID:  hasilMulai.Sesi.ID,
	}
}

func TestSinkronJawabanBatch(t *testing.T) {
	ctx := context.Background()
	h := setupHarnessSinkron(t)

	t.Run("1. Idempoten: kirim batch yang sama dua kali menghasilkan data identik di DB", func(t *testing.T) {
		t1 := time.Now().Add(-20 * time.Second).Truncate(time.Second)
		t2 := time.Now().Add(-15 * time.Second).Truncate(time.Second)

		batch := []sesi.ItemJawabanInput{
			{SoalID: h.soal1ID, Jawaban: "B", ClientTS: t1},
			{SoalID: h.soal2ID, Jawaban: "B", ClientTS: t2},
		}

		// Kirim pertama
		hasil1, err := sesi.SinkronJawaban(ctx, h.db, h.siswaID, h.ujianID, batch)
		if err != nil {
			t.Fatalf("sinkron 1 error: %v", err)
		}
		if hasil1.Tersinkron != 2 || hasil1.Diabaikan != 0 {
			t.Errorf("hasil1 = %+v, want tersinkron=2, diabaikan=0", hasil1)
		}

		// Verifikasi di database
		var jawaban1, jawaban2 string
		var benar1, benar2 sql.NullInt64
		_ = h.db.QueryRow("SELECT jawaban_dipilih, is_benar FROM jawaban_siswa WHERE siswa_id = ? AND soal_id = ?", h.siswaID, h.soal1ID).Scan(&jawaban1, &benar1)
		_ = h.db.QueryRow("SELECT jawaban_dipilih, is_benar FROM jawaban_siswa WHERE siswa_id = ? AND soal_id = ?", h.siswaID, h.soal2ID).Scan(&jawaban2, &benar2)

		if jawaban1 != "B" || benar1.Int64 != 1 {
			t.Errorf("soal 1: jawaban=%q, is_benar=%d", jawaban1, benar1.Int64)
		}
		if jawaban2 != "B" || benar2.Int64 != 1 {
			t.Errorf("soal 2: jawaban=%q, is_benar=%d", jawaban2, benar2.Int64)
		}

		// Kirim kedua (payload identik)
		hasil2, err := sesi.SinkronJawaban(ctx, h.db, h.siswaID, h.ujianID, batch)
		if err != nil {
			t.Fatalf("sinkron 2 error: %v", err)
		}
		if hasil2.Tersinkron != 2 || hasil2.Diabaikan != 0 {
			t.Errorf("hasil2 = %+v, want tersinkron=2, diabaikan=0", hasil2)
		}

		// Pastikan baris di DB tidak berlipat ganda
		var count int
		_ = h.db.QueryRow("SELECT COUNT(*) FROM jawaban_siswa WHERE siswa_id = ? AND ujian_id = ?", h.siswaID, h.ujianID).Scan(&count)
		if count != 2 {
			t.Errorf("jumlah jawaban di DB = %d, want 2", count)
		}
	})

	t.Run("2. client_ts lebih usang tidak menimpa jawaban baru", func(t *testing.T) {
		tBaru := time.Now().Add(-5 * time.Second).Truncate(time.Second)
		batchBaru := []sesi.ItemJawabanInput{
			{SoalID: h.soal1ID, Jawaban: "B", ClientTS: tBaru},
		}
		_, err := sesi.SinkronJawaban(ctx, h.db, h.siswaID, h.ujianID, batchBaru)
		if err != nil {
			t.Fatalf("kirim batch baru gagal: %v", err)
		}

		// Kirim jawaban dengan client_ts 15 detik yang lalu (lebih usang)
		tLama := time.Now().Add(-20 * time.Second).Truncate(time.Second)
		batchLama := []sesi.ItemJawabanInput{
			{SoalID: h.soal1ID, Jawaban: "A", ClientTS: tLama},
		}
		hasilLama, err := sesi.SinkronJawaban(ctx, h.db, h.siswaID, h.ujianID, batchLama)
		if err != nil {
			t.Fatalf("kirim batch lama gagal: %v", err)
		}
		if hasilLama.Diabaikan != 1 || hasilLama.Tersinkron != 0 {
			t.Errorf("hasilLama = %+v, want diabaikan=1, tersinkron=0", hasilLama)
		}

		// Pastikan jawaban di DB tetap "B"
		var finalJawaban string
		_ = h.db.QueryRow("SELECT jawaban_dipilih FROM jawaban_siswa WHERE siswa_id = ? AND soal_id = ?", h.siswaID, h.soal1ID).Scan(&finalJawaban)
		if finalJawaban != "B" {
			t.Errorf("jawaban di DB = %q, want 'B' (tidak boleh tertimpa jawaban lama)", finalJawaban)
		}
	})

	t.Run("3. Soal ujian lain ditolak", func(t *testing.T) {
		// Buat ujian kedua
		resUjianLain, err := h.db.Exec(`
			INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai)
			VALUES (1, 'Ujian Lain', 60, NOW() - INTERVAL 10 MINUTE, NOW() + INTERVAL 50 MINUTE)
		`)
		if err != nil {
			t.Fatalf("insert ujian lain: %v", err)
		}
		ujianLainID, _ := resUjianLain.LastInsertId()

		// Buat soal milik ujian lain
		resSoalLain, err := h.db.Exec(`
			INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, jawaban_benar)
			VALUES (?, 'pg', 'Soal ujian asing', 2, 'A', 'B', 'A')
		`, ujianLainID)
		if err != nil {
			t.Fatalf("insert soal lain: %v", err)
		}
		soalAsingID, _ := resSoalLain.LastInsertId()

		batch := []sesi.ItemJawabanInput{
			{SoalID: h.soal1ID, Jawaban: "B"},
			{SoalID: int(soalAsingID), Jawaban: "A"},
		}

		_, err = sesi.SinkronJawaban(ctx, h.db, h.siswaID, h.ujianID, batch)
		if !errors.Is(err, sesi.ErrSoalUjianLain) {
			t.Errorf("err = %v, want ErrSoalUjianLain", err)
		}
	})

	t.Run("4. Batasi ukuran batch maksimal 100 item", func(t *testing.T) {
		batchBesar := make([]sesi.ItemJawabanInput, 101)
		for i := 0; i < 101; i++ {
			batchBesar[i] = sesi.ItemJawabanInput{SoalID: h.soal1ID, Jawaban: "A"}
		}

		_, err := sesi.SinkronJawaban(ctx, h.db, h.siswaID, h.ujianID, batchBesar)
		if !errors.Is(err, sesi.ErrBatchTerlaluBesar) {
			t.Errorf("err = %v, want ErrBatchTerlaluBesar", err)
		}
	})

	t.Run("5. Menolak jika waktu melewati batas_waktu + grace", func(t *testing.T) {
		// Mundurkan batas_waktu sesi ke 10 menit yang lalu (melewati grace 120 detik)
		_, _ = h.db.Exec("UPDATE sesi_ujian SET batas_waktu = NOW() - INTERVAL 10 MINUTE WHERE id = ?", h.sesiID)

		batch := []sesi.ItemJawabanInput{
			{SoalID: h.soal1ID, Jawaban: "B"},
		}
		_, err := sesi.SinkronJawaban(ctx, h.db, h.siswaID, h.ujianID, batch)
		if !errors.Is(err, sesi.ErrWaktuHabis) {
			t.Errorf("err = %v, want ErrWaktuHabis", err)
		}

		// Kembalikan batas_waktu
		_, _ = h.db.Exec("UPDATE sesi_ujian SET batas_waktu = NOW() + INTERVAL 50 MINUTE WHERE id = ?", h.sesiID)
	})

	t.Run("6. Jawaban menjodohkan dan essay", func(t *testing.T) {
		// Buat soal menjodohkan
		pasanganJSON := `[{"kiri":"1","kanan":"A"},{"kiri":"2","kanan":"B"}]`
		resMenjodohkan, _ := h.db.Exec(`
			INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, jawaban_benar)
			VALUES (?, 'menjodohkan', 'Jodohkan angka & huruf', 2, ?)
		`, h.ujianID, pasanganJSON)
		menjodohkanID, _ := resMenjodohkan.LastInsertId()

		// Buat soal essay
		resEssay, _ := h.db.Exec(`
			INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin)
			VALUES (?, 'essay', 'Jelaskan konsep X', 4)
		`, h.ujianID)
		essayID, _ := resEssay.LastInsertId()

		batch := []sesi.ItemJawabanInput{
			{SoalID: int(menjodohkanID), Jawaban: json.RawMessage(pasanganJSON)},
			{SoalID: int(essayID), Jawaban: "Jawaban essay siswa"},
		}

		hasil, err := sesi.SinkronJawaban(ctx, h.db, h.siswaID, h.ujianID, batch)
		if err != nil {
			t.Fatalf("sinkron menjodohkan/essay gagal: %v", err)
		}
		if hasil.Tersinkron != 2 {
			t.Errorf("tersinkron = %d, want 2", hasil.Tersinkron)
		}

		// Periksa is_benar
		var benarMenjodohkan sql.NullInt64
		var benarEssay sql.NullInt64
		_ = h.db.QueryRow("SELECT is_benar FROM jawaban_siswa WHERE siswa_id = ? AND soal_id = ?", h.siswaID, menjodohkanID).Scan(&benarMenjodohkan)
		_ = h.db.QueryRow("SELECT is_benar FROM jawaban_siswa WHERE siswa_id = ? AND soal_id = ?", h.siswaID, essayID).Scan(&benarEssay)

		if benarMenjodohkan.Int64 != 1 {
			t.Errorf("is_benar menjodohkan = %d, want 1", benarMenjodohkan.Int64)
		}
		if benarEssay.Valid {
			t.Errorf("is_benar essay harus NULL, got %d", benarEssay.Int64)
		}
	})
}
