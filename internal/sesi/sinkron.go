package sesi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cbt-go/internal/penilaian"
)

var (
	ErrBatchTerlaluBesar = errors.New("sesi: ukuran batch melebihi batas maksimal 100 item")
	ErrFormatDataInvalid = errors.New("sesi: format data tidak valid, harus berupa array jawaban")
	ErrSoalInvalid       = errors.New("sesi: item jawaban tidak lengkap atau soal_id tidak valid")
	ErrSoalUjianLain     = errors.New("sesi: soal tidak ditemukan atau bukan milik ujian ini")
	ErrWaktuHabis        = errors.New("sesi: waktu ujian telah berakhir dan melewati batas toleransi")
)

// ItemJawabanInput memuat satu butir jawaban siswa yang dikirim dalam batch sinkronisasi.
type ItemJawabanInput struct {
	SoalID   int       `json:"soal_id"`
	Jawaban  any       `json:"jawaban"`
	ClientTS time.Time `json:"client_ts"`
}

// HasilSinkron memuat ringkasan hasil proses sinkronisasi batch.
type HasilSinkron struct {
	Success    bool `json:"success"`
	Tersinkron int  `json:"tersinkron"`
	Diabaikan  int  `json:"diabaikan"`
	Total      int  `json:"total"`
}

type soalInfo struct {
	ID           int
	TipeSoal     penilaian.TipeSoal
	JawabanBenar sql.NullString
}

type existingJawaban struct {
	SoalID         int
	JawabanDipilih sql.NullString
	ClientTS       sql.NullTime
}

// SinkronJawaban memproses batch jawaban siswa secara idempoten dan atomik.
func SinkronJawaban(ctx context.Context, db *sql.DB, siswaID, ujianID int, items []ItemJawabanInput) (*HasilSinkron, error) {
	if len(items) > 100 {
		return nil, ErrBatchTerlaluBesar
	}
	if len(items) == 0 {
		return &HasilSinkron{Success: true, Tersinkron: 0, Diabaikan: 0, Total: 0}, nil
	}

	// 1. Cek sesi ujian siswa
	var sesiID int
	var waktuMulai, batasWaktu time.Time
	var status string
	err := db.QueryRowContext(ctx, `
		SELECT id, waktu_mulai, batas_waktu, status
		FROM sesi_ujian
		WHERE siswa_id = ? AND ujian_id = ?
	`, siswaID, ujianID).Scan(&sesiID, &waktuMulai, &batasWaktu, &status)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSesiNotFound
		}
		return nil, fmt.Errorf("query sesi: %w", err)
	}

	if status == StatusKeluarPaksa {
		return nil, ErrKeluarPaksa
	}
	if status != StatusSedangUjian {
		return nil, ErrSesiSelesai
	}

	// 2. Waktu server DB dan validasi batas waktu + grace
	var nowDB time.Time
	err = db.QueryRowContext(ctx, "SELECT NOW()").Scan(&nowDB)
	if err != nil {
		return nil, fmt.Errorf("query now db: %w", err)
	}

	graceDeadline := batasWaktu.Add(time.Duration(DefaultGraceDetik) * time.Second)
	if nowDB.After(graceDeadline) {
		return nil, ErrWaktuHabis
	}

	// 3. Validasi bahwa seluruh soal_id milik ujian_id ini
	uniqueIDsMap := make(map[int]struct{})
	for _, it := range items {
		if it.SoalID <= 0 {
			return nil, ErrSoalInvalid
		}
		uniqueIDsMap[it.SoalID] = struct{}{}
	}

	uniqueIDs := make([]int, 0, len(uniqueIDsMap))
	for sid := range uniqueIDsMap {
		uniqueIDs = append(uniqueIDs, sid)
	}

	soalMap, err := ambilSoalMap(ctx, db, ujianID, uniqueIDs)
	if err != nil {
		return nil, err
	}

	for _, sid := range uniqueIDs {
		if _, ok := soalMap[sid]; !ok {
			return nil, fmt.Errorf("%w (ID %d)", ErrSoalUjianLain, sid)
		}
	}

	// 4. Ambil jawaban yang sudah tersimpan untuk verifikasi client_ts
	existingMap, err := ambilExistingJawabanMap(ctx, db, siswaID, ujianID, uniqueIDs)
	if err != nil {
		return nil, err
	}

	// 5. Eksekusi upsert dalam satu transaksi
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var tersinkron, diabaikan int

	for _, it := range items {
		sInfo := soalMap[it.SoalID]

		// Bounding client_ts: tidak boleh di masa depan
		clientTS := it.ClientTS
		if clientTS.IsZero() || clientTS.After(nowDB) {
			clientTS = nowDB
		}
		clientTS = clientTS.Truncate(time.Second)

		// Periksa toleransi batas waktu (D-009)
		izin := BolehTerimaJawaban(status, batasWaktu, clientTS, nowDB, DefaultGraceDetik)
		if !izin.Boleh {
			diabaikan++
			continue
		}

		// Timestamp yang lebih lama tidak boleh menimpa yang lebih baru
		if exist, ok := existingMap[it.SoalID]; ok && exist.ClientTS.Valid {
			if clientTS.Unix() < exist.ClientTS.Time.Unix() {
				diabaikan++
				continue
			}
		}

		jawabanStr := normalisasiJawabanString(it.Jawaban)
		var kunciStr string
		if sInfo.JawabanBenar.Valid {
			kunciStr = sInfo.JawabanBenar.String
		}

		isBenarPtr := hitungIsBenar(sInfo.TipeSoal, jawabanStr, kunciStr)

		_, err = tx.ExecContext(ctx, `
			INSERT INTO jawaban_siswa (siswa_id, ujian_id, soal_id, jawaban_dipilih, is_benar, client_ts)
			VALUES (?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				jawaban_dipilih = VALUES(jawaban_dipilih),
				is_benar        = VALUES(is_benar),
				client_ts       = VALUES(client_ts)
		`, siswaID, ujianID, it.SoalID, jawabanStr, isBenarPtr, clientTS)
		if err != nil {
			return nil, fmt.Errorf("upsert jawaban: %w", err)
		}

		// Update map existing
		existingMap[it.SoalID] = existingJawaban{
			SoalID:         it.SoalID,
			JawabanDipilih: sql.NullString{String: jawabanStr, Valid: true},
			ClientTS:       sql.NullTime{Time: clientTS, Valid: true},
		}

		tersinkron++
	}

	// Update last_seen pada sesi_ujian
	_, err = tx.ExecContext(ctx, "UPDATE sesi_ujian SET last_seen = NOW() WHERE id = ?", sesiID)
	if err != nil {
		return nil, fmt.Errorf("update last_seen: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &HasilSinkron{
		Success:    true,
		Tersinkron: tersinkron,
		Diabaikan:  diabaikan,
		Total:      len(items),
	}, nil
}

func normalisasiJawabanString(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case json.RawMessage:
		return string(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

func hitungIsBenar(tipeSoal penilaian.TipeSoal, jawabanStr, kunci string) *int {
	if tipeSoal == penilaian.TipeEssay {
		return nil
	}
	var isBenar int
	if tipeSoal == penilaian.TipePG {
		if penilaian.CekJawabanPG(jawabanStr, kunci) {
			isBenar = 1
		}
	} else if tipeSoal == penilaian.TipeMenjodohkan {
		if penilaian.CekJawabanMenjodohkan(jawabanStr, kunci) {
			isBenar = 1
		}
	}
	return &isBenar
}

func ambilSoalMap(ctx context.Context, db *sql.DB, ujianID int, uniqueIDs []int) (map[int]soalInfo, error) {
	if len(uniqueIDs) == 0 {
		return make(map[int]soalInfo), nil
	}

	placeholders := make([]string, len(uniqueIDs))
	args := make([]any, 0, len(uniqueIDs)+1)
	args = append(args, ujianID)
	for i, sid := range uniqueIDs {
		placeholders[i] = "?"
		args = append(args, sid)
	}

	query := fmt.Sprintf(`
		SELECT id, tipe_soal, jawaban_benar
		FROM soal
		WHERE ujian_id = ? AND id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query soal batch: %w", err)
	}
	defer rows.Close()

	soalMap := make(map[int]soalInfo)
	for rows.Next() {
		var s soalInfo
		var tipeStr string
		if err := rows.Scan(&s.ID, &tipeStr, &s.JawabanBenar); err != nil {
			return nil, fmt.Errorf("scan soal: %w", err)
		}
		s.TipeSoal = penilaian.TipeSoal(tipeStr)
		soalMap[s.ID] = s
	}

	return soalMap, nil
}

func ambilExistingJawabanMap(ctx context.Context, db *sql.DB, siswaID, ujianID int, uniqueIDs []int) (map[int]existingJawaban, error) {
	if len(uniqueIDs) == 0 {
		return make(map[int]existingJawaban), nil
	}

	placeholders := make([]string, len(uniqueIDs))
	args := make([]any, 0, len(uniqueIDs)+2)
	args = append(args, siswaID, ujianID)
	for i, sid := range uniqueIDs {
		placeholders[i] = "?"
		args = append(args, sid)
	}

	query := fmt.Sprintf(`
		SELECT soal_id, jawaban_dipilih, client_ts
		FROM jawaban_siswa
		WHERE siswa_id = ? AND ujian_id = ? AND soal_id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query existing jawaban: %w", err)
	}
	defer rows.Close()

	existingMap := make(map[int]existingJawaban)
	for rows.Next() {
		var e existingJawaban
		if err := rows.Scan(&e.SoalID, &e.JawabanDipilih, &e.ClientTS); err != nil {
			return nil, fmt.Errorf("scan existing: %w", err)
		}
		existingMap[e.SoalID] = e
	}

	return existingMap, nil
}
