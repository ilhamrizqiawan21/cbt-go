package auth

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"cbt-go/internal/config"
	"cbt-go/internal/sesi"
	"cbt-go/web"
)

// dummyBcryptHash adalah hash bcrypt valid untuk mencegah timing attack / user enumeration bila NIS tidak ditemukan.
const dummyBcryptHash = "$2a$10$7EqJtq98hPqEX7fNZaFWoOuR10m6fSj/G6g0fJp1MZZf4uYcKxRzC"

type SiswaData struct {
	ID       int
	Nama     string
	NIS      string
	Kelas    string
	PinUjian string
}

type UjianInfo struct {
	ID         int
	NamaUjian  string
	NamaKelas  string
	NamaMapel  string
	TokenUjian sql.NullString
}

// LoginSiswaHandler memproses form login siswa dengan verifikasi PIN, kelas, token, dan memulai sesi.
func LoginSiswaHandler(db *sql.DB, sm *scs.SessionManager, cfg config.Config, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = web.Render(w, "login.gohtml", map[string]any{"Error": "Form tidak valid"})
			return
		}

		nis := strings.TrimSpace(r.FormValue("nis"))
		pin := strings.TrimSpace(r.FormValue("pin"))
		ujianIDStr := strings.TrimSpace(r.FormValue("ujian_id"))
		tokenUjian := strings.TrimSpace(r.FormValue("token_ujian"))
		rawDevice := strings.TrimSpace(strings.ToLower(r.FormValue("device_type")))

		deviceType := "laptop"
		if rawDevice == "hp" {
			deviceType = "hp"
		}

		if nis == "" || pin == "" || ujianIDStr == "" {
			_ = web.Render(w, "login.gohtml", map[string]any{"Error": "NIS, PIN, dan Ujian harus diisi"})
			return
		}

		ujianID, err := strconv.Atoi(ujianIDStr)
		if err != nil {
			_ = web.Render(w, "login.gohtml", map[string]any{"Error": "Ujian tidak valid"})
			return
		}

		ctx := r.Context()

		// 1. Cek data siswa berdasarkan NIS
		var s SiswaData
		var kelasNull sql.NullString
		err = db.QueryRowContext(ctx, `
			SELECT id, nama, nis, kelas, pin_ujian
			FROM siswa
			WHERE nis = ?
		`, nis).Scan(&s.ID, &s.Nama, &s.NIS, &kelasNull, &s.PinUjian)

		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				// Mencegah timing attack / user enumeration
				_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(pin))
				_ = web.Render(w, "login.gohtml", map[string]any{"Error": "NIS atau PIN salah"})
				return
			}
			if logger != nil {
				logger.Error("login-siswa: query siswa gagal", "err", err)
			}
			_ = web.Render(w, "login.gohtml", map[string]any{"Error": "Terjadi kesalahan server"})
			return
		}
		if kelasNull.Valid {
			s.Kelas = kelasNull.String
		}

		// 2. Verifikasi PIN dengan bcrypt
		if err := bcrypt.CompareHashAndPassword([]byte(s.PinUjian), []byte(pin)); err != nil {
			_ = web.Render(w, "login.gohtml", map[string]any{"Error": "NIS atau PIN salah"})
			return
		}

		// 3. Validasi Ujian dalam jendela waktu dan cocok dengan pengajaran
		var u UjianInfo
		err = db.QueryRowContext(ctx, `
			SELECT u.id, u.nama_ujian, k.nama_kelas, mp.nama_mapel, u.token_ujian
			FROM ujian u
			JOIN pengajaran p ON u.pengajaran_id = p.id
			JOIN kelas k ON p.kelas_id = k.id
			JOIN mata_pelajaran mp ON p.mapel_id = mp.id
			WHERE u.id = ? AND `+sesi.KondisiJendelaSiswa("u")+`
		`, ujianID, s.ID).Scan(&u.ID, &u.NamaUjian, &u.NamaKelas, &u.NamaMapel, &u.TokenUjian)

		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				_ = web.Render(w, "login.gohtml", map[string]any{"Error": "Ujian tidak tersedia atau belum dimulai/sudah berakhir"})
				return
			}
			if logger != nil {
				logger.Error("login-siswa: query ujian gagal", "err", err)
			}
			_ = web.Render(w, "login.gohtml", map[string]any{"Error": "Terjadi kesalahan server"})
			return
		}

		// 4. Validasi kelas siswa = kelas pengajaran ujian
		if s.Kelas != u.NamaKelas {
			_ = web.Render(w, "login.gohtml", map[string]any{"Error": "Ujian ini tidak diperuntukkan bagi kelas Anda"})
			return
		}

		// 5. Validasi Token Ujian jika diaktifkan (case-insensitive & constant-time)
		if u.TokenUjian.Valid && strings.TrimSpace(u.TokenUjian.String) != "" {
			reqToken := strings.ToUpper(strings.TrimSpace(u.TokenUjian.String))
			if tokenUjian == "" {
				_ = web.Render(w, "login.gohtml", map[string]any{"Error": "Token ujian wajib diisi untuk ujian ini"})
				return
			}
			inToken := strings.ToUpper(tokenUjian)
			if subtle.ConstantTimeCompare([]byte(inToken), []byte(reqToken)) != 1 {
				_ = web.Render(w, "login.gohtml", map[string]any{"Error": "Token ujian salah. Silakan minta token yang benar ke pengawas"})
				return
			}
		}

		// 6. Cek apakah sudah pernah menyelesaikan ujian ini
		var nilaiID int
		err = db.QueryRowContext(ctx, `
			SELECT id FROM nilai_ujian
			WHERE siswa_id = ? AND ujian_id = ?
		`, s.ID, ujianID).Scan(&nilaiID)
		if err == nil {
			_ = web.Render(w, "login.gohtml", map[string]any{"Error": "Anda sudah mengerjakan ujian ini sebelumnya!"})
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			if logger != nil {
				logger.Error("login-siswa: query nilai_ujian gagal", "err", err)
			}
			_ = web.Render(w, "login.gohtml", map[string]any{"Error": "Terjadi kesalahan server"})
			return
		}

		// 7. Mulai atau lanjut sesi ujian (take-over bila ada sesi aktif)
		hasilSesi, err := sesi.MulaiAtauLanjut(ctx, db, s.ID, ujianID, deviceType)
		if err != nil {
			if errors.Is(err, sesi.ErrKeluarPaksa) {
				_ = web.Render(w, "login.gohtml", map[string]any{"Error": "Akses ujian Anda telah dicabut karena pelanggaran. Hubungi pengawas."})
				return
			}
			if errors.Is(err, sesi.ErrSesiSelesai) {
				_ = web.Render(w, "login.gohtml", map[string]any{"Error": "Anda sudah menyelesaikan ujian ini."})
				return
			}
			if logger != nil {
				logger.Error("login-siswa: MulaiAtauLanjut gagal", "err", err)
			}
			_ = web.Render(w, "login.gohtml", map[string]any{"Error": "Terjadi kesalahan server"})
			return
		}

		// 8. Simpan session siswa
		if sm != nil {
			_ = sm.RenewToken(ctx)
			sm.Put(ctx, "peran", "siswa")
			sm.Put(ctx, "siswaId", s.ID)
			sm.Put(ctx, "siswaNama", s.Nama)
			sm.Put(ctx, "siswaNis", s.NIS)
			sm.Put(ctx, "ujianId", ujianID)
			sm.Put(ctx, "deviceToken", hasilSesi.Sesi.DeviceToken)
			sm.Put(ctx, "deviceType", deviceType)
		}

		if logger != nil {
			if hasilSesi.TakeOver {
				logger.Info("sesi ujian diambil alih", "nis", s.NIS, "ujian_id", ujianID, "device_type", deviceType)
			} else {
				logger.Info("siswa login ujian", "nis", s.NIS, "ujian_id", ujianID, "device_type", deviceType)
			}
		}

		http.Redirect(w, r, "/ujian", http.StatusFound)
	}
}

// LogoutHandler menghancurkan session pengguna dan mengarahkan kembali ke /login.
func LogoutHandler(sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sm != nil {
			_ = sm.Destroy(r.Context())
		}
		http.Redirect(w, r, "/login", http.StatusFound)
	}
}

// UjianHandler menampilkan halaman ujian CBT untuk siswa yang sedang login.
func UjianHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if sm == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		siswaID := sm.GetInt(ctx, "siswaId")
		ujianID := sm.GetInt(ctx, "ujianId")
		siswaNama := sm.GetString(ctx, "siswaNama")
		siswaNis := sm.GetString(ctx, "siswaNis")
		deviceType := sm.GetString(ctx, "deviceType")
		if deviceType == "" {
			deviceType = "laptop"
		}

		if siswaID == 0 || ujianID == 0 {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		var namaUjian, namaMapel string
		var tglMulai time.Time
		err := db.QueryRowContext(ctx, `
			SELECT u.nama_ujian, mp.nama_mapel, u.tanggal_mulai
			FROM ujian u
			JOIN pengajaran p ON u.pengajaran_id = p.id
			JOIN mata_pelajaran mp ON p.mapel_id = mp.id
			WHERE u.id = ?
		`, ujianID).Scan(&namaUjian, &namaMapel, &tglMulai)

		if err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		data := map[string]any{
			"Siswa": map[string]any{
				"ID":   siswaID,
				"Nama": siswaNama,
				"NIS":  siswaNis,
			},
			"UjianID":    ujianID,
			"NamaUjian":  namaUjian,
			"NamaMapel":  namaMapel,
			"DeviceType": deviceType,
		}

		_ = web.Render(w, "ujian.gohtml", data)
	}
}
