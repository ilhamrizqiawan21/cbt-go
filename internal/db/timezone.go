package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CheckTimezoneOffset adalah fungsi murni yang membandingkan offset zona waktu
// aplikasi (Go) dengan offset zona waktu database (MySQL).
// Keduanya diukur dalam menit selisih dari UTC.
func CheckTimezoneOffset(goOffsetMenit, dbOffsetMenit int) (bool, string) {
	if goOffsetMenit == dbOffsetMenit {
		return false, ""
	}

	pesan := fmt.Sprintf(
		"selisih zona waktu terdeteksi: APP_TIMEZONE ber-offset %+d menit dari UTC, sedangkan MySQL ber-offset %+d menit dari UTC. Samakan zona waktu agar timer ujian dan NOW() konsisten (lihat docs/ERD.md §3)",
		goOffsetMenit,
		dbOffsetMenit,
	)
	return true, pesan
}

// VerifyTimezone mengambil offset zona waktu database MySQL dan membandingkannya
// dengan lokasi waktu aplikasi. Mengembalikan beda=true dan pesan peringatan bila tidak sama.
func VerifyTimezone(ctx context.Context, pool *sql.DB, loc *time.Location) (bool, string, error) {
	var dbOffsetMenit int
	query := "SELECT TIMESTAMPDIFF(MINUTE, UTC_TIMESTAMP(), NOW())"
	if err := pool.QueryRowContext(ctx, query).Scan(&dbOffsetMenit); err != nil {
		return false, "", fmt.Errorf("gagal query offset zona waktu MySQL: %w", err)
	}

	_, goOffsetDetik := time.Now().In(loc).Zone()
	goOffsetMenit := goOffsetDetik / 60

	beda, pesan := CheckTimezoneOffset(goOffsetMenit, dbOffsetMenit)
	return beda, pesan, nil
}
