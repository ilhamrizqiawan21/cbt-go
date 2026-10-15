package httpserver

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/auth"
	"cbt-go/internal/sesi"
	"cbt-go/internal/soal"
)

// registerAPISiswaRoutes mendaftarkan rute API untuk siswa yang sedang ujian.
func registerAPISiswaRoutes(mux *http.ServeMux, d Deps) {
	sqlDB, _ := d.DB.(*sql.DB)

	requireSiswa := auth.RequireSiswaAPI(d.SessionManager, sqlDB)

	// GET /api/sesi
	mux.Handle("GET /api/sesi", requireSiswa(GetSesiHandler(sqlDB, d.SessionManager)))

	// GET /api/soal/{ujianId}
	mux.Handle("GET /api/soal/{ujianId}", requireSiswa(GetSoalHandler(sqlDB, d.SessionManager)))

	// POST /api/sinkron-jawaban
	mux.Handle("POST /api/sinkron-jawaban", requireSiswa(SinkronJawabanHandler(sqlDB, d.SessionManager)))

	// POST /api/simpan-jawaban (legacy)
	mux.Handle("POST /api/simpan-jawaban", requireSiswa(SimpanJawabanHandler(sqlDB, d.SessionManager)))
}

// GetSesiHandler mengembalikan status sesi, sisa waktu, dan jawaban tersimpan siswa.
func GetSesiHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sm == nil || db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Server error"})
			return
		}

		siswaID := sm.GetInt(r.Context(), "siswaId")
		ujianID := sm.GetInt(r.Context(), "ujianId")

		info, err := sesi.AmbilSesiInfo(r.Context(), db, siswaID, ujianID)
		if err != nil {
			if errors.Is(err, sesi.ErrSesiNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Sesi ujian tidak ditemukan"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Server error"})
			return
		}

		writeJSON(w, http.StatusOK, info)
	}
}

// GetSoalHandler mengembalikan paket soal ujian tanpa kunci jawaban, diacak secara deterministik.
func GetSoalHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sm == nil || db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Server error"})
			return
		}

		ujianIdStr := r.PathValue("ujianId")
		ujianID, err := strconv.Atoi(ujianIdStr)
		if err != nil || ujianID <= 0 {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Akses ujian tidak diizinkan"})
			return
		}

		sessionUjianID := sm.GetInt(r.Context(), "ujianId")
		siswaID := sm.GetInt(r.Context(), "siswaId")

		if ujianID != sessionUjianID {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Akses ujian tidak diizinkan"})
			return
		}

		paket, err := soal.AmbilPaketSoal(r.Context(), db, siswaID, ujianID)
		if err != nil {
			if errors.Is(err, soal.ErrUjianWaktuInvalid) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "Ujian sudah berakhir atau belum dimulai"})
				return
			}
			if errors.Is(err, soal.ErrSoalNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Soal tidak ditemukan"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Server error"})
			return
		}

		writeJSON(w, http.StatusOK, paket)
	}
}
