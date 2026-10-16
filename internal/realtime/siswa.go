package realtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/coder/websocket"

	"cbt-go/internal/sesi"
)

// OnMonitorNotifyFunc adalah callback opsional yang dipanggil ketika state sesi siswa berubah
// (mis. siswa mulai/reconnect, pelanggaran baru, selesai ujian, atau putus koneksi)
// agar layanan monitor (T2.4) dapat menjadwalkan broadcast ke room admin.
type OnMonitorNotifyFunc func(ujianID int, immediateEvent string, immediateData any)

// RegisterSiswaEvents mendaftarkan penangan event WebSocket siswa (siswa-siap, pindah-tab,
// copy-paste, keluar-fullscreen, selesai-ujian) dan disconnect ke Hub.
func RegisterSiswaEvents(hub *Hub, db *sql.DB, onNotify OnMonitorNotifyFunc) {
	if hub == nil || db == nil {
		return
	}

	hub.SetSiswaHandlers(
		func(ctx context.Context, client *Client, env Envelope) {
			handleSiswaEvent(ctx, db, client, env, onNotify)
		},
		func(ctx context.Context, client *Client) {
			handleSiswaDisconnect(ctx, db, client, onNotify)
		},
	)
}

func handleSiswaEvent(ctx context.Context, db *sql.DB, client *Client, env Envelope, onNotify OnMonitorNotifyFunc) {
	siswaID := client.SiswaID()
	ujianID := client.UjianID()
	deviceType := client.DeviceType()
	if deviceType == "" {
		deviceType = "laptop"
	}

	if siswaID <= 0 || ujianID <= 0 {
		client.Send("error", map[string]any{"message": "Data siswa/ujian tidak valid"})
		return
	}

	switch env.Event {
	case "siswa-siap":
		handleSiswaSiap(ctx, db, client, siswaID, ujianID, deviceType, onNotify)
	case "pindah-tab":
		handlePelanggaran(ctx, db, client, siswaID, ujianID, deviceType, "pindah_tab", "pindah tab", onNotify)
	case "copy-paste":
		handlePelanggaran(ctx, db, client, siswaID, ujianID, deviceType, "copy_paste", "copy-paste", onNotify)
	case "keluar-fullscreen":
		// Kebijakan Perangkat (DESIGN §3.7 & app.js:295):
		// Pada HP (mobile browser), keluar fullscreen sering terpicu oleh keyboard virtual / notifikasi sistem,
		// sehingga sinyal keluar-fullscreen dari perangkat 'hp' diabaikan sepenuhnya oleh server.
		if deviceType == "hp" {
			return
		}
		handlePelanggaran(ctx, db, client, siswaID, ujianID, deviceType, "keluar_fullscreen", "keluar dari layar penuh", onNotify)
	case "selesai-ujian":
		handleSelesaiUjianWS(ctx, db, client, siswaID, ujianID, onNotify)
	}
}

func handleSiswaSiap(
	ctx context.Context,
	db *sql.DB,
	client *Client,
	siswaID, ujianID int,
	deviceType string,
	onNotify OnMonitorNotifyFunc,
) {
	// 1. Cek apakah ujian masih dalam rentang waktu aktif menggunakan jam DB (NOW())
	var durasi int
	err := db.QueryRowContext(ctx, `
		SELECT durasi
		FROM ujian
		WHERE id = ? AND NOW() BETWEEN tanggal_mulai AND tanggal_selesai
	`, ujianID).Scan(&durasi)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			client.Send("error", map[string]any{"message": "Ujian sudah berakhir atau belum dimulai"})
			return
		}
		client.Send("error", map[string]any{"message": "Gagal memulai ujian"})
		return
	}

	// 2. Cek apakah sesi sudah ada di tabel sesi_ujian
	var sesiID int
	var status string
	var batasWaktu, dbNow time.Time
	var dbDeviceType sql.NullString

	err = db.QueryRowContext(ctx, `
		SELECT id, status, batas_waktu, device_type, NOW()
		FROM sesi_ujian
		WHERE siswa_id = ? AND ujian_id = ?
	`, siswaID, ujianID).Scan(&sesiID, &status, &batasWaktu, &dbDeviceType, &dbNow)

	if errors.Is(err, sql.ErrNoRows) {
		// Jika sesi belum ada, buat melalui MulaiAtauLanjut
		hasil, errMulai := sesi.MulaiAtauLanjut(ctx, db, siswaID, ujianID, deviceType)
		if errMulai != nil {
			if errors.Is(errMulai, sesi.ErrKeluarPaksa) {
				client.Send("error", map[string]any{"message": "Akses ujian Anda telah dicabut karena pelanggaran."})
				go client.Close(websocket.StatusNormalClosure, "keluar_paksa")
				return
			}
			client.Send("error", map[string]any{"message": "Gagal memulai ujian"})
			return
		}
		sesiID = hasil.Sesi.ID
		status = hasil.Sesi.Status
		batasWaktu = hasil.Sesi.BatasWaktu
		dbDeviceType = sql.NullString{String: hasil.Sesi.DeviceType, Valid: hasil.Sesi.DeviceType != ""}
		_ = db.QueryRowContext(ctx, "SELECT NOW()").Scan(&dbNow)
	} else if err != nil {
		client.Send("error", map[string]any{"message": "Gagal memulai ujian"})
		return
	}

	// 3. Tolak jika sesi sudah dicabut (keluar_paksa)
	if status == sesi.StatusKeluarPaksa {
		client.Send("error", map[string]any{"message": "Akses ujian Anda telah dicabut karena pelanggaran."})
		go client.Close(websocket.StatusNormalClosure, "keluar_paksa")
		return
	}

	// 4. Perbarui socket_id, device_type, dan last_seen tanpa mengubah waktu_mulai / batas_waktu
	_, _ = db.ExecContext(ctx, `
		UPDATE sesi_ujian
		SET socket_id = ?, device_type = COALESCE(NULLIF(?, ''), device_type), last_seen = NOW()
		WHERE id = ?
	`, client.ID(), deviceType, sesiID)

	finalDevice := deviceType
	if finalDevice == "" && dbDeviceType.Valid && dbDeviceType.String != "" {
		finalDevice = dbDeviceType.String
	}

	sisaDetik := sesi.HitungSisaDetik(batasWaktu, dbNow)

	client.Send("mulai-ujian", map[string]any{
		"durasi":      durasi,
		"sisa_detik":  sisaDetik,
		"batas_waktu": batasWaktu,
		"device_type": finalDevice,
	})

	if onNotify != nil {
		onNotify(ujianID, "", nil)
	}
}

func handlePelanggaran(
	ctx context.Context,
	db *sql.DB,
	client *Client,
	siswaID, ujianID int,
	deviceType, jenisDB, labelJenis string,
	onNotify OnMonitorNotifyFunc,
) {
	// 1. Catat ke log_kecurangan beserta device_type
	_, err := db.ExecContext(ctx, `
		INSERT INTO log_kecurangan (siswa_id, ujian_id, jenis_kecurangan, device_type)
		VALUES (?, ?, ?, ?)
	`, siswaID, ujianID, jenisDB, deviceType)
	if err != nil {
		return
	}

	// 2. Hitung total pelanggaran aktif (setelah buka_kunci terakhir, tidak termasuk ambil_alih_sesi)
	jumlah, err := sesi.HitungPelanggaran(ctx, db, siswaID, ujianID)
	if err != nil {
		return
	}

	// 3. Ambil batas_pelanggaran dari tabel ujian (default 3)
	batas := 3
	_ = db.QueryRowContext(ctx, `
		SELECT COALESCE(batas_pelanggaran, 3) FROM ujian WHERE id = ?
	`, ujianID).Scan(&batas)
	if batas <= 0 {
		batas = 3
	}

	if jumlah >= batas {
		_, _ = db.ExecContext(ctx, `
			UPDATE sesi_ujian
			SET status = 'keluar_paksa'
			WHERE siswa_id = ? AND ujian_id = ?
		`, siswaID, ujianID)

		client.Send("paksa-submit", map[string]any{
			"message": fmt.Sprintf("⚠️ Batas pelanggaran (%d/%d) terlampaui. Ujian Anda diakhiri.", jumlah, batas),
		})
	} else {
		sisa := batas - jumlah
		client.Send("peringatan", map[string]any{
			"pesan": fmt.Sprintf("⚠️ Peringatan! Anda telah melakukan %s. Total pelanggaran: %d/%d. Sisa %d kesempatan.", labelJenis, jumlah, batas, sisa),
		})
	}

	if onNotify != nil {
		onNotify(ujianID, "pelanggaran-baru", map[string]any{
			"siswa_id":          siswaID,
			"ujian_id":          ujianID,
			"jenis_kecurangan":  jenisDB,
			"total_pelanggaran": jumlah,
			"batas":             batas,
		})
	}
}

func handleSelesaiUjianWS(
	ctx context.Context,
	db *sql.DB,
	client *Client,
	siswaID, ujianID int,
	onNotify OnMonitorNotifyFunc,
) {
	_, _ = db.ExecContext(ctx, `
		UPDATE sesi_ujian
		SET status = 'selesai', selesai_pada = COALESCE(selesai_pada, NOW()), last_seen = NOW()
		WHERE siswa_id = ? AND ujian_id = ?
	`, siswaID, ujianID)

	if onNotify != nil {
		onNotify(ujianID, "", nil)
	}

	go client.Close(websocket.StatusNormalClosure, "selesai-ujian")
}

func handleSiswaDisconnect(
	ctx context.Context,
	db *sql.DB,
	client *Client,
	onNotify OnMonitorNotifyFunc,
) {
	siswaID := client.SiswaID()
	ujianID := client.UjianID()
	if siswaID <= 0 || ujianID <= 0 {
		return
	}

	// Saat koneksi putus, JANGAN ubah status sesi menjadi selesai/keluar_paksa.
	// Cukup perbarui last_seen = NOW() agar saat siswa tersambung kembali, ujian dapat dilanjutkan.
	_, _ = db.ExecContext(ctx, `
		UPDATE sesi_ujian
		SET last_seen = NOW()
		WHERE siswa_id = ? AND ujian_id = ?
	`, siswaID, ujianID)

	if onNotify != nil {
		onNotify(ujianID, "", nil)
	}
}

