package httpserver_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cbt-go/internal/config"
	"cbt-go/internal/httpserver"
	"cbt-go/internal/sesi"
	"cbt-go/internal/testdb"
)

func setupAPISiswaTestApp(t *testing.T, db *sql.DB) (http.Handler, *httpserver.Deps) {
	t.Helper()

	cfg := config.Config{
		Env:      config.EnvTest,
		Timezone: time.UTC,
	}
	sm := httpserver.NewSessionManager(db, cfg)

	deps := httpserver.Deps{
		DB:             db,
		Config:         cfg,
		SessionManager: sm,
	}

	h := httpserver.New(deps)
	return h, &deps
}

func setupAPISiswaData(t *testing.T, db *sql.DB) (int, int, string) {
	t.Helper()
	ctx := context.Background()

	res, err := db.ExecContext(ctx, "INSERT INTO guru (nip, nama, username, password) VALUES ('19800101', 'Guru 1', 'guru1', 'hash')")
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

	res, err = db.ExecContext(ctx, "INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('T0001', 'Siswa Tes', 'VII-A', 'hash')")
	if err != nil {
		t.Fatalf("insert siswa: %v", err)
	}
	siswaID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, `
		INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai, acak_soal, acak_pilihan, batas_pelanggaran)
		VALUES (?, 'Ujian Harian', 90, NOW() - INTERVAL 1 HOUR, NOW() + INTERVAL 2 HOUR, 1, 1, 3)
	`, pengajaranID)
	if err != nil {
		t.Fatalf("insert ujian: %v", err)
	}
	ujianID, _ := res.LastInsertId()

	// Masukkan 3 soal: PG Arab, Menjodohkan, Essay
	soalArab := "عَيِّنِ الفِعْلَ الْمَاضِيَ مِنَ الْجُمَلِ التَّالِيَةِ: كَتَبَ التِّلْمِيذُ الدَّرْسَ"
	resSoal1, err := db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
		VALUES (?, 'pg', ?, 2, 'كَتَبَ', 'التِّلْمِيذُ', 'الدَّرْسَ', 'فِي الفَصْلِ', 'A')
	`, ujianID, soalArab)
	if err != nil {
		t.Fatalf("insert soal arab: %v", err)
	}
	soal1ID, _ := resSoal1.LastInsertId()

	_, err = db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, opsi_tambahan, jawaban_benar)
		VALUES (?, 'menjodohkan', 'Jodohkan istilah:', 2, '{"pasangan":[{"kiri":"A","kanan":"1"},{"kiri":"B","kanan":"2"}],"pengecoh":["3"]}', 'kunci')
	`, ujianID)
	if err != nil {
		t.Fatalf("insert soal menjodohkan: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, jawaban_benar)
		VALUES (?, 'essay', 'Tuliskan hikmah belajar bahasa Arab!', 4, '')
	`, ujianID)
	if err != nil {
		t.Fatalf("insert soal essay: %v", err)
	}

	// Buat sesi
	hasilSesi, err := sesi.MulaiAtauLanjut(ctx, db, int(siswaID), int(ujianID), "laptop")
	if err != nil {
		t.Fatalf("MulaiAtauLanjut: %v", err)
	}

	// Simpan satu jawaban untuk soal 1
	_, err = db.ExecContext(ctx, `
		INSERT INTO jawaban_siswa (siswa_id, ujian_id, soal_id, jawaban_dipilih, is_benar, client_ts)
		VALUES (?, ?, ?, 'A', 1, NOW())
	`, siswaID, ujianID, soal1ID)
	if err != nil {
		t.Fatalf("insert jawaban: %v", err)
	}

	return int(siswaID), int(ujianID), hasilSesi.Sesi.DeviceToken
}

func createSiswaSessionCookie(t *testing.T, deps *httpserver.Deps, siswaID, ujianID int, deviceToken string) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest("GET", "/", nil)
	ctx, err := deps.SessionManager.Load(req.Context(), "")
	if err != nil {
		t.Fatalf("session load: %v", err)
	}
	deps.SessionManager.Put(ctx, "peran", "siswa")
	deps.SessionManager.Put(ctx, "siswaId", siswaID)
	deps.SessionManager.Put(ctx, "ujianId", ujianID)
	deps.SessionManager.Put(ctx, "deviceToken", deviceToken)

	token, _, err := deps.SessionManager.Commit(ctx)
	if err != nil {
		t.Fatalf("session commit: %v", err)
	}

	return &http.Cookie{
		Name:  deps.SessionManager.Cookie.Name,
		Value: token,
	}
}

func TestGetSesiRoute(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)

	handler, deps := setupAPISiswaTestApp(t, db)
	siswaID, ujianID, deviceToken := setupAPISiswaData(t, db)

	t.Run("401 bila tidak login", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/sesi", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("409 bila sesi diambil alih (deviceToken mismatch)", func(t *testing.T) {
		cookie := createSiswaSessionCookie(t, deps, siswaID, ujianID, "token-lama-sudah-usang")
		req := httptest.NewRequest("GET", "/api/sesi", nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Errorf("status = %d, want 409", rec.Code)
		}
	})

	t.Run("200 berhasil mendapatkan sesi dan jawaban tersimpan", func(t *testing.T) {
		cookie := createSiswaSessionCookie(t, deps, siswaID, ujianID, deviceToken)
		req := httptest.NewRequest("GET", "/api/sesi", nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
		}

		var resp struct {
			Status           string            `json:"status"`
			SisaDetik        int               `json:"sisa_detik"`
			BatasWaktu       string            `json:"batas_waktu"`
			JawabanTersimpan map[string]string `json:"jawaban_tersimpan"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal error: %v, raw: %s", err, rec.Body.String())
		}

		if resp.Status != "sedang_ujian" {
			t.Errorf("status = %s, want sedang_ujian", resp.Status)
		}
		if resp.SisaDetik <= 0 {
			t.Errorf("sisa_detik harus > 0, dapat %d", resp.SisaDetik)
		}
		if len(resp.JawabanTersimpan) != 1 {
			t.Errorf("jawaban_tersimpan harus punya 1 elemen, dapat %v", resp.JawabanTersimpan)
		}
	})
}

func TestGetSoalRoute(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)

	handler, deps := setupAPISiswaTestApp(t, db)
	siswaID, ujianID, deviceToken := setupAPISiswaData(t, db)

	t.Run("401 bila tidak login", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/soal/1", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("403 bila ujianId tidak cocok dengan session", func(t *testing.T) {
		cookie := createSiswaSessionCookie(t, deps, siswaID, ujianID, deviceToken)
		req := httptest.NewRequest("GET", "/api/soal/9999", nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403, body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("200 berhasil mendapatkan soal tanpa kunci jawaban", func(t *testing.T) {
		cookie := createSiswaSessionCookie(t, deps, siswaID, ujianID, deviceToken)
		path := "/api/soal/" + strings.TrimSpace(string(rune('0'+ujianID)))
		if ujianID >= 10 {
			path = "/api/soal/" + time.Now().Format("2006") // dummy, kita gunakan path terformat
		}
		path = "/api/soal/" + strings.ReplaceAll(jsonNumber(ujianID), " ", "")

		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
		}

		rawJSON := rec.Body.String()

		// Kunci jawaban tidak boleh ada di JSON
		terlarang := []string{"jawaban_benar", "is_benar", "kunci"}
		for _, k := range terlarang {
			if strings.Contains(strings.ToLower(rawJSON), k) {
				t.Fatalf("JSON response mengandung kata terlarang '%s': %s", k, rawJSON)
			}
		}

		// Bahasa Arab dan harakat harus utuh
		soalArab := "عَيِّنِ الفِعْلَ الْمَاضِيَ مِنَ الْجُمَلِ التَّالِيَةِ: كَتَبَ التِّلْمِيذُ الدَّرْسَ"
		if !strings.Contains(rawJSON, soalArab) {
			t.Errorf("teks soal Arab berharakat tidak ditemukan utuh di JSON: %s", rawJSON)
		}
	})
}

func jsonNumber(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestPersistentSeedAndDifferentStudents(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)

	ctx := context.Background()

	// 1. Setup master data
	res, err := db.ExecContext(ctx, "INSERT INTO guru (nip, nama, username, password) VALUES ('19800101', 'Guru X', 'gurux', 'hash')")
	if err != nil {
		t.Fatalf("insert guru: %v", err)
	}
	guruID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO kelas (nama_kelas) VALUES ('X-A')")
	if err != nil {
		t.Fatalf("insert kelas: %v", err)
	}
	kelasID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO mata_pelajaran (nama_mapel) VALUES ('Biologi')")
	if err != nil {
		t.Fatalf("insert mapel: %v", err)
	}
	mapelID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO pengajaran (guru_id, kelas_id, mapel_id) VALUES (?, ?, ?)", guruID, kelasID, mapelID)
	if err != nil {
		t.Fatalf("insert pengajaran: %v", err)
	}
	pengajaranID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, `
		INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai, acak_soal, acak_pilihan, batas_pelanggaran)
		VALUES (?, 'Ujian Biologi', 90, NOW() - INTERVAL 1 HOUR, NOW() + INTERVAL 2 HOUR, 1, 1, 3)
	`, pengajaranID)
	if err != nil {
		t.Fatalf("insert ujian: %v", err)
	}
	ujianID := int(resLastID(t, res))

	// Masukkan 6 soal PG
	for i := 1; i <= 6; i++ {
		_, err = db.ExecContext(ctx, `
			INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
			VALUES (?, 'pg', ?, 2, 'Pilihan 1', 'Pilihan 2', 'Pilihan 3', 'Pilihan 4', 'A')
		`, ujianID, "Pertanyaan ke-"+jsonNumber(i))
		if err != nil {
			t.Fatalf("insert soal %d: %v", i, err)
		}
	}

	// Masukkan Siswa 1 & Siswa 2
	res, err = db.ExecContext(ctx, "INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('T0001', 'Siswa 1', 'X-A', 'hash')")
	if err != nil {
		t.Fatalf("insert siswa 1: %v", err)
	}
	siswa1ID := int(resLastID(t, res))

	res, err = db.ExecContext(ctx, "INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('T0002', 'Siswa 2', 'X-A', 'hash')")
	if err != nil {
		t.Fatalf("insert siswa 2: %v", err)
	}
	siswa2ID := int(resLastID(t, res))

	// Buat sesi Siswa 1 & Siswa 2
	hasilSesi1, err := sesi.MulaiAtauLanjut(ctx, db, siswa1ID, ujianID, "laptop")
	if err != nil {
		t.Fatalf("MulaiAtauLanjut siswa 1: %v", err)
	}
	hasilSesi2, err := sesi.MulaiAtauLanjut(ctx, db, siswa2ID, ujianID, "hp")
	if err != nil {
		t.Fatalf("MulaiAtauLanjut siswa 2: %v", err)
	}

	// --- Instance Server 1 (Sebelum restart) ---
	handler1, deps1 := setupAPISiswaTestApp(t, db)
	cookieSiswa1A := createSiswaSessionCookie(t, deps1, siswa1ID, ujianID, hasilSesi1.Sesi.DeviceToken)

	req1A := httptest.NewRequest("GET", "/api/soal/"+jsonNumber(ujianID), nil)
	req1A.AddCookie(cookieSiswa1A)
	rec1A := httptest.NewRecorder()
	handler1.ServeHTTP(rec1A, req1A)

	if rec1A.Code != http.StatusOK {
		t.Fatalf("Server 1: status = %d, body: %s", rec1A.Code, rec1A.Body.String())
	}

	type SoalItem struct {
		ID      int `json:"id"`
		Pilihan []struct {
			Key  string `json:"key"`
			Text string `json:"text"`
		} `json:"pilihan"`
	}

	var dataSoal1A []SoalItem
	if err := json.Unmarshal(rec1A.Body.Bytes(), &dataSoal1A); err != nil {
		t.Fatalf("unmarshal 1A: %v", err)
	}

	urutanID1A := extractIDs(dataSoal1A)
	urutanPilihan1A := extractPilihanKeys(dataSoal1A)

	// --- Instance Server 2 (Simulasi setelah restart server) ---
	handler2, deps2 := setupAPISiswaTestApp(t, db)
	cookieSiswa1B := createSiswaSessionCookie(t, deps2, siswa1ID, ujianID, hasilSesi1.Sesi.DeviceToken)

	req1B := httptest.NewRequest("GET", "/api/soal/"+jsonNumber(ujianID), nil)
	req1B.AddCookie(cookieSiswa1B)
	rec1B := httptest.NewRecorder()
	handler2.ServeHTTP(rec1B, req1B)

	if rec1B.Code != http.StatusOK {
		t.Fatalf("Server 2: status = %d, body: %s", rec1B.Code, rec1B.Body.String())
	}

	var dataSoal1B []SoalItem
	if err := json.Unmarshal(rec1B.Body.Bytes(), &dataSoal1B); err != nil {
		t.Fatalf("unmarshal 1B: %v", err)
	}

	urutanID1B := extractIDs(dataSoal1B)
	urutanPilihan1B := extractPilihanKeys(dataSoal1B)

	// Verifikasi: Urutan soal dan pilihan untuk siswa yang sama HARUS IDENTIK sebelum dan sesudah restart
	if len(urutanID1A) != len(urutanID1B) {
		t.Fatalf("panjang soal berbeda: %d vs %d", len(urutanID1A), len(urutanID1B))
	}
	for i := range urutanID1A {
		if urutanID1A[i] != urutanID1B[i] {
			t.Errorf("urutan soal berubah sesudah restart pada indeks %d: %d vs %d", i, urutanID1A[i], urutanID1B[i])
		}
	}
	for i := range urutanPilihan1A {
		if urutanPilihan1A[i] != urutanPilihan1B[i] {
			t.Errorf("urutan pilihan berubah sesudah restart pada indeks %d: %s vs %s", i, urutanPilihan1A[i], urutanPilihan1B[i])
		}
	}

	// Siswa 2 mengambil soal pada server 2
	cookieSiswa2 := createSiswaSessionCookie(t, deps2, siswa2ID, ujianID, hasilSesi2.Sesi.DeviceToken)
	req2 := httptest.NewRequest("GET", "/api/soal/"+jsonNumber(ujianID), nil)
	req2.AddCookie(cookieSiswa2)
	rec2 := httptest.NewRecorder()
	handler2.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("Siswa 2: status = %d, body: %s", rec2.Code, rec2.Body.String())
	}

	var dataSoal2 []SoalItem
	if err := json.Unmarshal(rec2.Body.Bytes(), &dataSoal2); err != nil {
		t.Fatalf("unmarshal 2: %v", err)
	}

	urutanID2 := extractIDs(dataSoal2)

	// Verifikasi: Seed siswa 1 dan siswa 2 di database harus berbeda
	if hasilSesi1.Sesi.Seed == hasilSesi2.Sesi.Seed {
		t.Errorf("seed siswa 1 dan siswa 2 tidak boleh sama: %d", hasilSesi1.Sesi.Seed)
	}

	// Verifikasi: Urutan 6 butir soal siswa 1 dan siswa 2 harus berbeda
	samaSemua := true
	for i := range urutanID1A {
		if urutanID1A[i] != urutanID2[i] {
			samaSemua = false
			break
		}
	}
	if samaSemua {
		t.Errorf("urutan soal siswa 1 dan siswa 2 persis sama padahal seed berbeda: %v", urutanID1A)
	}
}

func resLastID(t *testing.T, res sql.Result) int64 {
	t.Helper()
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

func extractIDs[T any](items []T) []int {
	type hasID interface {
		GetID() int
	}
	// Manual unmarshal slice helper
	var ids []int
	b, _ := json.Marshal(items)
	var raw []struct {
		ID int `json:"id"`
	}
	_ = json.Unmarshal(b, &raw)
	for _, r := range raw {
		ids = append(ids, r.ID)
	}
	return ids
}

func extractPilihanKeys[T any](items []T) []string {
	var keys []string
	b, _ := json.Marshal(items)
	var raw []struct {
		Pilihan []struct {
			Key string `json:"key"`
		} `json:"pilihan"`
	}
	_ = json.Unmarshal(b, &raw)
	for _, r := range raw {
		var rowKeys []string
		for _, p := range r.Pilihan {
			rowKeys = append(rowKeys, p.Key)
		}
		keys = append(keys, strings.Join(rowKeys, ","))
	}
	return keys
}
