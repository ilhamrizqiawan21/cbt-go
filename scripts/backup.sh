#!/usr/bin/env bash
# ==============================================================================
# scripts/backup.sh — Skrip Backup Database Otomatis & Rotasi untuk CBT Sekolah
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# 1. Muat .env jika ada (tanpa menimpa variabel env yang sudah diset di shell)
if [ -f "$PROJECT_ROOT/.env" ]; then
    while IFS='=' read -r key val || [ -n "$key" ]; do
        # Abaikan baris komentar atau kosong
        [[ "$key" =~ ^[[:space:]]*# ]] && continue
        [[ -z "$key" ]] && continue
        key="$(echo "$key" | tr -d '[:space:]')"
        val="$(echo "$val" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'$//")"
        if [ -n "$key" ] && [ -z "${!key:-}" ]; then
            export "$key"="$val"
        fi
    done < "$PROJECT_ROOT/.env"
fi

# 2. Nilai Default
TARGET_DB="${DB_NAME:-cbt_sekolah}"
DB_USER_VAL="${DB_USER:-root}"
DB_PASS_VAL="${DB_PASSWORD:-}"
DB_HOST_VAL="${DB_HOST:-127.0.0.1}"
DB_PORT_VAL="${DB_PORT:-3306}"
DB_SOCKET_VAL="${DB_SOCKET:-}"
BACKUP_DIR="${BACKUP_DIR:-$PROJECT_ROOT/backups}"
KEEP_COUNT="${BACKUP_KEEP:-30}"
USE_GZIP=true

# 3. Parsing Argumen CLI
while [[ $# -gt 0 ]]; do
    case "$1" in
        --db=*)
            TARGET_DB="${1#*=}"
            shift
            ;;
        --out-dir=*)
            BACKUP_DIR="${1#*=}"
            shift
            ;;
        --keep=*)
            KEEP_COUNT="${1#*=}"
            shift
            ;;
        --user=*)
            DB_USER_VAL="${1#*=}"
            shift
            ;;
        --password=*)
            DB_PASS_VAL="${1#*=}"
            shift
            ;;
        --host=*)
            DB_HOST_VAL="${1#*=}"
            shift
            ;;
        --port=*)
            DB_PORT_VAL="${1#*=}"
            shift
            ;;
        --socket=*)
            DB_SOCKET_VAL="${1#*=}"
            shift
            ;;
        --no-gzip)
            USE_GZIP=false
            shift
            ;;
        --help|-h)
            echo "Penggunaan: $0 [OPSI]"
            echo "Opsi:"
            echo "  --db=<nama_db>       Target database (default: DB_NAME atau cbt_sekolah)"
            echo "  --out-dir=<folder>   Direktori penyimpanan backup (default: ./backups)"
            echo "  --keep=<jumlah>      Jumlah file backup terakhir yang dipertahankan (default: 30)"
            echo "  --user=<db_user>     User database (default: DB_USER atau root)"
            echo "  --password=<db_pass> Password database (default: DB_PASSWORD)"
            echo "  --socket=<path>      UNIX domain socket (default: DB_SOCKET)"
            echo "  --host=<db_host>     Host database (default: DB_HOST atau 127.0.0.1)"
            echo "  --port=<db_port>     Port database (default: DB_PORT atau 3306)"
            echo "  --no-gzip            Jangan kompres hasil dump dengan gzip"
            exit 0
            ;;
        *)
            echo "[ERROR] Opsi tidak dikenal: $1" >&2
            exit 1
            ;;
    esac
done

# 4. Deteksi Tool Dump (mariadb-dump atau mysqldump)
if command -v mariadb-dump >/dev/null 2>&1; then
    DUMP_BIN="mariadb-dump"
elif command -v mysqldump >/dev/null 2>&1; then
    DUMP_BIN="mysqldump"
else
    echo "[ERROR] Tidak ditemukan mariadb-dump atau mysqldump pada sistem." >&2
    exit 1
fi

mkdir -p "$BACKUP_DIR"

TIMESTAMP="$(date +'%Y%m%d_%H%M%S')"
BASE_NAME="cbt_${TARGET_DB}_${TIMESTAMP}"

# 5. Susun Parameter Koneksi
CONN_ARGS=(-u "$DB_USER_VAL")

if [ -n "$DB_PASS_VAL" ]; then
    CONN_ARGS+=("-p$DB_PASS_VAL")
fi

if [ -n "$DB_SOCKET_VAL" ] && [ -S "$DB_SOCKET_VAL" ]; then
    CONN_ARGS+=(-S "$DB_SOCKET_VAL")
else
    CONN_ARGS+=(-h "$DB_HOST_VAL" -P "$DB_PORT_VAL")
fi

echo "[BACKUP] Memulai dump database: $TARGET_DB menggunakan $DUMP_BIN..."

# 6. Eksekusi Dump
if [ "$USE_GZIP" = true ]; then
    OUT_FILE="$BACKUP_DIR/${BASE_NAME}.sql.gz"
    "$DUMP_BIN" "${CONN_ARGS[@]}" \
        --single-transaction \
        --quick \
        --routines \
        --triggers \
        --no-tablespaces \
        "$TARGET_DB" | gzip -c > "$OUT_FILE"
else
    OUT_FILE="$BACKUP_DIR/${BASE_NAME}.sql"
    "$DUMP_BIN" "${CONN_ARGS[@]}" \
        --single-transaction \
        --quick \
        --routines \
        --triggers \
        --no-tablespaces \
        "$TARGET_DB" > "$OUT_FILE"
fi

# Validasi ukuran berkas > 0
if [ ! -s "$OUT_FILE" ]; then
    echo "[ERROR] Backup gagal atau berkas kosong: $OUT_FILE" >&2
    rm -f "$OUT_FILE"
    exit 1
fi

FILE_SIZE="$(du -h "$OUT_FILE" | cut -f1)"
echo "[BACKUP] Selesai: $OUT_FILE ($FILE_SIZE)"

# 7. Rotasi Berkas Backup Lama
if [ "$KEEP_COUNT" -gt 0 ]; then
    # Cari semua berkas backup untuk database terkait di direktori backup
    BACKUP_FILES=($(find "$BACKUP_DIR" -maxdepth 1 -type f -name "cbt_${TARGET_DB}_*.sql*" | sort))
    TOTAL_FILES="${#BACKUP_FILES[@]}"

    if [ "$TOTAL_FILES" -gt "$KEEP_COUNT" ]; then
        EXCESS=$((TOTAL_FILES - KEEP_COUNT))
        echo "[BACKUP] Melakukan rotasi log: menghapus $EXCESS backup tertua (mempertahankan $KEEP_COUNT file)..."
        for ((i=0; i<EXCESS; i++)); do
            OLD_FILE="${BACKUP_FILES[$i]}"
            rm -f "$OLD_FILE"
            echo "[BACKUP] Menghapus: $(basename "$OLD_FILE")"
        done
    fi
fi

echo "[BACKUP] Operasi backup selesai dengan sukses."

