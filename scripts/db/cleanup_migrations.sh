#!/bin/sh
set -eu

usage() {
  cat <<'USAGE'
Usage: scripts/db/cleanup_migrations.sh --migrations DIR --generated-dir DIR

Replaces the existing migration SQL files with the validated baseline produced
by merge_migrations.sh. The generated baseline must use the next migration
version after the highest current migration and must be newer than the currently
installed baseline.
USAGE
}

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
migrations_dir=""
generated_dir=""
backup_dir=""
staged_file=""
temporary_dir=""
restore_required=false

# shellcheck source=scripts/db/migration_helpers.sh
. "$script_dir/migration_helpers.sh"

cleanup() {
  status=$?
  trap - EXIT HUP INT TERM

  if [ "$restore_required" = true ] && [ -n "$backup_dir" ] && [ -d "$backup_dir" ]; then
    echo "Cleanup failed; restoring original migration files..." >&2
    rm -f "$migrations_dir"/*.sql
    for migration_file in "$backup_dir"/*.sql; do
      [ -f "$migration_file" ] || continue
      mv "$migration_file" "$migrations_dir/"
    done
  fi

  [ -n "$staged_file" ] && rm -f "$staged_file"
  [ -n "$backup_dir" ] && rm -rf "$backup_dir"
  [ -n "$temporary_dir" ] && rm -rf "$temporary_dir"
  exit "$status"
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
  --generated-dir)
    [ "$#" -ge 2 ] || {
      usage >&2
      exit 2
    }
    generated_dir=$2
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
[ -n "$generated_dir" ] || {
  echo "--generated-dir is required" >&2
  exit 2
}

case "$migrations_dir" in
/*) ;;
*) migrations_dir="$PWD/$migrations_dir" ;;
esac
case "$generated_dir" in
/*) ;;
*) generated_dir="$PWD/$generated_dir" ;;
esac

[ -d "$migrations_dir" ] || {
  echo "Migrations directory not found: $migrations_dir" >&2
  exit 1
}
[ -d "$generated_dir" ] || {
  echo "Generated baseline directory not found: $generated_dir" >&2
  exit 1
}

migrations_dir=$(CDPATH= cd -- "$migrations_dir" && pwd -P)
generated_dir=$(CDPATH= cd -- "$generated_dir" && pwd -P)

case "$generated_dir/" in
"$migrations_dir/" | "$migrations_dir/"*)
  echo "Generated baseline directory must be outside the migrations directory: $generated_dir" >&2
  exit 1
  ;;
esac

temporary_dir=$(mktemp -d "${TMPDIR:-/tmp}/kumbuka-migration-cleanup.XXXXXX")
migration_list="$temporary_dir/migrations.txt"

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
expected_name="$(format_migration_version "$next_version")_baseline.sql"
generated_file="$generated_dir/$expected_name"

[ -f "$generated_file" ] || {
  echo "Expected generated baseline not found: $generated_file" >&2
  echo "Run the merge step before cleanup." >&2
  exit 1
}

generated_count=0
for candidate in "$generated_dir"/*_baseline.sql; do
  [ -f "$candidate" ] || continue
  generated_count=$((generated_count + 1))
done

if [ "$generated_count" -ne 1 ]; then
  echo "Generated directory must contain exactly one baseline SQL file; found $generated_count." >&2
  exit 1
fi

migration_count=$(wc -l <"$migration_list" | tr -d ' ')
staged_file="$migrations_dir/.$expected_name.tmp.$$"
backup_dir="$migrations_dir/.migration-backup.$$"

mkdir "$backup_dir"
cp "$generated_file" "$staged_file"
restore_required=true

while IFS="	" read -r _ migration_file; do
  mv "$migration_file" "$backup_dir/"
done <"$migration_list"

mv "$staged_file" "$migrations_dir/$expected_name"
staged_file=""
restore_required=false

rm -rf "$backup_dir"
backup_dir=""
rm -f "$generated_file"
rmdir "$generated_dir" 2>/dev/null || true

rm -rf "$temporary_dir"
temporary_dir=""
trap - EXIT HUP INT TERM

echo "Replaced $migration_count migration files with $migrations_dir/$expected_name"
