#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

mkdir -p "$TEMP_DIR/bin" "$TEMP_DIR/project/scripts" "$TEMP_DIR/migrations"
cp "$ROOT_DIR/scripts/migrate.sh" "$TEMP_DIR/project/scripts/migrate.sh"

cat > "$TEMP_DIR/bin/migrate" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
: > "$MIGRATE_CALLED_FILE"
EOF
chmod +x "$TEMP_DIR/bin/migrate"

password_sentinel='DB_PASSWORD_MUST_NOT_REACH_OUTPUT_7f31a9'
dsn_sentinel='DB_URL_CREDENTIAL_MUST_NOT_REACH_OUTPUT_92c4e6'
database_url="postgres://audit_user:${dsn_sentinel}@db.example.invalid:5432/audit?sslmode=require"

run_case() {
    local name="$1"
    shift
    local marker="$TEMP_DIR/migrate-called-$name"
    local output

    rm -f "$marker"
    if ! output=$(
        env \
            PATH="$TEMP_DIR/bin:$PATH" \
            DB_PASSWORD="$password_sentinel" \
            DB_URL="$database_url" \
            MIGRATIONS_DIR="$TEMP_DIR/migrations" \
            MIGRATE_CALLED_FILE="$marker" \
            bash "$TEMP_DIR/project/scripts/migrate.sh" "$@" 2>&1
    ); then
        echo "migration credential-output check failed to execute case: $name" >&2
        return 1
    fi

    if [ ! -f "$marker" ]; then
        echo "migration credential-output check did not invoke migrate for case: $name" >&2
        return 1
    fi
    if grep -Fq "$password_sentinel" <<< "$output"; then
        echo "scripts/migrate.sh exposed DB_PASSWORD in case: $name" >&2
        return 1
    fi
    if grep -Fq "$dsn_sentinel" <<< "$output"; then
        echo "scripts/migrate.sh exposed credentials embedded in DB_URL in case: $name" >&2
        return 1
    fi
}

run_case up up
run_case down down
run_case version version
run_case force force 42
run_case goto goto 42

echo "migration command output contains no database credentials"
