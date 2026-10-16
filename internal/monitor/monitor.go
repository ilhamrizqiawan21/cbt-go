// Package monitor menyediakan layanan pemantauan ujian langsung (realtime) untuk admin.
package monitor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// StatusMonitor adalah status turunan siswa yang ditampilkan pada dashboard admin.
type StatusMonitor string

const (
	StatusOnline     StatusMonitor = "online"
	StatusOffline    StatusMonitor = "offline"
	StatusSelesai    StatusMonitor = "selesai"
	StatusTerkunci   StatusMonitor = "terkunci"
	StatusBelumMasuk StatusMonitor = "belum_masuk"
)

// SiswaMonitor memuat data ringkas status dan progres satu siswa dalam ujian.
type SiswaMonitor struct {
	SiswaID           int           `json:"siswa_id"`
	NIS               string        `json:"nis"`
	Nama              string        `json:"nama"`
	Kelas             string        `json:"kelas"`
	SesiID            *int          `json:"sesi_id"`
	Status            StatusMonitor `json:"status"`
	StatusSesi        string        `json:"status_sesi"`
	DeviceType        string        `json:"device_type"`
	LastSeen          *time.Time    `json:"last_seen"`
	SisaDetik         int           `json:"sisa_detik"`
	JumlahPelanggaran int           `json:"jumlah_pelanggaran"`
	SoalDijawab       int           `json:"soal_dijawab"`
	TotalSoal         int           `json:"total_soal"`
	Persentase        int           `json:"persentase"`
}

// RingkasanMonitor merekap jumlah siswa berdasarkan status pemantauan.
type RingkasanMonitor struct {
	TotalSiswa int `json:"total_siswa"`
	Online     int `json:"online"`
	Offline    int `json:"offline"`
	Selesai    int `json:"selesai"`
	Terkunci   int `json:"terkunci"`
	BelumMasuk int `json:"belum_masuk"`
}

// SnapshotData adalah struktur lengkap yang dikembalikan oleh endpoint monitor dan siaran WebSocket admin.
type SnapshotData struct {
	UjianID   int              `json:"ujian_id"`
	NamaUjian string           `json:"nama_ujian"`
	TotalSoal int              `json:"total_soal"`
	Ringkasan RingkasanMonitor `json:"ringkasan"`
	Siswa     []SiswaMonitor   `json:"siswa"`
}

// Broadcaster adalah interface abstraksi untuk Hub WebSocket.
// Mengikuti prinsip Go: didefinisikan di sisi pemakai dengan metode minimal yang dibutuhkan.
type Broadcaster interface {
	HasAdminListeners(ujianID int) bool
	BroadcastToAdminRoom(ujianID int, event string, data any)
}

// Service mengelola pengambilan snapshot data monitor dan penyiaran update ke room admin.
type Service struct {
	db          *sql.DB
	broadcaster Broadcaster
	nowFunc     func() time.Time
}

// NewService membuat instance Service pemantauan baru.
func NewService(db *sql.DB, broadcaster Broadcaster) *Service {
	return &Service{
		db:          db,
		broadcaster: broadcaster,
		nowFunc:     time.Now,
	}
}

// SetNowFunc menyuntikkan fungsi waktu untuk pengujian deterministik.
func (s *Service) SetNowFunc(fn func() time.Time) {
	s.nowFunc = fn
}

// HitungStatusTurunan menghitung status tampilan siswa dalam dashboard pemantauan admin.
// Batas deteksi offline adalah 45 detik sejak aktivitas terakhir (lastSeen).
// Aturan:
// - Belum memiliki sesi (statusSesi == "") -> "belum_masuk"
// - Sesi "selesai" -> "selesai"
// - Sesi "keluar_paksa" -> "terkunci"
// - Sesi "sedang_ujian":
//   - jika lastSeen nil atau now - lastSeen >= 45s -> "offline"
//   - jika now - lastSeen < 45s -> "online"
func HitungStatusTurunan(statusSesi string, lastSeen *time.Time, now time.Time) StatusMonitor {
	switch statusSesi {
	case "":
		return StatusBelumMasuk
	case "selesai":
		return StatusSelesai
	case "keluar_paksa":
		return StatusTerkunci
	case "sedang_ujian":
		if lastSeen == nil || lastSeen.IsZero() {
			return StatusOffline
		}
		diff := now.Sub(*lastSeen)
		if diff < 0 {
			// Clock skew kecil pada sistem
			return StatusOnline
		}
		if diff < 45*time.Second {
			return StatusOnline
		}
		return StatusOffline
	default:
		return StatusOffline
	}
}

// SiarkanUpdate memeriksa keberadaan admin di room ujian, lalu mengambil snapshot dan menyiarkannya.
// OPTIMASI WAJIB (DESIGN §3.6): jika tidak ada admin yang terhubung di room ujian tersebut,
// fungsi ini langsung kembali tanpa mengeksekusi kueri database sama sekali.
func (s *Service) SiarkanUpdate(ctx context.Context, ujianID int) error {
	if s.broadcaster == nil || !s.broadcaster.HasAdminListeners(ujianID) {
		return nil
	}

	snap, err := s.Snapshot(ctx, ujianID)
	if err != nil {
		return fmt.Errorf("siarkan update monitor: %w", err)
	}

	s.broadcaster.BroadcastToAdminRoom(ujianID, "monitor:update", snap)
	return nil
}

// Snapshot mengambil data pemantauan ujian untuk seluruh siswa di kelas terkait.
// Didesain efisien tanpa masalah N+1 kueri (menggunakan JOIN dan agregasi dalam 2 kueri tetap).
func (s *Service) Snapshot(ctx context.Context, ujianID int) (*SnapshotData, error) {
	if s.db == nil {
		return nil, errors.New("koneksi database tidak tersedia")
	}

	// 1. Ambil info ujian dan total soal (Kueri 1)
	var namaUjian string
	var totalSoal int
	err := s.db.QueryRowContext(ctx, `
		SELECT u.nama_ujian, COALESCE((SELECT COUNT(*) FROM soal WHERE ujian_id = u.id), 0) AS total_soal
		FROM ujian u
		WHERE u.id = ?
	`, ujianID).Scan(&namaUjian, &totalSoal)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("ujian dengan id %d tidak ditemukan: %w", ujianID, err)
		}
		return nil, fmt.Errorf("ambil info ujian: %w", err)
	}

	// 2. Ambil seluruh siswa beserta sesi dan agregasi jawaban tersimpan (Kueri 2 - JOIN tanpa N+1)
	rows, err := s.db.QueryContext(ctx, `
		SELECT 
			s.id AS siswa_id,
			s.nis,
			s.nama,
			s.kelas,
			su.id AS sesi_id,
			COALESCE(su.status, '') AS status_sesi,
			COALESCE(su.device_type, '') AS device_type,
			su.last_seen,
			su.batas_waktu,
			COALESCE(su.jumlah_pelanggaran, 0) AS jumlah_pelanggaran,
			COALESCE(COUNT(js.id), 0) AS soal_dijawab
		FROM siswa s
		JOIN kelas k ON k.nama_kelas = s.kelas
		JOIN pengajaran p ON p.kelas_id = k.id
		JOIN ujian u ON u.pengajaran_id = p.id
		LEFT JOIN sesi_ujian su ON su.siswa_id = s.id AND su.ujian_id = u.id
		LEFT JOIN jawaban_siswa js ON js.sesi_id = su.id
		WHERE u.id = ?
		GROUP BY s.id, s.nis, s.nama, s.kelas, su.id, su.status, su.device_type, su.last_seen, su.batas_waktu, su.jumlah_pelanggaran
		ORDER BY s.nama ASC
	`, ujianID)
	if err != nil {
		return nil, fmt.Errorf("ambil daftar siswa monitor: %w", err)
	}
	defer rows.Close()

	now := s.nowFunc()
	var listSiswa []SiswaMonitor
	var ringkasan RingkasanMonitor

	for rows.Next() {
		var sm SiswaMonitor
		var sesiID sql.NullInt64
		var lastSeen, batasWaktu sql.NullTime

		err := rows.Scan(
			&sm.SiswaID,
			&sm.NIS,
			&sm.Nama,
			&sm.Kelas,
			&sesiID,
			&sm.StatusSesi,
			&sm.DeviceType,
			&lastSeen,
			&batasWaktu,
			&sm.JumlahPelanggaran,
			&sm.SoalDijawab,
		)
		if err != nil {
			return nil, fmt.Errorf("scan baris siswa monitor: %w", err)
		}

		if sesiID.Valid {
			sID := int(sesiID.Int64)
			sm.SesiID = &sID
		}

		var lastSeenPtr *time.Time
		if lastSeen.Valid {
			tVal := lastSeen.Time
			sm.LastSeen = &tVal
			lastSeenPtr = &tVal
		}

		sm.Status = HitungStatusTurunan(sm.StatusSesi, lastSeenPtr, now)
		sm.TotalSoal = totalSoal

		if totalSoal > 0 {
			sm.Persentase = int(float64(sm.SoalDijawab) / float64(totalSoal) * 100)
		}

		if batasWaktu.Valid && sm.StatusSesi == "sedang_ujian" {
			sisa := int(batasWaktu.Time.Sub(now).Seconds())
			if sisa < 0 {
				sisa = 0
			}
			sm.SisaDetik = sisa
		}

		// Update ringkasan
		ringkasan.TotalSiswa++
		switch sm.Status {
		case StatusOnline:
			ringkasan.Online++
		case StatusOffline:
			ringkasan.Offline++
		case StatusSelesai:
			ringkasan.Selesai++
		case StatusTerkunci:
			ringkasan.Terkunci++
		case StatusBelumMasuk:
			ringkasan.BelumMasuk++
		}

		listSiswa = append(listSiswa, sm)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterasi rows monitor: %w", err)
	}

	if listSiswa == nil {
		listSiswa = []SiswaMonitor{}
	}

	return &SnapshotData{
		UjianID:   ujianID,
		NamaUjian: namaUjian,
		TotalSoal: totalSoal,
		Ringkasan: ringkasan,
		Siswa:     listSiswa,
	}, nil
}
