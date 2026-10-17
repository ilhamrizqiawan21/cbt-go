package realtime_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/coder/websocket"

	"cbt-go/internal/httpserver"
	"cbt-go/internal/realtime"
	"cbt-go/internal/sesi"
	"cbt-go/internal/testdb"
)

type seededSiswaTestData struct {
	SiswaID int
	UjianID int
	Soal1ID int
}

func seedSiswaEventData(t *testing.T, db *sql.DB) seededSiswaTestData {
	t.Helper()
	ctx := context.Background()

	resGuru, err := db.ExecContext(ctx, "INSERT INTO guru (nip, nama, username, password) VALUES ('19800101', 'Guru Test', 'guru_test', 'hash')")
	if err != nil {
		t.Fatalf("insert guru: %v", err)
	}
	guruID, _ := resGuru.LastInsertId()

	resKelas, err := db.ExecContext(ctx, "INSERT INTO kelas (nama_kelas) VALUES ('IX-A')")
	if err != nil {
		t.Fatalf("insert kelas: %v", err)
	}
	kelasID, _ := resKelas.LastInsertId()

	resMapel, err := db.ExecContext(ctx, "INSERT INTO mata_pelajaran (nama_mapel) VALUES ('IPA')")
	if err != nil {
		t.Fatalf("insert mapel: %v", err)
	}
	mapelID, _ := resMapel.LastInsertId()

	resPengajaran, err := db.ExecContext(ctx, "INSERT INTO pengajaran (guru_id, kelas_id, mapel_id) VALUES (?, ?, ?)", guruID, kelasID, mapelID)
	if err != nil {
		t.Fatalf("insert pengajaran: %v", err)
	}
	pengajaranID, _ := resPengajaran.LastInsertId()

	resSiswa, err := db.ExecContext(ctx, "INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('S001', 'Siswa Satu', 'IX-A', 'hash')")
	if err != nil {
		t.Fatalf("insert siswa: %v", err)
	}
	siswaID, _ := resSiswa.LastInsertId()

	resUjian, err := db.ExecContext(ctx, `
		INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai, acak_soal, acak_pilihan, batas_pelanggaran)
		VALUES (?, 'Ujian Realtime', 60, NOW() - INTERVAL 10 MINUTE, NOW() + INTERVAL 50 MINUTE, 1, 1, 3)
	`, pengajaranID)
	if err != nil {
		t.Fatalf("insert ujian: %v", err)
	}
	ujianID, _ := resUjian.LastInsertId()

	resSoal, err := db.ExecContext(ctx, `
		INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
		VALUES (?, 'pg', 'Soal 1?', 1, 'A', 'B', 'C', 'D', 'A')
	`, ujianID)
	if err != nil {
		t.Fatalf("insert soal: %v", err)
	}
	soal1ID, _ := resSoal.LastInsertId()

	return seededSiswaTestData{
		SiswaID: int(siswaID),
		UjianID: int(ujianID),
		Soal1ID: int(soal1ID),
	}
}

func setupSiswaEventTestServer(t *testing.T, db *sql.DB, deviceType string) (*httptest.Server, *realtime.Hub, string, seededSiswaTestData) {
	t.Helper()
	testdb.Reset(t, db)
	seeded := seedSiswaEventData(t, db)

	sm := scs.New()
	sm.Lifetime = 24 * time.Hour

	hub := realtime.NewHub(sm, realtime.Config{
		PingInterval:    500 * time.Millisecond,
		WriteTimeout:    2 * time.Second,
		RateLimitPerSec: 50,
		RateBurst:       100,
	})
	realtime.RegisterSiswaEvents(hub, db, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /test-login", func(w http.ResponseWriter, r *http.Request) {
		dev := r.URL.Query().Get("device")
		if dev == "" {
			dev = deviceType
		}
		_, err := sesi.MulaiAtauLanjut(r.Context(), db, seeded.SiswaID, seeded.UjianID, dev)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sm.Put(r.Context(), "peran", "siswa")
		sm.Put(r.Context(), "siswaId", seeded.SiswaID)
		sm.Put(r.Context(), "ujianId", seeded.UjianID)
		sm.Put(r.Context(), "deviceType", dev)
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("GET /ws", hub.ServeSiswaWS())
	mux.Handle("GET /api/cek-status-ujian", httpserver.CekStatusUjianHandler(db, sm))

	srv := httptest.NewServer(sm.LoadAndSave(mux))
	t.Cleanup(func() {
		hub.Close()
		srv.Close()
	})

	resp, err := http.Get(srv.URL + "/test-login?device=" + deviceType)
	if err != nil {
		t.Fatalf("gagal login test: %v", err)
	}
	resp.Body.Close()

	var cookieHeader []string
	for _, c := range resp.Cookies() {
		cookieHeader = append(cookieHeader, c.Name+"="+c.Value)
	}
	return srv, hub, strings.Join(cookieHeader, "; "), seeded
}

func dialSiswaWS(t *testing.T, srvURL, cookie string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := dialWS(ctx, srvURL, "/ws", cookie)
	if err != nil {
		t.Fatalf("gagal dial ws siswa: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	return conn
}

func sendSiswaEvent(t *testing.T, conn *websocket.Conn, event string, data any) {
	t.Helper()
	raw, err := json.Marshal(realtime.OutEnvelope{
		Event: event,
		Data:  data,
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		t.Fatalf("write ws event %s: %v", event, err)
	}
}

func readSiswaEvent(t *testing.T, conn *websocket.Conn, timeout time.Duration) realtime.Envelope {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, raw, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read ws event: %v", err)
	}
	var env realtime.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal ws event: %v", err)
	}
	return env
}

// TestSiswaSiapDanReconnect menguji event siswa-siap -> mulai-ujian, reconnect tidak mereset timer,
// dan disconnect hanya memperbarui last_seen (padanan sesi_endpoint_socket.test.js).
func TestSiswaSiapDanReconnect(t *testing.T) {
	db := testdb.Open(t)
	srv, _, cookie, seeded := setupSiswaEventTestServer(t, db, "laptop")

	conn1 := dialSiswaWS(t, srv.URL, cookie)
	sendSiswaEvent(t, conn1, "siswa-siap", map[string]any{})
	env := readSiswaEvent(t, conn1, 2*time.Second)

	if env.Event != "mulai-ujian" {
		t.Fatalf("event = %q, ingin mulai-ujian", env.Event)
	}

	var data1 struct {
		Durasi     int    `json:"durasi"`
		SisaDetik  int    `json:"sisa_detik"`
		DeviceType string `json:"device_type"`
		BatasWaktu string `json:"batas_waktu"`
	}
	if err := json.Unmarshal(env.Data, &data1); err != nil {
		t.Fatalf("gagal decode mulai-ujian: %v", err)
	}
	if data1.Durasi <= 0 || data1.SisaDetik <= 0 {
		t.Fatalf("durasi/sisa_detik harus > 0, dapat %+v", data1)
	}
	if data1.DeviceType != "laptop" {
		t.Fatalf("device_type = %q, ingin laptop", data1.DeviceType)
	}

	// Simpan jawaban lalu putus koneksi
	ctx := context.Background()
	_, err := sesi.SinkronJawaban(ctx, db, seeded.SiswaID, seeded.UjianID, []sesi.ItemJawabanInput{
		{SoalID: seeded.Soal1ID, Jawaban: "A", ClientTS: time.Now()},
	})
	if err != nil {
		t.Fatalf("gagal simpan jawaban: %v", err)
	}

	// Tutup koneksi 1 (disconnect)
	_ = conn1.Close(websocket.StatusNormalClosure, "putus sementara")
	time.Sleep(150 * time.Millisecond)

	// Pastikan status masih sedang_ujian dan jawaban tidak hilang
	info, err := sesi.AmbilSesiInfo(ctx, db, seeded.SiswaID, seeded.UjianID)
	if err != nil {
		t.Fatalf("gagal ambil sesi info: %v", err)
	}
	if info.Status != sesi.StatusSedangUjian {
		t.Fatalf("status pasca disconnect = %q, ingin sedang_ujian", info.Status)
	}
	if len(info.JawabanTersimpan) != 1 {
		t.Fatalf("jawaban harus tetap tersimpan, dapat %d", len(info.JawabanTersimpan))
	}

	// Majukan waktu_mulai / mundurkan batas_waktu sedikit untuk verifikasi reconnect tidak mereset ke durasi penuh
	_, err = db.ExecContext(ctx, `
		UPDATE sesi_ujian
		SET batas_waktu = DATE_ADD(NOW(), INTERVAL 15 MINUTE)
		WHERE siswa_id = ? AND ujian_id = ?
	`, seeded.SiswaID, seeded.UjianID)
	if err != nil {
		t.Fatalf("gagal update batas_waktu: %v", err)
	}

	conn2 := dialSiswaWS(t, srv.URL, cookie)
	sendSiswaEvent(t, conn2, "siswa-siap", map[string]any{})
	env2 := readSiswaEvent(t, conn2, 2*time.Second)
	if env2.Event != "mulai-ujian" {
		t.Fatalf("event reconnect = %q, ingin mulai-ujian", env2.Event)
	}
	var data2 struct {
		SisaDetik int `json:"sisa_detik"`
	}
	_ = json.Unmarshal(env2.Data, &data2)
	if data2.SisaDetik > 905 || data2.SisaDetik < 880 {
		t.Fatalf("sisa_detik reconnect = %d, seharusnya sekitar 900 dtk (15 menit), bukan reset ke durasi penuh", data2.SisaDetik)
	}
}

// TestKebijakanPerangkatDanPelanggaran menguji aturan anti-curang per perangkat (device_type.test.js & kebijakan_perangkat.test.js).
func TestKebijakanPerangkatDanPelanggaran(t *testing.T) {
	db := testdb.Open(t)

	t.Run("HP mengabaikan keluar-fullscreen tetapi menghitung pindah-tab dan copy-paste sampai keluar_paksa", func(t *testing.T) {
		srv, _, cookie, seeded := setupSiswaEventTestServer(t, db, "hp")

		conn := dialSiswaWS(t, srv.URL, cookie)
		sendSiswaEvent(t, conn, "siswa-siap", map[string]any{})
		_ = readSiswaEvent(t, conn, 2*time.Second) // mulai-ujian

		// 1. Kirim keluar-fullscreen pada HP -> harus diabaikan
		sendSiswaEvent(t, conn, "keluar-fullscreen", map[string]any{})
		time.Sleep(100 * time.Millisecond)

		jumlah, err := sesi.HitungPelanggaran(context.Background(), db, seeded.SiswaID, seeded.UjianID)
		if err != nil {
			t.Fatalf("gagal hitung pelanggaran: %v", err)
		}
		if jumlah != 0 {
			t.Fatalf("keluar-fullscreen pada HP harus diabaikan, dapat jumlah = %d", jumlah)
		}

		// 2. Kirim pindah-tab (1/3) -> peringatan
		sendSiswaEvent(t, conn, "pindah-tab", map[string]any{})
		ev1 := readSiswaEvent(t, conn, 2*time.Second)
		if ev1.Event != "peringatan" {
			t.Fatalf("event 1 = %q, ingin peringatan", ev1.Event)
		}

		// 3. Kirim copy-paste (2/3) -> peringatan
		sendSiswaEvent(t, conn, "copy-paste", map[string]any{})
		ev2 := readSiswaEvent(t, conn, 2*time.Second)
		if ev2.Event != "peringatan" {
			t.Fatalf("event 2 = %q, ingin peringatan", ev2.Event)
		}

		// Verifikasi device_type di log_kecurangan tercatat 'hp'
		var devType string
		err = db.QueryRowContext(context.Background(), `
			SELECT device_type FROM log_kecurangan WHERE siswa_id = ? AND ujian_id = ? LIMIT 1
		`, seeded.SiswaID, seeded.UjianID).Scan(&devType)
		if err != nil || devType != "hp" {
			t.Fatalf("device_type di log_kecurangan = %q (err=%v), ingin hp", devType, err)
		}

		// 4. Kirim pindah-tab (3/3) -> mencapai batas_pelanggaran (3) -> paksa-submit & keluar_paksa
		sendSiswaEvent(t, conn, "pindah-tab", map[string]any{})
		ev3 := readSiswaEvent(t, conn, 2*time.Second)
		if ev3.Event != "paksa-submit" {
			t.Fatalf("event 3 = %q, ingin paksa-submit", ev3.Event)
		}

		var dataKunci struct {
			Terkunci bool `json:"terkunci"`
		}
		if err := json.Unmarshal(ev3.Data, &dataKunci); err != nil || !dataKunci.Terkunci {
			t.Fatalf("paksa-submit karena pelanggaran harus membawa terkunci=true, dapat %s", ev3.Data)
		}

		var statusDB string
		err = db.QueryRowContext(context.Background(), `
			SELECT status FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?
		`, seeded.SiswaID, seeded.UjianID).Scan(&statusDB)
		if err != nil || statusDB != sesi.StatusKeluarPaksa {
			t.Fatalf("status di DB = %q (err=%v), ingin keluar_paksa", statusDB, err)
		}

		// 5. selesai-ujian lewat WebSocket saat terkunci tidak boleh mengubah status (kunci masih bisa dibuka).
		sendSiswaEvent(t, conn, "selesai-ujian", map[string]any{})
		time.Sleep(200 * time.Millisecond)
		_ = db.QueryRowContext(context.Background(), `SELECT status FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?`, seeded.SiswaID, seeded.UjianID).Scan(&statusDB)
		if statusDB != sesi.StatusKeluarPaksa {
			t.Fatalf("status setelah selesai-ujian WS saat terkunci = %q, ingin tetap keluar_paksa", statusDB)
		}
	})

	t.Run("selesai-ujian lewat WebSocket menghitung nilai (cadangan bila API gagal)", func(t *testing.T) {
		srv, _, cookie, seeded := setupSiswaEventTestServer(t, db, "laptop")
		conn := dialSiswaWS(t, srv.URL, cookie)
		sendSiswaEvent(t, conn, "siswa-siap", map[string]any{})
		_ = readSiswaEvent(t, conn, 2*time.Second)

		sendSiswaEvent(t, conn, "selesai-ujian", map[string]any{})
		deadline := time.Now().Add(2 * time.Second)
		var n int
		for time.Now().Before(deadline) {
			_ = db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM nilai_ujian WHERE siswa_id = ? AND ujian_id = ?`, seeded.SiswaID, seeded.UjianID).Scan(&n)
			if n == 1 {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if n != 1 {
			t.Fatalf("nilai_ujian tidak terbentuk setelah selesai-ujian via WebSocket")
		}
	})

	t.Run("Laptop menghitung keluar-fullscreen sebagai pelanggaran", func(t *testing.T) {
		srv, _, cookie, seeded := setupSiswaEventTestServer(t, db, "laptop")

		conn := dialSiswaWS(t, srv.URL, cookie)
		sendSiswaEvent(t, conn, "siswa-siap", map[string]any{})
		_ = readSiswaEvent(t, conn, 2*time.Second) // mulai-ujian

		sendSiswaEvent(t, conn, "keluar-fullscreen", map[string]any{})
		ev := readSiswaEvent(t, conn, 2*time.Second)
		if ev.Event != "peringatan" {
			t.Fatalf("event keluar-fullscreen laptop = %q, ingin peringatan", ev.Event)
		}

		jumlah, err := sesi.HitungPelanggaran(context.Background(), db, seeded.SiswaID, seeded.UjianID)
		if err != nil || jumlah != 1 {
			t.Fatalf("jumlah pelanggaran laptop = %d (err=%v), ingin 1", jumlah, err)
		}
	})
}

// TestCekStatusUjianEndpoint menguji GET /api/cek-status-ujian pada berbagai kondisi sesi.
func TestCekStatusUjianEndpoint(t *testing.T) {
	db := testdb.Open(t)
	srv, _, cookie, seeded := setupSiswaEventTestServer(t, db, "laptop")

	// 1. Tanpa cookie -> 401 not_logged_in
	respUnauth, err := http.Get(srv.URL + "/api/cek-status-ujian")
	if err != nil {
		t.Fatalf("gagal request: %v", err)
	}
	respUnauth.Body.Close()
	if respUnauth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status unauth = %d, ingin 401", respUnauth.StatusCode)
	}

	// 2. Dengan cookie aktif -> valid: true
	req, _ := http.NewRequest("GET", srv.URL+"/api/cek-status-ujian", nil)
	req.Header.Set("Cookie", cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("gagal request: %v", err)
	}
	var status1 struct {
		Valid             bool `json:"valid"`
		JumlahPelanggaran int  `json:"jumlah_pelanggaran"`
		Batas             int  `json:"batas"`
		Sisa              int  `json:"sisa"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&status1)
	resp.Body.Close()

	if !status1.Valid || status1.Batas != 3 || status1.Sisa != 3 {
		t.Fatalf("status aktif tidak sesuai: %+v", status1)
	}

	// 3. Ubah status sesi menjadi keluar_paksa -> valid: false, reason: keluar_paksa
	_, err = db.ExecContext(context.Background(), `
		UPDATE sesi_ujian SET status = 'keluar_paksa' WHERE siswa_id = ? AND ujian_id = ?
	`, seeded.SiswaID, seeded.UjianID)
	if err != nil {
		t.Fatalf("gagal update keluar_paksa: %v", err)
	}

	req2, _ := http.NewRequest("GET", srv.URL+"/api/cek-status-ujian", nil)
	req2.Header.Set("Cookie", cookie)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("gagal request 2: %v", err)
	}
	var status2 struct {
		Valid  bool   `json:"valid"`
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(resp2.Body).Decode(&status2)
	resp2.Body.Close()

	if status2.Valid || status2.Reason != "keluar_paksa" {
		t.Fatalf("status keluar_paksa tidak sesuai: %+v", status2)
	}
}
