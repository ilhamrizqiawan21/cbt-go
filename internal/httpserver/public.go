package httpserver

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/alexedwards/scs/v2"

	"cbt-go/web"
)

// Querier memungkinkan query SQL berkonteks.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// DaftarUjianItem adalah representasi respons JSON untuk GET /api/daftar-ujian.
type DaftarUjianItem struct {
	ID         int    `json:"id"`
	NamaUjian  string `json:"nama_ujian"`
	NamaKelas  string `json:"nama_kelas"`
	ButuhToken bool   `json:"butuh_token"`
}

// registerPublicRoutes mendaftarkan rute-rute publik yang tidak memerlukan autentikasi.
func registerPublicRoutes(mux *http.ServeMux, db any, sm *scs.SessionManager) {
	// Landing page (hanya persis "/")
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if sm != nil {
			if sm.GetInt(r.Context(), "adminId") > 0 {
				http.Redirect(w, r, "/admin/dashboard", http.StatusFound)
				return
			}
			if sm.GetInt(r.Context(), "guruId") > 0 {
				http.Redirect(w, r, "/guru/dashboard", http.StatusFound)
				return
			}
			if sm.GetInt(r.Context(), "siswaId") > 0 && sm.GetInt(r.Context(), "ujianId") > 0 {
				http.Redirect(w, r, "/ujian", http.StatusFound)
				return
			}
		}
		_ = web.Render(w, "landing.gohtml", nil)
	})

	// Login Siswa
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		_ = web.Render(w, "login.gohtml", map[string]any{"Error": nil})
	})

	// Login Admin
	mux.HandleFunc("GET /login-admin", func(w http.ResponseWriter, r *http.Request) {
		_ = web.Render(w, "login-admin.gohtml", map[string]any{"Error": nil})
	})

	// Login Guru
	mux.HandleFunc("GET /login-guru", func(w http.ResponseWriter, r *http.Request) {
		_ = web.Render(w, "login-guru.gohtml", map[string]any{"Error": nil})
	})

	// API Daftar Ujian Aktif
	mux.HandleFunc("GET /api/daftar-ujian", func(w http.ResponseWriter, r *http.Request) {
		q, ok := db.(Querier)
		if !ok || q == nil {
			writeJSON(w, http.StatusOK, []DaftarUjianItem{})
			return
		}

		query := `
			SELECT u.id, u.nama_ujian, k.nama_kelas,
			       (u.token_ujian IS NOT NULL AND TRIM(u.token_ujian) != '') AS butuh_token
			FROM ujian u
			JOIN pengajaran p ON u.pengajaran_id = p.id
			JOIN kelas k ON p.kelas_id = k.id
			WHERE u.tanggal_mulai <= NOW() AND (u.tanggal_selesai >= NOW() OR EXISTS (
				SELECT 1 FROM izin_susulan iz WHERE iz.ujian_id = u.id AND iz.tanggal_selesai >= NOW()))
			ORDER BY u.id ASC
		`
		rows, err := q.QueryContext(r.Context(), query)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "gagal mengambil daftar ujian"})
			return
		}
		defer rows.Close()

		items := make([]DaftarUjianItem, 0)
		for rows.Next() {
			var it DaftarUjianItem
			var butuhTokenInt int
			if err := rows.Scan(&it.ID, &it.NamaUjian, &it.NamaKelas, &butuhTokenInt); err != nil {
				continue
			}
			it.ButuhToken = butuhTokenInt == 1
			items = append(items, it)
		}
		writeJSON(w, http.StatusOK, items)
	})

	// Aset statis (css, js, fonts, images)
	staticH, err := web.StaticHandler()
	if err == nil {
		mux.Handle("GET /css/", staticH)
		mux.Handle("GET /js/", staticH)
		mux.Handle("GET /fonts/", staticH)
		mux.Handle("GET /images/", staticH)
		mux.Handle("GET /favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/images/favicon.ico"
			staticH.ServeHTTP(w, r2)
		}))
	}

	// 404 Handler untuk rute yang tidak dikenal
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/api/daftar-ujian" {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		web.RenderError(w, http.StatusNotFound, "Halaman yang Anda cari tidak ditemukan.")
	})
}
