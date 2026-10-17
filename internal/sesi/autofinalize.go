package sesi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"cbt-go/internal/penilaian"
)

// TutupSesiKadaluarsa memeriksa dan memfinalisasi seluruh sesi ujian berstatus 'sedang_ujian' atau 'keluar_paksa'
// yang waktu pengerjaannya telah melewati batas_waktu ditambah masa toleransi grace period (D-009).
// Sesi keluar_paksa yang tidak dibuka kunci sampai waktunya habis ikut dinilai dengan jawaban
// yang sudah tersinkron, agar siswa tetap memiliki nilai.
func TutupSesiKadaluarsa(ctx context.Context, db *sql.DB, graceDetik int) ([]*penilaian.Hasil, error) {
	if graceDetik <= 0 {
		graceDetik = DefaultGraceDetik
	}

	rows, err := db.QueryContext(ctx, `
		SELECT siswa_id, ujian_id
		FROM sesi_ujian
		WHERE status IN ('sedang_ujian', 'keluar_paksa')
		  AND batas_waktu IS NOT NULL
		  AND NOW() > DATE_ADD(batas_waktu, INTERVAL ? SECOND)
	`, graceDetik)
	if err != nil {
		return nil, fmt.Errorf("query sesi kadaluarsa: %w", err)
	}
	defer rows.Close()

	type targetSesi struct {
		siswaID int
		ujianID int
	}
	var targets []targetSesi
	for rows.Next() {
		var t targetSesi
		if err := rows.Scan(&t.siswaID, &t.ujianID); err != nil {
			return nil, fmt.Errorf("scan target sesi: %w", err)
		}
		targets = append(targets, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterasi sesi kadaluarsa: %w", err)
	}

	var hasilList []*penilaian.Hasil
	for _, t := range targets {
		if ctx.Err() != nil {
			break
		}
		res, err := Finalize(ctx, db, t.siswaID, t.ujianID)
		if err != nil {
			// Lewati bila terjadi kegagalan pada satu sesi agar sesi lainnya tetap diproses
			continue
		}
		hasilList = append(hasilList, res)
	}

	return hasilList, nil
}

// AutoFinalizer mengelola siklus hidup background worker penutupan otomatis sesi kadaluarsa.
type AutoFinalizer struct {
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	stopOnce sync.Once
}

// Stop menghentikan background worker secara tertib dan menunggu iterasi yang sedang berjalan selesai.
func (af *AutoFinalizer) Stop() {
	af.stopOnce.Do(func() {
		af.cancel()
		af.wg.Wait()
	})
}

// StartAutoFinalizeJob menjalankan background goroutine dengan ticker periodik untuk
// memfinalisasi sesi yang telah kadaluarsa. Worker dapat dihentikan kapan saja via context
// atau dengan memanggil method Stop() pada instance AutoFinalizer yang dikembalikan.
func StartAutoFinalizeJob(parentCtx context.Context, db *sql.DB, interval time.Duration, graceDetik int, logger *slog.Logger) *AutoFinalizer {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if graceDetik <= 0 {
		graceDetik = DefaultGraceDetik
	}

	ctx, cancel := context.WithCancel(parentCtx)
	af := &AutoFinalizer{
		cancel: cancel,
	}

	af.wg.Add(1)
	go func() {
		defer af.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				hasilList, err := TutupSesiKadaluarsa(ctx, db, graceDetik)
				if err != nil && !errors.Is(err, context.Canceled) {
					if logger != nil {
						logger.Error("auto-finalize job error", "err", err)
					}
				} else if len(hasilList) > 0 && logger != nil {
					logger.Info("auto-finalize menutup sesi kadaluarsa", "jumlah", len(hasilList))
				}
			}
		}
	}()

	return af
}
