package soal_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cbt-go/internal/soal"
	"cbt-go/internal/testdb"
)

// TestBersihkanGambarUjian memastikan berkas gambar tidak tertinggal yatim saat
// seluruh soal sebuah ujian dihapus lewat cascade FK (menghapus ujian).
// HapusSoal/EditSoal sudah membersihkan berkas, tetapi DELETE FROM ujian
// menghapus baris soal tanpa melewati kode Go, sehingga berkasnya harus
// dikumpulkan lebih dulu.
func TestBersihkanGambarUjian(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)

	ctx := context.Background()

	res, err := db.ExecContext(ctx, "INSERT INTO guru (nip, nama, username, password) VALUES ('19800102', 'Guru Gambar', 'guru_gambar', 'hash')")
	if err != nil {
		t.Fatalf("insert guru: %v", err)
	}
	guruID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO kelas (nama_kelas) VALUES ('VII-G')")
	if err != nil {
		t.Fatalf("insert kelas: %v", err)
	}
	kelasID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO mata_pelajaran (nama_mapel) VALUES ('Seni Rupa')")
	if err != nil {
		t.Fatalf("insert mapel: %v", err)
	}
	mapelID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO pengajaran (guru_id, mapel_id, kelas_id) VALUES (?, ?, ?)", guruID, mapelID, kelasID)
	if err != nil {
		t.Fatalf("insert pengajaran: %v", err)
	}
	pengajaranID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, `INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai)
		VALUES (?, 'Ujian Bergambar', 60, NOW(), DATE_ADD(NOW(), INTERVAL 1 DAY))`, pengajaranID)
	if err != nil {
		t.Fatalf("insert ujian: %v", err)
	}
	ujianID, _ := res.LastInsertId()

	// Dua berkas gambar nyata milik ujian tersebut.
	namaA := simpanGambarUji(t)
	namaB := simpanGambarUji(t)
	for _, nama := range []string{namaA, namaB} {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO soal (ujian_id, tipe_soal, teks_soal, gambar, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar) VALUES (?, 'pg', 'Soal bergambar', ?, 2, 'A', 'B', 'C', 'D', 'A')",
			ujianID, nama); err != nil {
			t.Fatalf("insert soal: %v", err)
		}
	}

	// Gambar milik ujian LAIN tidak boleh ikut terhapus.
	namaLain := simpanGambarUji(t)
	res, err = db.ExecContext(ctx, `INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai)
		VALUES (?, 'Ujian Lain', 60, NOW(), DATE_ADD(NOW(), INTERVAL 1 DAY))`, pengajaranID)
	if err != nil {
		t.Fatalf("insert ujian lain: %v", err)
	}
	ujianLainID, _ := res.LastInsertId()
	if _, err := db.ExecContext(ctx,
		"INSERT INTO soal (ujian_id, tipe_soal, teks_soal, gambar, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar) VALUES (?, 'pg', 'Soal ujian lain', ?, 2, 'A', 'B', 'C', 'D', 'A')",
		ujianLainID, namaLain); err != nil {
		t.Fatalf("insert soal ujian lain: %v", err)
	}

	// Kumpulkan nama gambar SEBELUM menghapus ujian, lalu hapus (cascade), lalu bersihkan.
	namaGambar, err := soal.GambarUjian(ctx, db, int(ujianID))
	if err != nil {
		t.Fatalf("GambarUjian: %v", err)
	}
	if len(namaGambar) != 2 {
		t.Fatalf("GambarUjian mengembalikan %d nama, ingin 2", len(namaGambar))
	}

	if _, err := db.ExecContext(ctx, "DELETE FROM ujian WHERE id = ?", ujianID); err != nil {
		t.Fatalf("delete ujian: %v", err)
	}

	soal.BersihkanGambarYatim(ctx, db, namaGambar)

	for _, nama := range []string{namaA, namaB} {
		if berkasGambarAda(t, nama) {
			t.Errorf("berkas %s masih ada setelah ujian dihapus (gambar yatim)", nama)
		}
	}
	if !berkasGambarAda(t, namaLain) {
		t.Errorf("berkas %s milik ujian lain ikut terhapus", namaLain)
	}

	// Bersihkan sisa berkas uji.
	soal.HapusBerkasGambar(namaLain)
}

// simpanGambarUji menyimpan satu PNG kecil lewat SimpanGambar dan mengembalikan namanya.
func simpanGambarUji(t *testing.T) string {
	t.Helper()
	// PNG 1x1 minimal yang valid.
	png := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T',
		0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05,
		0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00,
		0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
	}
	nama, err := soal.SimpanGambar(strings.NewReader(string(png)))
	if err != nil {
		t.Fatalf("SimpanGambar: %v", err)
	}
	t.Cleanup(func() { soal.HapusBerkasGambar(nama) })
	return nama
}

func berkasGambarAda(t *testing.T, nama string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(soal.DirGambar, nama))
	return err == nil
}
