package httpserver_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"cbt-go/internal/config"
	"cbt-go/internal/httpserver"
	"cbt-go/internal/sesi"
	"cbt-go/internal/testdb"
)

const e2eDefaultPIN = "1234"

func setupKetahananE2EData(t *testing.T, db *sql.DB) (siswaID, ujianID, soal1ID, soal2ID, soal3ID int) {
	t.Helper()
	ctx := context.Background()

	hashPIN, err := bcrypt.GenerateFromPassword([]byte(e2eDefaultPIN), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash pin: %v", err)
	}

	resGuru, err := db.ExecContext(ctx, "INSERT INTO guru (nip, nama, username, password) VALUES ('19800101', 'Guru E2E', 'guru_e2e', ?)", string(hashPIN))
	if err != nil {
		t.Fatalf("insert guru: %v", err)
	}
	guruID, _ := resGuru.LastInsertId()

	resKelas, err := db.ExecContext(ctx, "INSERT INTO kelas (nama_kelas) VALUES ('X-A')")
	if err != nil {
		t.Fatalf("insert kelas: %v", err)
	}
	kelasID, _ := resKelas.LastInsertId()

	resMapel, err := db.ExecContext(ctx, "INSERT INTO mata_pelajaran (nama_mapel) VALUES ('Matematika')")
	if err != nil {
		t.Fatalf("insert mapel: %v", err)
	}
	mapelID, _ := resMapel.LastInsertId()

	resPengajaran, err := db.ExecContext(ctx, "INSERT INTO pengajaran (guru_id, kelas_id, mapel_id) VALUES (?, ?, ?)", guruID, kelasID, mapelID)
	if err != nil {
		t.Fatalf("insert pengajaran: %v", err)
	}
	pengajaranID, _ := resPengajaran.LastInsertId()

	resSiswa, err := db.ExecContext(ctx, "INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('T0001', 'Siswa E2E', 'X-A', ?)", string(hashPIN))
	if err != nil {
		t.Fatalf("insert siswa: %v", err)
	}
	sID, _ := resSiswa.LastInsertId()
	siswaID = int(sID)

	resUjian, err := db.ExecContext(ctx, `
		INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai, acak_soal, acak_pilihan, batas_pelanggaran)
		VALUES (?, 'Ujian E2E', 60, NOW() - INTERVAL 10 MINUTE, NOW() + INTERVAL 50 MINUTE, 1, 1, 3)
	`, pengajaranID)
	if err != nil {
		t.Fatalf("insert ujian: %v", err)
	}
	uID, _ := resUjian.LastInsertId()
	ujianID = int(uID)

	res1, err := db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
		VALUES (?, 'pg', 'Hasil dari 2 + 2?', 2, '3', '4', '5', '6', 'B')
	`, ujianID)
	if err != nil {
		t.Fatalf("insert soal 1: %v", err)
	}
	id1, _ := res1.LastInsertId()
	soal1ID = int(id1)

	res2, err := db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
		VALUES (?, 'pg', 'Hasil dari 5 x 5?', 2, '20', '25', '30', '35', 'B')
	`, ujianID)
	if err != nil {
		t.Fatalf("insert soal 2: %v", err)
	}
	id2, _ := res2.LastInsertId()
	soal2ID = int(id2)

	res3, err := db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
		VALUES (?, 'pg', 'Hasil dari 10 - 4?', 2, '6', '7', '8', '9', 'A')
	`, ujianID)
	if err != nil {
		t.Fatalf("insert soal 3: %v", err)
	}
	id3, _ := res3.LastInsertId()
	soal3ID = int(id3)

	return siswaID, ujianID, soal1ID, soal2ID, soal3ID
}

func doLoginSiswaE2E(t *testing.T, app http.Handler, nis, pin string, ujianID int, deviceType string) *http.Cookie {
	t.Helper()
	form := url.Values{
		"nis":         {nis},
		"pin":         {pin},
		"ujian_id":    {strconv.Itoa(ujianID)},
		"device_type": {deviceType},
	}
	req := httptest.NewRequest(http.MethodPost, "/login-siswa", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("login siswa gagal, status = %d, body = %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login siswa tidak mengembalikan cookie session")
	}
	return cookies[0]
}

// TestKetahananE2E adalah padanan tests/ketahanan_e2e.test.js untuk memverifikasi ketahanan sesi end-to-end.
func TestKetahananE2E(t *testing.T) {
	db := testdb.Open(t)

	t.Run("Skenario 1: Siswa login, jawab, putus koneksi, login ulang (takeover), lanjut sesi tanpa reset waktu", func(t *testing.T) {
		testdb.Reset(t, db)
		siswaID, ujianID, soal1ID, soal2ID, _ := setupKetahananE2EData(t, db)
		app, _ := setupAPISiswaTestApp(t, db)

		// 1. Siswa login pertama kali di laptop
		cookie1 := doLoginSiswaE2E(t, app, "T0001", e2eDefaultPIN, ujianID, "laptop")

		var waktuMulaiAwal, batasWaktuAwal time.Time
		err := db.QueryRow(`SELECT waktu_mulai, batas_waktu FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?`, siswaID, ujianID).
			Scan(&waktuMulaiAwal, &batasWaktuAwal)
		if err != nil {
			t.Fatalf("query sesi awal: %v", err)
		}

		// 2. Siswa menjawab soal ke-1
		body1, _ := json.Marshal(map[string]any{
			"jawaban": []map[string]any{
				{"soal_id": soal1ID, "jawaban": "B", "client_ts": time.Now().Format(time.RFC3339)},
			},
		})
		reqJawab1 := httptest.NewRequest(http.MethodPost, "/api/sinkron-jawaban", bytes.NewReader(body1))
		reqJawab1.Header.Set("Content-Type", "application/json")
		reqJawab1.AddCookie(cookie1)
		recJawab1 := httptest.NewRecorder()
		app.ServeHTTP(recJawab1, reqJawab1)
		if recJawab1.Code != http.StatusOK {
			t.Fatalf("sinkron 1 gagal: %d %s", recJawab1.Code, recJawab1.Body.String())
		}

		// 3. Siswa putus koneksi lalu login ulang dari HP (take-over)
		cookie2 := doLoginSiswaE2E(t, app, "T0001", e2eDefaultPIN, ujianID, "hp")

		// Verifikasi waktu_mulai dan batas_waktu tidak berubah
		var waktuMulaiLanjut, batasWaktuLanjut time.Time
		var statusLanjut string
		err = db.QueryRow(`SELECT waktu_mulai, batas_waktu, status FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?`, siswaID, ujianID).
			Scan(&waktuMulaiLanjut, &batasWaktuLanjut, &statusLanjut)
		if err != nil {
			t.Fatalf("query sesi lanjut: %v", err)
		}
		if !waktuMulaiLanjut.Equal(waktuMulaiAwal) {
			t.Errorf("waktu_mulai berubah saat takeover: awal=%v, lanjut=%v", waktuMulaiAwal, waktuMulaiLanjut)
		}
		if !batasWaktuLanjut.Equal(batasWaktuAwal) {
			t.Errorf("batas_waktu berubah saat takeover: awal=%v, lanjut=%v", batasWaktuAwal, batasWaktuLanjut)
		}
		if statusLanjut != sesi.StatusSedangUjian {
			t.Errorf("status = %q, ingin %q", statusLanjut, sesi.StatusSedangUjian)
		}

		// Verifikasi jawaban soal 1 tetap ada di GET /api/sesi
		reqSesi := httptest.NewRequest(http.MethodGet, "/api/sesi", nil)
		reqSesi.AddCookie(cookie2)
		recSesi := httptest.NewRecorder()
		app.ServeHTTP(recSesi, reqSesi)
		if recSesi.Code != http.StatusOK {
			t.Fatalf("GET /api/sesi gagal: %d", recSesi.Code)
		}
		var sesiResp struct {
			JawabanTersimpan map[string]string `json:"jawaban_tersimpan"`
		}
		_ = json.NewDecoder(recSesi.Body).Decode(&sesiResp)
		if sesiResp.JawabanTersimpan[strconv.Itoa(soal1ID)] != "B" {
			t.Errorf("jawaban soal 1 = %q, ingin 'B'", sesiResp.JawabanTersimpan[strconv.Itoa(soal1ID)])
		}

		// 4. Siswa lanjut menjawab soal ke-2 di perangkat baru
		body2, _ := json.Marshal(map[string]any{
			"jawaban": []map[string]any{
				{"soal_id": soal2ID, "jawaban": "B", "client_ts": time.Now().Format(time.RFC3339)},
			},
		})
		reqJawab2 := httptest.NewRequest(http.MethodPost, "/api/sinkron-jawaban", bytes.NewReader(body2))
		reqJawab2.Header.Set("Content-Type", "application/json")
		reqJawab2.AddCookie(cookie2)
		recJawab2 := httptest.NewRecorder()
		app.ServeHTTP(recJawab2, reqJawab2)
		if recJawab2.Code != http.StatusOK {
			t.Fatalf("sinkron 2 gagal: %d", recJawab2.Code)
		}

		var totalJawaban int
		_ = db.QueryRow(`SELECT COUNT(*) FROM jawaban_siswa WHERE siswa_id = ? AND ujian_id = ?`, siswaID, ujianID).Scan(&totalJawaban)
		if totalJawaban != 2 {
			t.Errorf("total jawaban di DB = %d, ingin 2", totalJawaban)
		}
	})

	t.Run("Skenario 2: Restart server saat ujian sedang berlangsung (cookie tetap valid, timer & seed konsisten)", func(t *testing.T) {
		testdb.Reset(t, db)
		siswaID, ujianID, soal1ID, soal2ID, _ := setupKetahananE2EData(t, db)

		// Server Instance 1
		app1, _ := setupAPISiswaTestApp(t, db)
		cookie := doLoginSiswaE2E(t, app1, "T0001", e2eDefaultPIN, ujianID, "laptop")

		body1, _ := json.Marshal(map[string]any{
			"jawaban": []map[string]any{
				{"soal_id": soal1ID, "jawaban": "B", "client_ts": time.Now().Format(time.RFC3339)},
			},
		})
		reqJawab1 := httptest.NewRequest(http.MethodPost, "/api/sinkron-jawaban", bytes.NewReader(body1))
		reqJawab1.Header.Set("Content-Type", "application/json")
		reqJawab1.AddCookie(cookie)
		recJawab1 := httptest.NewRecorder()
		app1.ServeHTTP(recJawab1, reqJawab1)
		if recJawab1.Code != http.StatusOK {
			t.Fatalf("sinkron di app1 gagal: %d", recJawab1.Code)
		}

		var seedAwal int64
		_ = db.QueryRow(`SELECT seed FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?`, siswaID, ujianID).Scan(&seedAwal)

		// Server Instance 2 (simulasi restart proses server dengan store MySQL)
		app2, _ := setupAPISiswaTestApp(t, db)
		reqSesi2 := httptest.NewRequest(http.MethodGet, "/api/sesi", nil)
		reqSesi2.AddCookie(cookie)
		recSesi2 := httptest.NewRecorder()
		app2.ServeHTTP(recSesi2, reqSesi2)
		if recSesi2.Code != http.StatusOK {
			t.Fatalf("GET /api/sesi di app2 setelah restart gagal: %d %s", recSesi2.Code, recSesi2.Body.String())
		}

		var sesiResp struct {
			SisaDetik        int               `json:"sisa_detik"`
			JawabanTersimpan map[string]string `json:"jawaban_tersimpan"`
		}
		_ = json.NewDecoder(recSesi2.Body).Decode(&sesiResp)
		if sesiResp.SisaDetik <= 0 {
			t.Errorf("sisa_detik = %d, harus > 0", sesiResp.SisaDetik)
		}
		if sesiResp.JawabanTersimpan[strconv.Itoa(soal1ID)] != "B" {
			t.Errorf("jawaban_tersimpan soal 1 = %q, ingin 'B'", sesiResp.JawabanTersimpan[strconv.Itoa(soal1ID)])
		}

		var seedSetelahRestart int64
		_ = db.QueryRow(`SELECT seed FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?`, siswaID, ujianID).Scan(&seedSetelahRestart)
		if seedSetelahRestart != seedAwal {
			t.Errorf("seed berubah setelah restart: %d != %d", seedSetelahRestart, seedAwal)
		}

		// Lanjut jawab soal ke-2 di Server Instance 2
		body2, _ := json.Marshal(map[string]any{
			"jawaban": []map[string]any{
				{"soal_id": soal2ID, "jawaban": "B", "client_ts": time.Now().Format(time.RFC3339)},
			},
		})
		reqJawab2 := httptest.NewRequest(http.MethodPost, "/api/sinkron-jawaban", bytes.NewReader(body2))
		reqJawab2.Header.Set("Content-Type", "application/json")
		reqJawab2.AddCookie(cookie)
		recJawab2 := httptest.NewRecorder()
		app2.ServeHTTP(recJawab2, reqJawab2)
		if recJawab2.Code != http.StatusOK {
			t.Fatalf("sinkron di app2 gagal: %d", recJawab2.Code)
		}
	})

	t.Run("Skenario 3: Sesi kedaluwarsa tanpa siswa online otomatis difinalisasi oleh background job", func(t *testing.T) {
		testdb.Reset(t, db)
		siswaID, ujianID, soal1ID, soal2ID, _ := setupKetahananE2EData(t, db)
		app, _ := setupAPISiswaTestApp(t, db)

		cookie := doLoginSiswaE2E(t, app, "T0001", e2eDefaultPIN, ujianID, "hp")

		// Siswa menjawab 2 dari 3 soal PG dengan benar lalu offline
		body, _ := json.Marshal(map[string]any{
			"jawaban": []map[string]any{
				{"soal_id": soal1ID, "jawaban": "B", "client_ts": time.Now().Format(time.RFC3339)},
				{"soal_id": soal2ID, "jawaban": "B", "client_ts": time.Now().Format(time.RFC3339)},
			},
		})
		reqJawab := httptest.NewRequest(http.MethodPost, "/api/sinkron-jawaban", bytes.NewReader(body))
		reqJawab.Header.Set("Content-Type", "application/json")
		reqJawab.AddCookie(cookie)
		recJawab := httptest.NewRecorder()
		app.ServeHTTP(recJawab, reqJawab)
		if recJawab.Code != http.StatusOK {
			t.Fatalf("sinkron gagal: %d", recJawab.Code)
		}

		// Majukan batas_waktu ke masa lampau melewati grace period (5 menit lalu)
		_, err := db.Exec(`UPDATE sesi_ujian SET batas_waktu = NOW() - INTERVAL 5 MINUTE WHERE siswa_id = ? AND ujian_id = ?`, siswaID, ujianID)
		if err != nil {
			t.Fatalf("update batas_waktu: %v", err)
		}

		hasilTutup, err := sesi.TutupSesiKadaluarsa(context.Background(), db, sesi.DefaultGraceDetik)
		if err != nil {
			t.Fatalf("TutupSesiKadaluarsa: %v", err)
		}
		if len(hasilTutup) != 1 {
			t.Fatalf("len(hasilTutup) = %d, ingin 1", len(hasilTutup))
		}

		var status string
		var selesaiPada sql.NullTime
		_ = db.QueryRow(`SELECT status, selesai_pada FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?`, siswaID, ujianID).
			Scan(&status, &selesaiPada)
		if status != sesi.StatusSelesai || !selesaiPada.Valid {
			t.Errorf("status = %q, selesai_pada = %+v, ingin selesai dan valid", status, selesaiPada)
		}

		// 2 benar dari 3 soal PG (poin 2 per soal = 4/6 -> round(66.67) = 67)
		var benar, kosong, nilai int
		err = db.QueryRow(`SELECT benar, kosong, ROUND(nilai) FROM nilai_ujian WHERE siswa_id = ? AND ujian_id = ?`, siswaID, ujianID).
			Scan(&benar, &kosong, &nilai)
		if err != nil {
			t.Fatalf("query nilai_ujian: %v", err)
		}
		if benar != 2 || kosong != 1 || nilai != 67 {
			t.Errorf("benar=%d kosong=%d nilai=%d, ingin benar=2 kosong=1 nilai=67", benar, kosong, nilai)
		}
	})

	t.Run("Skenario 4: Sesi keluar_paksa menolak sinkron, setelah dibuka kunci tetap menyimpan jawaban lama dan bisa selesai", func(t *testing.T) {
		testdb.Reset(t, db)
		siswaID, ujianID, soal1ID, soal2ID, soal3ID := setupKetahananE2EData(t, db)

		cfg := config.Config{
			Env:                 config.EnvTest,
			Timezone:            time.UTC,
			TampilkanNilaiSiswa: true,
		}
		sm := httpserver.NewSessionManager(db, cfg)
		app := httpserver.New(httpserver.Deps{
			DB:             db,
			Config:         cfg,
			SessionManager: sm,
		})

		cookie := doLoginSiswaE2E(t, app, "T0001", e2eDefaultPIN, ujianID, "laptop")

		// Jawab soal 1
		body1, _ := json.Marshal(map[string]any{
			"jawaban": []map[string]any{
				{"soal_id": soal1ID, "jawaban": "B", "client_ts": time.Now().Format(time.RFC3339)},
			},
		})
		req1 := httptest.NewRequest(http.MethodPost, "/api/sinkron-jawaban", bytes.NewReader(body1))
		req1.Header.Set("Content-Type", "application/json")
		req1.AddCookie(cookie)
		rec1 := httptest.NewRecorder()
		app.ServeHTTP(rec1, req1)
		if rec1.Code != http.StatusOK {
			t.Fatalf("sinkron 1 gagal: %d", rec1.Code)
		}

		// Sesi dikunci keluar_paksa
		_, _ = db.Exec(`UPDATE sesi_ujian SET status = 'keluar_paksa' WHERE siswa_id = ? AND ujian_id = ?`, siswaID, ujianID)

		// Sinkronisasi saat keluar_paksa harus ditolak 403
		reqBlocked := httptest.NewRequest(http.MethodPost, "/api/sinkron-jawaban", bytes.NewReader(body1))
		reqBlocked.Header.Set("Content-Type", "application/json")
		reqBlocked.AddCookie(cookie)
		recBlocked := httptest.NewRecorder()
		app.ServeHTTP(recBlocked, reqBlocked)
		if recBlocked.Code != http.StatusForbidden {
			t.Fatalf("sinkron saat keluar_paksa status = %d, ingin 403", recBlocked.Code)
		}

		// Buka kunci kembali ke sedang_ujian (tanpa menghapus jawaban)
		_, _ = db.Exec(`UPDATE sesi_ujian SET status = 'sedang_ujian' WHERE siswa_id = ? AND ujian_id = ?`, siswaID, ujianID)

		// Lanjut jawab soal 2 & 3
		body2, _ := json.Marshal(map[string]any{
			"jawaban": []map[string]any{
				{"soal_id": soal2ID, "jawaban": "B", "client_ts": time.Now().Format(time.RFC3339)},
				{"soal_id": soal3ID, "jawaban": "A", "client_ts": time.Now().Format(time.RFC3339)},
			},
		})
		req2 := httptest.NewRequest(http.MethodPost, "/api/sinkron-jawaban", bytes.NewReader(body2))
		req2.Header.Set("Content-Type", "application/json")
		req2.AddCookie(cookie)
		rec2 := httptest.NewRecorder()
		app.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusOK {
			t.Fatalf("sinkron 2 setelah buka kunci gagal: %d %s", rec2.Code, rec2.Body.String())
		}

		// Selesai ujian -> ketiga jawaban (termasuk soal 1 sebelum keluar_paksa) dihitung -> nilai 100
		reqSelesai := httptest.NewRequest(http.MethodPost, "/api/selesai-ujian", nil)
		reqSelesai.AddCookie(cookie)
		recSelesai := httptest.NewRecorder()
		app.ServeHTTP(recSelesai, reqSelesai)
		if recSelesai.Code != http.StatusOK {
			t.Fatalf("selesai ujian gagal: %d %s", recSelesai.Code, recSelesai.Body.String())
		}

		var hasil struct {
			Benar int `json:"benar"`
			Nilai int `json:"nilai"`
		}
		_ = json.NewDecoder(recSelesai.Body).Decode(&hasil)
		if hasil.Benar != 3 || hasil.Nilai != 100 {
			t.Errorf("hasil selesai = %+v, ingin benar=3 nilai=100", hasil)
		}
	})
}
