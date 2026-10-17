package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestBackupRestoreScripts memverifikasi kelengkapan, hak eksekusi, dan perilaku CLI
// dari scripts/backup.sh dan scripts/restore.sh sebagai padanan tests/backup_restore.test.js.
func TestBackupRestoreScripts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skrip backup/restore adalah shell script untuk server Linux: Windows tidak punya bit izin eksekusi POSIX dan tidak dapat meng-exec berkas .sh secara langsung")
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	projectRoot := filepath.Dir(wd)
	if filepath.Base(wd) == "cbt-go" {
		projectRoot = wd
	}

	backupSh := filepath.Join(projectRoot, "scripts", "backup.sh")
	restoreSh := filepath.Join(projectRoot, "scripts", "restore.sh")

	t.Run("File Skrip Tersedia dan Executable", func(t *testing.T) {
		for _, scriptPath := range []string{backupSh, restoreSh} {
			info, err := os.Stat(scriptPath)
			if err != nil {
				t.Fatalf("Skrip %s tidak ditemukan: %v", scriptPath, err)
			}
			// Pastikan mode memiliki bit executable
			if info.Mode()&0111 == 0 {
				t.Errorf("Skrip %s harus memiliki izin eksekusi (executable), mode: %v", scriptPath, info.Mode())
			}
		}
	})

	t.Run("Backup Script Help Menu", func(t *testing.T) {
		cmd := exec.Command(backupSh, "--help")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("backup.sh --help gagal: %v, out: %s", err, string(out))
		}
		outStr := string(out)
		if !strings.Contains(outStr, "--db=") || !strings.Contains(outStr, "--keep=") {
			t.Errorf("backup.sh --help tidak memuat opsi yang diharapkan: %s", outStr)
		}
	})

	t.Run("Restore Script Help Menu", func(t *testing.T) {
		cmd := exec.Command(restoreSh, "--help")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("restore.sh --help gagal: %v, out: %s", err, string(out))
		}
		outStr := string(out)
		if !strings.Contains(outStr, "--file=") || !strings.Contains(outStr, "--db=") {
			t.Errorf("restore.sh --help tidak memuat opsi yang diharapkan: %s", outStr)
		}
	})

	t.Run("Restore Menolak Eksekusi Tanpa Parameter File", func(t *testing.T) {
		cmd := exec.Command(restoreSh)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("restore.sh tanpa argumen harusnya exit dengan kode error, dapat sukses. Output: %s", string(out))
		}
		if !strings.Contains(string(out), "--file=<path_file_backup> wajib ditentukan") {
			t.Errorf("Pesan error tidak sesuai: %s", string(out))
		}
	})
}

// TestSystemdUnitSyntax memverifikasi sintaks berkas unit systemd yang didokumentasikan di docs/DEPLOYMENT.md.
func TestSystemdUnitSyntax(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	projectRoot := filepath.Dir(wd)
	if filepath.Base(wd) == "cbt-go" {
		projectRoot = wd
	}

	deployDoc := filepath.Join(projectRoot, "docs", "DEPLOYMENT.md")
	content, err := os.ReadFile(deployDoc)
	if err != nil {
		t.Fatalf("baca docs/DEPLOYMENT.md: %v", err)
	}

	docStr := string(content)
	if !strings.Contains(docStr, "Restart=always") {
		t.Errorf("docs/DEPLOYMENT.md harus memuat 'Restart=always'")
	}
	if !strings.Contains(docStr, "ExecStart=") {
		t.Errorf("docs/DEPLOYMENT.md harus memuat 'ExecStart='")
	}
	if !strings.Contains(docStr, "LimitNOFILE=65535") {
		t.Errorf("docs/DEPLOYMENT.md harus memuat LimitNOFILE=65535")
	}
}
