// Package admin mengelola fitur dan data operasional untuk peran administrator.
package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrDuplicateUsername   = errors.New("username sudah digunakan")
	ErrDuplicateKelas      = errors.New("nama kelas sudah ada")
	ErrDuplicatePengajaran = errors.New("pengajaran untuk kombinasi guru, mapel, dan kelas ini sudah ada")
	ErrNotFound            = errors.New("data tidak ditemukan")
	ErrInvalidInput        = errors.New("data input tidak lengkap atau tidak valid")
)

type Guru struct {
	ID       int    `json:"id"`
	NIP      string `json:"nip"`
	Nama     string `json:"nama"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
}

type Kelas struct {
	ID        int    `json:"id"`
	NamaKelas string `json:"nama_kelas"`
}

type MataPelajaran struct {
	ID        int    `json:"id"`
	NamaMapel string `json:"nama_mapel"`
}

type Pengajaran struct {
	ID        int    `json:"id"`
	GuruID    int    `json:"guru_id"`
	MapelID   int    `json:"mapel_id"`
	KelasID   int    `json:"kelas_id"`
	NamaGuru  string `json:"nama_guru,omitempty"`
	NamaMapel string `json:"nama_mapel,omitempty"`
	NamaKelas string `json:"nama_kelas,omitempty"`
}

func isDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return true
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "1062") || strings.Contains(errStr, "duplicate")
}

// ======================== GURU ========================

func ListGuru(ctx context.Context, db *sql.DB) ([]Guru, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, nip, nama, username, password
		FROM guru
		ORDER BY nama ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list guru: %w", err)
	}
	defer rows.Close()

	var res []Guru
	for rows.Next() {
		var g Guru
		if err := rows.Scan(&g.ID, &g.NIP, &g.Nama, &g.Username, &g.Password); err != nil {
			return nil, fmt.Errorf("scan guru: %w", err)
		}
		res = append(res, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if res == nil {
		res = []Guru{}
	}
	return res, nil
}

func GetGuruByID(ctx context.Context, db *sql.DB, id int) (*Guru, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}
	var g Guru
	err := db.QueryRowContext(ctx, `
		SELECT id, nip, nama, username, password
		FROM guru
		WHERE id = ?
	`, id).Scan(&g.ID, &g.NIP, &g.Nama, &g.Username, &g.Password)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get guru by id: %w", err)
	}
	return &g, nil
}

func TambahGuru(ctx context.Context, db *sql.DB, nip, nama, username, password string) (int, error) {
	username = strings.TrimSpace(username)
	nama = strings.TrimSpace(nama)
	if username == "" || nama == "" || password == "" {
		return 0, ErrInvalidInput
	}
	if db == nil {
		return 0, errors.New("database tidak tersedia")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return 0, fmt.Errorf("hash password guru: %w", err)
	}

	res, err := db.ExecContext(ctx, `
		INSERT INTO guru (nip, nama, username, password)
		VALUES (?, ?, ?, ?)
	`, nip, nama, username, string(hash))
	if err != nil {
		if isDuplicateError(err) {
			return 0, ErrDuplicateUsername
		}
		return 0, fmt.Errorf("insert guru: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func EditGuru(ctx context.Context, db *sql.DB, id int, nip, nama, username, passwordBaru string) error {
	username = strings.TrimSpace(username)
	nama = strings.TrimSpace(nama)
	if id <= 0 || username == "" || nama == "" {
		return ErrInvalidInput
	}
	if db == nil {
		return errors.New("database tidak tersedia")
	}

	var err error
	if strings.TrimSpace(passwordBaru) != "" {
		hash, hErr := bcrypt.GenerateFromPassword([]byte(passwordBaru), 10)
		if hErr != nil {
			return fmt.Errorf("hash password guru: %w", hErr)
		}
		_, err = db.ExecContext(ctx, `
			UPDATE guru
			SET nip = ?, nama = ?, username = ?, password = ?
			WHERE id = ?
		`, nip, nama, username, string(hash), id)
	} else {
		_, err = db.ExecContext(ctx, `
			UPDATE guru
			SET nip = ?, nama = ?, username = ?
			WHERE id = ?
		`, nip, nama, username, id)
	}

	if err != nil {
		if isDuplicateError(err) {
			return ErrDuplicateUsername
		}
		return fmt.Errorf("update guru: %w", err)
	}
	return nil
}

func HapusGuru(ctx context.Context, db *sql.DB, id int) error {
	if db == nil {
		return errors.New("database tidak tersedia")
	}
	_, err := db.ExecContext(ctx, "DELETE FROM guru WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete guru: %w", err)
	}
	return nil
}

// ======================== KELAS ========================

func ListKelas(ctx context.Context, db *sql.DB) ([]Kelas, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}
	rows, err := db.QueryContext(ctx, "SELECT id, nama_kelas FROM kelas ORDER BY nama_kelas ASC")
	if err != nil {
		return nil, fmt.Errorf("list kelas: %w", err)
	}
	defer rows.Close()

	var res []Kelas
	for rows.Next() {
		var k Kelas
		if err := rows.Scan(&k.ID, &k.NamaKelas); err != nil {
			return nil, fmt.Errorf("scan kelas: %w", err)
		}
		res = append(res, k)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if res == nil {
		res = []Kelas{}
	}
	return res, nil
}

func GetKelasByID(ctx context.Context, db *sql.DB, id int) (*Kelas, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}
	var k Kelas
	err := db.QueryRowContext(ctx, "SELECT id, nama_kelas FROM kelas WHERE id = ?", id).Scan(&k.ID, &k.NamaKelas)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get kelas by id: %w", err)
	}
	return &k, nil
}

func TambahKelas(ctx context.Context, db *sql.DB, namaKelas string) (int, error) {
	namaKelas = strings.TrimSpace(namaKelas)
	if namaKelas == "" {
		return 0, ErrInvalidInput
	}
	if db == nil {
		return 0, errors.New("database tidak tersedia")
	}

	res, err := db.ExecContext(ctx, "INSERT INTO kelas (nama_kelas) VALUES (?)", namaKelas)
	if err != nil {
		if isDuplicateError(err) {
			return 0, ErrDuplicateKelas
		}
		return 0, fmt.Errorf("insert kelas: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func EditKelas(ctx context.Context, db *sql.DB, id int, namaKelas string) error {
	namaKelas = strings.TrimSpace(namaKelas)
	if id <= 0 || namaKelas == "" {
		return ErrInvalidInput
	}
	if db == nil {
		return errors.New("database tidak tersedia")
	}

	_, err := db.ExecContext(ctx, "UPDATE kelas SET nama_kelas = ? WHERE id = ?", namaKelas, id)
	if err != nil {
		if isDuplicateError(err) {
			return ErrDuplicateKelas
		}
		return fmt.Errorf("update kelas: %w", err)
	}
	return nil
}

func HapusKelas(ctx context.Context, db *sql.DB, id int) error {
	if db == nil {
		return errors.New("database tidak tersedia")
	}
	_, err := db.ExecContext(ctx, "DELETE FROM kelas WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete kelas: %w", err)
	}
	return nil
}

// ======================== MATA PELAJARAN ========================

func ListMapel(ctx context.Context, db *sql.DB) ([]MataPelajaran, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}
	rows, err := db.QueryContext(ctx, "SELECT id, nama_mapel FROM mata_pelajaran ORDER BY nama_mapel ASC")
	if err != nil {
		return nil, fmt.Errorf("list mapel: %w", err)
	}
	defer rows.Close()

	var res []MataPelajaran
	for rows.Next() {
		var m MataPelajaran
		if err := rows.Scan(&m.ID, &m.NamaMapel); err != nil {
			return nil, fmt.Errorf("scan mapel: %w", err)
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if res == nil {
		res = []MataPelajaran{}
	}
	return res, nil
}

func GetMapelByID(ctx context.Context, db *sql.DB, id int) (*MataPelajaran, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}
	var m MataPelajaran
	err := db.QueryRowContext(ctx, "SELECT id, nama_mapel FROM mata_pelajaran WHERE id = ?", id).Scan(&m.ID, &m.NamaMapel)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get mapel by id: %w", err)
	}
	return &m, nil
}

func TambahMapel(ctx context.Context, db *sql.DB, namaMapel string) (int, error) {
	namaMapel = strings.TrimSpace(namaMapel)
	if namaMapel == "" {
		return 0, ErrInvalidInput
	}
	if db == nil {
		return 0, errors.New("database tidak tersedia")
	}

	res, err := db.ExecContext(ctx, "INSERT INTO mata_pelajaran (nama_mapel) VALUES (?)", namaMapel)
	if err != nil {
		return 0, fmt.Errorf("insert mapel: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func EditMapel(ctx context.Context, db *sql.DB, id int, namaMapel string) error {
	namaMapel = strings.TrimSpace(namaMapel)
	if id <= 0 || namaMapel == "" {
		return ErrInvalidInput
	}
	if db == nil {
		return errors.New("database tidak tersedia")
	}

	_, err := db.ExecContext(ctx, "UPDATE mata_pelajaran SET nama_mapel = ? WHERE id = ?", namaMapel, id)
	if err != nil {
		return fmt.Errorf("update mapel: %w", err)
	}
	return nil
}

func HapusMapel(ctx context.Context, db *sql.DB, id int) error {
	if db == nil {
		return errors.New("database tidak tersedia")
	}
	_, err := db.ExecContext(ctx, "DELETE FROM mata_pelajaran WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete mapel: %w", err)
	}
	return nil
}

// ======================== PENGAJARAN ========================

func ListPengajaran(ctx context.Context, db *sql.DB) ([]Pengajaran, error) {
	if db == nil {
		return nil, errors.New("database tidak tersedia")
	}
	rows, err := db.QueryContext(ctx, `
		SELECT p.id, p.guru_id, p.mapel_id, p.kelas_id, g.nama, m.nama_mapel, k.nama_kelas
		FROM pengajaran p
		JOIN guru g ON g.id = p.guru_id
		JOIN mata_pelajaran m ON m.id = p.mapel_id
		JOIN kelas k ON k.id = p.kelas_id
		ORDER BY k.nama_kelas ASC, m.nama_mapel ASC, g.nama ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list pengajaran: %w", err)
	}
	defer rows.Close()

	var res []Pengajaran
	for rows.Next() {
		var p Pengajaran
		if err := rows.Scan(&p.ID, &p.GuruID, &p.MapelID, &p.KelasID, &p.NamaGuru, &p.NamaMapel, &p.NamaKelas); err != nil {
			return nil, fmt.Errorf("scan pengajaran: %w", err)
		}
		res = append(res, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if res == nil {
		res = []Pengajaran{}
	}
	return res, nil
}

func TambahPengajaran(ctx context.Context, db *sql.DB, guruID, mapelID, kelasID int) (int, error) {
	if guruID <= 0 || mapelID <= 0 || kelasID <= 0 {
		return 0, ErrInvalidInput
	}
	if db == nil {
		return 0, errors.New("database tidak tersedia")
	}

	// Cek apakah kombinasi pengajaran sudah ada
	var existingID int
	err := db.QueryRowContext(ctx, `
		SELECT id FROM pengajaran
		WHERE guru_id = ? AND mapel_id = ? AND kelas_id = ?
	`, guruID, mapelID, kelasID).Scan(&existingID)

	if err == nil {
		return 0, ErrDuplicatePengajaran
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("cek pengajaran: %w", err)
	}

	res, err := db.ExecContext(ctx, `
		INSERT INTO pengajaran (guru_id, mapel_id, kelas_id)
		VALUES (?, ?, ?)
	`, guruID, mapelID, kelasID)
	if err != nil {
		if isDuplicateError(err) {
			return 0, ErrDuplicatePengajaran
		}
		return 0, fmt.Errorf("insert pengajaran: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func HapusPengajaran(ctx context.Context, db *sql.DB, id int) error {
	if db == nil {
		return errors.New("database tidak tersedia")
	}
	_, err := db.ExecContext(ctx, "DELETE FROM pengajaran WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete pengajaran: %w", err)
	}
	return nil
}
