// Command migrate menjalankan migrasi skema database menggunakan goose.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"

	"cbt-go/internal/config"
	"cbt-go/internal/db"
	"cbt-go/migrations"
)

var errProductionDownBlocked = errors.New("migrasi: perintah 'down' dilarang pada mode production demi keamanan data")

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fsFlags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	dbFlag := fsFlags.String("db", "", "nama database target (menggantikan DB_NAME di konfigurasi)")
	if err := fsFlags.Parse(args); err != nil {
		return err
	}

	cmdArgs := fsFlags.Args()
	if len(cmdArgs) == 0 {
		return errors.New("perintah tidak ditentukan\nPenggunaan: migrate [--db=<nama>] <up|down|status|version>")
	}

	// .env opsional
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("membaca .env: %w", err)
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	if *dbFlag != "" {
		cfg.DB.Name = *dbFlag
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DB, cfg.Timezone)
	if err != nil {
		return err
	}
	defer pool.Close()

	return runCommand(ctx, pool, cfg.Env, cmdArgs)
}

// runCommand memisahkan logika eksekusi perintah agar mudah diuji.
func runCommand(ctx context.Context, pool *sql.DB, env config.Env, args []string) error {
	if len(args) == 0 {
		return errors.New("perintah kosong")
	}

	subcmd := args[0]
	switch subcmd {
	case "up", "status", "version":
		// diperbolehkan di semua environment
	case "down":
		if env == config.EnvProduction {
			return errProductionDownBlocked
		}
	default:
		return fmt.Errorf("perintah %q tidak dikenal (pilihan: up, down, status, version)", subcmd)
	}

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("mysql"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}

	switch subcmd {
	case "up":
		if err := goose.UpContext(ctx, pool, "."); err != nil && !errors.Is(err, goose.ErrNoNextVersion) {
			return fmt.Errorf("goose up: %w", err)
		}
	case "down":
		if err := goose.DownContext(ctx, pool, "."); err != nil {
			return fmt.Errorf("goose down: %w", err)
		}
	case "status":
		if err := goose.StatusContext(ctx, pool, "."); err != nil {
			return fmt.Errorf("goose status: %w", err)
		}
	case "version":
		if err := goose.VersionContext(ctx, pool, "."); err != nil {
			return fmt.Errorf("goose version: %w", err)
		}
	}
	return nil
}
