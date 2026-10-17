package sesi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"cbt-go/internal/penilaian"
)

// UpdateHeartbeat memperbarui waktu last_seen pada sesi ujian siswa (POST /api/heartbeat).
func UpdateHeartbeat(ctx context.Context, db DBTX, siswaID, ujianID int) error {
	res, err := db.ExecContext(ctx, `
		UPDATE sesi_ujian
		SET last_seen = NOW()
		WHERE siswa_id = ? AND ujian_id = ?
	`, siswaID, ujianID)
	if err != nil {
		return fmt.Errorf("update heartbeat: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update heartbeat rows affected: %w", err)
	}
	if affected == 0 {
		// Pada MySQL (tanpa clientFoundRows), RowsAffected mengembalikan 0 apabila nilai last_seen
		// sudah berada pada detik yang sama dengan NOW(). Verifikasi apakah baris sesi memang ada.
		var id int
		err := db.QueryRowContext(ctx, `
			SELECT id FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?
		`, siswaID, ujianID).Scan(&id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrSesiNotFound
			}
			return fmt.Errorf("cek eksistensi sesi heartbeat: %w", err)
		}
	}

	return nil
}

// HitungUlangNilai mengambil data soal, jawaban siswa, dan nilai essay,
// menghitung skor akhir menggunakan penilaian.Hitung, lalu menyimpannya ke tabel nilai_ujian secara idempoten.
// Fungsi ini aman dipanggil berulang kali dan tidak mengubah status sesi_ujian (dipakai juga di T3.7 untuk penilaian essay guru).
func HitungUlangNilai(ctx context.Context, db DBTX, siswaID, ujianID int) (*penilaian.Hasil, error) {
	// 1. Ambil seluruh butir soal milik ujian
	rowsSoal, err := db.QueryContext(ctx, `
		SELECT id, tipe_soal, poin, COALESCE(jawaban_benar, '')
		FROM soal
		WHERE ujian_id = ?
	`, ujianID)
	if err != nil {
		return nil, fmt.Errorf("query soal: %w", err)
	}
	defer rowsSoal.Close()

	var soalList []penilaian.Soal
	for rowsSoal.Next() {
		var s penilaian.Soal
		var tipe string
		if err := rowsSoal.Scan(&s.ID, &tipe, &s.Poin, &s.JawabanBenar); err != nil {
			return nil, fmt.Errorf("scan soal: %w", err)
		}
		s.TipeSoal = penilaian.TipeSoal(tipe)
		soalList = append(soalList, s)
	}
	if err := rowsSoal.Err(); err != nil {
		return nil, fmt.Errorf("iterasi soal: %w", err)
	}

	// 2. Ambil seluruh jawaban tersimpan siswa
	rowsJwb, err := db.QueryContext(ctx, `
		SELECT soal_id, COALESCE(jawaban_dipilih, '')
		FROM jawaban_siswa
		WHERE siswa_id = ? AND ujian_id = ?
	`, siswaID, ujianID)
	if err != nil {
		return nil, fmt.Errorf("query jawaban siswa: %w", err)
	}
	defer rowsJwb.Close()

	jawabanMap := make(map[int]string)
	for rowsJwb.Next() {
		var soalID int
		var jawaban string
		if err := rowsJwb.Scan(&soalID, &jawaban); err != nil {
			return nil, fmt.Errorf("scan jawaban siswa: %w", err)
		}
		jawabanMap[soalID] = jawaban
	}
	if err := rowsJwb.Err(); err != nil {
		return nil, fmt.Errorf("iterasi jawaban siswa: %w", err)
	}

	// 3. Ambil nilai essay yang sudah diberikan guru (jika ada)
	rowsEssay, err := db.QueryContext(ctx, `
		SELECT soal_id, skor
		FROM nilai_essay
		WHERE siswa_id = ? AND ujian_id = ?
	`, siswaID, ujianID)
	if err != nil {
		return nil, fmt.Errorf("query nilai essay: %w", err)
	}
	defer rowsEssay.Close()

	skorEssay := make(map[int]int)
	for rowsEssay.Next() {
		var soalID int
		var skor int
		if err := rowsEssay.Scan(&soalID, &skor); err != nil {
			return nil, fmt.Errorf("scan nilai essay: %w", err)
		}
		skorEssay[soalID] = skor
	}
	if err := rowsEssay.Err(); err != nil {
		return nil, fmt.Errorf("iterasi nilai essay: %w", err)
	}

	// 4. Hitung skor murni
	hasil := penilaian.Hitung(soalList, jawabanMap, skorEssay)

	// 5. Simpan ke nilai_ujian secara idempoten via ON DUPLICATE KEY UPDATE
	_, err = db.ExecContext(ctx, `
		INSERT INTO nilai_ujian (
			siswa_id, ujian_id, poin_otomatis, poin_essay, poin_maks,
			nilai, benar, salah, kosong, status_koreksi, selesai_pada
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())
		ON DUPLICATE KEY UPDATE
			poin_otomatis  = VALUES(poin_otomatis),
			poin_essay     = VALUES(poin_essay),
			poin_maks      = VALUES(poin_maks),
			nilai          = VALUES(nilai),
			benar          = VALUES(benar),
			salah          = VALUES(salah),
			kosong         = VALUES(kosong),
			status_koreksi = VALUES(status_koreksi),
			selesai_pada   = COALESCE(selesai_pada, NOW())
	`,
		siswaID, ujianID,
		hasil.PoinOtomatis, hasil.PoinEssay, hasil.PoinMaks,
		hasil.Nilai, hasil.Benar, hasil.Salah, hasil.Kosong,
		string(hasil.StatusKoreksi),
	)
	if err != nil {
		return nil, fmt.Errorf("upsert nilai_ujian: %w", err)
	}

	return &hasil, nil
}

// Finalize memfinalisasi sesi ujian siswa: menghitung nilai, menyimpannya ke tabel nilai_ujian,
// dan menandai status sesi_ujian menjadi 'selesai' dengan waktu selesai_pada.
// Dijalankan dalam satu transaksi database atomik dan bersifat idempoten.
// Dipakai admin (paksa selesai) dan job penutupan otomatis, termasuk untuk sesi keluar_paksa.
func Finalize(ctx context.Context, db *sql.DB, siswaID, ujianID int) (*penilaian.Hasil, error) {
	return finalize(ctx, db, siswaID, ujianID, false)
}

// FinalizeOlehSiswa dipanggil saat siswa sendiri mengumpulkan ujian. Sesi keluar_paksa ditolak
// (ErrKeluarPaksa) agar admin masih bisa membuka kunci tanpa kehilangan data (PRD FR-12);
// status diperiksa di dalam transaksi yang sama (FOR UPDATE) supaya tidak balapan dengan penguncian.
func FinalizeOlehSiswa(ctx context.Context, db *sql.DB, siswaID, ujianID int) (*penilaian.Hasil, error) {
	return finalize(ctx, db, siswaID, ujianID, true)
}

func finalize(ctx context.Context, db *sql.DB, siswaID, ujianID int, tolakTerkunci bool) (*penilaian.Hasil, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("finalize begin tx: %w", err)
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRowContext(ctx, `
		SELECT status FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ? FOR UPDATE
	`, siswaID, ujianID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSesiNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finalize kunci baris sesi: %w", err)
	}
	if tolakTerkunci && status == StatusKeluarPaksa {
		return nil, ErrKeluarPaksa
	}

	// Hitung dan simpan nilai dalam transaksi
	hasil, err := HitungUlangNilai(ctx, tx, siswaID, ujianID)
	if err != nil {
		return nil, err
	}

	// Ubah status sesi menjadi selesai
	if _, err := tx.ExecContext(ctx, `
		UPDATE sesi_ujian
		SET status = 'selesai',
		    selesai_pada = COALESCE(selesai_pada, NOW())
		WHERE siswa_id = ? AND ujian_id = ?
	`, siswaID, ujianID); err != nil {
		return nil, fmt.Errorf("finalize update sesi_ujian: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("finalize commit: %w", err)
	}

	return hasil, nil
}

// FinalizeByID memfinalisasi sesi ujian berdasarkan id baris sesi_ujian.
func FinalizeByID(ctx context.Context, db *sql.DB, sesiID int) (*penilaian.Hasil, error) {
	var siswaID, ujianID int
	err := db.QueryRowContext(ctx, `
		SELECT siswa_id, ujian_id
		FROM sesi_ujian
		WHERE id = ?
	`, sesiID).Scan(&siswaID, &ujianID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSesiNotFound
		}
		return nil, fmt.Errorf("finalize by id query: %w", err)
	}

	return Finalize(ctx, db, siswaID, ujianID)
}
