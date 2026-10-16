#!/usr/bin/env bash
# ==============================================================================
# scripts/restore.sh — Skrip Restore Database untuk CBT Sekolah
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# 1. Muat .env jika ada
if [ -f "$PROJECT_ROOT/.env" ]; then
    while IFS='=' read -r key val || [ -n "$key" ]; do
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
TARGET_DB="${DB_NAME:-cbt_sekolah_go}"
DB_USER_VAL="${DB_USER:-root}"
DB_PASS_VAL="${DB_PASSWORD:-}"
DB_HOST_VAL="${DB_HOST:-127.0.0.1}"
DB_PORT_VAL="${DB_PORT:-3306}"
DB_SOCKET_VAL="${DB_SOCKET:-}"
RESTORE_FILE=""
ASSUME_YES=false

# 3. Parsing Argumen CLI
while [[ $# -gt 0 ]]; do
    case "$1" in
        --file=*)
            RESTORE_FILE="${1#*=}"
            shift
            ;;
        --db=*)
            TARGET_DB="${1#*=}"
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
        --yes|-y)
            ASSUME_YES=true
            shift
            ;;
        --help|-h)
            echo "Penggunaan: $0 --file=<path_file_backup> [OPSI]"
            echo "Opsi:"
            echo "  --file=<path>        Path berkas backup (.sql atau .sql.gz) [WAJIB]"
            echo "  --db=<nama_db>       Target database restore (default: DB_NAME atau cbt_sekolah_go)"
            echo "  --user=<db_user>     User database (default: DB_USER atau root)"
            echo "  --password=<db_pass> Password database (default: DB_PASSWORD)"
            echo "  --socket=<path>      UNIX domain socket (default: DB_SOCKET)"
            echo "  --host=<db_host>     Host database (default: DB_HOST atau 127.0.0.1)"
            echo "  --port=<db_port>     Port database (default: DB_PORT atau 3306)"
            echo "  --yes, -y            Lewati konfirmasi interaktif"
            exit 0
            ;;
        *)
            echo "[ERROR] Opsi tidak dikenal: $1" >&2
            exit 1
            ;;
    esac
done

if [ -z "$RESTORE_FILE" ]; then
    echo "[ERROR] Parameter --file=<path_file_backup> wajib ditentukan." >&2
    exit 1
fi

if [ ! -f "$RESTORE_FILE" ]; then
    echo "[ERROR] Berkas backup tidak ditemukan: $RESTORE_FILE" >&2
    exit 1
fi

# 4. Deteksi Tool Client (mariadb atau mysql)
if command -v mariadb >/dev/null 2>&1; then
    CLI_BIN="mariadb"
elif command -v mysql >/dev/null 2>&1; then
    CLI_BIN="mysql"
else
    echo "[ERROR] Tidak ditemukan client mysql atau mariadb pada sistem." >&2
    exit 1
fi

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

echo "[RESTORE] Berkas sumber : $RESTORE_FILE"
echo "[RESTORE] Target database: $TARGET_DB"

# 6. Eksekusi Restore
if [[ "$RESTORE_FILE" == *.gz ]]; then
    echo "[RESTORE] Mendekompresi dan mengimpor dump sql.gz..."
    gunzip -c "$RESTORE_FILE" | "$CLI_BIN" "${CONN_ARGS[@]}" "$TARGET_DB"
else
    echo "[RESTORE] Mengimpor file .sql..."
    "$CLI_BIN" "${CONN_ARGS[@]}" "$TARGET_DB" < "$RESTORE_FILE"
fi

echo "[RESTORE] Operasi restore ke database $TARGET_DB berhasil."

