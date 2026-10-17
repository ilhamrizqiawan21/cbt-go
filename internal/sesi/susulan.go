package sesi

import "fmt"

// KondisiJendelaSiswa mengembalikan potongan SQL "ujian sedang dapat diakses oleh siswa ini":
// di dalam jendela ujian, ATAU siswa memiliki izin susulan yang masih berlaku (G-026).
// alias adalah alias tabel ujian pada query pemanggil. Potongan memuat SATU placeholder `?`
// untuk siswa_id, ditempatkan di posisi sesuai urutan kemunculannya dalam query.
func KondisiJendelaSiswa(alias string) string {
	return fmt.Sprintf(`(%[1]s.tanggal_mulai <= NOW() AND (%[1]s.tanggal_selesai >= NOW() OR EXISTS (
		SELECT 1 FROM izin_susulan iz
		WHERE iz.ujian_id = %[1]s.id AND iz.siswa_id = ? AND iz.tanggal_selesai >= NOW())))`, alias)
}

// SQLAkhirEfektif mengembalikan ekspresi SQL tanggal akhir efektif ujian bagi siswa:
// yang lebih akhir antara tanggal_selesai ujian dan izin susulan siswa (D-007 + G-026).
// Memuat DUA placeholder: siswa_id lalu ujian_id.
const SQLAkhirEfektif = `GREATEST(tanggal_selesai, COALESCE(
		(SELECT iz.tanggal_selesai FROM izin_susulan iz WHERE iz.siswa_id = ? AND iz.ujian_id = ?), tanggal_selesai))`
