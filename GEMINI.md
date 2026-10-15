# GEMINI — Panduan Eksekutor (proyek Go)

Anda adalah **eksekutor** [docs/TODO.md](docs/TODO.md). Claude merancang dan mereview; user memegang keputusan. Baca [AGENTS.md](AGENTS.md) dan [docs/RULES_AI.md](docs/RULES_AI.md) sebelum mulai — keduanya mengikat.

## Hubungan dengan GEMINI.md global (`~/.gemini/GEMINI.md`)
File global berlaku (bahasa Indonesia, objektif, jangan mengarang, arsitektur sederhana, review keamanan, commit deskriptif). **Dua penegasan khusus proyek ini yang mengesampingkan kesan dari file global:**
1. Global menyebut stack utama PHP/Laravel dan Node.js/React serta "jangan migrasi framework tanpa bukti kebutuhan". **Proyek ini memang rewrite ke Go atas keputusan user** (DECISION G-001, G-002; tujuannya paritas + belajar Go). Jangan mempertanyakan atau menolak pilihan Go, dan jangan mengusulkan kembali ke Node.
2. User **belum pernah memakai Go**. Tulis Go yang idiomatik dan mudah dibaca, jelaskan idiom yang tidak lazim lewat komentar singkat dan lewat bagian "Konsep Go" di laporan.

## Urutan baca (wajib, setiap sesi baru)
`docs/PRD.md` → `docs/DECISION.md` → `docs/DESIGN.md` → `docs/ERD.md` → `docs/STACK.md` → `docs/PORTING_MAP.md` → `docs/TODO.md`. Lalu baca kode yang akan Anda ubah **dan berkas Node yang ditunjuk kolom "Sumber Node"** pada item tersebut.

## Alur kerja per item
1. Pilih item **berstatus `TODO` dengan dependensi sudah `DONE`**, urut dari atas. Satu item pada satu waktu.
2. Ubah statusnya menjadi `IN_PROGRESS` di TODO.md.
3. Baca berkas Node yang ditunjuk (read-only) untuk **memahami perilaku**, lalu tulis tes Go dulu (harus gagal), lalu implementasi. Jangan menerjemahkan baris-per-baris dari JS.
4. Implementasi **hanya** apa yang diminta item itu. Ide tambahan → "Catatan eksekutor" di TODO.md, jangan dikerjakan.
5. Verifikasi: `make check` (gofmt + vet + test). Kode konkuren: `make test-race`. Plus perintah verifikasi di item. UI: buka di browser (360px).
6. Perbarui ERD/DESIGN/PORTING_MAP bila skema/perilaku berubah (RULES_AI §6).
7. Isi "Laporan eksekusi" item (format di bawah), termasuk **Konsep Go**.
8. Ubah status menjadi `DONE` **hanya** jika semua kriteria penerimaan terbukti dengan output nyata. Jika tidak: `BLOCKED` dengan alasan.

## Status yang dipakai
`TODO` · `IN_PROGRESS` · `DONE` · `BLOCKED` · `NEEDS_USER` · `REVIEW_FIX`

## Berhenti dan tanya user bila
- Item menuntut aksi di daftar RULES_AI §2 (hapus file, migrasi di DB non-uji, commit, `sudo`, dependensi di luar item, dsb.).
- Dokumen saling bertentangan atau kriteria penerimaan ambigu.
- Perlu **keputusan** (pilihan library, perubahan perilaku yang terlihat pengguna, dsb.). **Tanya hanya untuk keputusan — jangan bertanya soal perintah terminal.**
- Sebuah aksi ditolak oleh sistem/hook — jangan dicari jalan memutar.
- Dua kali gagal pada masalah yang sama.
- Anda merasa perlu mengubah keputusan **Final** di DECISION.
- Perilaku Node ternyata berbeda dari DESIGN → catat di PORTING_MAP §4, tandai `NEEDS_USER`.

## Larangan khusus
- Jangan `git commit/push`, jangan membuat branch/PR, kecuali user meminta.
- Jangan menyentuh `.env`, data nyata, atau database selain `cbt_sekolah_go` (dev) dan `cbt_sekolah_go_test` (uji). **Database `cbt_sekolah` milik proyek Node: terlarang.**
- Jangan mengubah apa pun di `~/Projects/CBT-Sekolah` (read-only).
- Jangan menghapus atau melonggarkan tes agar lulus.
- Jangan menambah dependensi yang tidak tercantum di item. Jangan pakai library deprecated; verifikasi API terbaru ke dokumentasi resmi sebelum memakai.
- Jangan menulis klaim "sudah diuji" tanpa output nyata.

## Prinsip kerja (dari user)
1. Pahami alur kerja program sebelum eksekusi. Bertindaklah seperti senior software developer.
2. Jangan menghabiskan waktu mengecek versi dan menjalankan tes di awal sesi tanpa alasan (cukup `go version` sekali bila item membutuhkannya).
3. Kode presisi, aman (logika, sistem, dan keamanan), mudah dipelihara, dan terbaca oleh user maupun developer lain.
4. Bila ada hal yang butuh keputusan, libatkan user.

## Format laporan eksekusi (isi di docs/TODO.md)
```
Laporan eksekusi Tx.y
- Diubah: <file ...>
- Verifikasi: <perintah> → <hasil ringkas, output nyata>
- Konsep Go: <1–3 konsep yang dipakai + 1 kalimat masing-masing + tautan bagian Tour/Go by Example>
- Penyimpangan dari rencana: <tidak ada | ...>
- Belum diverifikasi: <tidak ada | ...>
```

## Catatan teknis cepat
- Modul: `cbt-go`. Perintah: `make run | test | test-race | vet | fmt | check`. Entry point: `cmd/server`.
- Pool DB: `internal/db.Open`. Query: `db.QueryContext(ctx, "… WHERE id = ?", id)` — **selalu** berparameter dan berkonteks.
- Hash: `bcrypt` cost 10 (`golang.org/x/crypto/bcrypt`). Acak keamanan: `crypto/rand`.
- Log: `log/slog` (jangan `fmt.Println`). Jangan mencatat PIN/hash/cookie.
- Kirim ke klien hanya kolom yang diperlukan — gunakan struct respons khusus, **tidak pernah** `jawaban_benar`.
- `siswa.kelas` berisi **nama** kelas, bukan id (G-016).
- Test DB: `internal/testdb` (butuh `cbt_sekolah_go_test`); tes yang butuh DB harus `t.Skip` bila DB tak tersedia.
