package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"cbt-go/internal/testdb"
)

func TestGantiPasswordMandiri(t *testing.T) {
	db := testdb.Open(t)
	testdb.Reset(t, db)
	app, _ := setupAPISiswaTestApp(t, db)

	hash, _ := bcrypt.GenerateFromPassword([]byte("lama-12345"), 4)
	db.Exec("INSERT INTO admin (username, password) VALUES ('adm1', ?), ('adm2', ?)", string(hash), string(hash))
	db.Exec("INSERT INTO guru (nip, nama, username, password) VALUES ('1', 'Guru Satu', 'gr1', ?)", string(hash))

	kirim := func(method, path string, f url.Values, c *http.Cookie) *httptest.ResponseRecorder {
		var req *http.Request
		if f != nil {
			req = httptest.NewRequest(method, path, strings.NewReader(f.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		if c != nil {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		return rec
	}
	login := func(path, user, pass string) *http.Cookie {
		rec := kirim("POST", path, url.Values{"username": {user}, "password": {pass}}, nil)
		if rec.Code != http.StatusFound {
			return nil
		}
		for _, c := range rec.Result().Cookies() {
			if c.Name == "cbt_session" {
				return c
			}
		}
		return nil
	}

	a1, a2 := login("/login-admin", "adm1", "lama-12345"), login("/login-admin", "adm1", "lama-12345")
	lain := login("/login-admin", "adm2", "lama-12345")
	if a1 == nil || a2 == nil || lain == nil {
		t.Fatal("login awal gagal")
	}

	// Tanpa login diarahkan; guru tidak bisa membuka akun admin.
	if rec := kirim("GET", "/admin/akun", nil, nil); rec.Code != http.StatusFound {
		t.Errorf("anonim: status = %d, want 302", rec.Code)
	}
	if rec := kirim("GET", "/admin/akun", nil, a1); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "adm1") {
		t.Fatalf("halaman akun: status = %d", rec.Code)
	}

	gagal := func(nama string, f url.Values, want string) {
		t.Helper()
		rec := kirim("POST", "/admin/akun/password", f, a1)
		if rec.Code == http.StatusSeeOther || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: status = %d, ingin pesan %q", nama, rec.Code, want)
		}
	}
	gagal("password lama salah", url.Values{"password_lama": {"salah"}, "password_baru": {"baru-12345"}, "konfirmasi": {"baru-12345"}}, "Password lama salah")
	gagal("konfirmasi beda", url.Values{"password_lama": {"lama-12345"}, "password_baru": {"baru-12345"}, "konfirmasi": {"beda"}}, "Konfirmasi password tidak sama")
	gagal("terlalu pendek", url.Values{"password_lama": {"lama-12345"}, "password_baru": {"pendek"}, "konfirmasi": {"pendek"}}, "minimal 8")
	gagal("sama dengan lama", url.Values{"password_lama": {"lama-12345"}, "password_baru": {"lama-12345"}, "konfirmasi": {"lama-12345"}}, "berbeda")
	gagal("terlalu panjang", url.Values{"password_lama": {"lama-12345"}, "password_baru": {strings.Repeat("x", 73)}, "konfirmasi": {strings.Repeat("x", 73)}}, "terlalu panjang")

	// Berhasil: sesi ini tetap hidup (token dirotasi), sesi lain milik adm1 dicabut, adm2 tak terpengaruh.
	rec := kirim("POST", "/admin/akun/password", url.Values{"password_lama": {"lama-12345"}, "password_baru": {"baru-12345"}, "konfirmasi": {"baru-12345"}}, a1)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("ganti password: status = %d, body %s", rec.Code, rec.Body.String())
	}
	var baru *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "cbt_session" {
			baru = c
		}
	}
	if baru == nil || baru.Value == a1.Value {
		t.Fatal("token sesi harus dirotasi setelah ganti password")
	}
	if rec := kirim("GET", "/admin/akun", nil, baru); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "berhasil diganti") {
		t.Errorf("sesi sekarang harus tetap hidup dan menampilkan sukses: status = %d", rec.Code)
	}
	if rec := kirim("GET", "/admin/akun", nil, a2); rec.Code != http.StatusFound {
		t.Errorf("sesi lain harus dicabut, status = %d", rec.Code)
	}
	if rec := kirim("GET", "/admin/akun", nil, a1); rec.Code != http.StatusFound {
		t.Errorf("token lama harus tidak berlaku setelah rotasi, status = %d", rec.Code)
	}
	if rec := kirim("GET", "/admin/akun", nil, lain); rec.Code != http.StatusOK {
		t.Errorf("sesi admin lain tidak boleh terpengaruh, status = %d", rec.Code)
	}
	if login("/login-admin", "adm1", "lama-12345") != nil {
		t.Error("password lama tidak boleh dapat dipakai login")
	}
	if login("/login-admin", "adm1", "baru-12345") == nil {
		t.Error("password baru harus dapat dipakai login")
	}

	// Guru mengganti passwordnya sendiri lewat rutenya.
	g := login("/login-guru", "gr1", "lama-12345")
	if g == nil {
		t.Fatal("login guru gagal")
	}
	if rec := kirim("POST", "/guru/akun/password", url.Values{"password_lama": {"lama-12345"}, "password_baru": {"guru-baru-1"}, "konfirmasi": {"guru-baru-1"}}, g); rec.Code != http.StatusSeeOther {
		t.Fatalf("guru ganti password: status = %d, body %s", rec.Code, rec.Body.String())
	}
	if login("/login-guru", "gr1", "guru-baru-1") == nil {
		t.Error("guru harus bisa login dengan password baru")
	}
	// Admin tidak bisa memakai rute guru.
	if rec := kirim("POST", "/guru/akun/password", url.Values{"password_lama": {"x"}, "password_baru": {"yyyyyyyy"}, "konfirmasi": {"yyyyyyyy"}}, lain); rec.Code == http.StatusSeeOther {
		t.Error("admin tidak boleh memakai rute akun guru")
	}
}
