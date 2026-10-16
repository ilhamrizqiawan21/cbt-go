package admin_test

import (
	"context"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"cbt-go/internal/admin"
	"cbt-go/internal/testdb"
)

func TestMasterData_Guru(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)

	ctx := context.Background()

	// 1. Tambah guru berhasil
	id, err := admin.TambahGuru(ctx, db, "19800101", "Pak Guru Satu", "guru1", "rahasia123")
	if err != nil {
		t.Fatalf("TambahGuru gagal: %v", err)
	}
	if id <= 0 {
		t.Fatalf("id guru harus > 0, got %d", id)
	}

	// Verifikasi hash bcrypt
	list, err := admin.ListGuru(ctx, db)
	if err != nil {
		t.Fatalf("ListGuru gagal: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("panjang list = %d, want 1", len(list))
	}
	g := list[0]
	if g.Username != "guru1" || g.Nama != "Pak Guru Satu" {
		t.Errorf("data guru tidak cocok: %+v", g)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(g.Password), []byte("rahasia123")); err != nil {
		t.Errorf("password hash bcrypt tidak valid: %v", err)
	}

	// 2. Tambah guru dengan username duplikat -> harus menghasilkan ErrDuplicateUsername
	_, err = admin.TambahGuru(ctx, db, "19800102", "Pak Guru Kloning", "guru1", "pass")
	if err != admin.ErrDuplicateUsername {
		t.Errorf("tambah username duplikat = %v, want %v", err, admin.ErrDuplicateUsername)
	}

	// 3. Edit guru tanpa mengubah password
	err = admin.EditGuru(ctx, db, id, "19800101-rev", "Pak Guru Satu Update", "guru1", "")
	if err != nil {
		t.Fatalf("EditGuru gagal: %v", err)
	}
	gUpdated, err := admin.GetGuruByID(ctx, db, id)
	if err != nil {
		t.Fatalf("GetGuruByID gagal: %v", err)
	}
	if gUpdated.Nama != "Pak Guru Satu Update" || gUpdated.NIP != "19800101-rev" {
		t.Errorf("data update tidak cocok: %+v", gUpdated)
	}
	// Password lama harus tetap utuh
	if err := bcrypt.CompareHashAndPassword([]byte(gUpdated.Password), []byte("rahasia123")); err != nil {
		t.Errorf("password lama harus dipertahankan bila password baru kosong: %v", err)
	}

	// 4. Edit guru dengan password baru
	err = admin.EditGuru(ctx, db, id, "19800101-rev", "Pak Guru Satu Update", "guru1", "passwordbaru456")
	if err != nil {
		t.Fatalf("EditGuru password baru gagal: %v", err)
	}
	gUpdatedPass, _ := admin.GetGuruByID(ctx, db, id)
	if err := bcrypt.CompareHashAndPassword([]byte(gUpdatedPass.Password), []byte("passwordbaru456")); err != nil {
		t.Errorf("password baru harus ter-hash bcrypt: %v", err)
	}

	// 5. Hapus guru
	err = admin.HapusGuru(ctx, db, id)
	if err != nil {
		t.Fatalf("HapusGuru gagal: %v", err)
	}
	listAfter, _ := admin.ListGuru(ctx, db)
	if len(listAfter) != 0 {
		t.Errorf("setelah hapus panjang list = %d, want 0", len(listAfter))
	}
}

func TestMasterData_Kelas(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)

	ctx := context.Background()

	// 1. Tambah kelas
	id, err := admin.TambahKelas(ctx, db, "VII-A")
	if err != nil {
		t.Fatalf("TambahKelas gagal: %v", err)
	}
	if id <= 0 {
		t.Fatalf("id kelas harus > 0, got %d", id)
	}

	// 2. Tambah nama_kelas duplikat -> ErrDuplicateKelas
	_, err = admin.TambahKelas(ctx, db, "VII-A")
	if err != admin.ErrDuplicateKelas {
		t.Errorf("tambah kelas duplikat = %v, want %v", err, admin.ErrDuplicateKelas)
	}

	// 3. Edit kelas
	err = admin.EditKelas(ctx, db, id, "VII-A Unggulan")
	if err != nil {
		t.Fatalf("EditKelas gagal: %v", err)
	}
	k, err := admin.GetKelasByID(ctx, db, id)
	if err != nil {
		t.Fatalf("GetKelasByID gagal: %v", err)
	}
	if k.NamaKelas != "VII-A Unggulan" {
		t.Errorf("nama kelas = %q, want 'VII-A Unggulan'", k.NamaKelas)
	}

	// 4. Hapus kelas
	err = admin.HapusKelas(ctx, db, id)
	if err != nil {
		t.Fatalf("HapusKelas gagal: %v", err)
	}
	list, _ := admin.ListKelas(ctx, db)
	if len(list) != 0 {
		t.Errorf("setelah hapus panjang list kelas = %d, want 0", len(list))
	}
}

func TestMasterData_Mapel(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)

	ctx := context.Background()

	// 1. Tambah mapel
	id, err := admin.TambahMapel(ctx, db, "Matematika")
	if err != nil {
		t.Fatalf("TambahMapel gagal: %v", err)
	}
	if id <= 0 {
		t.Fatalf("id mapel harus > 0, got %d", id)
	}

	// 2. Edit mapel
	err = admin.EditMapel(ctx, db, id, "Matematika Peminatan")
	if err != nil {
		t.Fatalf("EditMapel gagal: %v", err)
	}
	m, err := admin.GetMapelByID(ctx, db, id)
	if err != nil {
		t.Fatalf("GetMapelByID gagal: %v", err)
	}
	if m.NamaMapel != "Matematika Peminatan" {
		t.Errorf("nama mapel = %q, want 'Matematika Peminatan'", m.NamaMapel)
	}

	// 3. Hapus mapel
	err = admin.HapusMapel(ctx, db, id)
	if err != nil {
		t.Fatalf("HapusMapel gagal: %v", err)
	}
	list, _ := admin.ListMapel(ctx, db)
	if len(list) != 0 {
		t.Errorf("setelah hapus panjang list mapel = %d, want 0", len(list))
	}
}

func TestMasterData_Pengajaran(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)

	ctx := context.Background()

	// Setup data relasi
	guruID, _ := admin.TambahGuru(ctx, db, "123", "Guru IPA", "guruipa", "pass")
	kelasID, _ := admin.TambahKelas(ctx, db, "VIII-B")
	mapelID, _ := admin.TambahMapel(ctx, db, "IPA Terpadu")

	// 1. Tambah pengajaran
	id, err := admin.TambahPengajaran(ctx, db, guruID, mapelID, kelasID)
	if err != nil {
		t.Fatalf("TambahPengajaran gagal: %v", err)
	}
	if id <= 0 {
		t.Fatalf("id pengajaran harus > 0, got %d", id)
	}

	// 2. Tambah kombinasi yang sama -> tolak dengan ErrDuplicatePengajaran
	_, err = admin.TambahPengajaran(ctx, db, guruID, mapelID, kelasID)
	if err != admin.ErrDuplicatePengajaran {
		t.Errorf("tambah pengajaran duplikat = %v, want %v", err, admin.ErrDuplicatePengajaran)
	}

	// 3. ListPengajaran memuat nama guru, kelas, mapel
	list, err := admin.ListPengajaran(ctx, db)
	if err != nil {
		t.Fatalf("ListPengajaran gagal: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("panjang list pengajaran = %d, want 1", len(list))
	}
	p := list[0]
	if p.NamaGuru != "Guru IPA" || p.NamaKelas != "VIII-B" || p.NamaMapel != "IPA Terpadu" {
		t.Errorf("detail pengajaran JOIN tidak sesuai: %+v", p)
	}

	// 4. Hapus pengajaran
	err = admin.HapusPengajaran(ctx, db, id)
	if err != nil {
		t.Fatalf("HapusPengajaran gagal: %v", err)
	}
	listAfter, _ := admin.ListPengajaran(ctx, db)
	if len(listAfter) != 0 {
		t.Errorf("setelah hapus panjang list pengajaran = %d, want 0", len(listAfter))
	}
}

func TestMasterData_ValidasiInput(t *testing.T) {
	ctx := context.Background()

	// 1. Nil DB
	if _, err := admin.ListGuru(ctx, nil); err == nil {
		t.Error("ListGuru dengan db nil harus error")
	}
	if _, err := admin.TambahGuru(ctx, nil, "1", "a", "u", "p"); err == nil {
		t.Error("TambahGuru dengan db nil harus error")
	}

	// 2. Input kosong
	if _, err := admin.TambahGuru(ctx, nil, "", "", "", ""); err != admin.ErrInvalidInput {
		t.Errorf("TambahGuru input kosong = %v, want ErrInvalidInput", err)
	}
	if err := admin.EditGuru(ctx, nil, 0, "", "", "", ""); err != admin.ErrInvalidInput {
		t.Errorf("EditGuru input kosong = %v, want ErrInvalidInput", err)
	}
	if _, err := admin.TambahKelas(ctx, nil, "   "); err != admin.ErrInvalidInput {
		t.Errorf("TambahKelas input kosong = %v, want ErrInvalidInput", err)
	}
	if err := admin.EditKelas(ctx, nil, 1, ""); err != admin.ErrInvalidInput {
		t.Errorf("EditKelas input kosong = %v, want ErrInvalidInput", err)
	}
	if _, err := admin.TambahMapel(ctx, nil, "   "); err != admin.ErrInvalidInput {
		t.Errorf("TambahMapel input kosong = %v, want ErrInvalidInput", err)
	}
	if err := admin.EditMapel(ctx, nil, 1, ""); err != admin.ErrInvalidInput {
		t.Errorf("EditMapel input kosong = %v, want ErrInvalidInput", err)
	}
	if _, err := admin.TambahPengajaran(ctx, nil, 0, 1, 1); err != admin.ErrInvalidInput {
		t.Errorf("TambahPengajaran ID invalid = %v, want ErrInvalidInput", err)
	}
}
