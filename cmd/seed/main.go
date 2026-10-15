// Command seed mengisi database development/test dengan data sintetis awal.
// Command ini menolak berjalan pada mode production.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"

	"cbt-go/internal/config"
	"cbt-go/internal/db"
)

var errProductionSeedBlocked = errors.New("seed: seeding dilarang pada mode production demi integritas data")

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fsFlags := flag.NewFlagSet("seed", flag.ContinueOnError)
	dbFlag := fsFlags.String("db", "", "nama database target (menggantikan DB_NAME di konfigurasi)")
	if err := fsFlags.Parse(args); err != nil {
		return err
	}

	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("membaca .env: %w", err)
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	if *dbFlag != "" {
		cfg.DB.Name = *dbFlag
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DB, cfg.Timezone)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := runSeed(ctx, pool, cfg.Env); err != nil {
		return err
	}

	fmt.Println("[SEED] Berhasil mengisi data sintetis ke database:", cfg.DB.Name)
	return nil
}

func runSeed(ctx context.Context, pool *sql.DB, env config.Env) error {
	if env == config.EnvProduction {
		return errProductionSeedBlocked
	}

	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("gagal memulai transaksi seed: %w", err)
	}
	defer tx.Rollback()

	// 1. Admin default: admin / admin123
	adminHash, err := bcrypt.GenerateFromPassword([]byte("admin123"), 10)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO admin (username, password)
		VALUES ('admin', ?)
		ON DUPLICATE KEY UPDATE password = VALUES(password)
	`, string(adminHash))
	if err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}

	// 2. Kelas
	classes := []string{"VII-A", "VII-B", "VIII-A", "IX-A"}
	for _, k := range classes {
		if _, err := tx.ExecContext(ctx, "INSERT IGNORE INTO kelas (nama_kelas) VALUES (?)", k); err != nil {
			return fmt.Errorf("seed kelas %s: %w", k, err)
		}
	}

	// 3. Mata Pelajaran
	mapels := []string{"Matematika", "Bahasa Indonesia", "Ilmu Pengetahuan Alam", "Bahasa Arab"}
	for _, m := range mapels {
		var id int
		err := tx.QueryRowContext(ctx, "SELECT id FROM mata_pelajaran WHERE nama_mapel = ?", m).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := tx.ExecContext(ctx, "INSERT INTO mata_pelajaran (nama_mapel) VALUES (?)", m); err != nil {
				return fmt.Errorf("seed mapel %s: %w", m, err)
			}
		} else if err != nil {
			return fmt.Errorf("cek mapel %s: %w", m, err)
		}
	}

	// 4. Guru default: guru / guru123
	guruHash, err := bcrypt.GenerateFromPassword([]byte("guru123"), 10)
	if err != nil {
		return fmt.Errorf("hash guru password: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO guru (nip, nama, username, password)
		VALUES ('198501012010011001', 'Budi Santoso, S.Pd.', 'guru', ?)
		ON DUPLICATE KEY UPDATE password = VALUES(password)
	`, string(guruHash))
	if err != nil {
		return fmt.Errorf("seed guru: %w", err)
	}

	var guruID, mapelMatematikaID, mapelArabID, kelasVIIAID int
	if err := tx.QueryRowContext(ctx, "SELECT id FROM guru WHERE username = 'guru'").Scan(&guruID); err != nil {
		return fmt.Errorf("ambil id guru: %w", err)
	}
	if err := tx.QueryRowContext(ctx, "SELECT id FROM kelas WHERE nama_kelas = 'VII-A'").Scan(&kelasVIIAID); err != nil {
		return fmt.Errorf("ambil id kelas VII-A: %w", err)
	}
	if err := tx.QueryRowContext(ctx, "SELECT id FROM mata_pelajaran WHERE nama_mapel = 'Matematika'").Scan(&mapelMatematikaID); err != nil {
		return fmt.Errorf("ambil id mapel Matematika: %w", err)
	}
	if err := tx.QueryRowContext(ctx, "SELECT id FROM mata_pelajaran WHERE nama_mapel = 'Bahasa Arab'").Scan(&mapelArabID); err != nil {
		return fmt.Errorf("ambil id mapel Bahasa Arab: %w", err)
	}

	// 5. Pengajaran
	ensurePengajaran := func(gID, mID, kID int) (int, error) {
		_, err := tx.ExecContext(ctx, `
			INSERT IGNORE INTO pengajaran (guru_id, mapel_id, kelas_id)
			VALUES (?, ?, ?)
		`, gID, mID, kID)
		if err != nil {
			return 0, err
		}
		var pID int
		err = tx.QueryRowContext(ctx, `
			SELECT id FROM pengajaran
			WHERE guru_id = ? AND mapel_id = ? AND kelas_id = ?
		`, gID, mID, kID).Scan(&pID)
		return pID, err
	}

	pengajaranMtkID, err := ensurePengajaran(guruID, mapelMatematikaID, kelasVIIAID)
	if err != nil {
		return fmt.Errorf("seed pengajaran Matematika: %w", err)
	}
	pengajaranArabID, err := ensurePengajaran(guruID, mapelArabID, kelasVIIAID)
	if err != nil {
		return fmt.Errorf("seed pengajaran Bahasa Arab: %w", err)
	}

	// 6. Siswa sintetis (10 siswa, PIN 1234)
	pinHash, err := bcrypt.GenerateFromPassword([]byte("1234"), 10)
	if err != nil {
		return fmt.Errorf("hash PIN siswa: %w", err)
	}

	siswaList := []struct {
		nis   string
		nama  string
		kelas string
	}{
		{"1001", "Ahmad Rizki", "VII-A"},
		{"1002", "Siti Rahma", "VII-A"},
		{"T0001", "Siswa Sintetis 1", "VII-A"},
		{"T0002", "Siswa Sintetis 2", "VII-A"},
		{"T0003", "Siswa Sintetis 3", "VII-A"},
		{"T0004", "Siswa Sintetis 4", "VII-A"},
		{"T0005", "Siswa Sintetis 5", "VII-A"},
		{"T0006", "Siswa Sintetis 6", "VII-A"},
		{"T0007", "Siswa Sintetis 7", "VII-A"},
		{"T0008", "Siswa Sintetis 8", "VII-A"},
	}

	for _, s := range siswaList {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO siswa (nis, nama, kelas, pin_ujian)
			VALUES (?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE nama = VALUES(nama), kelas = VALUES(kelas), pin_ujian = VALUES(pin_ujian)
		`, s.nis, s.nama, s.kelas, string(pinHash))
		if err != nil {
			return fmt.Errorf("seed siswa %s: %w", s.nis, err)
		}
	}

	// 7. Ujian 1: Simulasi CBT Matematika
	var ujianMtkID int
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM ujian WHERE pengajaran_id = ? AND nama_ujian = 'Simulasi CBT Matematika'
	`, pengajaranMtkID).Scan(&ujianMtkID)
	if errors.Is(err, sql.ErrNoRows) {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai, acak_soal, acak_pilihan, batas_pelanggaran)
			VALUES (?, 'Simulasi CBT Matematika', 60, NOW() - INTERVAL 1 HOUR, NOW() + INTERVAL 24 HOUR, 1, 1, 3)
		`, pengajaranMtkID)
		if err != nil {
			return fmt.Errorf("buat ujian matematika: %w", err)
		}
		id, _ := res.LastInsertId()
		ujianMtkID = int(id)

		// Soal Matematika
		soalMtk := []struct {
			tipe     string
			teks     string
			poin     int
			pa       string
			pb       string
			pc       string
			pd       string
			jawaban  string
			opsiJson string
		}{
			{
				tipe:    "pg",
				teks:    "Hasil dari 15 + 27 adalah...",
				poin:    2,
				pa:      "40",
				pb:      "42",
				pc:      "44",
				pd:      "46",
				jawaban: "B",
			},
			{
				tipe:    "pg",
				teks:    "Rumus luas persegi dengan panjang sisi s adalah...",
				poin:    2,
				pa:      "s x s",
				pb:      "2 x s",
				pc:      "4 x s",
				pd:      "s + s",
				jawaban: "A",
			},
			{
				tipe:     "menjodohkan",
				teks:     "Jodohkan bangun datar berikut dengan jumlah sisinya:",
				poin:     2,
				jawaban:  `[{"kiri":"Segitiga","kanan":"3"},{"kiri":"Segiempat","kanan":"4"},{"kiri":"Segilima","kanan":"5"}]`,
				opsiJson: `{"pasangan":[{"kiri":"Segitiga","kanan":"3"},{"kiri":"Segiempat","kanan":"4"},{"kiri":"Segilima","kanan":"5"}]}`,
			},
			{
				tipe:    "essay",
				teks:    "Jelaskan langkah-langkah menghitung volume kubus dengan rusuk 5 cm! (Dijawab di lembar kertas)",
				poin:    4,
				jawaban: "",
			},
		}

		for _, sm := range soalMtk {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar, opsi_tambahan)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, ujianMtkID, sm.tipe, sm.teks, sm.poin, sm.pa, sm.pb, sm.pc, sm.pd, sm.jawaban, sm.opsiJson)
			if err != nil {
				return fmt.Errorf("seed soal matematika: %w", err)
			}
		}
	} else if err != nil {
		return fmt.Errorf("cek ujian matematika: %w", err)
	}

	// 8. Ujian 2: Ujian Bahasa Arab (RTL)
	var ujianArabID int
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM ujian WHERE pengajaran_id = ? AND nama_ujian = 'Ujian Bahasa Arab'
	`, pengajaranArabID).Scan(&ujianArabID)
	if errors.Is(err, sql.ErrNoRows) {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai, acak_soal, acak_pilihan, batas_pelanggaran)
			VALUES (?, 'Ujian Bahasa Arab', 90, NOW() - INTERVAL 1 HOUR, NOW() + INTERVAL 24 HOUR, 1, 1, 3)
		`, pengajaranArabID)
		if err != nil {
			return fmt.Errorf("buat ujian bahasa arab: %w", err)
		}
		id, _ := res.LastInsertId()
		ujianArabID = int(id)

		// Soal Arab
		soalArab := []struct {
			tipe    string
			teks    string
			poin    int
			pa      string
			pb      string
			pc      string
			pd      string
			jawaban string
		}{
			{
				tipe:    "pg",
				teks:    "ما معنى كلمة \"كِتَابٌ\" باللغة الإندونيسية؟",
				poin:    2,
				pa:      "Buku",
				pb:      "Pena",
				pc:      "Meja",
				pd:      "Kursi",
				jawaban: "A",
			},
			{
				tipe:    "pg",
				teks:    "أَيْنَ تَدْرُسُ؟ ... فِي الْمَدْرَسَةِ",
				poin:    2,
				pa:      "أَدْرُسُ",
				pb:      "تَدْرُسُ",
				pc:      "يَدْرُسُ",
				pd:      "نَدْرُسُ",
				jawaban: "A",
			},
			{
				tipe:    "essay",
				teks:    "اُكْتُبْ ثَلَاثَ جُمَلٍ مُفِيدَةٍ عَنِ الْمَدْرَسَةِ! (Dijawab di lembar kertas)",
				poin:    4,
				jawaban: "",
			},
		}

		for _, sa := range soalArab {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, ujianArabID, sa.tipe, sa.teks, sa.poin, sa.pa, sa.pb, sa.pc, sa.pd, sa.jawaban)
			if err != nil {
				return fmt.Errorf("seed soal arab: %w", err)
			}
		}
	} else if err != nil {
		return fmt.Errorf("cek ujian arab: %w", err)
	}

	return tx.Commit()
}
