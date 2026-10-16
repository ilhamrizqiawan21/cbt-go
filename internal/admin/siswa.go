package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"cbt-go/internal/excel"
)

var (
	ErrDuplicateNIS = errors.New("NIS sudah terdaftar")
)

type Siswa struct {
	ID       int    `json:"id"`
	NIS      string `json:"nis"`
	Nama     string `json:"nama"`
	Kelas    string `json:"kelas"`
	PINUjian string `json:"pin_ujian,omitempty"`
}

func ListSiswa(ctx context.Context, db *sql.DB, kelasFilter string) ([]Siswa, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}

	query := "SELECT id, nis, nama, kelas, pin_ujian FROM siswa"
	var args []any
	if strings.TrimSpace(kelasFilter) != "" {
		query += " WHERE kelas = ?"
		args = append(args, strings.TrimSpace(kelasFilter))
	}
	query += " ORDER BY kelas ASC, nama ASC"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list siswa: %w", err)
	}
	defer rows.Close()

	var res []Siswa
	for rows.Next() {
		var s Siswa
		if err := rows.Scan(&s.ID, &s.NIS, &s.Nama, &s.Kelas, &s.PINUjian); err != nil {
			return nil, fmt.Errorf("scan siswa: %w", err)
		}
		res = append(res, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if res == nil {
		res = []Siswa{}
	}
	return res, nil
}

func GetSiswaByID(ctx context.Context, db *sql.DB, id int) (*Siswa, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}
	var s Siswa
	err := db.QueryRowContext(ctx, `
		SELECT id, nis, nama, kelas, pin_ujian
		FROM siswa
		WHERE id = ?
	`, id).Scan(&s.ID, &s.NIS, &s.Nama, &s.Kelas, &s.PINUjian)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get siswa by id: %w", err)
	}
	return &s, nil
}

func TambahSiswa(ctx context.Context, db *sql.DB, nis, nama, kelas, pin string) (int, error) {
	nis = strings.TrimSpace(nis)
	nama = strings.TrimSpace(nama)
	kelas = strings.TrimSpace(kelas)
	pin = strings.TrimSpace(pin)
	if nis == "" || nama == "" || kelas == "" || pin == "" {
		return 0, ErrInvalidInput
	}
	if db == nil {
		return 0, errors.New("database tidak tersedia")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(pin), 10)
	if err != nil {
		return 0, fmt.Errorf("hash PIN siswa: %w", err)
	}

	res, err := db.ExecContext(ctx, `
		INSERT INTO siswa (nis, nama, kelas, pin_ujian)
		VALUES (?, ?, ?, ?)
	`, nis, nama, kelas, string(hash))
	if err != nil {
		if isDuplicateError(err) {
			return 0, ErrDuplicateNIS
		}
		return 0, fmt.Errorf("insert siswa: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func EditSiswa(ctx context.Context, db *sql.DB, id int, nis, nama, kelas, pinBaru string) error {
	nis = strings.TrimSpace(nis)
	nama = strings.TrimSpace(nama)
	kelas = strings.TrimSpace(kelas)
	if id <= 0 || nis == "" || nama == "" || kelas == "" {
		return ErrInvalidInput
	}
	if db == nil {
		return errors.New("database tidak tersedia")
	}

	var err error
	if strings.TrimSpace(pinBaru) != "" {
		hash, hErr := bcrypt.GenerateFromPassword([]byte(strings.TrimSpace(pinBaru)), 10)
		if hErr != nil {
			return fmt.Errorf("hash PIN siswa baru: %w", hErr)
		}
		_, err = db.ExecContext(ctx, `
			UPDATE siswa
			SET nis = ?, nama = ?, kelas = ?, pin_ujian = ?
			WHERE id = ?
		`, nis, nama, kelas, string(hash), id)
	} else {
		_, err = db.ExecContext(ctx, `
			UPDATE siswa
			SET nis = ?, nama = ?, kelas = ?
			WHERE id = ?
		`, nis, nama, kelas, id)
	}

	if err != nil {
		if isDuplicateError(err) {
			return ErrDuplicateNIS
		}
		return fmt.Errorf("update siswa: %w", err)
	}
	return nil
}

func HapusSiswa(ctx context.Context, db *sql.DB, id int) error {
	if db == nil {
		return errors.New("database tidak tersedia")
	}
	_, err := db.ExecContext(ctx, "DELETE FROM siswa WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete siswa: %w", err)
	}
	return nil
}

// ImportSiswaBatch memproses daftar siswa dari impor Excel ke dalam database secara transactional.
// Baris yang mengalami duplikasi NIS atau kelas tidak valid dilaporkan dalam laporan gagal.
func ImportSiswaBatch(ctx context.Context, db *sql.DB, items []excel.SiswaImportItem) (int, []excel.ImportError, error) {
	if db == nil {
		return 0, nil, errors.New("database tidak tersedia")
	}

	var berhasil int
	var laporanGagal []excel.ImportError

	for _, it := range items {
		var existsID int
		err := db.QueryRowContext(ctx, "SELECT id FROM siswa WHERE nis = ?", it.NIS).Scan(&existsID)
		if err == nil {
			laporanGagal = append(laporanGagal, excel.ImportError{
				Baris: it.Baris,
				Pesan: fmt.Sprintf("NIS %s sudah terdaftar di database", it.NIS),
			})
			continue
		} else if !errors.Is(err, sql.ErrNoRows) {
			return berhasil, laporanGagal, fmt.Errorf("cek NIS baris %d: %w", it.Baris, err)
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(it.PIN), 10)
		if err != nil {
			laporanGagal = append(laporanGagal, excel.ImportError{
				Baris: it.Baris,
				Pesan: "Gagal memproses enkripsi PIN",
			})
			continue
		}

		_, err = db.ExecContext(ctx, `
			INSERT INTO siswa (nis, nama, kelas, pin_ujian)
			VALUES (?, ?, ?, ?)
		`, it.NIS, it.Nama, it.Kelas, string(hash))
		if err != nil {
			if isDuplicateError(err) {
				laporanGagal = append(laporanGagal, excel.ImportError{
					Baris: it.Baris,
					Pesan: fmt.Sprintf("NIS %s sudah terdaftar di database", it.NIS),
				})
				continue
			}
			return berhasil, laporanGagal, fmt.Errorf("insert siswa baris %d: %w", it.Baris, err)
		}

		berhasil++
	}

	return berhasil, laporanGagal, nil
}
