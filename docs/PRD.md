# PRD — CBT Sekolah (versi Go)

Status: **Disetujui sebagai dasar implementasi** · Versi 1.0 · Dokumen terkait: [DESIGN](DESIGN.md), [ERD](ERD.md), [DECISION](DECISION.md), [STACK](STACK.md), [PORTING_MAP](PORTING_MAP.md), [TODO](TODO.md)

> Proyek ini adalah **penulisan ulang** aplikasi CBT Sekolah (Node.js/Express) ke Go, di folder terpisah. **Konsep, aturan bisnis, dan kontrak HTTP sama**; yang berbeda hanya teknologi (lihat [DECISION](DECISION.md) G-001…G-016). Aplikasi Node di `~/Projects/CBT-Sekolah` adalah **referensi perilaku dan pembanding performa**, bukan untuk diubah.

## 1. Latar belakang
Selama 3 tahun ujian sekolah (SAS kelas 7/8/9, SAT kelas 7/8, dan ujian semester 2 kelas 9) di MTs Al-Ihsan Batujajar dilaksanakan dengan HP siswa memakai Exambro (Riyu) dan Google Form. Masalah yang terjadi:

1. Soal Arab di Google Form kecil dan sulit dibaca walau di-zoom.
2. Siswa sering putus koneksi sehingga **mengulang ujian dari 0**.
3. Kecurangan: membuka tab baru untuk mencari jawaban, membawa dua HP.
4. Essay dikerjakan di kertas, tetapi penilaiannya tidak terintegrasi dengan nilai PG.

## 2. Tujuan
| ID | Tujuan | Ukuran keberhasilan |
|----|--------|---------------------|
| G1 | Siswa yang putus koneksi **tidak kehilangan progres** | Login ulang melanjutkan sesi dengan jawaban dan sisa waktu yang benar; tanpa campur tangan admin |
| G2 | Soal Arab nyaman dibaca di HP dan laptop | Teks RTL, font Arab lokal, ukuran dapat diperbesar; lolos pemeriksaan visual di layar 360px |
| G3 | Penilaian sesuai aturan sekolah | Nilai akhir otomatis = PG + menjodohkan (otomatis) + essay (input guru), skala 100 |
| G4 | Admin dapat memantau siswa secara langsung | Dashboard menampilkan status, progres, pelanggaran; ada aksi buka kunci/tambah waktu/paksa selesai |
| G5 | Menekan kecurangan semampu software | Aturan berbeda per perangkat; acak soal/pilihan per siswa; log pelanggaran |
| G6 | Berjalan tanpa biaya hosting | Server di perangkat sekolah, **teruji 500 klien serentak** |
| G7 | **Paritas** dengan versi Node | Semua perilaku di PRD ini terpenuhi; tes padanan lulus ([PORTING_MAP](PORTING_MAP.md)); kontrak HTTP identik |
| G8 | **Belajar Go** sambil membangun | Kode idiomatik dan terbaca; tiap item TODO memuat catatan konsep Go yang dipakai |

## 3. Pengguna
- **Admin** — mengelola data master, ujian, dan **memantau ujian secara langsung** (peran pengawas).
- **Guru** — mengelola soal mapel yang diampu, menginput nilai essay dari kertas, melihat hasil.
- **Siswa** (±410 saat ini, target kapasitas **500 serentak**) — mengerjakan ujian dari **HP** (sebagian lewat Exambro) atau **laptop**, sering dengan kuota sendiri; WiFi sekolah tidak mampu menampung semua.

## 4. Ruang lingkup
### Termasuk
- Tiga tipe soal: pilihan ganda (PG), menjodohkan, essay (tampil saja).
- Durasi per ujian (contoh 90 dan 120 menit — Arab dan Matematika).
- Ketahanan sesi, timer server, sinkronisasi jawaban, penutupan otomatis.
- Dukungan Arab/RTL, tampilan responsif HP.
- Penilaian berbobot, input nilai essay, ekspor Excel.
- Dashboard pemantauan admin real-time.
- Kebijakan anti-curang per perangkat, token ujian opsional.
- Deployment di laptop server sekolah (binary tunggal + systemd), backup, uji beban.

### Tidak termasuk (v1)
- Siswa mengetik/mengunggah jawaban essay (dikerjakan di kertas, dinilai guru).
- Penilaian essay otomatis.
- Hosting berbayar/VPS (lihat D-001).
- Deteksi HP kedua (tidak mungkin oleh software).
- Aplikasi native / PWA terpasang.
- Mengubah aplikasi Node, atau menjalankan keduanya sebagai satu sistem. Dua aplikasi ini **paralel dan independen** (database terpisah).
- Fitur baru di luar paritas, kecuali daftar Fase 6 (DECISION G-022…G-030: gambar soal, impor soal Excel, salin soal/duplikat ujian, ujian susulan per siswa, ganti password mandiri, analisis butir, daftar hadir). Ide lain dicatat di TODO "Catatan eksekutor", bukan dikerjakan.

## 5. Aturan penilaian (final, identik dengan versi Node)
- Soal **PG**: benar = **2** poin, salah/kosong = 0.
- Soal **menjodohkan**: satu soal = **2** poin jika **semua pasangan benar**, selain itu 0 (tanpa nilai parsial).
- Soal **essay**: **5 soal × 4 poin = 20**, dinilai guru per soal (0–4) dari lembar kertas.
- **Nilai akhir** = `(poin PG + poin menjodohkan + poin essay) / total poin maksimal ujian × 100`, dibulatkan ke bilangan bulat terdekat (0,5 naik).
- Contoh: 40 soal PG/menjodohkan (80) + essay (20) = 100.
- Selama essay belum selesai dinilai, status koreksi = `menunggu_essay` dan nilai akhir belum final.

## 6. Kebutuhan fungsional
| ID | Kebutuhan | Prioritas |
|----|-----------|-----------|
| FR-01 | Login siswa dengan NIS+PIN (PIN di-hash bcrypt) dan hanya untuk ujian kelasnya | Must |
| FR-02 | Login ulang pada sesi aktif **melanjutkan** sesi; perangkat lama otomatis keluar | Must |
| FR-03 | Sisa waktu dihitung server dari waktu mulai, durasi, dan tambahan waktu | Must |
| FR-04 | Paket soal diunduh sekali; jawaban tersimpan lokal dan disinkron batch, idempoten | Must |
| FR-05 | Sesi yang waktunya habis ditutup otomatis oleh server dan dinilai | Must |
| FR-06 | Urutan acak soal/pilihan tetap sama setiap sambung ulang (seed di DB) | Must |
| FR-07 | Teks Arab tampil RTL dengan font Arab lokal; kontrol ukuran teks | Must |
| FR-08 | Perhitungan nilai sesuai §5 | Must |
| FR-09 | Essay tampil sebagai bacaan, tanpa kolom jawaban | Must |
| FR-10 | Guru menginput nilai essay per soal (form dan impor Excel) | Must |
| FR-11 | Dashboard admin: online/offline/selesai/terkunci, progres, jumlah pelanggaran, perangkat | Must |
| FR-12 | Admin: buka kunci (tanpa hapus data), tambah waktu, paksa selesai | Must |
| FR-13 | Kebijakan anti-curang per tipe perangkat (laptop ketat, HP ringan) | Should |
| FR-14 | Token ujian opsional per ujian, dibagikan pengawas | Should |
| FR-15 | Ekspor hasil Excel memuat nilai PG, essay, akhir, status koreksi | Must |
| FR-16 | Import siswa dari Excel | Must |

## 7. Kebutuhan non-fungsional
| ID | Kebutuhan |
|----|-----------|
| NFR-01 | Kapasitas **500 siswa serentak**; uji beban lulus sebelum hari ujian (ambang di TODO T4.3) |
| NFR-02 | Server restart tidak membuat siswa logout atau kehilangan jawaban (session di MySQL, seed di DB) |
| NFR-03 | Hemat kuota: paket soal kecil, aset di-cache, tanpa gambar berat untuk teks Arab |
| NFR-04 | Kunci jawaban tidak pernah dikirim ke klien |
| NFR-05 | Login dibatasi per **NIS** (bukan hanya IP; banyak siswa berbagi IP WiFi sekolah) |
| NFR-06 | Backup database otomatis berkala; pemulihan terdokumentasi |
| NFR-07 | Tanpa secret di kode; konfigurasi lewat `.env` / environment |
| NFR-08 | Fungsi penilaian dan sesi memiliki test otomatis; `go test -race` bersih |
| NFR-09 | Satu binary + folder `.env`; tanpa runtime tambahan di server (G-015) |

## 8. Kriteria selesai proyek (G7)
1. Semua item TODO `DONE` dan lulus review R-FINAL-GO (Claude).
2. Setiap tes Node di [PORTING_MAP](PORTING_MAP.md) §3 punya padanan Go yang lulus (atau alasan tertulis mengapa tidak relevan).
3. `loadtest` 500 klien: p95 `sinkron-jawaban` < 500 ms, error < 1 %, 0 jawaban hilang; **angka dilaporkan berdampingan dengan versi Node** (bukan sekadar lulus).

## 9. Pertanyaan terbuka
| ID | Pertanyaan | Default sementara |
|----|-----------|-------------------|
| O-1 | Domain/alamat akses tunnel gratis | **Ditunda** — D-002 |
| O-2 | Apakah siswa melihat nilai setelah selesai? | Tidak; hanya konfirmasi "jawaban terkirim" (flag `TAMPILKAN_NILAI_SISWA`) |
| O-3 | Apakah ujian perlu dibatasi per kelas lewat jadwal sesi? | Hanya jendela `tanggal_mulai`–`tanggal_selesai` |
