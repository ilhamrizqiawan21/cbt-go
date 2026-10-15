package auth_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"cbt-go/internal/auth"
	"cbt-go/internal/config"
	"cbt-go/internal/testdb"
)

type testHarness struct {
	db           *sql.DB
	sm           *scs.SessionManager
	siswa1ID     int
	siswa2ID     int
	ujianID      int
	pengajaranID int
}

func setupTestHarness(t *testing.T) *testHarness {
	t.Helper()
	db := testdb.Open(t)
	testdb.Reset(t, db)

	// Buat 2 kelas: VII-A dan VII-B
	resKelasA, err := db.Exec("INSERT INTO kelas (nama_kelas) VALUES ('VII-A')")
	if err != nil {
		t.Fatalf("insert kelas VII-A: %v", err)
	}
	kelasAID, _ := resKelasA.LastInsertId()

	_, err = db.Exec("INSERT INTO kelas (nama_kelas) VALUES ('VII-B')")
	if err != nil {
		t.Fatalf("insert kelas VII-B: %v", err)
	}

	// Buat mapel
	resMapel, err := db.Exec("INSERT INTO mata_pelajaran (nama_mapel) VALUES ('Matematika')")
	if err != nil {
		t.Fatalf("insert mapel: %v", err)
	}
	mapelID, _ := resMapel.LastInsertId()

	// Buat guru
	resGuru, err := db.Exec("INSERT INTO guru (nip, nama, username, password) VALUES ('19800101', 'Guru 1', 'guru1', 'hash')")
	if err != nil {
		t.Fatalf("insert guru: %v", err)
	}
	guruID, _ := resGuru.LastInsertId()

	// Buat pengajaran untuk kelas VII-A
	resPengajaran, err := db.Exec("INSERT INTO pengajaran (guru_id, mapel_id, kelas_id) VALUES (?, ?, ?)", guruID, mapelID, kelasAID)
	if err != nil {
		t.Fatalf("insert pengajaran: %v", err)
	}
	pengajaranID, _ := resPengajaran.LastInsertId()

	// Hash PIN "1234"
	hashPin, err := bcrypt.GenerateFromPassword([]byte("1234"), 10)
	if err != nil {
		t.Fatalf("hash pin: %v", err)
	}

	// Siswa 1 (kelas VII-A)
	resSiswa1, err := db.Exec("INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('T0001', 'Siswa Test 1', 'VII-A', ?)", string(hashPin))
	if err != nil {
		t.Fatalf("insert siswa 1: %v", err)
	}
	siswa1ID, _ := resSiswa1.LastInsertId()

	// Siswa 2 (kelas VII-B)
	resSiswa2, err := db.Exec("INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ('T0002', 'Siswa Test 2', 'VII-B', ?)", string(hashPin))
	if err != nil {
		t.Fatalf("insert siswa 2: %v", err)
	}
	siswa2ID, _ := resSiswa2.LastInsertId()

	// Ujian untuk pengajaran kelas VII-A (aktif saat ini)
	resUjian, err := db.Exec(`
		INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai)
		VALUES (?, 'Ujian Harian MTK', 60, NOW() - INTERVAL 1 HOUR, NOW() + INTERVAL 2 HOUR)
	`, pengajaranID)
	if err != nil {
		t.Fatalf("insert ujian: %v", err)
	}
	ujianID, _ := resUjian.LastInsertId()

	sm := scs.New()
	sm.Lifetime = 8 * time.Hour

	return &testHarness{
		db:           db,
		sm:           sm,
		siswa1ID:     int(siswa1ID),
		siswa2ID:     int(siswa2ID),
		ujianID:      int(ujianID),
		pengajaranID: int(pengajaranID),
	}
}

func executePostForm(handler http.Handler, form url.Values, cookie *http.Cookie) (*httptest.ResponseRecorder, *http.Cookie) {
	req := httptest.NewRequest("POST", "/login-siswa", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		req.AddCookie(cookie)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session" || c.Name == "cbt_session" {
			sessionCookie = c
			break
		}
	}
	return rec, sessionCookie
}

func TestLoginSiswa(t *testing.T) {
	h := setupTestHarness(t)
	cfg := config.Config{}

	loginHandler := h.sm.LoadAndSave(auth.LoginSiswaHandler(h.db, h.sm, cfg, nil))

	t.Run("1. Login sukses: PIN benar & kelas cocok", func(t *testing.T) {
		form := url.Values{
			"nis":         {"T0001"},
			"pin":         {"1234"},
			"ujian_id":    {strings.TrimSpace(url.QueryEscape("1"))},
			"device_type": {"laptop"},
		}
		// Isi ID ujian aktual
		form.Set("ujian_id", string(rune('0'+h.ujianID)))
		if h.ujianID >= 10 {
			form.Set("ujian_id", "1") // disesuaikan bawah
		}
		form.Set("ujian_id", url.QueryEscape(string([]byte{byte('0' + h.ujianID)})))
		// Gunakan strconv
		form.Set("ujian_id", "1") // di harness ID 1
		rec, cookie := executePostForm(loginHandler, form, nil)

		if rec.Code != http.StatusFound {
			t.Fatalf("status = %d, want 302; body = %s", rec.Code, rec.Body.String())
		}
		if loc := rec.Header().Get("Location"); loc != "/ujian" {
			t.Errorf("location = %q, want /ujian", loc)
		}
		if cookie == nil {
			t.Error("cookie session harus ada setelah login")
		}
	})

	t.Run("2. Ditolak jika PIN salah", func(t *testing.T) {
		form := url.Values{
			"nis":      {"T0001"},
			"pin":      {"pin_salah"},
			"ujian_id": {"1"},
		}
		rec, _ := executePostForm(loginHandler, form, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "NIS atau PIN salah") {
			t.Errorf("body harus memuat pesan 'NIS atau PIN salah': %s", rec.Body.String())
		}
	})

	t.Run("3. Ditolak jika NIS tidak ditemukan (pesan generik)", func(t *testing.T) {
		form := url.Values{
			"nis":      {"NIS_TIDAK_ADA"},
			"pin":      {"1234"},
			"ujian_id": {"1"},
		}
		rec, _ := executePostForm(loginHandler, form, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "NIS atau PIN salah") {
			t.Errorf("body harus memuat pesan generik 'NIS atau PIN salah': %s", rec.Body.String())
		}
	})

	t.Run("4. Ditolak jika kelas siswa tidak cocok dengan ujian", func(t *testing.T) {
		// Siswa T0002 kelas VII-B, ujian untuk VII-A
		form := url.Values{
			"nis":      {"T0002"},
			"pin":      {"1234"},
			"ujian_id": {"1"},
		}
		rec, _ := executePostForm(loginHandler, form, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "tidak diperuntukkan bagi kelas Anda") {
			t.Errorf("body harus memuat error kelas tidak cocok: %s", rec.Body.String())
		}
	})

	t.Run("5. Ditolak jika ujian sudah berakhir atau belum mulai", func(t *testing.T) {
		// Buat ujian kedaluwarsa
		res, _ := h.db.Exec(`
			INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai)
			VALUES (?, 'Ujian Lampau', 60, NOW() - INTERVAL 5 HOUR, NOW() - INTERVAL 3 HOUR)
		`, h.pengajaranID)
		lampauID, _ := res.LastInsertId()

		form := url.Values{
			"nis":      {"T0001"},
			"pin":      {"1234"},
			"ujian_id": {string(rune('0' + lampauID))},
		}
		rec, _ := executePostForm(loginHandler, form, nil)
		if !strings.Contains(rec.Body.String(), "tidak tersedia atau belum dimulai/sudah berakhir") {
			t.Errorf("body harus memuat error waktu ujian: %s", rec.Body.String())
		}
	})

	t.Run("6. Ujian dengan token", func(t *testing.T) {
		// Pasang token ujian
		_, _ = h.db.Exec("UPDATE ujian SET token_ujian = 'RAHASIA' WHERE id = 1")

		// a. Tanpa token
		form1 := url.Values{
			"nis":      {"T0001"},
			"pin":      {"1234"},
			"ujian_id": {"1"},
		}
		rec1, _ := executePostForm(loginHandler, form1, nil)
		if !strings.Contains(rec1.Body.String(), "Token ujian wajib diisi") {
			t.Errorf("body harus memuat error token wajib: %s", rec1.Body.String())
		}

		// b. Token salah
		form2 := url.Values{
			"nis":         {"T0001"},
			"pin":         {"1234"},
			"ujian_id":    {"1"},
			"token_ujian": {"SALAH1"},
		}
		rec2, _ := executePostForm(loginHandler, form2, nil)
		if !strings.Contains(rec2.Body.String(), "Token ujian salah") {
			t.Errorf("body harus memuat error token salah: %s", rec2.Body.String())
		}

		// c. Token benar (case-insensitive)
		form3 := url.Values{
			"nis":         {"T0001"},
			"pin":         {"1234"},
			"ujian_id":    {"1"},
			"token_ujian": {"rahasia"},
		}
		rec3, _ := executePostForm(loginHandler, form3, nil)
		if rec3.Code != http.StatusFound {
			t.Errorf("status = %d, want 302; body = %s", rec3.Code, rec3.Body.String())
		}
	})

	t.Run("7. Ditolak jika siswa sudah menyelesaikan ujian", func(t *testing.T) {
		// Pastikan token_ujian NULL
		_, _ = h.db.Exec("UPDATE ujian SET token_ujian = NULL WHERE id = 1")

		// Insert nilai_ujian untuk siswa 1
		_, _ = h.db.Exec("INSERT INTO nilai_ujian (siswa_id, ujian_id, nilai) VALUES (?, 1, 90)", h.siswa1ID)

		form := url.Values{
			"nis":      {"T0001"},
			"pin":      {"1234"},
			"ujian_id": {"1"},
		}
		rec, _ := executePostForm(loginHandler, form, nil)
		if !strings.Contains(rec.Body.String(), "sudah mengerjakan ujian ini sebelumnya") {
			t.Errorf("body harus memuat error sudah mengerjakan: %s", rec.Body.String())
		}

		// Bersihkan nilai_ujian
		_, _ = h.db.Exec("DELETE FROM nilai_ujian WHERE siswa_id = ?", h.siswa1ID)
	})

	t.Run("8. Takeover login dan RequireSiswaAPI 409", func(t *testing.T) {
		// Pastikan token_ujian NULL
		_, _ = h.db.Exec("UPDATE ujian SET token_ujian = NULL WHERE id = 1")
		// Hapus sesi lama jika ada
		_, _ = h.db.Exec("DELETE FROM sesi_ujian WHERE siswa_id = ?", h.siswa1ID)

		// 1. Siswa login di laptop
		form1 := url.Values{
			"nis":         {"T0001"},
			"pin":         {"1234"},
			"ujian_id":    {"1"},
			"device_type": {"laptop"},
		}
		rec1, cookie1 := executePostForm(loginHandler, form1, nil)
		if rec1.Code != http.StatusFound {
			t.Fatalf("login 1 gagal: %d", rec1.Code)
		}

		// Ambil token sesi laptop dari DB
		var tokenLaptop string
		_ = h.db.QueryRow("SELECT device_token FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = 1", h.siswa1ID).Scan(&tokenLaptop)

		// 2. Siswa login di HP (take-over)
		form2 := url.Values{
			"nis":         {"T0001"},
			"pin":         {"1234"},
			"ujian_id":    {"1"},
			"device_type": {"hp"},
		}
		rec2, cookie2 := executePostForm(loginHandler, form2, nil)
		if rec2.Code != http.StatusFound {
			t.Fatalf("login 2 takeover gagal: %d", rec2.Code)
		}

		var tokenHP string
		_ = h.db.QueryRow("SELECT device_token FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = 1", h.siswa1ID).Scan(&tokenHP)

		if tokenHP == tokenLaptop {
			t.Errorf("tokenHP harus beda dari tokenLaptop: got %q", tokenHP)
		}

		// 3. API endpoint dilindungi RequireSiswaAPI
		dummyAPI := h.sm.LoadAndSave(auth.RequireSiswaAPI(h.sm, h.db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		})))

		// HP (cookie2) harus 200 OK
		reqHP := httptest.NewRequest("GET", "/api/tes", nil)
		if cookie2 != nil {
			reqHP.AddCookie(cookie2)
		}
		recHP := httptest.NewRecorder()
		dummyAPI.ServeHTTP(recHP, reqHP)
		if recHP.Code != http.StatusOK {
			t.Errorf("API HP status = %d, want 200; body = %s", recHP.Code, recHP.Body.String())
		}

		// Laptop (cookie1) harus 409 Conflict karena sudah diambil alih
		reqLaptop := httptest.NewRequest("GET", "/api/tes", nil)
		if cookie1 != nil {
			reqLaptop.AddCookie(cookie1)
		}
		recLaptop := httptest.NewRecorder()
		dummyAPI.ServeHTTP(recLaptop, reqLaptop)
		if recLaptop.Code != http.StatusConflict {
			t.Errorf("API Laptop status = %d, want 409 Conflict; body = %s", recLaptop.Code, recLaptop.Body.String())
		}
	})

	t.Run("9. Halaman ujian dan Logout", func(t *testing.T) {
		ujianHandler := h.sm.LoadAndSave(auth.RequireSiswa(h.sm)(auth.UjianHandler(h.db, h.sm)))
		logoutHandler := h.sm.LoadAndSave(auth.LogoutHandler(h.sm))

		// Tanpa login -> redirect ke /login
		reqAnon := httptest.NewRequest("GET", "/ujian", nil)
		recAnon := httptest.NewRecorder()
		ujianHandler.ServeHTTP(recAnon, reqAnon)
		if recAnon.Code != http.StatusFound || recAnon.Header().Get("Location") != "/login" {
			t.Errorf("anonim /ujian status = %d, location = %q", recAnon.Code, recAnon.Header().Get("Location"))
		}

		// Login siswa 1
		form := url.Values{
			"nis":      {"T0001"},
			"pin":      {"1234"},
			"ujian_id": {"1"},
		}
		_, cookie := executePostForm(loginHandler, form, nil)

		// Akses /ujian dengan cookie -> 200 OK
		reqSiswa := httptest.NewRequest("GET", "/ujian", nil)
		reqSiswa.AddCookie(cookie)
		recSiswa := httptest.NewRecorder()
		ujianHandler.ServeHTTP(recSiswa, reqSiswa)
		if recSiswa.Code != http.StatusOK {
			t.Errorf("/ujian status = %d, want 200; body = %s", recSiswa.Code, recSiswa.Body.String())
		}
		if !strings.Contains(recSiswa.Body.String(), "Siswa Test 1") {
			t.Errorf("/ujian harus memuat nama siswa: %s", recSiswa.Body.String())
		}

		// Logout
		reqLogout := httptest.NewRequest("GET", "/logout", nil)
		reqLogout.AddCookie(cookie)
		recLogout := httptest.NewRecorder()
		logoutHandler.ServeHTTP(recLogout, reqLogout)
		if recLogout.Code != http.StatusFound || recLogout.Header().Get("Location") != "/login" {
			t.Errorf("/logout status = %d, location = %q", recLogout.Code, recLogout.Header().Get("Location"))
		}
	})
}
