package soal_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"cbt-go/internal/sesi"
	"cbt-go/internal/soal"
	"cbt-go/internal/testdb"
)

func TestAcakSoalDeterministik(t *testing.T) {
	daftarSoal := make([]soal.SoalKlien, 10)
	for i := 0; i < 10; i++ {
		daftarSoal[i] = soal.SoalKlien{
			ID:       i + 1,
			Tipe:     "pg",
			TeksSoal: "Soal nomor " + string(rune('0'+i)),
			Poin:     2,
		}
	}

	seed1 := 123456
	seed2 := 654321

	hasil1A := soal.AcakSoal(daftarSoal, seed1)
	hasil1B := soal.AcakSoal(daftarSoal, seed1)
	hasil2 := soal.AcakSoal(daftarSoal, seed2)

	// Dua pemanggilan dengan seed sama harus menghasilkan urutan identik
	if len(hasil1A) != len(hasil1B) {
		t.Fatalf("panjang tidak sama: %d vs %d", len(hasil1A), len(hasil1B))
	}
	for i := range hasil1A {
		if hasil1A[i].ID != hasil1B[i].ID {
			t.Fatalf("pada indeks %d: ID tidak sama (%d vs %d)", i, hasil1A[i].ID, hasil1B[i].ID)
		}
	}

	// Pemanggilan dengan seed berbeda harus menghasilkan urutan yang berbeda
	samaSemua := true
	for i := range hasil1A {
		if hasil1A[i].ID != hasil2[i].ID {
			samaSemua = false
			break
		}
	}
	if samaSemua {
		t.Errorf("hasil pengacakan dengan seed berbeda (%d vs %d) menghasilkan urutan yang persis sama", seed1, seed2)
	}
}

func TestAcakPilihanDeterministik(t *testing.T) {
	pilihanAwal := []soal.PilihanKlien{
		{Key: "A", Text: "Opsi A"},
		{Key: "B", Text: "Opsi B"},
		{Key: "C", Text: "Opsi C"},
		{Key: "D", Text: "Opsi D"},
	}

	seed := 998877
	soalID1 := 10
	soalID2 := 20

	pilihan1A := soal.AcakPilihan(pilihanAwal, seed, soalID1)
	pilihan1B := soal.AcakPilihan(pilihanAwal, seed, soalID1)
	pilihan2 := soal.AcakPilihan(pilihanAwal, seed, soalID2)

	// Pemanggilan pada soal yang sama dengan seed sama harus identik
	for i := range pilihan1A {
		if pilihan1A[i].Key != pilihan1B[i].Key {
			t.Fatalf("pada indeks %d: pilihan key berbeda (%s vs %s)", i, pilihan1A[i].Key, pilihan1B[i].Key)
		}
	}

	// Pada soal berbeda (soalID1 vs soalID2), urutan pilihan harus memiliki variasi
	beda := false
	for i := range pilihan1A {
		if pilihan1A[i].Key != pilihan2[i].Key {
			beda = true
			break
		}
	}
	if !beda {
		t.Logf("Catatan: kemungkinan permutasi sama antara soal %d dan %d", soalID1, soalID2)
	}
}

func TestFormatMenjodohkan(t *testing.T) {
	opsiJSON := `{"pasangan":[{"kiri":"Kucing","kanan":"Mamalia"},{"kiri":"Merpati","kanan":"Aves"}],"pengecoh":["Reptil"]}`

	kiri, opsiKanan, err := soal.FormatMenjodohkan(opsiJSON, 12345, 1)
	if err != nil {
		t.Fatalf("FormatMenjodohkan gagal: %v", err)
	}

	if len(kiri) != 2 {
		t.Fatalf("panjang kiri harus 2, dapat %d", len(kiri))
	}
	if kiri[0] != "Kucing" || kiri[1] != "Merpati" {
		t.Errorf("urutan kiri tidak sesuai: %v", kiri)
	}

	if len(opsiKanan) != 3 {
		t.Fatalf("panjang opsiKanan harus 3 (2 pasangan + 1 pengecoh), dapat %d", len(opsiKanan))
	}

	// Pastikan semua opsi ada di opsiKanan
	harusAda := map[string]bool{"Mamalia": false, "Aves": false, "Reptil": false}
	for _, opt := range opsiKanan {
		harusAda[opt] = true
	}
	for k, v := range harusAda {
		if !v {
			t.Errorf("opsi %s tidak ditemukan di opsiKanan: %v", k, opsiKanan)
		}
	}
}

func TestSoalKlienTanpaJawabanBenar(t *testing.T) {
	soalPG := soal.SoalKlien{
		ID:       1,
		Tipe:     "pg",
		TeksSoal: "Berapa 1 + 1?",
		Poin:     2,
		Pilihan: []soal.PilihanKlien{
			{Key: "A", Text: "1"},
			{Key: "B", Text: "2"},
		},
	}

	soalMenjodohkan := soal.SoalKlien{
		ID:        2,
		Tipe:      "menjodohkan",
		TeksSoal:  "Jodohkan hewan",
		Poin:      2,
		Kiri:      []string{"Kucing", "Ayam"},
		OpsiKanan: []string{"Aves", "Mamalia"},
	}

	soalEssay := soal.SoalKlien{
		ID:       3,
		Tipe:     "essay",
		TeksSoal: "Jelaskan proses fotosintesis!",
		Poin:     4,
	}

	daftar := []soal.SoalKlien{soalPG, soalMenjodohkan, soalEssay}
	data, err := json.Marshal(daftar)
	if err != nil {
		t.Fatalf("json marshal gagal: %v", err)
	}

	rawJSON := string(data)

	// Pastikan tidak ada kunci jawaban atau indikator kebenaran
	kataKunciTerlarang := []string{
		"jawaban_benar",
		"jawaban",
		"is_benar",
		"kunci",
	}

	for _, kata := range kataKunciTerlarang {
		if strings.Contains(strings.ToLower(rawJSON), kata) {
			t.Errorf("JSON output tidak boleh mengandung kata '%s': %s", kata, rawJSON)
		}
	}
}

func TestAmbilPaketSoalIntegration(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)

	ctx := context.Background()

	// 1. Setup data master: guru, kelas, mapel, pengajaran
	res, err := db.ExecContext(ctx, "INSERT INTO guru (nip, nama, username, password) VALUES ('19800101', 'Guru Arab', 'guru_arab', 'hash')")
	if err != nil {
		t.Fatalf("insert guru: %v", err)
	}
	guruID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO kelas (nama_kelas) VALUES ('VII-A')")
	if err != nil {
		t.Fatalf("insert kelas: %v", err)
	}
	kelasID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO mata_pelajaran (nama_mapel) VALUES ('Bahasa Arab')")
	if err != nil {
		t.Fatalf("insert mapel: %v", err)
	}
	mapelID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO pengajaran (guru_id, kelas_id, mapel_id) VALUES (?, ?, ?)", guruID, kelasID, mapelID)
	if err != nil {
		t.Fatalf("insert pengajaran: %v", err)
	}
	pengajaranID, _ := res.LastInsertId()

	// 2. Setup siswa
	res, err = db.ExecContext(ctx, "INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('T0001', 'Siswa 1', 'VII-A', 'hash')")
	if err != nil {
		t.Fatalf("insert siswa: %v", err)
	}
	siswaID, _ := res.LastInsertId()

	// 3. Setup ujian (acak_soal=1, acak_pilihan=1)
	res, err = db.ExecContext(ctx, `
		INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai, acak_soal, acak_pilihan, batas_pelanggaran)
		VALUES (?, 'Ujian Bahasa Arab Test', 60, NOW() - INTERVAL 1 HOUR, NOW() + INTERVAL 2 HOUR, 1, 1, 3)
	`, pengajaranID)
	if err != nil {
		t.Fatalf("insert ujian: %v", err)
	}
	ujianID, _ := res.LastInsertId()

	// 4. Masukkan soal bahasa Arab berharakat (utf8mb4)
	soalArab := "عَيِّنِ الفِعْلَ الْمَاضِيَ مِنَ الْجُمَلِ التَّالِيَةِ: كَتَبَ التِّلْمِيذُ الدَّرْسَ"
	optA := "كَتَبَ"
	optB := "التِّلْمِيذُ"
	optC := "الدَّرْسَ"
	optD := "فِي الفَصْلِ"

	_, err = db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
		VALUES (?, 'pg', ?, 2, ?, ?, ?, ?, 'A')
	`, ujianID, soalArab, optA, optB, optC, optD)
	if err != nil {
		t.Fatalf("insert soal arab: %v", err)
	}

	// Masukkan beberapa soal tambahan untuk pengujian urutan
	for i := 2; i <= 5; i++ {
		_, err = db.ExecContext(ctx, `
			INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
			VALUES (?, 'pg', ?, 2, 'P1', 'P2', 'P3', 'P4', 'B')
		`, ujianID, "Soal ke-"+string(rune('0'+i)))
		if err != nil {
			t.Fatalf("insert soal %d: %v", i, err)
		}
	}

	// 5. Buat sesi siswa via sesi.MulaiAtauLanjut
	hasilMulai, err := sesi.MulaiAtauLanjut(ctx, db, int(siswaID), int(ujianID), "hp")
	if err != nil {
		t.Fatalf("MulaiAtauLanjut: %v", err)
	}

	// 6. Panggil AmbilPaketSoal
	paket1, err := soal.AmbilPaketSoal(ctx, db, int(siswaID), int(ujianID))
	if err != nil {
		t.Fatalf("AmbilPaketSoal 1 gagal: %v", err)
	}

	if len(paket1) != 5 {
		t.Fatalf("jumlah soal harus 5, dapat %d", len(paket1))
	}

	// Cari soal Arab dan pastikan harakat utuh
	var ditemukanArab bool
	for _, s := range paket1 {
		if s.TeksSoal == soalArab {
			ditemukanArab = true
			if len(s.Pilihan) != 4 {
				t.Fatalf("pilihan soal arab harus 4, dapat %d", len(s.Pilihan))
			}
			// Pastikan pilihan Arab tersimpan utuh
			pilihanTeks := make(map[string]bool)
			for _, p := range s.Pilihan {
				pilihanTeks[p.Text] = true
			}
			if !pilihanTeks[optA] || !pilihanTeks[optB] || !pilihanTeks[optC] || !pilihanTeks[optD] {
				t.Errorf("pilihan teks Arab tidak utuh: %v", pilihanTeks)
			}
		}
	}
	if !ditemukanArab {
		t.Errorf("soal teks Arab tidak ditemukan dalam paket soal")
	}

	// Panggil ulang AmbilPaketSoal dan pastikan urutan identik dengan sesi seed yang sama
	paket2, err := soal.AmbilPaketSoal(ctx, db, int(siswaID), int(ujianID))
	if err != nil {
		t.Fatalf("AmbilPaketSoal 2 gagal: %v", err)
	}

	for i := range paket1 {
		if paket1[i].ID != paket2[i].ID {
			t.Fatalf("urutan soal berubah antar panggilan untuk sesi yang sama (indeks %d: %d vs %d)", i, paket1[i].ID, paket2[i].ID)
		}
		for j := range paket1[i].Pilihan {
			if paket1[i].Pilihan[j].Key != paket2[i].Pilihan[j].Key {
				t.Fatalf("urutan pilihan berubah untuk soal %d (indeks %d: %s vs %s)", paket1[i].ID, j, paket1[i].Pilihan[j].Key, paket2[i].Pilihan[j].Key)
			}
		}
	}

	_ = hasilMulai
}
