#!/bin/sh
set -eu

usage() {
  cat <<'USAGE'
Usage: scripts/db/merge_migrations.sh --migrations DIR --output-dir DIR

Applies all PostgreSQL migrations to a temporary database, dumps the resulting
initial database state as SQL, and validates that the generated baseline can be
applied to a fresh database.

The generated baseline uses the next migration version after the highest
migration currently present. For example, migrations through version 8 produce
009_baseline.sql, while migrations through version 99 produce 100_baseline.sql.
At least one migration newer than the current baseline must exist before another
baseline can be generated.
USAGE
}

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
compose_file="$script_dir/compose.yaml"
migrations_dir=""
output_dir=""
compose_project="kumbuka-migrations-$$"
service_name="postgres"
validation_database="kumbuka_baseline_validation"
temporary_dir=""
publish_file=""

# shellcheck source=scripts/db/migration_helpers.sh
. "$script_dir/migration_helpers.sh"

compose() {
  docker compose -f "$compose_file" -p "$compose_project" "$@"
}

cleanup() {
  status=$?
  trap - EXIT HUP INT TERM

  echo "Cleaning up Docker Compose stack..."
  compose down --remove-orphans >/dev/null 2>&1 || true

  if [ -n "$temporary_dir" ]; then
    rm -rf "$temporary_dir"
  fi
  if [ -n "$publish_file" ]; then
    rm -f "$publish_file"
  fi

  exit "$status"
}

normalize_directory() {
  directory=$1
  mkdir -p "$directory"
  CDPATH= cd -- "$directory" && pwd -P
}

sanitize_dump() {
  input_file=$1
  destination_file=$2

  # PostgreSQL 18 wraps plain-text dumps in psql-only \restrict/\unrestrict
  # directives. Kumbuka sends migration SQL directly through pgx, so remove
  # those directives and reject any other psql meta-command that remains.
  sed \
    -e '/^\\restrict /d' \
    -e '/^\\unrestrict /d' \
    "$input_file" >"$destination_file"

  if grep -n '^[[:space:]]*\\' "$destination_file" >/dev/null; then
    echo "Generated baseline contains unsupported psql meta-commands:" >&2
    grep -n '^[[:space:]]*\\' "$destination_file" >&2 || true
    return 1
  fi
}

dump_database() {
  database=$1
  destination_file=$2

  compose exec -T "$service_name" pg_dump \
    -U kumbuka \
    -d "$database" \
    --no-owner \
    --no-privileges \
    --no-tablespaces \
    --inserts >"$destination_file"
}

apply_sql_file() {
  database=$1
  sql_file=$2

  compose exec -T "$service_name" psql \
    -X \
    --set=ON_ERROR_STOP=1 \
    -U kumbuka \
    -d "$database" <"$sql_file" >/dev/null
}

while [ "$#" -gt 0 ]; do
  case "$1" in
  --migrations)
    [ "$#" -ge 2 ] || {
      usage >&2
      exit 2
    }
    migrations_dir=$2
    shift 2
    ;;
  --output-dir)
    [ "$#" -ge 2 ] || {
      usage >&2
      exit 2
    }
    output_dir=$2
    shift 2
    ;;
  -h | --help)
    usage
    exit 0
    ;;
  *)
    echo "Unknown argument: $1" >&2
    usage >&2
    exit 2
    ;;
  esac
done

[ -n "$migrations_dir" ] || {
  echo "--migrations directory is required" >&2
  exit 2
}
[ -n "$output_dir" ] || {
  echo "--output-dir is required" >&2
  exit 2
}
[ -f "$compose_file" ] || {
  echo "Docker Compose file not found: $compose_file" >&2
  exit 1
}
command -v docker >/dev/null 2>&1 || {
  echo "docker is required" >&2
  exit 1
}

case "$migrations_dir" in
/*) ;;
*) migrations_dir="$PWD/$migrations_dir" ;;
esac
case "$output_dir" in
/*) ;;
*) output_dir="$PWD/$output_dir" ;;
esac

[ -d "$migrations_dir" ] || {
  echo "Migrations directory not found: $migrations_dir" >&2
  exit 1
}

migrations_dir=$(CDPATH= cd -- "$migrations_dir" && pwd -P)
output_dir=$(normalize_directory "$output_dir")

case "$output_dir/" in
"$migrations_dir/" | "$migrations_dir/"*)
  echo "Output directory must be outside the migrations directory: $output_dir" >&2
  exit 1
  ;;
esac

temporary_dir=$(mktemp -d "${TMPDIR:-/tmp}/kumbuka-migrations.XXXXXX")
migration_list="$temporary_dir/migrations.txt"
raw_dump="$temporary_dir/baseline.raw.sql"
candidate="$temporary_dir/baseline.sql"

trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

collect_migrations "$migrations_dir" "$migration_list"
highest_version=$(highest_migration_version "$migration_list")
baseline_version=$(current_baseline_version "$migration_list")

if [ -n "$baseline_version" ] && [ "$highest_version" -le "$baseline_version" ]; then
  echo "No newer migrations to consolidate." >&2
  echo "Current baseline is version $baseline_version and highest migration is version $highest_version." >&2
  exit 1
fi

next_version=$(next_baseline_version "$highest_version")
baseline_name="$(format_migration_version "$next_version")_baseline.sql"
output_file="$output_dir/$baseline_name"
publish_file="$output_file.tmp.$$"

echo "===================================================="
echo "Starting temporary PostgreSQL via Docker Compose..."
echo "===================================================="
compose up -d --wait

echo "Database is ready. Applying migration files in order..."
echo "----------------------------------------------------"
while IFS="	" read -r _ migration_file; do
  migration_name=$(basename "$migration_file")
  echo "Applying: $migration_name"
  apply_sql_file kumbuka "$migration_file"
done <"$migration_list"

echo "----------------------------------------------------"
echo "All migrations applied successfully."
echo "Generating $baseline_name..."
echo "===================================================="

# Dump the complete state produced by migrations, not just the schema. Some
# migrations create required seed rows. --inserts avoids COPY/\. constructs,
# which are psql input syntax rather than SQL executable through pgx.
dump_database kumbuka "$raw_dump"
sanitize_dump "$raw_dump" "$candidate"

echo "Validating generated baseline against a fresh database..."
compose exec -T "$service_name" createdb -U kumbuka "$validation_database"
apply_sql_file "$validation_database" "$candidate"

# Keep the generated directory deterministic: it contains only the newest
# generated baseline, which makes the cleanup step unambiguous.
rm -f "$output_dir"/*_baseline.sql
cp "$candidate" "$publish_file"
mv "$publish_file" "$output_file"
publish_file=""

echo "===================================================="
echo "SUCCESS: validated baseline written to $output_file"
echo "===================================================="
