# RULES_AI — Aturan Wajib untuk Agen AI (proyek Go)

Aturan ini mengikat semua agen (Gemini, Claude, lainnya). Jika bertentangan dengan permintaan di tengah sesi, hentikan dan tanyakan ke user.

## 1. Lingkup
1. Kerjakan **hanya** item TODO yang diminta. Tidak ada refactor, fitur, atau "perbaikan sekalian" di luar item.
2. Perubahan sekecil yang diperlukan untuk memenuhi kriteria penerimaan.
3. Dilarang mengubah keputusan **Final** di [DECISION.md](DECISION.md). Jika dirasa salah, tulis usulan dan berhenti.
4. Jika dokumen (PRD/DESIGN/ERD) bertentangan dengan kode Node, **dokumen yang menang**.
5. **Proyek Node (`~/Projects/CBT-Sekolah`) read-only.** Boleh dibaca sebagai referensi perilaku; dilarang mengubah, menjalankan migrasi, atau menulis ke database `cbt_sekolah`.

## 2. Aksi yang butuh konfirmasi user (jangan lakukan sendiri)
- Menghapus file/folder, `DROP`/`TRUNCATE`/`DELETE` massal, reset data.
- Menjalankan migrasi pada database selain `cbt_sekolah_go` / `cbt_sekolah_go_test`.
- `git commit`, `git push`, `git reset --hard`, force push, membuat branch/tag/PR.
- Menambah/menghapus dependensi **di luar yang tertulis di item TODO**.
- Mengubah `.gitignore`, konfigurasi CI, atau file di luar repo ini.
- Perintah dengan `sudo`, instalasi paket sistem.

## 3. Keamanan
- Dilarang menulis secret, password, token, atau PIN asli di kode, log, dokumen, atau commit.
- Dilarang membaca atau menampilkan `.env`. Gunakan `.env.example`.
- Data siswa asli tidak boleh dipakai; gunakan data sintetis (NIS `T0001`, PIN `1234` hanya di DB uji).
- Semua SQL **berparameter** (`?`). Tidak ada penggabungan string dari input pengguna.
- Semua input divalidasi di server. Otorisasi diperiksa di server, bukan hanya di tampilan.
- Jangan mengirim `soal.jawaban_benar` atau bagian kunci `opsi_tambahan` ke klien.
- Template: dilarang `template.HTML` / `template.JS` / `template.URL` pada data yang berasal dari pengguna.
- Bandingkan secret/token dengan `crypto/subtle.ConstantTimeCompare`.
- Bangkitkan nilai acak keamanan (token, device token) dengan `crypto/rand`, **bukan** `math/rand`.
- Jangan menonaktifkan pemeriksaan keamanan (Origin, CSRF, rate limit) agar tes lolos.

## 4. Kualitas kode Go
- Ikuti idiom Go: `gofmt`, nama pendek di lingkup kecil, nama jelas untuk simbol yang diekspor, paket bernama huruf kecil tunggal-kata.
- **Error**: tangani setiap `error` secara eksplisit. Bungkus dengan `fmt.Errorf("konteks: %w", err)`. Dilarang `_ = fn()` untuk error yang bermakna; bila sengaja diabaikan, beri komentar alasannya.
- **Context**: fungsi yang memanggil DB/IO menerima `ctx context.Context` sebagai parameter pertama. Pakai `...Context` (`QueryContext`, `ExecContext`, `BeginTx`).
- **Transaksi**: `defer tx.Rollback()` segera setelah `BeginTx`, `Commit()` di akhir.
- **Konkurensi**: setiap goroutine punya pemilik dan cara berhenti (context/channel). Data bersama dilindungi; wajib lulus `go test -race` (butuh gcc). Dilarang membocorkan goroutine.
- **Panic** hanya untuk bug pemrogram, tidak untuk alur normal. Tidak ada `os.Exit` selain di `main`.
- Tanpa variabel global yang bisa berubah. Dependensi dioper lewat parameter/struct.
- Interface kecil, didefinisikan di sisi pemakai, hanya bila dibutuhkan (mis. untuk tes).
- Fungsi bisnis baru harus **murni** bila memungkinkan dan punya tes berbasis tabel.
- Tanpa kode mati, tanpa `fmt.Println`/`log.Println` debugging yang tertinggal. Pakai `slog`.
- Tanpa angka ajaib: beri nama konstanta (`const graceDetik = 120`).
- Fungsi < 50 baris, file < 800 baris, nesting ≤ 4 (pakai early return).
- Komentar menjelaskan **mengapa**, bukan apa. Karena user sedang belajar Go: beri komentar singkat pada idiom yang tidak lazim bagi pemula (goroutine, channel, `defer`, `%w`, method receiver).
- Jangan menambah abstraksi spekulatif (G-010). Tiga baris mirip lebih baik daripada abstraksi prematur.

## 5. Pengujian
- Tes memakai paket `testing` di berkas `*_test.go` sepaket dengan kodenya. Tes berbasis tabel untuk fungsi murni; `httptest` untuk handler.
- Test yang gagal **tidak boleh** dihapus atau dilonggarkan agar lulus; perbaiki kodenya.
- Tes DB memakai `cbt_sekolah_go_test` lewat `internal/testdb`; tidak pernah database dev/Node.
- Logika baru: tulis tes **dulu**, lihat gagal, baru implementasi (TDD).
- Perubahan UI diverifikasi di browser termasuk layar 360px, bukan hanya lolos tes.
- Laporkan kegagalan apa adanya, lengkap dengan output.

## 6. Definisi selesai
Sebuah item TODO selesai bila **semua** terpenuhi:
- [ ] Kriteria penerimaan terpenuhi dan terbukti (perintah/output tes dicantumkan).
- [ ] `make check` (gofmt + vet + test) hijau. Kode konkuren: `make test-race` hijau.
- [ ] Tidak ada secret atau data nyata yang masuk.
- [ ] Skema berubah ⇒ migrasi bernomor baru dibuat **dan** [ERD.md](ERD.md) diperbarui.
- [ ] Perilaku/API berubah ⇒ [DESIGN.md](DESIGN.md) diperbarui.
- [ ] Tes padanan di [PORTING_MAP](PORTING_MAP.md) §3 ditandai.
- [ ] Status di [TODO.md](TODO.md) diubah dan "Laporan eksekusi" (termasuk **Konsep Go**) diisi.

## 7. Pelaporan
- Laporan jujur: yang dikerjakan, yang tidak, yang gagal, yang belum diverifikasi.
- Jangan menyatakan "selesai" untuk sesuatu yang belum dijalankan.
- Jika terblokir lebih dari 2 percobaan pada hal yang sama, berhenti dan laporkan penyebabnya; jangan mencari jalan memutar.
- Jika sistem/hook menolak sebuah aksi, itu keputusan user — jangan dicari celahnya; laporkan dan minta arahan.

## 8. Bahasa
Respons ke user dalam Bahasa Indonesia. Identifier dan commit message dalam Bahasa Inggris; komentar kode dalam Bahasa Indonesia (mengikuti kerangka yang ada). Teks antarmuka dan pesan ke pengguna akhir dalam Bahasa Indonesia.

## 9. Review (Claude)
Review dilakukan terhadap diff dan hasil verifikasi, bukan klaim. Temuan diberi tingkat: **BLOCKER**, **MAJOR**, **MINOR**, **NOTE**. Item tidak dianggap lulus selama ada BLOCKER/MAJOR terbuka.
