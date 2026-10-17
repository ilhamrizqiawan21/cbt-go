package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"unicode/utf8"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"cbt-go/web"
)

const (
	// MinPanjangPassword adalah panjang minimum password akun admin/guru yang diganti sendiri.
	MinPanjangPassword = 8
	// maksByteBcrypt: bcrypt hanya memakai 72 byte pertama; lebih dari itu ditolak agar tidak diam-diam terpotong.
	maksByteBcrypt = 72
)

// AkunConfig mengikat halaman dan aksi "ganti password" ke satu peran ("admin" atau "guru").
type AkunConfig struct {
	DB     *sql.DB
	SM     *scs.SessionManager
	Peran  string                // "admin" | "guru"
	Izin   func(key string) bool // pembatas laju (mis. RateLimiterAdmin.Allow); nil = tanpa batas
	Logger *slog.Logger
}

func (c AkunConfig) tabel() string {
	if c.Peran == "guru" {
		return "guru"
	}
	return "admin"
}

func (c AkunConfig) kunciSesi() string { return c.Peran + "Id" }

func (c AkunConfig) halaman(r *http.Request, flashErr, flashOK string) map[string]any {
	return map[string]any{
		"Peran":      c.Peran,
		"Username":   c.SM.GetString(r.Context(), "username"),
		"NamaGuru":   c.SM.GetString(r.Context(), "namaGuru"),
		"ActiveMenu": "akun",
		"Error":      flashErr,
		"Success":    flashOK,
		"MinPanjang": MinPanjangPassword,
	}
}

// AkunPageHandler menampilkan formulir ganti password.
func AkunPageHandler(c AkunConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = web.Render(w, "akun.gohtml", c.halaman(r, "", c.SM.PopString(r.Context(), "flash_success")))
	}
}

// GantiPasswordHandler memproses POST: password_lama, password_baru, konfirmasi.
// Setelah berhasil, token sesi ini dirotasi dan SEMUA sesi lain milik akun yang sama dihapus
// (perangkat lain harus login ulang dengan password baru).
func GantiPasswordHandler(c AkunConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := c.SM.GetInt(r.Context(), c.kunciSesi())
		tampil := func(code int, pesan string) {
			w.WriteHeader(code)
			_ = web.Render(w, "akun.gohtml", c.halaman(r, pesan, ""))
		}
		if id <= 0 {
			http.Redirect(w, r, "/login-"+c.Peran, http.StatusFound)
			return
		}
		if c.Izin != nil && !c.Izin(fmt.Sprintf("ganti-password:%s:%d", c.Peran, id)) {
			tampil(http.StatusTooManyRequests, "Terlalu banyak percobaan. Silakan tunggu 15 menit.")
			return
		}
		if err := r.ParseForm(); err != nil {
			tampil(http.StatusBadRequest, "Format data form tidak valid")
			return
		}
		lama, baru, konfirmasi := r.FormValue("password_lama"), r.FormValue("password_baru"), r.FormValue("konfirmasi")

		switch {
		case lama == "" || baru == "":
			tampil(http.StatusBadRequest, "Password lama dan password baru wajib diisi")
			return
		case baru != konfirmasi:
			tampil(http.StatusBadRequest, "Konfirmasi password tidak sama")
			return
		case utf8.RuneCountInString(baru) < MinPanjangPassword:
			tampil(http.StatusBadRequest, fmt.Sprintf("Password baru minimal %d karakter", MinPanjangPassword))
			return
		case len(baru) > maksByteBcrypt:
			tampil(http.StatusBadRequest, "Password baru terlalu panjang (maksimal 72 byte)")
			return
		case baru == lama:
			tampil(http.StatusBadRequest, "Password baru harus berbeda dari password lama")
			return
		}

		var hash string
		err := c.DB.QueryRowContext(r.Context(), "SELECT password FROM "+c.tabel()+" WHERE id = ?", id).Scan(&hash)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) && c.Logger != nil {
				c.Logger.Error("ganti password: baca akun", "peran", c.Peran, "err", err)
			}
			tampil(http.StatusInternalServerError, "Terjadi kesalahan pada server")
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(lama)) != nil {
			tampil(http.StatusBadRequest, "Password lama salah")
			return
		}

		baruHash, err := bcrypt.GenerateFromPassword([]byte(baru), 10)
		if err != nil {
			if c.Logger != nil {
				c.Logger.Error("ganti password: hash", "peran", c.Peran, "err", err)
			}
			tampil(http.StatusInternalServerError, "Terjadi kesalahan pada server")
			return
		}
		if _, err := c.DB.ExecContext(r.Context(), "UPDATE "+c.tabel()+" SET password = ? WHERE id = ?", string(baruHash), id); err != nil {
			if c.Logger != nil {
				c.Logger.Error("ganti password: simpan", "peran", c.Peran, "err", err)
			}
			tampil(http.StatusInternalServerError, "Terjadi kesalahan pada server")
			return
		}

		// Hapus sesi lain sebelum merotasi token ini, agar yang dikecualikan adalah sesi sekarang.
		if err := hapusSesiLain(r.Context(), c.SM, c.kunciSesi(), id); err != nil && c.Logger != nil {
			c.Logger.Warn("ganti password: gagal menghapus sesi lain", "peran", c.Peran, "err", err)
		}
		if err := c.SM.RenewToken(r.Context()); err != nil && c.Logger != nil {
			c.Logger.Warn("ganti password: gagal rotasi token", "peran", c.Peran, "err", err)
		}
		c.SM.Put(r.Context(), "flash_success", "Password berhasil diganti. Perangkat lain telah dikeluarkan.")
		http.Redirect(w, r, "/"+c.Peran+"/akun", http.StatusSeeOther)
	}
}

// hapusSesiLain menghapus semua sesi tersimpan yang memuat kunci=id kecuali sesi pada ctx.
func hapusSesiLain(ctx context.Context, sm *scs.SessionManager, kunci string, id int) error {
	sekarang := sm.Token(ctx)
	var hapus []string
	err := sm.Iterate(ctx, func(ctx context.Context) error {
		if sm.GetInt(ctx, kunci) == id && sm.Token(ctx) != sekarang {
			hapus = append(hapus, sm.Token(ctx))
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, tok := range hapus {
		if err := sm.Store.Delete(tok); err != nil {
			return err
		}
	}
	return nil
}
