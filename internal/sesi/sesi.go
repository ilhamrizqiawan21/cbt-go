package sesi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Sentinel errors untuk paket sesi.
var (
	ErrSesiNotFound  = errors.New("sesi: sesi ujian tidak ditemukan")
	ErrUjianNotFound = errors.New("sesi: ujian tidak ditemukan")
	ErrKeluarPaksa   = errors.New("sesi: akses ujian dicabut karena pelanggaran")
	ErrSesiSelesai   = errors.New("sesi: Anda sudah menyelesaikan ujian ini")
)

const (
	// DefaultGraceDetik adalah toleransi waktu setelah batas_waktu untuk pengiriman batch (D-009).
	DefaultGraceDetik = 120

	StatusSedangUjian = "sedang_ujian"
	StatusSelesai     = "selesai"
	StatusKeluarPaksa = "keluar_paksa"
)

// StatusMulai menandakan apakah pemanggilan MulaiAtauLanjut membuat sesi baru atau melanjutkan sesi lama.
type StatusMulai string

const (
	MulaiBaru   StatusMulai = "baru"
	MulaiLanjut StatusMulai = "lanjut"
)

// SesiUjian memuat data baris sesi ujian siswa.
type SesiUjian struct {
	ID            int        `json:"id"`
	SiswaID       int        `json:"siswa_id"`
	UjianID       int        `json:"ujian_id"`
	WaktuMulai    time.Time  `json:"waktu_mulai"`
	BatasWaktu    time.Time  `json:"batas_waktu"`
	TambahanMenit int        `json:"tambahan_menit"`
	Seed          int        `json:"seed"`
	DeviceToken   string     `json:"device_token"`
	DeviceType    string     `json:"device_type"`
	LastSeen      time.Time  `json:"last_seen"`
	Status        string     `json:"status"`
	SelesaiPada   *time.Time `json:"selesai_pada,omitempty"`
}

// IzinJawaban merupakan hasil pemeriksaan boleh tidaknya jawaban diterima server.
type IzinJawaban struct {
	Boleh  bool   `json:"boleh"`
	Grace  bool   `json:"grace"`
	Alasan string `json:"alasan,omitempty"`
}

// HasilMulai adalah keluaran dari MulaiAtauLanjut.
type HasilMulai struct {
	Status   StatusMulai `json:"status"`
	TakeOver bool        `json:"take_over"`
	Sesi     SesiUjian   `json:"sesi"`
}

// DBTX adalah interface abstraksi untuk *sql.DB dan *sql.Tx.
type DBTX interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// HitungBatasWaktu menghitung batas akhir waktu pengerjaan: waktu_mulai + durasi + tambahan_menit.
// Batas waktu tidak boleh melewati tanggal_selesai ujian (D-007).
func HitungBatasWaktu(waktuMulai time.Time, durasiMenit int, tambahanMenit int, tanggalSelesaiUjian *time.Time) time.Time {
	batas := waktuMulai.Add(time.Duration(durasiMenit+tambahanMenit) * time.Minute)
	if tanggalSelesaiUjian != nil && batas.After(*tanggalSelesaiUjian) {
		batas = *tanggalSelesaiUjian
	}
	return batas
}

// HitungSisaDetik menghitung sisa waktu pengerjaan server-authoritative dalam detik.
func HitungSisaDetik(batasWaktu time.Time, now time.Time) int {
	if batasWaktu.IsZero() || now.After(batasWaktu) {
		return 0
	}
	sisa := int(batasWaktu.Sub(now).Seconds())
	if sisa < 0 {
		return 0
	}
	return sisa
}

// BolehTerimaJawaban memeriksa apakah jawaban siswa masih dapat diterima oleh server (D-009).
func BolehTerimaJawaban(status string, batasWaktu time.Time, clientTs, now time.Time, graceDetik int) IzinJawaban {
	if status == StatusKeluarPaksa {
		return IzinJawaban{Boleh: false, Alasan: "keluar_paksa"}
	}
	if status == StatusSelesai {
		return IzinJawaban{Boleh: false, Alasan: "selesai"}
	}
	if status != StatusSedangUjian {
		return IzinJawaban{Boleh: false, Alasan: "status_tidak_valid"}
	}
	if graceDetik <= 0 {
		graceDetik = DefaultGraceDetik
	}
	if clientTs.IsZero() {
		clientTs = now
	}

	// Normal: server masih dalam batas waktu
	if !now.After(batasWaktu) {
		return IzinJawaban{Boleh: true, Grace: false}
	}

	// Server lewat batas waktu: cek jendela toleransi grace period (D-009)
	selisihDetik := now.Sub(batasWaktu).Seconds()
	if selisihDetik <= float64(graceDetik) {
		if !clientTs.After(batasWaktu) {
			return IzinJawaban{Boleh: true, Grace: true}
		}
		return IzinJawaban{Boleh: false, Alasan: "client_ts_melewati_batas"}
	}

	return IzinJawaban{Boleh: false, Alasan: "melewati_grace_period"}
}

// HitungPelanggaran menghitung jumlah pelanggaran aktif (hanya pindah_tab, keluar_fullscreen, copy_paste)
// yang terjadi setelah event 'buka_kunci' terakhir dari admin.
func HitungPelanggaran(ctx context.Context, db DBTX, siswaID, ujianID int) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM log_kecurangan
		WHERE siswa_id = ? AND ujian_id = ?
		  AND jenis_kecurangan IN ('pindah_tab', 'keluar_fullscreen', 'copy_paste')
		  AND id > COALESCE((
		      SELECT MAX(id) FROM log_kecurangan
		      WHERE siswa_id = ? AND ujian_id = ? AND jenis_kecurangan = 'buka_kunci'
		  ), 0)`
	var count int
	err := db.QueryRowContext(ctx, query, siswaID, ujianID, siswaID, ujianID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("hitung pelanggaran: %w", err)
	}
	return count, nil
}

// GenerateToken membuat UUID v4 baru berbasis random byte crypto/rand.
func GenerateToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// GenerateSeed membuat bilangan bulat 31-bit positif acak untuk seed pengacakan soal.
func GenerateSeed() (int, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, fmt.Errorf("generate seed: %w", err)
	}
	val := int(binary.BigEndian.Uint32(b[:]) & 0x7fffffff)
	if val == 0 {
		val = 1
	}
	return val, nil
}

// MulaiAtauLanjut memulai sesi baru bila belum ada, atau mengambil alih sesi aktif bila sudah berjalan (D-008).
// Transaksi aman dari race condition dua login bersamaan.
func MulaiAtauLanjut(ctx context.Context, db *sql.DB, siswaID, ujianID int, deviceType string) (*HasilMulai, error) {
	// Jika terjadi konflik konkuren saat pembuatan baris awal (MySQL error 1062 duplicate key),
	// ulangi agar transaksi membaca baris yang baru saja terbentuk.
	for percobaan := 0; percobaan < 3; percobaan++ {
		hasil, err := jalankanMulaiAtauLanjut(ctx, db, siswaID, ujianID, deviceType)
		if err != nil {
			var mysqlErr *mysql.MySQLError
			if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
				continue
			}
			return nil, err
		}
		return hasil, nil
	}
	return nil, fmt.Errorf("sesi: gagal memulai sesi setelah beberapa percobaan")
}

func jalankanMulaiAtauLanjut(ctx context.Context, db *sql.DB, siswaID, ujianID int, deviceType string) (*HasilMulai, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 1. Periksa sesi yang sudah ada dengan kunci FOR UPDATE
	var s SesiUjian
	err = tx.QueryRowContext(ctx, `
		SELECT id, siswa_id, ujian_id, waktu_mulai, batas_waktu, tambahan_menit, seed, device_token, device_type, last_seen, status, selesai_pada
		FROM sesi_ujian
		WHERE siswa_id = ? AND ujian_id = ?
		FOR UPDATE
	`, siswaID, ujianID).Scan(
		&s.ID, &s.SiswaID, &s.UjianID, &s.WaktuMulai, &s.BatasWaktu, &s.TambahanMenit,
		&s.Seed, &s.DeviceToken, &s.DeviceType, &s.LastSeen, &s.Status, &s.SelesaiPada,
	)

	if err == nil {
		// Sesi sudah ada di database
		if s.Status == StatusKeluarPaksa {
			return nil, ErrKeluarPaksa
		}
		if s.Status == StatusSelesai {
			return nil, ErrSesiSelesai
		}
		if s.Status == StatusSedangUjian {
			// Sesi aktif: lakukan take-over
			newToken, err := GenerateToken()
			if err != nil {
				return nil, err
			}
			targetDevice := s.DeviceType
			if deviceType != "" {
				targetDevice = deviceType
			}

			_, err = tx.ExecContext(ctx, `
				UPDATE sesi_ujian
				SET device_token = ?, device_type = ?, last_seen = NOW()
				WHERE id = ?
			`, newToken, targetDevice, s.ID)
			if err != nil {
				return nil, fmt.Errorf("update takeover: %w", err)
			}

			// Catat log audit takeover dan log kecurangan
			_, _ = tx.ExecContext(ctx, `
				INSERT INTO audit_admin (admin_id, sesi_id, aksi, detail)
				VALUES (NULL, ?, 'takeover', ?)
			`, s.ID, fmt.Sprintf("Takeover perangkat ke %s", targetDevice))

			_, _ = tx.ExecContext(ctx, `
				INSERT INTO log_kecurangan (siswa_id, ujian_id, jenis_kecurangan, device_type)
				VALUES (?, ?, 'ambil_alih_sesi', ?)
			`, siswaID, ujianID, targetDevice)

			// Ambil waktu last_seen terbaru dari DB
			err = tx.QueryRowContext(ctx, `
				SELECT last_seen, device_token, device_type FROM sesi_ujian WHERE id = ?
			`, s.ID).Scan(&s.LastSeen, &s.DeviceToken, &s.DeviceType)
			if err != nil {
				return nil, fmt.Errorf("read updated sesi: %w", err)
			}

			if err := tx.Commit(); err != nil {
				return nil, fmt.Errorf("commit takeover: %w", err)
			}

			return &HasilMulai{
				Status:   MulaiLanjut,
				TakeOver: true,
				Sesi:     s,
			}, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("query sesi: %w", err)
	}

	// 2. Sesi belum ada: ambil data ujian
	var ujianDurasi int
	var ujianTglMulai, ujianTglSelesai time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT durasi, tanggal_mulai, tanggal_selesai
		FROM ujian
		WHERE id = ?
	`, ujianID).Scan(&ujianDurasi, &ujianTglMulai, &ujianTglSelesai)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUjianNotFound
		}
		return nil, fmt.Errorf("query ujian: %w", err)
	}

	var dbNow time.Time
	err = tx.QueryRowContext(ctx, "SELECT NOW()").Scan(&dbNow)
	if err != nil {
		return nil, fmt.Errorf("query db now: %w", err)
	}

	batasWaktu := HitungBatasWaktu(dbNow, ujianDurasi, 0, &ujianTglSelesai)
	token, err := GenerateToken()
	if err != nil {
		return nil, err
	}
	seed, err := GenerateSeed()
	if err != nil {
		return nil, err
	}

	if deviceType == "" {
		deviceType = "laptop"
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO sesi_ujian
		(siswa_id, ujian_id, waktu_mulai, batas_waktu, tambahan_menit, seed, device_token, device_type, last_seen, status)
		VALUES (?, ?, ?, ?, 0, ?, ?, ?, ?, ?)
	`, siswaID, ujianID, dbNow, batasWaktu, seed, token, deviceType, dbNow, StatusSedangUjian)
	if err != nil {
		return nil, err
	}

	sesiID, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit new sesi: %w", err)
	}

	s = SesiUjian{
		ID:            int(sesiID),
		SiswaID:       siswaID,
		UjianID:       ujianID,
		WaktuMulai:    dbNow,
		BatasWaktu:    batasWaktu,
		TambahanMenit: 0,
		Seed:          seed,
		DeviceToken:   token,
		DeviceType:    deviceType,
		LastSeen:      dbNow,
		Status:        StatusSedangUjian,
	}

	return &HasilMulai{
		Status:   MulaiBaru,
		TakeOver: false,
		Sesi:     s,
	}, nil
}

// Perpanjang menambah durasi ujian pada sesi aktif dan memajukan batas_waktu.
func Perpanjang(ctx context.Context, db *sql.DB, sesiID int, menit int) (*SesiUjian, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE sesi_ujian
		SET tambahan_menit = tambahan_menit + ?,
		    batas_waktu = DATE_ADD(batas_waktu, INTERVAL ? MINUTE)
		WHERE id = ?
	`, menit, menit, sesiID)
	if err != nil {
		return nil, fmt.Errorf("perpanjang sesi: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return nil, ErrSesiNotFound
	}

	var s SesiUjian
	err = db.QueryRowContext(ctx, `
		SELECT id, siswa_id, ujian_id, waktu_mulai, batas_waktu, tambahan_menit, seed, device_token, device_type, last_seen, status, selesai_pada
		FROM sesi_ujian
		WHERE id = ?
	`, sesiID).Scan(
		&s.ID, &s.SiswaID, &s.UjianID, &s.WaktuMulai, &s.BatasWaktu, &s.TambahanMenit,
		&s.Seed, &s.DeviceToken, &s.DeviceType, &s.LastSeen, &s.Status, &s.SelesaiPada,
	)
	if err != nil {
		return nil, fmt.Errorf("query sesi updated: %w", err)
	}

	return &s, nil
}

// SesiInfo memuat informasi sesi ujian aktif yang dikirimkan ke klien (GET /api/sesi).
type SesiInfo struct {
	Status           string            `json:"status"`
	SisaDetik        int               `json:"sisa_detik"`
	BatasWaktu       time.Time         `json:"batas_waktu"`
	JawabanTersimpan map[string]string `json:"jawaban_tersimpan"`
}

// AmbilSesiInfo mengambil status sesi, sisa detik waktu ujian (server-authoritative),
// batas waktu, dan daftar jawaban yang sudah tersimpan untuk siswa pada suatu ujian.
func AmbilSesiInfo(ctx context.Context, db DBTX, siswaID, ujianID int) (*SesiInfo, error) {
	var status string
	var batasWaktu, dbNow time.Time

	err := db.QueryRowContext(ctx, `
		SELECT status, batas_waktu, NOW()
		FROM sesi_ujian
		WHERE siswa_id = ? AND ujian_id = ?
	`, siswaID, ujianID).Scan(&status, &batasWaktu, &dbNow)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSesiNotFound
		}
		return nil, fmt.Errorf("ambil sesi info: %w", err)
	}

	sisaDetik := HitungSisaDetik(batasWaktu, dbNow)

	rows, err := db.QueryContext(ctx, `
		SELECT soal_id, jawaban_dipilih
		FROM jawaban_siswa
		WHERE siswa_id = ? AND ujian_id = ?
	`, siswaID, ujianID)
	if err != nil {
		return nil, fmt.Errorf("ambil jawaban tersimpan: %w", err)
	}
	defer rows.Close()

	jawabanTersimpan := make(map[string]string)
	for rows.Next() {
		var soalID int
		var jawaban string
		if err := rows.Scan(&soalID, &jawaban); err != nil {
			return nil, fmt.Errorf("scan jawaban tersimpan: %w", err)
		}
		jawabanTersimpan[strconv.Itoa(soalID)] = jawaban
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterasi jawaban tersimpan: %w", err)
	}

	return &SesiInfo{
		Status:           status,
		SisaDetik:        sisaDetik,
		BatasWaktu:       batasWaktu,
		JawabanTersimpan: jawabanTersimpan,
	}, nil
}
