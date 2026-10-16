package httpserver

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"

	"cbt-go/internal/auth"
	"cbt-go/internal/config"
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

	// POST /api/heartbeat
	mux.Handle("POST /api/heartbeat", requireSiswa(HeartbeatHandler(sqlDB, d.SessionManager)))

	// POST /api/selesai-ujian
	mux.Handle("POST /api/selesai-ujian", requireSiswa(SelesaiUjianHandler(sqlDB, d.SessionManager, d.Config)))

	// GET /api/cek-status-ujian
	mux.HandleFunc("GET /api/cek-status-ujian", CekStatusUjianHandler(sqlDB, d.SessionManager))
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

// SinkronJawabanHandler memproses batch pengiriman jawaban siswa secara idempoten dan atomik.
func SinkronJawabanHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sm == nil || db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Server error"})
			return
		}

		// Batasi ukuran request payload maksimal 1 MB menggunakan http.MaxBytesReader
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "Ukuran payload melebihi batas maksimal"})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Format data tidak valid, harus berupa array jawaban"})
			return
		}

		rawStr := strings.TrimSpace(string(raw))
		var items []sesi.ItemJawabanInput

		if strings.HasPrefix(rawStr, "[") {
			if err := json.Unmarshal(raw, &items); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Format data tidak valid, harus berupa array jawaban"})
				return
			}
		} else if strings.HasPrefix(rawStr, "{") {
			var wrapper struct {
				Jawaban []sesi.ItemJawabanInput `json:"jawaban"`
				Items   []sesi.ItemJawabanInput `json:"items"`
			}
			if err := json.Unmarshal(raw, &wrapper); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Format data tidak valid, harus berupa array jawaban"})
				return
			}
			if wrapper.Jawaban != nil {
				items = wrapper.Jawaban
			} else if wrapper.Items != nil {
				items = wrapper.Items
			} else {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Format data tidak valid, harus berupa array jawaban"})
				return
			}
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Format data tidak valid, harus berupa array jawaban"})
			return
		}

		if len(items) > 100 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Ukuran batch melebihi batas maksimal 100 item"})
			return
		}

		siswaID := sm.GetInt(r.Context(), "siswaId")
		ujianID := sm.GetInt(r.Context(), "ujianId")

		hasil, err := sesi.SinkronJawaban(r.Context(), db, siswaID, ujianID, items)
		if err != nil {
			if errors.Is(err, sesi.ErrBatchTerlaluBesar) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if errors.Is(err, sesi.ErrSoalInvalid) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if errors.Is(err, sesi.ErrSoalUjianLain) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if errors.Is(err, sesi.ErrWaktuHabis) {
				writeJSON(w, http.StatusForbidden, map[string]string{
					"error": "Waktu ujian telah berakhir dan melewati batas toleransi",
					"code":  "waktu_habis",
				})
				return
			}
			if errors.Is(err, sesi.ErrSesiNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Sesi ujian tidak ditemukan"})
				return
			}
			if errors.Is(err, sesi.ErrKeluarPaksa) || errors.Is(err, sesi.ErrSesiSelesai) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "Sesi ujian tidak aktif atau sudah selesai"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Gagal sinkron jawaban"})
			return
		}

		writeJSON(w, http.StatusOK, hasil)
	}
}

// SimpanJawabanHandler menyediakan kompatibilitas ke belakang untuk penyimpanan jawaban per butir (legacy).
func SimpanJawabanHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sm == nil || db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Server error"})
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		var req struct {
			UjianID int `json:"ujian_id"`
			SoalID  int `json:"soal_id"`
			Jawaban any `json:"jawaban"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Data tidak lengkap"})
			return
		}

		siswaID := sm.GetInt(r.Context(), "siswaId")
		sessionUjianID := sm.GetInt(r.Context(), "ujianId")

		if req.UjianID != sessionUjianID {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Ujian tidak sesuai"})
			return
		}

		if req.SoalID <= 0 || req.Jawaban == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Data tidak lengkap"})
			return
		}

		items := []sesi.ItemJawabanInput{
			{
				SoalID:   req.SoalID,
				Jawaban:  req.Jawaban,
				ClientTS: time.Now(),
			},
		}

		_, err := sesi.SinkronJawaban(r.Context(), db, siswaID, sessionUjianID, items)
		if err != nil {
			if errors.Is(err, sesi.ErrWaktuHabis) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "Waktu ujian habis"})
				return
			}
			if errors.Is(err, sesi.ErrSoalUjianLain) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Soal tidak ditemukan"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Gagal simpan"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"success": true, "soal_id": req.SoalID})
	}
}

// HeartbeatHandler memperbarui last_seen sesi siswa yang sedang aktif ujian.
func HeartbeatHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sm == nil || db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Server error"})
			return
		}

		siswaID := sm.GetInt(r.Context(), "siswaId")
		ujianID := sm.GetInt(r.Context(), "ujianId")

		if err := sesi.UpdateHeartbeat(r.Context(), db, siswaID, ujianID); err != nil {
			if errors.Is(err, sesi.ErrSesiNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Sesi tidak ditemukan"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Server error"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"success":   true,
			"timestamp": time.Now(),
		})
	}
}

// SelesaiUjianHandler memfinalisasi sesi ujian siswa dan menghitung nilai akhir.
func SelesaiUjianHandler(db *sql.DB, sm *scs.SessionManager, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sm == nil || db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Server error"})
			return
		}

		siswaID := sm.GetInt(r.Context(), "siswaId")
		ujianID := sm.GetInt(r.Context(), "ujianId")

		hasil, err := sesi.Finalize(r.Context(), db, siswaID, ujianID)
		if err != nil {
			if errors.Is(err, sesi.ErrSesiNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Sesi ujian tidak ditemukan"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Gagal simpan nilai"})
			return
		}

		if cfg.TampilkanNilaiSiswa {
			writeJSON(w, http.StatusOK, hasil)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"message": "Jawaban terkirim",
		})
	}
}

// CekStatusUjianHandler memeriksa apakah sesi ujian siswa masih valid, waktu belum habis,
// dan belum terkena keluar_paksa, sekaligus mengembalikan ringkasan jumlah pelanggaran (GET /api/cek-status-ujian).
func CekStatusUjianHandler(db *sql.DB, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sm == nil || db == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"valid": false, "reason": "server_error"})
			return
		}

		peran := sm.GetString(r.Context(), "peran")
		siswaID := sm.GetInt(r.Context(), "siswaId")
		ujianID := sm.GetInt(r.Context(), "ujianId")

		if peran != "siswa" || siswaID <= 0 || ujianID <= 0 {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"valid":  false,
				"reason": "not_logged_in",
			})
			return
		}

		// 1. Cek apakah ujian masih berada dalam rentang tanggal_mulai .. tanggal_selesai
		var batasPelanggaran int
		err := db.QueryRowContext(r.Context(), `
			SELECT COALESCE(batas_pelanggaran, 3)
			FROM ujian
			WHERE id = ? AND NOW() BETWEEN tanggal_mulai AND tanggal_selesai
		`, ujianID).Scan(&batasPelanggaran)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusOK, map[string]any{
					"valid":  false,
					"reason": "waktu_habis",
				})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"valid": false, "reason": "server_error"})
			return
		}
		if batasPelanggaran <= 0 {
			batasPelanggaran = 3
		}

		// 2. Cek apakah status sesi_ujian adalah keluar_paksa
		var statusSesi string
		err = db.QueryRowContext(r.Context(), `
			SELECT status FROM sesi_ujian WHERE siswa_id = ? AND ujian_id = ?
		`, siswaID, ujianID).Scan(&statusSesi)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"valid": false, "reason": "server_error"})
			return
		}
		if statusSesi == sesi.StatusKeluarPaksa {
			writeJSON(w, http.StatusOK, map[string]any{
				"valid":  false,
				"reason": "keluar_paksa",
			})
			return
		}

		// 3. Hitung jumlah pelanggaran aktif
		jumlah, err := sesi.HitungPelanggaran(r.Context(), db, siswaID, ujianID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"valid": false, "reason": "server_error"})
			return
		}

		sisa := batasPelanggaran - jumlah
		if sisa < 0 {
			sisa = 0
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"valid":              true,
			"jumlah_pelanggaran": jumlah,
			"batas":              batasPelanggaran,
			"sisa":               sisa,
		})
	}
}
