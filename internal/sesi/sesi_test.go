package sesi_test

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"cbt-go/internal/sesi"
	"cbt-go/internal/testdb"
)

func TestHitungBatasWaktu(t *testing.T) {
	t.Run("durasi normal tanpa tambahan menit", func(t *testing.T) {
		mulai := time.Date(2026, 10, 7, 8, 0, 0, 0, time.UTC)
		durasi := 60
		batas := sesi.HitungBatasWaktu(mulai, durasi, 0, nil)
		want := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)

		if !batas.Equal(want) {
			t.Errorf("batas = %v, want %v", batas, want)
		}
	})

	t.Run("durasi dengan tambahan menit", func(t *testing.T) {
		mulai := time.Date(2026, 10, 7, 8, 0, 0, 0, time.UTC)
		durasi := 60
		tambahan := 15
		batas := sesi.HitungBatasWaktu(mulai, durasi, tambahan, nil)
		want := time.Date(2026, 10, 7, 9, 15, 0, 0, time.UTC)

		if !batas.Equal(want) {
			t.Errorf("batas = %v, want %v", batas, want)
		}
	})

	t.Run("tidak melewati tanggal_selesai ujian (D-007)", func(t *testing.T) {
		mulai := time.Date(2026, 10, 7, 8, 0, 0, 0, time.UTC)
		durasi := 120 // normalnya selesai 10:00
		selesaiUjian := time.Date(2026, 10, 7, 9, 30, 0, 0, time.UTC)

		batas := sesi.HitungBatasWaktu(mulai, durasi, 0, &selesaiUjian)
		if !batas.Equal(selesaiUjian) {
			t.Errorf("batas = %v, want dipotong ke %v", batas, selesaiUjian)
		}
	})

	t.Run("batas lebih awal dari tanggal_selesai ujian tidak dipotong", func(t *testing.T) {
		mulai := time.Date(2026, 10, 7, 8, 0, 0, 0, time.UTC)
		durasi := 60 // selesai 09:00
		selesaiUjian := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)

		batas := sesi.HitungBatasWaktu(mulai, durasi, 0, &selesaiUjian)
		want := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
		if !batas.Equal(want) {
			t.Errorf("batas = %v, want %v", batas, want)
		}
	})
}

func TestHitungSisaDetik(t *testing.T) {
	batas := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		now  time.Time
		want int
	}{
		{
			name: "sisa 10 menit",
			now:  time.Date(2026, 10, 7, 8, 50, 0, 0, time.UTC),
			want: 600,
		},
		{
			name: "sisa 30 detik",
			now:  time.Date(2026, 10, 7, 8, 59, 30, 0, time.UTC),
			want: 30,
		},
		{
			name: "tepat batas waktu",
			now:  time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC),
			want: 0,
		},
		{
			name: "lewat batas waktu bernilai 0",
			now:  time.Date(2026, 10, 7, 9, 5, 0, 0, time.UTC),
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sesi.HitungSisaDetik(batas, tt.now)
			if got != tt.want {
				t.Errorf("HitungSisaDetik = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBolehTerimaJawaban(t *testing.T) {
	batas := time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	grace := 120 // 120 detik

	tests := []struct {
		name      string
		status    string
		now       time.Time
		clientTs  time.Time
		wantBoleh bool
		wantGrace bool
		alasan    string
	}{
		{
			name:      "status keluar_paksa ditolak",
			status:    "keluar_paksa",
			now:       batas.Add(-10 * time.Minute),
			clientTs:  batas.Add(-10 * time.Minute),
			wantBoleh: false,
			alasan:    "keluar_paksa",
		},
		{
			name:      "status selesai ditolak",
			status:    "selesai",
			now:       batas.Add(-10 * time.Minute),
			clientTs:  batas.Add(-10 * time.Minute),
			wantBoleh: false,
			alasan:    "selesai",
		},
		{
			name:      "status selain sedang_ujian ditolak",
			status:    "draft",
			now:       batas.Add(-10 * time.Minute),
			clientTs:  batas.Add(-10 * time.Minute),
			wantBoleh: false,
			alasan:    "status_tidak_valid",
		},
		{
			name:      "normal sebelum batas waktu",
			status:    "sedang_ujian",
			now:       batas.Add(-5 * time.Minute),
			clientTs:  batas.Add(-5 * time.Minute),
			wantBoleh: true,
			wantGrace: false,
		},
		{
			name:      "dalam toleransi grace dan client_ts <= batas waktu (D-009)",
			status:    "sedang_ujian",
			now:       batas.Add(60 * time.Second),  // 60 detik setelah batas
			clientTs:  batas.Add(-10 * time.Second), // dijawab sebelum batas
			wantBoleh: true,
			wantGrace: true,
		},
		{
			name:      "dalam toleransi grace tetapi client_ts > batas waktu",
			status:    "sedang_ujian",
			now:       batas.Add(60 * time.Second),
			clientTs:  batas.Add(30 * time.Second), // dijawab setelah batas habis
			wantBoleh: false,
			alasan:    "client_ts_melewati_batas",
		},
		{
			name:      "melewati toleransi grace period",
			status:    "sedang_ujian",
			now:       batas.Add(125 * time.Second), // lewat 120 detik
			clientTs:  batas.Add(-10 * time.Second),
			wantBoleh: false,
			alasan:    "melewati_grace_period",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			izin := sesi.BolehTerimaJawaban(tt.status, batas, tt.clientTs, tt.now, grace)
			if izin.Boleh != tt.wantBoleh {
				t.Errorf("Boleh = %v, want %v", izin.Boleh, tt.wantBoleh)
			}
			if izin.Grace != tt.wantGrace {
				t.Errorf("Grace = %v, want %v", izin.Grace, tt.wantGrace)
			}
			if !tt.wantBoleh && izin.Alasan != tt.alasan {
				t.Errorf("Alasan = %q, want %q", izin.Alasan, tt.alasan)
			}
		})
	}
}

// ── Tes Integrasi Database (testdb) ──

func setupDBUjian(t *testing.T) (*sql.DB, int, int) {
	t.Helper()
	db := testdb.Open(t)
	testdb.Reset(t, db)

	// Buat kelas
	resKelas, err := db.Exec("INSERT INTO kelas (nama_kelas) VALUES ('VII-A')")
	if err != nil {
		t.Fatalf("insert kelas: %v", err)
	}
	kelasID, _ := resKelas.LastInsertId()

	// Buat mapel
	resMapel, err := db.Exec("INSERT INTO mata_pelajaran (nama_mapel) VALUES ('Matematika')")
	if err != nil {
		t.Fatalf("insert mapel: %v", err)
	}
	mapelID, _ := resMapel.LastInsertId()

	// Buat guru
	resGuru, err := db.Exec("INSERT INTO guru (nip, nama, username, password) VALUES ('19800101', 'Guru 1', 'guru1', 'hash')")
	if err != nil {
		t.Fatalf("insert guru: %v", err)
	}
	guruID, _ := resGuru.LastInsertId()

	// Buat pengajaran
	resPengajaran, err := db.Exec("INSERT INTO pengajaran (guru_id, mapel_id, kelas_id) VALUES (?, ?, ?)", guruID, mapelID, kelasID)
	if err != nil {
		t.Fatalf("insert pengajaran: %v", err)
	}
	pengajaranID, _ := resPengajaran.LastInsertId()

	// Buat siswa
	resSiswa, err := db.Exec("INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('1001', 'Siswa 1', 'VII-A', 'hash')")
	if err != nil {
		t.Fatalf("insert siswa: %v", err)
	}
	siswaID, _ := resSiswa.LastInsertId()

	// Buat ujian (durasi 60 menit)
	resUjian, err := db.Exec(`
		INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai)
		VALUES (?, 'Ujian Harian', 60, NOW() - INTERVAL 1 HOUR, NOW() + INTERVAL 2 HOUR)
	`, pengajaranID)
	if err != nil {
		t.Fatalf("insert ujian: %v", err)
	}
	ujianID, _ := resUjian.LastInsertId()

	return db, int(siswaID), int(ujianID)
}

func TestMulaiAtauLanjutDanTakeover(t *testing.T) {
	ctx := context.Background()
	db, siswaID, ujianID := setupDBUjian(t)

	// 1. Mulai sesi baru
	hasil1, err := sesi.MulaiAtauLanjut(ctx, db, siswaID, ujianID, "laptop")
	if err != nil {
		t.Fatalf("MulaiAtauLanjut baru: %v", err)
	}

	if hasil1.Status != sesi.MulaiBaru {
		t.Errorf("Status = %q, want %q", hasil1.Status, sesi.MulaiBaru)
	}
	if hasil1.TakeOver {
		t.Error("TakeOver should be false for new session")
	}
	if hasil1.Sesi.DeviceToken == "" {
		t.Error("DeviceToken should not be empty")
	}
	if hasil1.Sesi.Seed <= 0 {
		t.Errorf("Seed = %d, want > 0", hasil1.Sesi.Seed)
	}
	if hasil1.Sesi.Status != "sedang_ujian" {
		t.Errorf("Status = %q, want 'sedang_ujian'", hasil1.Sesi.Status)
	}

	waktuMulai1 := hasil1.Sesi.WaktuMulai
	seed1 := hasil1.Sesi.Seed
	token1 := hasil1.Sesi.DeviceToken

	// 2. Login ulang pada sesi aktif (take-over)
	hasil2, err := sesi.MulaiAtauLanjut(ctx, db, siswaID, ujianID, "hp")
	if err != nil {
		t.Fatalf("MulaiAtauLanjut takeover: %v", err)
	}

	if hasil2.Status != sesi.MulaiLanjut {
		t.Errorf("Status = %q, want %q", hasil2.Status, sesi.MulaiLanjut)
	}
	if !hasil2.TakeOver {
		t.Error("TakeOver should be true")
	}
	if hasil2.Sesi.DeviceToken == token1 {
		t.Errorf("DeviceToken harus berubah saat takeover: got %q, want different from %q", hasil2.Sesi.DeviceToken, token1)
	}
	if hasil2.Sesi.DeviceType != "hp" {
		t.Errorf("DeviceType = %q, want 'hp'", hasil2.Sesi.DeviceType)
	}
	if !hasil2.Sesi.WaktuMulai.Equal(waktuMulai1) {
		t.Errorf("WaktuMulai tidak boleh direset: got %v, want %v", hasil2.Sesi.WaktuMulai, waktuMulai1)
	}
	if hasil2.Sesi.Seed != seed1 {
		t.Errorf("Seed tidak boleh direset: got %d, want %d", hasil2.Sesi.Seed, seed1)
	}

	// Periksa log audit takeover tercatat
	var countAudit int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_admin WHERE sesi_id = ? AND aksi = 'takeover'", hasil1.Sesi.ID).Scan(&countAudit)
	if err != nil {
		t.Fatalf("query audit_admin: %v", err)
	}
	if countAudit < 1 {
		t.Errorf("audit_admin takeover count = %d, want >= 1", countAudit)
	}

	// 3. Sesi keluar paksa ditolak
	_, err = db.ExecContext(ctx, "UPDATE sesi_ujian SET status = 'keluar_paksa' WHERE id = ?", hasil1.Sesi.ID)
	if err != nil {
		t.Fatalf("update keluar_paksa: %v", err)
	}

	_, err = sesi.MulaiAtauLanjut(ctx, db, siswaID, ujianID, "laptop")
	if !errors.Is(err, sesi.ErrKeluarPaksa) {
		t.Errorf("err = %v, want ErrKeluarPaksa", err)
	}

	// 4. Sesi selesai ditolak
	_, err = db.ExecContext(ctx, "UPDATE sesi_ujian SET status = 'selesai' WHERE id = ?", hasil1.Sesi.ID)
	if err != nil {
		t.Fatalf("update selesai: %v", err)
	}

	_, err = sesi.MulaiAtauLanjut(ctx, db, siswaID, ujianID, "laptop")
	if !errors.Is(err, sesi.ErrSesiSelesai) {
		t.Errorf("err = %v, want ErrSesiSelesai", err)
	}
}

func TestPerpanjangWaktu(t *testing.T) {
	ctx := context.Background()
	db, siswaID, ujianID := setupDBUjian(t)

	hasil, err := sesi.MulaiAtauLanjut(ctx, db, siswaID, ujianID, "laptop")
	if err != nil {
		t.Fatalf("MulaiAtauLanjut: %v", err)
	}

	batasAwal := hasil.Sesi.BatasWaktu

	updated, err := sesi.Perpanjang(ctx, db, hasil.Sesi.ID, 15)
	if err != nil {
		t.Fatalf("Perpanjang: %v", err)
	}

	if updated.TambahanMenit != 15 {
		t.Errorf("TambahanMenit = %d, want 15", updated.TambahanMenit)
	}

	selisih := updated.BatasWaktu.Sub(batasAwal)
	if selisih != 15*time.Minute {
		t.Errorf("selisih batas waktu = %v, want 15m", selisih)
	}
}

func TestHitungPelanggaran(t *testing.T) {
	ctx := context.Background()
	db, siswaID, ujianID := setupDBUjian(t)

	// Awalnya 0 pelanggaran
	jml, err := sesi.HitungPelanggaran(ctx, db, siswaID, ujianID)
	if err != nil {
		t.Fatalf("HitungPelanggaran: %v", err)
	}
	if jml != 0 {
		t.Errorf("jml = %d, want 0", jml)
	}

	// Tambah 2 kecurangan sungguhan dan 1 ambil_alih_sesi (tidak dihitung)
	_, _ = db.ExecContext(ctx, "INSERT INTO log_kecurangan (siswa_id, ujian_id, jenis_kecurangan) VALUES (?, ?, 'pindah_tab')", siswaID, ujianID)
	_, _ = db.ExecContext(ctx, "INSERT INTO log_kecurangan (siswa_id, ujian_id, jenis_kecurangan) VALUES (?, ?, 'copy_paste')", siswaID, ujianID)
	_, _ = db.ExecContext(ctx, "INSERT INTO log_kecurangan (siswa_id, ujian_id, jenis_kecurangan) VALUES (?, ?, 'ambil_alih_sesi')", siswaID, ujianID)

	jml, err = sesi.HitungPelanggaran(ctx, db, siswaID, ujianID)
	if err != nil {
		t.Fatalf("HitungPelanggaran: %v", err)
	}
	if jml != 2 {
		t.Errorf("jml = %d, want 2", jml)
	}

	// Admin buka kunci -> pelanggaran setelah buka kunci dihitung dari 0 lagi
	_, _ = db.ExecContext(ctx, "INSERT INTO log_kecurangan (siswa_id, ujian_id, jenis_kecurangan) VALUES (?, ?, 'buka_kunci')", siswaID, ujianID)

	jml, err = sesi.HitungPelanggaran(ctx, db, siswaID, ujianID)
	if err != nil {
		t.Fatalf("HitungPelanggaran: %v", err)
	}
	if jml != 0 {
		t.Errorf("jml setelah buka kunci = %d, want 0", jml)
	}

	// Pelanggaran baru setelah buka kunci
	_, _ = db.ExecContext(ctx, "INSERT INTO log_kecurangan (siswa_id, ujian_id, jenis_kecurangan) VALUES (?, ?, 'keluar_fullscreen')", siswaID, ujianID)
	jml, err = sesi.HitungPelanggaran(ctx, db, siswaID, ujianID)
	if err != nil {
		t.Fatalf("HitungPelanggaran: %v", err)
	}
	if jml != 1 {
		t.Errorf("jml = %d, want 1", jml)
	}
}

func TestMulaiAtauLanjutConcurrency(t *testing.T) {
	ctx := context.Background()
	db, siswaID, ujianID := setupDBUjian(t)

	// Dua goroutine memanggil MulaiAtauLanjut secara serentak
	var wg sync.WaitGroup
	wg.Add(2)

	hasilChan := make(chan *sesi.HasilMulai, 2)
	errChan := make(chan error, 2)

	go func() {
		defer wg.Done()
		h, err := sesi.MulaiAtauLanjut(ctx, db, siswaID, ujianID, "laptop")
		if err != nil {
			errChan <- err
			return
		}
		hasilChan <- h
	}()

	go func() {
		defer wg.Done()
		h, err := sesi.MulaiAtauLanjut(ctx, db, siswaID, ujianID, "hp")
		if err != nil {
			errChan <- err
			return
		}
		hasilChan <- h
	}()

	wg.Wait()
	close(hasilChan)
	close(errChan)

	for err := range errChan {
		t.Fatalf("concurrent MulaiAtauLanjut error: %v", err)
	}

	var results []*sesi.HasilMulai
	for h := range hasilChan {
		results = append(results, h)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Harus hanya ada tepat SATU sesi di database
	var countSesi int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?", siswaID, ujianID).Scan(&countSesi)
	if err != nil {
		t.Fatalf("query count sesi: %v", err)
	}
	if countSesi != 1 {
		t.Errorf("count sesi di DB = %d, want 1", countSesi)
	}
}

func TestAmbilSesiInfo(t *testing.T) {
	ctx := context.Background()
	db, siswaID, ujianID := setupDBUjian(t)

	// Belum ada sesi -> ErrSesiNotFound
	_, err := sesi.AmbilSesiInfo(ctx, db, siswaID, ujianID)
	if !errors.Is(err, sesi.ErrSesiNotFound) {
		t.Fatalf("seharusnya ErrSesiNotFound, dapat: %v", err)
	}

	// Buat sesi
	_, err = sesi.MulaiAtauLanjut(ctx, db, siswaID, ujianID, "laptop")
	if err != nil {
		t.Fatalf("MulaiAtauLanjut: %v", err)
	}

	// Simpan soal dan jawaban di jawaban_siswa
	resSoal, err := db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
		VALUES (?, 'pg', 'Contoh soal?', 2, 'A', 'B', 'C', 'D', 'B')
	`, ujianID)
	if err != nil {
		t.Fatalf("insert soal: %v", err)
	}
	soalID, _ := resSoal.LastInsertId()

	_, err = db.ExecContext(ctx, `
		INSERT INTO jawaban_siswa (siswa_id, ujian_id, soal_id, jawaban_dipilih, is_benar, client_ts)
		VALUES (?, ?, ?, 'B', 1, NOW())
	`, siswaID, ujianID, soalID)
	if err != nil {
		t.Fatalf("insert jawaban: %v", err)
	}

	// Ambil sesi info
	info, err := sesi.AmbilSesiInfo(ctx, db, siswaID, ujianID)
	if err != nil {
		t.Fatalf("AmbilSesiInfo: %v", err)
	}

	if info.Status != sesi.StatusSedangUjian {
		t.Errorf("status = %s, want %s", info.Status, sesi.StatusSedangUjian)
	}
	if info.SisaDetik <= 0 {
		t.Errorf("sisa detik harus > 0, dapat %d", info.SisaDetik)
	}
	if info.BatasWaktu.IsZero() {
		t.Errorf("batas waktu tidak boleh zero")
	}
	key := strconv.Itoa(int(soalID))
	if info.JawabanTersimpan[key] != "B" {
		t.Errorf("jawaban_tersimpan[%s] = %s, want B", key, info.JawabanTersimpan[key])
	}
}
