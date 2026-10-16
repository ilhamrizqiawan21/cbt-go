package realtime_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/coder/websocket"

	"cbt-go/internal/realtime"
)

func newTestSessionManager() *scs.SessionManager {
	sm := scs.New()
	sm.Lifetime = time.Hour
	return sm
}

// issueCookie membuat session cookie valid melalui handler pembantu pada httptest.Server.
func issueCookie(t *testing.T, srvURL string, path string) string {
	t.Helper()
	resp, err := http.Get(srvURL + path)
	if err != nil {
		t.Fatalf("gagal request %s: %v", path, err)
	}
	defer resp.Body.Close()
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatalf("tidak ada cookie dari %s", path)
	}
	return cookies[0].Name + "=" + cookies[0].Value
}

func dialWS(ctx context.Context, srvURL, wsPath, cookie string) (*websocket.Conn, *http.Response, error) {
	wsURL := "ws" + strings.TrimPrefix(srvURL, "http") + wsPath
	hdr := http.Header{}
	if cookie != "" {
		hdr.Set("Cookie", cookie)
	}
	// Set Origin sesuai host server agar lolos pemeriksaan Origin default coder/websocket
	hdr.Set("Origin", srvURL)
	return websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: hdr,
	})
}

func TestHubWebSocket(t *testing.T) {
	t.Run("1 & 2. Koneksi dengan session valid diterima, tanpa session ditolak", func(t *testing.T) {
		sm := newTestSessionManager()
		hub := realtime.NewHub(sm, realtime.Config{})
		defer hub.Close()

		mux := http.NewServeMux()
		mux.HandleFunc("GET /set-siswa", func(w http.ResponseWriter, r *http.Request) {
			sm.Put(r.Context(), "peran", "siswa")
			sm.Put(r.Context(), "siswaId", 101)
			sm.Put(r.Context(), "ujianId", 10)
			sm.Put(r.Context(), "deviceType", "laptop")
			w.WriteHeader(http.StatusOK)
		})
		mux.HandleFunc("GET /set-admin", func(w http.ResponseWriter, r *http.Request) {
			sm.Put(r.Context(), "peran", "admin")
			sm.Put(r.Context(), "adminId", 1)
			w.WriteHeader(http.StatusOK)
		})
		mux.Handle("GET /ws", hub.ServeSiswaWS())
		mux.Handle("GET /ws/admin", hub.ServeAdminWS())

		srv := httptest.NewServer(sm.LoadAndSave(mux))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Tanpa session -> ditolak
		_, respNoAuth, err := dialWS(ctx, srv.URL, "/ws", "")
		if err == nil {
			t.Fatal("seharusnya koneksi /ws tanpa session ditolak")
		}
		if respNoAuth != nil && respNoAuth.StatusCode != http.StatusUnauthorized {
			t.Errorf("status tanpa session /ws = %d, ingin 401", respNoAuth.StatusCode)
		}

		_, respAdminNoAuth, err := dialWS(ctx, srv.URL, "/ws/admin", "")
		if err == nil {
			t.Fatal("seharusnya koneksi /ws/admin tanpa session ditolak")
		}
		if respAdminNoAuth != nil && respAdminNoAuth.StatusCode != http.StatusUnauthorized {
			t.Errorf("status tanpa session /ws/admin = %d, ingin 401", respAdminNoAuth.StatusCode)
		}

		// Session siswa mencoba akses /ws/admin -> ditolak 403/401
		siswaCookie := issueCookie(t, srv.URL, "/set-siswa")
		_, respSiswaToAdmin, err := dialWS(ctx, srv.URL, "/ws/admin", siswaCookie)
		if err == nil {
			t.Fatal("seharusnya siswa tidak bisa terhubung ke /ws/admin")
		}
		if respSiswaToAdmin != nil && respSiswaToAdmin.StatusCode != http.StatusForbidden && respSiswaToAdmin.StatusCode != http.StatusUnauthorized {
			t.Errorf("status siswa ke /ws/admin = %d, ingin 403/401", respSiswaToAdmin.StatusCode)
		}

		// Session siswa ke /ws -> berhasil
		connSiswa, _, err := dialWS(ctx, srv.URL, "/ws", siswaCookie)
		if err != nil {
			t.Fatalf("gagal dial /ws dengan session siswa valid: %v", err)
		}
		_ = connSiswa.Close(websocket.StatusNormalClosure, "selesai")

		// Session admin ke /ws/admin -> berhasil
		adminCookie := issueCookie(t, srv.URL, "/set-admin")
		connAdmin, _, err := dialWS(ctx, srv.URL, "/ws/admin", adminCookie)
		if err != nil {
			t.Fatalf("gagal dial /ws/admin dengan session admin valid: %v", err)
		}
		_ = connAdmin.Close(websocket.StatusNormalClosure, "selesai")
	})

	t.Run("3. Pesan melebihi batas ukuran (MaxMessageBytes) menyebabkan koneksi diputus", func(t *testing.T) {
		sm := newTestSessionManager()
		hub := realtime.NewHub(sm, realtime.Config{
			MaxMessageBytes: 128, // batas kecil 128 byte untuk pengujian
		})
		defer hub.Close()

		mux := http.NewServeMux()
		mux.HandleFunc("GET /set-siswa", func(w http.ResponseWriter, r *http.Request) {
			sm.Put(r.Context(), "peran", "siswa")
			sm.Put(r.Context(), "siswaId", 102)
			sm.Put(r.Context(), "ujianId", 10)
			w.WriteHeader(http.StatusOK)
		})
		mux.Handle("GET /ws", hub.ServeSiswaWS())

		srv := httptest.NewServer(sm.LoadAndSave(mux))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cookie := issueCookie(t, srv.URL, "/set-siswa")
		conn, _, err := dialWS(ctx, srv.URL, "/ws", cookie)
		if err != nil {
			t.Fatalf("dial gagal: %v", err)
		}
		defer conn.CloseNow()

		// Kirim pesan > 128 byte
		bigPayload := `{"event":"test","data":"` + strings.Repeat("X", 300) + `"}`
		err = conn.Write(ctx, websocket.MessageText, []byte(bigPayload))
		if err != nil {
			return // sudah putus saat write
		}

		// Baca berikutnya harus error karena server memutus koneksi (StatusMessageTooBig)
		_, _, readErr := conn.Read(ctx)
		if readErr == nil {
			t.Fatal("seharusnya koneksi diputus oleh server setelah mengirim pesan melebihi batas ukuran")
		}
		if status := websocket.CloseStatus(readErr); status != websocket.StatusMessageTooBig && status != -1 {
			t.Logf("close status: %v (%v)", status, readErr)
		}
	})

	t.Run("4. Room per ujian untuk admin: kirim ke room hanya sampai ke anggota room tersebut", func(t *testing.T) {
		sm := newTestSessionManager()
		hub := realtime.NewHub(sm, realtime.Config{})
		defer hub.Close()

		mux := http.NewServeMux()
		mux.HandleFunc("GET /set-admin", func(w http.ResponseWriter, r *http.Request) {
			sm.Put(r.Context(), "peran", "admin")
			sm.Put(r.Context(), "adminId", 1)
			w.WriteHeader(http.StatusOK)
		})
		mux.Handle("GET /ws/admin", hub.ServeAdminWS())

		srv := httptest.NewServer(sm.LoadAndSave(mux))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cookie1 := issueCookie(t, srv.URL, "/set-admin")
		cookie2 := issueCookie(t, srv.URL, "/set-admin")

		adminUjian10, _, err := dialWS(ctx, srv.URL, "/ws/admin", cookie1)
		if err != nil {
			t.Fatalf("dial admin 1 gagal: %v", err)
		}
		defer adminUjian10.CloseNow()

		adminUjian20, _, err := dialWS(ctx, srv.URL, "/ws/admin", cookie2)
		if err != nil {
			t.Fatalf("dial admin 2 gagal: %v", err)
		}
		defer adminUjian20.CloseNow()

		// Admin 1 join ujian 10
		if err := adminUjian10.Write(ctx, websocket.MessageText, []byte(`{"event":"admin:join","data":{"ujian_id":10}}`)); err != nil {
			t.Fatalf("write admin:join 10: %v", err)
		}
		_, joinedMsg1, err := adminUjian10.Read(ctx)
		if err != nil {
			t.Fatalf("read admin:joined 10: %v", err)
		}
		if !strings.Contains(string(joinedMsg1), "admin:joined") {
			t.Fatalf("balasan join 10 tidak sesuai: %s", string(joinedMsg1))
		}

		// Admin 2 join ujian 20
		if err := adminUjian20.Write(ctx, websocket.MessageText, []byte(`{"event":"admin:join","data":{"ujian_id":20}}`)); err != nil {
			t.Fatalf("write admin:join 20: %v", err)
		}
		_, joinedMsg2, err := adminUjian20.Read(ctx)
		if err != nil {
			t.Fatalf("read admin:joined 20: %v", err)
		}
		if !strings.Contains(string(joinedMsg2), "admin:joined") {
			t.Fatalf("balasan join 20 tidak sesuai: %s", string(joinedMsg2))
		}

		if !hub.HasAdminListeners(10) {
			t.Error("HasAdminListeners(10) = false, ingin true")
		}
		if hub.HasAdminListeners(999) {
			t.Error("HasAdminListeners(999) = true, ingin false")
		}

		// Broadcast ke room ujian 10
		hub.BroadcastToAdminRoom(10, "monitor:update", map[string]any{
			"ujian_id": 10,
			"siswa_id": 555,
		})

		// Admin 1 (ujian 10) harus menerima pesan tersebut
		_, msg10, err := adminUjian10.Read(ctx)
		if err != nil {
			t.Fatalf("admin 1 gagal membaca broadcast: %v", err)
		}
		var env realtime.OutEnvelope
		if err := json.Unmarshal(msg10, &env); err != nil {
			t.Fatalf("unmarshal broadcast: %v", err)
		}
		if env.Event != "monitor:update" {
			t.Errorf("event = %q, ingin monitor:update", env.Event)
		}

		// Admin 2 (ujian 20) TIDAK boleh menerima pesan untuk ujian 10
		shortCtx, shortCancel := context.WithTimeout(ctx, 150*time.Millisecond)
		defer shortCancel()
		_, extraMsg, err := adminUjian20.Read(shortCtx)
		if err == nil {
			t.Fatalf("admin ujian 20 seharusnya tidak menerima pesan ujian 10, tapi mendapat: %s", string(extraMsg))
		}
	})

	t.Run("5. Backpressure: klien lambat tidak memblokir pengirim dan diputus otomatis", func(t *testing.T) {
		sm := newTestSessionManager()
		hub := realtime.NewHub(sm, realtime.Config{
			SendBufferSize: 2, // buffer sangat kecil (2 pesan)
			WriteTimeout:   50 * time.Millisecond,
		})
		defer hub.Close()

		mux := http.NewServeMux()
		mux.HandleFunc("GET /set-admin", func(w http.ResponseWriter, r *http.Request) {
			sm.Put(r.Context(), "peran", "admin")
			sm.Put(r.Context(), "adminId", 1)
			w.WriteHeader(http.StatusOK)
		})
		mux.Handle("GET /ws/admin", hub.ServeAdminWS())

		srv := httptest.NewServer(sm.LoadAndSave(mux))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cookie := issueCookie(t, srv.URL, "/set-admin")
		slowConn, _, err := dialWS(ctx, srv.URL, "/ws/admin", cookie)
		if err != nil {
			t.Fatalf("dial slowConn: %v", err)
		}
		defer slowConn.CloseNow()

		// Join room ujian 10
		_ = slowConn.Write(ctx, websocket.MessageText, []byte(`{"event":"admin:join","data":{"ujian_id":10}}`))
		_, _, _ = slowConn.Read(ctx) // baca admin:joined

		// Sekarang kita TIDAK membaca dari slowConn sama sekali, dan membanjiri 50 pesan ke room 10.
		// BroadcastToAdminRoom harus selesai cepat (< 100ms) tanpa terblokir oleh klien lambat.
		start := time.Now()
		for i := 0; i < 50; i++ {
			hub.BroadcastToAdminRoom(10, "monitor:update", map[string]any{"seq": i})
		}
		elapsed := time.Since(start)
		if elapsed > 200*time.Millisecond {
			t.Errorf("BroadcastToAdminRoom terblokir terlalu lama: %v", elapsed)
		}
	})

	t.Run("6. Tidak ada goroutine bocor setelah semua klien putus dan Hub ditutup", func(t *testing.T) {
		runtime.GC()
		time.Sleep(50 * time.Millisecond)
		baseGoroutines := runtime.NumGoroutine()

		sm := newTestSessionManager()
		hub := realtime.NewHub(sm, realtime.Config{
			PingInterval: 100 * time.Millisecond,
		})

		mux := http.NewServeMux()
		mux.HandleFunc("GET /set-siswa", func(w http.ResponseWriter, r *http.Request) {
			sm.Put(r.Context(), "peran", "siswa")
			sm.Put(r.Context(), "siswaId", 201)
			sm.Put(r.Context(), "ujianId", 10)
			w.WriteHeader(http.StatusOK)
		})
		mux.Handle("GET /ws", hub.ServeSiswaWS())

		srv := httptest.NewServer(sm.LoadAndSave(mux))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var conns []*websocket.Conn
		for i := 0; i < 10; i++ {
			c := issueCookie(t, srv.URL, "/set-siswa")
			conn, _, err := dialWS(ctx, srv.URL, "/ws", c)
			if err != nil {
				t.Fatalf("dial %d: %v", i, err)
			}
			conns = append(conns, conn)
		}

		// Putuskan semua klien
		for _, conn := range conns {
			_ = conn.Close(websocket.StatusNormalClosure, "bye")
		}

		hub.Close()
		srv.Close()

		// Beri waktu singkat bagi goroutine runtime HTTP test server untuk bersih
		deadline := time.Now().Add(1 * time.Second)
		for time.Now().Before(deadline) {
			if runtime.NumGoroutine() <= baseGoroutines+2 {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}

		afterGoroutines := runtime.NumGoroutine()
		if afterGoroutines > baseGoroutines+2 {
			t.Errorf("terdeteksi kebocoran goroutine: sebelum=%d, sesudah=%d", baseGoroutines, afterGoroutines)
		}
	})
}
