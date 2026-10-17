# MOCKUP UI — CBT Sekolah (Go)

Status: **Disetujui user 17 Oktober 2026 sebagai acuan tampilan.** Belum diimplementasikan ke `web/templates`. Dokumen terkait: [DESIGN](DESIGN.md) §6, [DECISION](DECISION.md) G-011 dan G-019, [REVIEW_KESIAPAN](REVIEW_KESIAPAN.md).

## 1. Isi folder `docs/mockup/`
Enam layar, satu berkas per layar. Berkas `.dc.html` adalah sumber kanvas Design di akun Claude (kanvas "Mockup CBT Sekolah"). Berkas ini **bukan halaman mandiri**: memerlukan runtime kanvas (`support.js`), jadi tidak bisa dibuka langsung di browser. Fungsinya sebagai sumber desain yang dapat dibaca eksekutor (markup, ukuran, warna, jarak).

- `1-login-siswa.dc.html` — login siswa: NIS, PIN, pilih ujian aktif, token ujian bila perlu.
- `2-ujian-hp.dc.html` — ujian di HP 390×844: timer, status sinkron, A−/A+, soal Arab RTL, navigator sebagai lembar bawah.
- `3-ujian-laptop.dc.html` — ujian di layar besar: navigator 20 soal di samping, indikator layar penuh dan pelanggaran.
- `4-admin-dashboard.dc.html` — dashboard admin: kartu ringkasan, tabel ujian, kesiapan server, perlu tindakan.
- `5-admin-monitor.dc.html` — monitor realtime: kartu status, filter, pencarian, buka kunci, tambah waktu, selesaikan.
- `6-guru-essay.dc.html` — lembar koreksi essay: skor 0–4 per soal, total essay, nilai akhir otomatis.

Data di mockup (nama siswa, angka, soal) hanyalah contoh. Teks Arab pada contoh ditulis oleh Claude dan **belum diverifikasi harakatnya oleh user**; jangan dipakai sebagai soal sungguhan.

## 2. Token desain
Dipertahankan dari `web/static/css/style.css` agar tidak terjadi perubahan identitas, dengan penajaman kontras.
- Primer biru indigo `#3b5bdb`, hover `#2b44b0`, latar lembut `#eef2ff`.
- Tinta gelap (header, sidebar) `#101533`, permukaan kedua `#1d2552`.
- Sukses atau dijawab `#0b8a68` (teks di atas putih tetap lolos kontras), latar `#dcf5ec`.
- Bahaya atau terkunci `#c92a2a`, teks `#a61e1e`, latar `#ffe3e3`.
- Peringatan `#8a5200` di atas `#fff1d6`.
- Latar halaman `#f4f6fb`, kartu putih, garis `#dfe3ee`, teks `#1b2038`, teks sekunder `#5b6280`.
- Font judul Sora, font isi DM Sans, font Arab Amiri. Radius kartu 16–20 px, kontrol 12 px.
- Area sentuh minimal 44 px. Teks soal Arab 28 px (HP) dan 38 px (laptop) pada skala 100%.
- Status selalu memakai **teks**, bukan warna saja.

## 3. Pemetaan layar ke berkas proyek
- Layar 1 → `web/templates/login.gohtml` dan `landing.gohtml`.
- Layar 2 dan 3 → `web/templates/ujian.gohtml` dan `web/static/js/ujian.js` (satu template responsif, bukan dua).
- Layar 4 → `admin_dashboard.gohtml` dan `admin_navbar.gohtml`.
- Layar 5 → `admin_monitor.gohtml`.
- Layar 6 → `guru_essay.gohtml` dan `guru_essay_list.gohtml`.

## 4. Aturan implementasi
1. Tetap `html/template` + Bootstrap 5 lokal + JS vanilla (G-011, G-019). Tidak ada CDN baru; `web/vendor_test.go` akan gagal bila ada.
2. Sajikan Sora dan DM Sans secara lokal (`web/static/fonts`, `@font-face`), lalu hapus `@import` Google Fonts. Mockup memakainya dari Google Fonts hanya demi kemudahan pratinjau.
3. Jangan mengubah id dan atribut `data-*` yang dipakai `ujian.js`, `tests/ui/monitor.ui.js`, dan `tests/ui/arab_hp.ui.js`. Bila harus berubah, perbarui tes UI di commit yang sama.
4. Tidak memakai `template.HTML` atau `template.JS` untuk data pengguna. Render nama siswa lewat `textContent`.
5. `dir="auto"` tetap pada teks soal dan pilihan. Tidak boleh ada scroll horizontal di 360 px (`arab_hp.ui.js`).
6. Setiap template baru atau berubah ditambahkan ke `web/templates_admin_test.go`.
7. Kerjakan per layar, satu commit deskriptif per layar. Mulai dari layar 2 dan 3 (jalur siswa), lalu 5, 6, 4, dan 1.

## 5. Elemen mockup yang melampaui paritas
PRD §4 melarang fitur baru di luar paritas. Elemen berikut **tidak punya data atau endpoint saat ini**. Perlakukan sebagai usulan dan minta keputusan user sebelum dikerjakan (catat sebagai keputusan G-xxx baru).
- Kartu **Kesiapan server** di dashboard (waktu backup terakhir, jumlah klien realtime) — butuh endpoint atau query baru.
- Kartu **Perlu tindakan** di dashboard (siswa terkunci, offline lama, essay menunggu) — butuh query agregasi.
- Ikon status "Layar penuh aktif" dan chip "Pelanggaran n dari batas" di header ujian laptop — datanya sudah ada di klien dan server (`jumlah_pelanggaran`, `batas`), tetapi tampilannya baru.
- Nilai akhir otomatis di lembar koreksi essay sebelum disimpan — pratinjau di klien saja; sumber kebenaran tetap `internal/penilaian`.
- Kartu status 134 peserta di monitor memakai total per ujian, bukan total seluruh siswa.

Elemen yang sudah punya padanan: filter dan pencarian monitor, buka kunci, tambah waktu preset 5/10/15/30, selesaikan, token ujian, unduh template dan impor Excel essay, navigator soal, A−/A+.
