package monitor_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/auth"
	"cbt-go/internal/monitor"
	"cbt-go/internal/testdb"
)

func TestHitungStatusTurunan(t *testing.T) {
	now := time.Date(2026, 10, 16, 18, 0, 0, 0, time.UTC)

	secBefore := func(sec int) *time.Time {
		tm := now.Add(-time.Duration(sec) * time.Second)
		return &tm
	}

	tests := []struct {
		name       string
		statusSesi string
		lastSeen   *time.Time
		want       monitor.StatusMonitor
	}{
		{
			name:       "belum_masuk (sesi kosong)",
			statusSesi: "",
			lastSeen:   nil,
			want:       monitor.StatusBelumMasuk,
		},
		{
			name:       "selesai (bahkan jika lastSeen baru)",
			statusSesi: "selesai",
			lastSeen:   secBefore(5),
			want:       monitor.StatusSelesai,
		},
		{
			name:       "terkunci / keluar_paksa (bahkan jika lastSeen baru)",
			statusSesi: "keluar_paksa",
			lastSeen:   secBefore(5),
			want:       monitor.StatusTerkunci,
		},
		{
			name:       "sedang_ujian tapi lastSeen nil -> offline",
			statusSesi: "sedang_ujian",
			lastSeen:   nil,
			want:       monitor.StatusOffline,
		},
		{
			name:       "sedang_ujian lastSeen 10s lalu (< 45s) -> online",
			statusSesi: "sedang_ujian",
			lastSeen:   secBefore(10),
			want:       monitor.StatusOnline,
		},
		{
			name:       "sedang_ujian lastSeen tepat 44s lalu -> online",
			statusSesi: "sedang_ujian",
			lastSeen:   secBefore(44),
			want:       monitor.StatusOnline,
		},
		{
			name:       "sedang_ujian lastSeen tepat batas 45s lalu -> offline",
			statusSesi: "sedang_ujian",
			lastSeen:   secBefore(45),
			want:       monitor.StatusOffline,
		},
		{
			name:       "sedang_ujian lastSeen 46s lalu -> offline",
			statusSesi: "sedang_ujian",
			lastSeen:   secBefore(46),
			want:       monitor.StatusOffline,
		},
		{
			name:       "sedang_ujian lastSeen 5 menit lalu -> offline",
			statusSesi: "sedang_ujian",
			lastSeen:   secBefore(300),
			want:       monitor.StatusOffline,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monitor.HitungStatusTurunan(tt.statusSesi, tt.lastSeen, now)
			if got != tt.want {
				t.Errorf("HitungStatusTurunan(%q, %v) = %v, want %v", tt.statusSesi, tt.lastSeen, got, tt.want)
			}
		})
	}
}

type mockBroadcaster struct {
	hasListeners bool
	broadcasts   []struct {
		ujianID int
		event   string
		data    any
	}
}

func (m *mockBroadcaster) HasAdminListeners(ujianID int) bool {
	return m.hasListeners
}

func (m *mockBroadcaster) BroadcastToAdminRoom(ujianID int, event string, data any) {
	m.broadcasts = append(m.broadcasts, struct {
		ujianID int
		event   string
		data    any
	}{ujianID: ujianID, event: event, data: data})
}

// mockDriver mengimplementasikan database/sql/driver.Driver untuk mencatat jumlah kueri
// dan mensimulasikan hasil 500 siswa secara deterministik tanpa koneksi database fisik.
type mockDriver struct {
	queryCount int
	totalSiswa int
}

func (d *mockDriver) Open(name string) (driver.Conn, error) {
	return &mockConn{d: d}, nil
}

type mockConn struct {
	d *mockDriver
}

func (c *mockConn) Prepare(query string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (c *mockConn) Close() error { return nil }

func (c *mockConn) Begin() (driver.Tx, error) { return nil, errors.New("not implemented") }

func (c *mockConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	return c.QueryContext(context.Background(), query, nil)
}

func (c *mockConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.d.queryCount++

	if strings.Contains(query, "FROM ujian") {
		// Kueri 1: info ujian
		return &sliceRows{
			columns: []string{"nama_ujian", "total_soal"},
			rows: [][]driver.Value{
				{"Ujian Skala 500 Siswa", int64(40)},
			},
		}, nil
	}

	if strings.Contains(query, "FROM siswa") {
		// Kueri 2: daftar 500 siswa sekaligus dalam satu kueri agregasi
		now := time.Now().UTC()
		var rows [][]driver.Value
		for i := 1; i <= c.d.totalSiswa; i++ {
			sesiID := int64(i)
			statusSesi := "sedang_ujian"
			lastSeen := now.Add(-10 * time.Second) // online
			if i > 400 {
				statusSesi = "selesai"
			} else if i > 350 {
				statusSesi = "keluar_paksa"
			} else if i > 300 {
				lastSeen = now.Add(-100 * time.Second) // offline
			}
			bw := now.Add(45 * time.Minute)

			rows = append(rows, []driver.Value{
				int64(i),                         // siswa_id
				fmt.Sprintf("NIS%04d", i),        // nis
				fmt.Sprintf("Siswa Nomor %d", i), // nama
				"VII-A",                          // kelas
				sesiID,                           // sesi_id
				statusSesi,                       // status_sesi
				"laptop",                         // device_type
				lastSeen,                         // last_seen
				bw,                               // batas_waktu
				int64(0),                         // jumlah_pelanggaran
				int64(25),                        // soal_dijawab
			})
		}
		return &sliceRows{
			columns: []string{
				"siswa_id", "nis", "nama", "kelas", "sesi_id",
				"status_sesi", "device_type", "last_seen", "batas_waktu",
				"jumlah_pelanggaran", "soal_dijawab",
			},
			rows: rows,
		}, nil
	}

	return nil, fmt.Errorf("kueri tidak dikenali: %s", query)
}

type sliceRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r *sliceRows) Columns() []string { return r.columns }

func (r *sliceRows) Close() error { return nil }

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

func TestSnapshot_500Siswa_TanpaNPlusOneQuery_QueryCounter(t *testing.T) {
	drv := &mockDriver{totalSiswa: 500}
	driverName := fmt.Sprintf("mock_counter_%d", time.Now().UnixNano())
	sql.Register(driverName, drv)

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open mock db: %v", err)
	}
	defer db.Close()

	b := &mockBroadcaster{hasListeners: true}
	svc := monitor.NewService(db, b)

	snap, err := svc.Snapshot(context.Background(), 1)
	if err != nil {
		t.Fatalf("Snapshot error: %v", err)
	}

	// VERIFIKASI WAJIB N+1:
	// Untuk 500 siswa, jumlah kueri ke database HARUS tepat 2 kueri (1 ujian + 1 siswa agregasi)
	// Tidak boleh ada kueri tambahan per-siswa (N+1).
	if drv.queryCount != 2 {
		t.Errorf("jumlah kueri = %d, ingin tepat 2 (bebas N+1)", drv.queryCount)
	}

	if snap.Ringkasan.TotalSiswa != 500 {
		t.Errorf("TotalSiswa = %d, want 500", snap.Ringkasan.TotalSiswa)
	}
	if len(snap.Siswa) != 500 {
		t.Errorf("len(Siswa) = %d, want 500", len(snap.Siswa))
	}

	// 1..300 online (300 siswa)
	if snap.Ringkasan.Online != 300 {
		t.Errorf("Ringkasan.Online = %d, want 300", snap.Ringkasan.Online)
	}
	// 301..350 offline (50 siswa)
	if snap.Ringkasan.Offline != 50 {
		t.Errorf("Ringkasan.Offline = %d, want 50", snap.Ringkasan.Offline)
	}
	// 351..400 terkunci (50 siswa)
	if snap.Ringkasan.Terkunci != 50 {
		t.Errorf("Ringkasan.Terkunci = %d, want 50", snap.Ringkasan.Terkunci)
	}
	// 401..500 selesai (100 siswa)
	if snap.Ringkasan.Selesai != 100 {
		t.Errorf("Ringkasan.Selesai = %d, want 100", snap.Ringkasan.Selesai)
	}
}

func TestSnapshot_TanpaNPlusOneQuery(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)

	ctx := context.Background()

	// 1. Buat guru, kelas, mapel, pengajaran
	res, err := db.ExecContext(ctx, "INSERT INTO guru (nip, nama, username, password) VALUES ('19800101', 'Guru Monitor', 'gurumonitor', 'hash')")
	if err != nil {
		t.Fatalf("insert guru: %v", err)
	}
	guruID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO kelas (nama_kelas) VALUES ('VII-MONITOR')")
	if err != nil {
		t.Fatalf("insert kelas: %v", err)
	}
	kelasID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO mata_pelajaran (nama_mapel) VALUES ('Matematika')")
	if err != nil {
		t.Fatalf("insert mapel: %v", err)
	}
	mapelID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO pengajaran (guru_id, mapel_id, kelas_id) VALUES (?, ?, ?)", guruID, mapelID, kelasID)
	if err != nil {
		t.Fatalf("insert pengajaran: %v", err)
	}
	pengajaranID, _ := res.LastInsertId()

	// 2. Buat Ujian & Soal
	now := time.Now().UTC()
	mulai := now.Add(-1 * time.Hour)
	selesai := now.Add(2 * time.Hour)

	res, err = db.ExecContext(ctx, `
		INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai, acak_soal, acak_pilihan, batas_pelanggaran)
		VALUES (?, 'Ujian Monitor Terpadu', 60, ?, ?, 1, 1, 3)
	`, pengajaranID, mulai, selesai)
	if err != nil {
		t.Fatalf("insert ujian: %v", err)
	}
	ujianID64, _ := res.LastInsertId()
	ujianID := int(ujianID64)

	// Masukkan 3 soal
	for i := 1; i <= 3; i++ {
		_, err = db.ExecContext(ctx, `
			INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a_d, jawaban_benar)
			VALUES (?, 'pg', 'Soal Test', 2, '["A","B","C","D"]', 'A')
		`, ujianID)
		if err != nil {
			t.Fatalf("insert soal: %v", err)
		}
	}

	// 3. Masukkan 5 siswa di kelas VII-MONITOR dengan status berbeda
	type siswaData struct {
		nis              string
		nama             string
		statusSesi       string
		lastSeenDeltaSec int // detik yang lalu
		pelanggaran      int
		jawabanCount     int
		expectStatusMon  monitor.StatusMonitor
	}

	siswaList := []siswaData{
		{
			nis:             "MON001",
			nama:            "Siswa Belum Masuk",
			statusSesi:      "", // tidak buat sesi
			expectStatusMon: monitor.StatusBelumMasuk,
		},
		{
			nis:              "MON002",
			nama:             "Siswa Online Aktif",
			statusSesi:       "sedang_ujian",
			lastSeenDeltaSec: 10,
			pelanggaran:      1,
			jawabanCount:     2,
			expectStatusMon:  monitor.StatusOnline,
		},
		{
			nis:              "MON003",
			nama:             "Siswa Offline Diam",
			statusSesi:       "sedang_ujian",
			lastSeenDeltaSec: 90,
			pelanggaran:      0,
			jawabanCount:     1,
			expectStatusMon:  monitor.StatusOffline,
		},
		{
			nis:              "MON004",
			nama:             "Siswa Terkunci",
			statusSesi:       "keluar_paksa",
			lastSeenDeltaSec: 5,
			pelanggaran:      3,
			jawabanCount:     1,
			expectStatusMon:  monitor.StatusTerkunci,
		},
		{
			nis:              "MON005",
			nama:             "Siswa Selesai",
			statusSesi:       "selesai",
			lastSeenDeltaSec: 120,
			pelanggaran:      0,
			jawabanCount:     3,
			expectStatusMon:  monitor.StatusSelesai,
		},
	}

	for _, s := range siswaList {
		rSiswa, err := db.ExecContext(ctx, "INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES (?, ?, 'VII-MONITOR', 'pin')", s.nis, s.nama)
		if err != nil {
			t.Fatalf("insert siswa: %v", err)
		}
		sID, _ := rSiswa.LastInsertId()

		if s.statusSesi != "" {
			ls := now.Add(-time.Duration(s.lastSeenDeltaSec) * time.Second)
			bw := now.Add(40 * time.Minute)
			rSesi, err := db.ExecContext(ctx, `
				INSERT INTO sesi_ujian (siswa_id, ujian_id, status, device_type, last_seen, batas_waktu, jumlah_pelanggaran)
				VALUES (?, ?, ?, 'laptop', ?, ?, ?)
			`, sID, ujianID, s.statusSesi, ls, bw, s.pelanggaran)
			if err != nil {
				t.Fatalf("insert sesi_ujian: %v", err)
			}
			sesiID, _ := rSesi.LastInsertId()

			// Masukkan jawaban
			for j := 1; j <= s.jawabanCount; j++ {
				_, err = db.ExecContext(ctx, `
					INSERT INTO jawaban_siswa (sesi_id, soal_id, jawaban_dipilih, client_ts)
					VALUES (?, ?, 'A', NOW())
				`, sesiID, j)
				if err != nil {
					t.Fatalf("insert jawaban: %v", err)
				}
			}
		}
	}

	b := &mockBroadcaster{hasListeners: true}
	svc := monitor.NewService(db, b)

	snap, err := svc.Snapshot(ctx, ujianID)
	if err != nil {
		t.Fatalf("Snapshot error: %v", err)
	}

	if snap.TotalSoal != 3 {
		t.Errorf("TotalSoal = %d, want 3", snap.TotalSoal)
	}
	if snap.Ringkasan.TotalSiswa != 5 {
		t.Errorf("Ringkasan.TotalSiswa = %d, want 5", snap.Ringkasan.TotalSiswa)
	}
	if snap.Ringkasan.Online != 1 {
		t.Errorf("Ringkasan.Online = %d, want 1", snap.Ringkasan.Online)
	}
	if snap.Ringkasan.Offline != 1 {
		t.Errorf("Ringkasan.Offline = %d, want 1", snap.Ringkasan.Offline)
	}
	if snap.Ringkasan.BelumMasuk != 1 {
		t.Errorf("Ringkasan.BelumMasuk = %d, want 1", snap.Ringkasan.BelumMasuk)
	}
	if snap.Ringkasan.Terkunci != 1 {
		t.Errorf("Ringkasan.Terkunci = %d, want 1", snap.Ringkasan.Terkunci)
	}
	if snap.Ringkasan.Selesai != 1 {
		t.Errorf("Ringkasan.Selesai = %d, want 1", snap.Ringkasan.Selesai)
	}

	if len(snap.Siswa) != 5 {
		t.Fatalf("jumlah siswa dalam snapshot = %d, want 5", len(snap.Siswa))
	}

	// Uji siaran update saat ada listener
	err = svc.SiarkanUpdate(ctx, ujianID)
	if err != nil {
		t.Fatalf("SiarkanUpdate error: %v", err)
	}
	if len(b.broadcasts) != 1 {
		t.Fatalf("broadcasts count = %d, want 1", len(b.broadcasts))
	}
	if b.broadcasts[0].event != "monitor:update" {
		t.Errorf("event = %q, want monitor:update", b.broadcasts[0].event)
	}
}

func TestEndpointGetMonitor(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)

	ctx := context.Background()

	// Seed ujian sederhana
	res, err := db.ExecContext(ctx, "INSERT INTO kelas (nama_kelas) VALUES ('VII-API')")
	if err != nil {
		t.Fatalf("insert kelas: %v", err)
	}
	kelasID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO guru (nip, nama, username, password) VALUES ('123', 'Guru', 'guruapi', 'h')")
	if err != nil {
		t.Fatalf("insert guru: %v", err)
	}
	guruID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO mata_pelajaran (nama_mapel) VALUES ('IPA')")
	if err != nil {
		t.Fatalf("insert mapel: %v", err)
	}
	mapelID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, "INSERT INTO pengajaran (guru_id, mapel_id, kelas_id) VALUES (?, ?, ?)", guruID, mapelID, kelasID)
	if err != nil {
		t.Fatalf("insert pengajaran: %v", err)
	}
	pengajaranID, _ := res.LastInsertId()

	res, err = db.ExecContext(ctx, `
		INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai, acak_soal, acak_pilihan, batas_pelanggaran)
		VALUES (?, 'Ujian API Monitor', 60, NOW() - INTERVAL 1 HOUR, NOW() + INTERVAL 2 HOUR, 1, 1, 3)
	`, pengajaranID)
	if err != nil {
		t.Fatalf("insert ujian: %v", err)
	}
	ujianID64, _ := res.LastInsertId()
	ujianID := int(ujianID64)

	sm := scs.New()
	svc := monitor.NewService(db, nil)
	handler := sm.LoadAndSave(auth.RequireAdminAPI(sm)(monitor.GetMonitorHandler(svc)))

	// 1. Tanpa session -> 401 Unauthorized
	req := httptest.NewRequest("GET", "/admin/api/monitor/1", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("tanpa session: status = %d, want 401", w.Code)
	}

	// 2. Session siswa -> 403 Forbidden
	req = httptest.NewRequest("GET", "/admin/api/monitor/1", nil)
	ctxSiswa, err := sm.Load(req.Context(), "")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	sm.Put(ctxSiswa, "peran", "siswa")
	sm.Put(ctxSiswa, "siswaId", 99)
	req = req.WithContext(ctxSiswa)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("session siswa: status = %d, want 403", w.Code)
	}

	// 3. Session admin valid
	req = httptest.NewRequest("GET", "/admin/api/monitor/1", nil)
	req.SetPathValue("ujianId", "1") // stdlib Go 1.22+ PathValue
	ctxAdmin, err := sm.Load(req.Context(), "")
	if err != nil {
		t.Fatalf("load session admin: %v", err)
	}
	sm.Put(ctxAdmin, "peran", "admin")
	sm.Put(ctxAdmin, "adminId", 1)
	req = req.WithContext(ctxAdmin)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("session admin valid: status = %d, body: %s", w.Code, w.Body.String())
	}

	var snap monitor.SnapshotData
	if err := json.NewDecoder(w.Body).Decode(&snap); err != nil {
		t.Fatalf("decode snapshot json: %v", err)
	}
	if snap.UjianID != ujianID {
		t.Errorf("snapshot.UjianID = %d, want %d", snap.UjianID, ujianID)
	}
}

func TestEndpointGetMonitor_UnitMock(t *testing.T) {
	drv := &mockDriver{totalSiswa: 5}
	driverName := fmt.Sprintf("mock_endpoint_%d", time.Now().UnixNano())
	sql.Register(driverName, drv)

	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open mock db: %v", err)
	}
	defer db.Close()

	sm := scs.New()
	svc := monitor.NewService(db, nil)
	handler := sm.LoadAndSave(auth.RequireAdminAPI(sm)(monitor.GetMonitorHandler(svc)))

	// 1. Tanpa session -> 401 Unauthorized
	req := httptest.NewRequest("GET", "/admin/api/monitor/1", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("tanpa session: status = %d, want 401", w.Code)
	}

	// 2. Session siswa -> 403 Forbidden
	req = httptest.NewRequest("GET", "/admin/api/monitor/1", nil)
	ctxSiswa, err := sm.Load(req.Context(), "")
	if err != nil {
		t.Fatalf("load session siswa: %v", err)
	}
	sm.Put(ctxSiswa, "peran", "siswa")
	sm.Put(ctxSiswa, "siswaId", 123)
	req = req.WithContext(ctxSiswa)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("session siswa: status = %d, want 403", w.Code)
	}

	// 3. Session admin valid, ID ujian bukan angka -> 400 Bad Request
	req = httptest.NewRequest("GET", "/admin/api/monitor/abc", nil)
	req.SetPathValue("ujianId", "abc")
	ctxAdmin, err := sm.Load(req.Context(), "")
	if err != nil {
		t.Fatalf("load session admin: %v", err)
	}
	sm.Put(ctxAdmin, "peran", "admin")
	sm.Put(ctxAdmin, "adminId", 1)
	req = req.WithContext(ctxAdmin)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("id ujian non-angka: status = %d, want 400", w.Code)
	}

	// 4. Session admin valid, ID ujian benar -> 200 OK
	req = httptest.NewRequest("GET", "/admin/api/monitor/1", nil)
	req.SetPathValue("ujianId", "1")
	req = req.WithContext(ctxAdmin)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("session admin: status = %d, want 200, body: %s", w.Code, w.Body.String())
	}

	var snap monitor.SnapshotData
	if err := json.NewDecoder(w.Body).Decode(&snap); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}

	if snap.TotalSoal != 40 {
		t.Errorf("snap.TotalSoal = %d, want 40", snap.TotalSoal)
	}
	if snap.Ringkasan.TotalSiswa != 5 {
		t.Errorf("snap.Ringkasan.TotalSiswa = %d, want 5", snap.Ringkasan.TotalSiswa)
	}
}
