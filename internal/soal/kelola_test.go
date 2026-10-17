package soal_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"cbt-go/internal/soal"
)

type sliceRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r *sliceRows) Columns() []string { return r.columns }
func (r *sliceRows) Close() error      { return nil }
func (r *sliceRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	for i, v := range r.rows[r.idx] {
		dest[i] = v
	}
	r.idx++
	return nil
}

type mockSoalDriver struct {
	lastInsertID int64
}

func (d *mockSoalDriver) Open(name string) (driver.Conn, error) {
	return &mockSoalConn{d: d}, nil
}

type mockSoalConn struct {
	d *mockSoalDriver
}

func (c *mockSoalConn) Prepare(query string) (driver.Stmt, error) {
	return &mockSoalStmt{conn: c, query: query}, nil
}
func (c *mockSoalConn) Close() error { return nil }
func (c *mockSoalConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

type mockSoalStmt struct {
	conn  *mockSoalConn
	query string
}

func (s *mockSoalStmt) Close() error  { return nil }
func (s *mockSoalStmt) NumInput() int { return -1 }
func (s *mockSoalStmt) Exec(args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.ExecContext(context.Background(), s.query, named)
}
func (s *mockSoalStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.QueryContext(context.Background(), s.query, named)
}

type mockResult struct {
	id int64
}

func (r mockResult) LastInsertId() (int64, error) { return r.id, nil }
func (r mockResult) RowsAffected() (int64, error) { return 1, nil }

func (c *mockSoalConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "INSERT INTO soal") {
		c.d.lastInsertID++
		return mockResult{id: c.d.lastInsertID}, nil
	}
	if strings.Contains(query, "UPDATE soal") {
		return mockResult{id: 1}, nil
	}
	if strings.Contains(query, "DELETE FROM soal") {
		return mockResult{id: 1}, nil
	}
	return mockResult{id: 1}, nil
}

func (c *mockSoalConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	// Cek otorisasi ujian guru
	if strings.Contains(query, "SELECT COUNT(*)") && strings.Contains(query, "FROM ujian u") {
		// args: ujianID, guruID
		var ujianID, guruID int64
		if len(args) >= 2 {
			ujianID, _ = args[0].Value.(int64)
			guruID, _ = args[1].Value.(int64)
		}
		// Skenario: Guru 1 mengampu Ujian 10; Guru 2 mengampu Ujian 20
		if (guruID == 1 && ujianID == 10) || (guruID == 2 && ujianID == 20) {
			return &sliceRows{columns: []string{"count"}, rows: [][]driver.Value{{int64(1)}}}, nil
		}
		return &sliceRows{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}}, nil
	}

	// Cek otorisasi soal guru
	if strings.Contains(query, "SELECT COUNT(*)") && strings.Contains(query, "FROM soal s") {
		// args: soalID, guruID
		var soalID, guruID int64
		if len(args) >= 2 {
			soalID, _ = args[0].Value.(int64)
			guruID, _ = args[1].Value.(int64)
		}
		// Soal 101 milik Ujian 10 (Guru 1); Soal 201 milik Ujian 20 (Guru 2)
		if (guruID == 1 && soalID == 101) || (guruID == 2 && soalID == 201) {
			return &sliceRows{columns: []string{"count"}, rows: [][]driver.Value{{int64(1)}}}, nil
		}
		return &sliceRows{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}}, nil
	}

	if strings.Contains(query, "FROM soal") && strings.Contains(query, "ujian_id = ?") {
		ans := "B"
		return &sliceRows{
			columns: []string{
				"id", "ujian_id", "tipe_soal", "teks_soal", "gambar", "poin",
				"pilihan_a", "pilihan_b", "pilihan_c", "pilihan_d", "jawaban_benar", "opsi_tambahan",
			},
			rows: [][]driver.Value{
				{
					int64(101), int64(10), "pg", "Berapa 1+1?", nil, int64(2),
					"1", "2", "3", "4", ans, nil,
				},
			},
		}, nil
	}

	if strings.Contains(query, "FROM soal") && strings.Contains(query, "id = ?") {
		ans := "B"
		return &sliceRows{
			columns: []string{
				"id", "ujian_id", "tipe_soal", "teks_soal", "gambar", "poin",
				"pilihan_a", "pilihan_b", "pilihan_c", "pilihan_d", "jawaban_benar", "opsi_tambahan",
			},
			rows: [][]driver.Value{
				{
					int64(101), int64(10), "pg", "Berapa 1+1?", nil, int64(2),
					"1", "2", "3", "4", ans, nil,
				},
			},
		}, nil
	}

	if strings.Contains(query, "FROM ujian u") && strings.Contains(query, "WHERE p.guru_id = ?") {
		return &sliceRows{
			columns: []string{"id", "nama_ujian", "durasi", "nama_mapel", "nama_kelas", "t_mulai", "t_selesai"},
			rows: [][]driver.Value{
				{int64(10), "PTS Matematika", int64(90), "Matematika", "VII-A", "2026-10-16 10:00", "2026-10-16 11:30"},
			},
		}, nil
	}

	return nil, fmt.Errorf("mock soal unknown query: %s", query)
}

func setupSoalMockDB(t *testing.T) *sql.DB {
	t.Helper()
	drv := &mockSoalDriver{}
	driverName := namaDriverMock("mock_soal")
	sql.Register(driverName, drv)

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open mock soal db: %v", err)
	}
	return db
}

func TestDefaultPoin(t *testing.T) {
	tests := []struct {
		tipe     string
		input    int
		expected int
	}{
		{"pg", 0, 2},
		{"PG", -1, 2},
		{"pg", 5, 5},
		{"menjodohkan", 0, 2},
		{"menjodohkan", 10, 10},
		{"essay", 0, 4},
		{"ESSAY", -5, 4},
		{"essay", 8, 8},
		{"unknown", 0, 2},
	}

	for _, tt := range tests {
		got := soal.DefaultPoin(tt.tipe, tt.input)
		if got != tt.expected {
			t.Errorf("DefaultPoin(%s, %d) = %d; want %d", tt.tipe, tt.input, got, tt.expected)
		}
	}
}

func TestSoalCRUDDanBatch(t *testing.T) {
	db := setupSoalMockDB(t)
	defer db.Close()
	ctx := context.Background()

	// 1. Tambah Soal PG dengan poin default
	ans := "B"
	s := soal.SoalKelola{
		UjianID:      10,
		TipeSoal:     "pg",
		TeksSoal:     "Berapa 1+1?",
		Poin:         0, // harus otomatis jadi 2
		PilihanA:     soalStringPtr("1"),
		PilihanB:     soalStringPtr("2"),
		JawabanBenar: &ans,
	}
	id, err := soal.TambahSoal(ctx, db, s)
	if err != nil || id <= 0 {
		t.Fatalf("TambahSoal gagal: %v", err)
	}

	// 2. Tambah Soal validasi teks kosong
	sKosong := s
	sKosong.TeksSoal = ""
	_, err = soal.TambahSoal(ctx, db, sKosong)
	if !errors.Is(err, soal.ErrTeksSoalKosong) {
		t.Errorf("harus ErrTeksSoalKosong, dapat: %v", err)
	}

	// 3. Tambah Soal tipe tidak valid
	sTipeSalah := s
	sTipeSalah.TipeSoal = "pilihan_ganda_kompleks"
	_, err = soal.TambahSoal(ctx, db, sTipeSalah)
	if !errors.Is(err, soal.ErrTipeSoalInvalid) {
		t.Errorf("harus ErrTipeSoalInvalid, dapat: %v", err)
	}

	// 4. Get Soal by ID
	sGet, err := soal.GetSoalByID(ctx, db, 101)
	if err != nil {
		t.Fatalf("GetSoalByID gagal: %v", err)
	}
	if sGet.TeksSoal != "Berapa 1+1?" {
		t.Errorf("teks soal salah: %s", sGet.TeksSoal)
	}

	// 5. Edit Soal
	s.ID = 101
	s.TeksSoal = "Berapa 2+2?"
	err = soal.EditSoal(ctx, db, s)
	if err != nil {
		t.Fatalf("EditSoal gagal: %v", err)
	}

	// 6. Hapus Soal
	err = soal.HapusSoal(ctx, db, 101)
	if err != nil {
		t.Fatalf("HapusSoal gagal: %v", err)
	}

	// 7. Batch Tambah Soal
	daftar := []soal.SoalKelola{
		{TipeSoal: "pg", TeksSoal: "Soal 1", Poin: 2, PilihanA: soalStringPtr("x"), PilihanB: soalStringPtr("y"), JawabanBenar: soalStringPtr("A")},
		{TipeSoal: "essay", TeksSoal: "Soal 2", Poin: 4},
	}
	total, err := soal.BatchTambahSoal(ctx, db, 10, daftar)
	if err != nil || total != 2 {
		t.Fatalf("BatchTambahSoal gagal: %v, total = %d", err, total)
	}
}

func soalStringPtr(s string) *string {
	return &s
}

// mockDriverSeq menomori nama driver mock agar selalu unik dalam satu proses uji.
// Sebelumnya nama dibentuk dari time.Now().UnixNano(); itu tidak aman karena di
// Windows dua panggilan berurutan dapat mengembalikan nilai yang sama, sehingga
// sql.Register panik dengan "Register called twice for driver ...".
var mockDriverSeq atomic.Uint64

// namaDriverMock mengembalikan nama driver unik dengan awalan yang diberikan.
func namaDriverMock(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, mockDriverSeq.Add(1))
}
