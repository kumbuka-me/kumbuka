#!/bin/sh
set -eu

usage() {
  cat <<'USAGE'
Usage: scripts/db/cleanup_migrations.sh --migrations DIR --generated-dir DIR

Replaces the existing migration SQL files with the validated baseline produced
by merge_migrations.sh. The generated baseline version must match the highest
current migration version and must be newer than the currently installed
baseline.
USAGE
}

migrations_dir=""
generated_dir=""
backup_dir=""
staged_file=""
restore_required=false

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
  exit "$status"
}

migration_version() {
  migration_name=$(basename "$1")
  version=${migration_name%%_*}

  if [ "$version" = "$migration_name" ] || [ -z "$version" ]; then
    echo "Migration filename must start with a numeric version followed by _: $migration_name" >&2
    return 1
  fi
  case "$version" in
  *[!0-9]*)
    echo "Migration filename has a non-numeric version: $migration_name" >&2
    return 1
    ;;
  esac

  version=$(printf '%s\n' "$version" | sed 's/^0*//')
  [ -n "$version" ] || version=0
  printf '%s\n' "$version"
}

format_version() {
  printf '%03d' "$1"
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

highest_version=""
current_baseline_version=""
migration_count=0
for migration_file in "$migrations_dir"/*.sql; do
  [ -f "$migration_file" ] || continue
  migration_count=$((migration_count + 1))
  version=$(migration_version "$migration_file") || exit 1

  if [ -z "$highest_version" ] || [ "$version" -gt "$highest_version" ]; then
    highest_version=$version
  fi

  case "$(basename "$migration_file")" in
  *_baseline.sql)
    if [ -z "$current_baseline_version" ] || [ "$version" -gt "$current_baseline_version" ]; then
      current_baseline_version=$version
    fi
    ;;
  esac
done

[ "$migration_count" -gt 0 ] || {
  echo "No migration SQL files found in $migrations_dir" >&2
  exit 1
}

expected_name="$(format_version "$highest_version")_baseline.sql"
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

if [ -n "$current_baseline_version" ] && [ "$highest_version" -le "$current_baseline_version" ]; then
  echo "No newer migrations to consolidate." >&2
  echo "Current baseline is version $current_baseline_version and highest migration is version $highest_version." >&2
  exit 1
fi

staged_file="$migrations_dir/.$expected_name.tmp.$$"
backup_dir="$migrations_dir/.migration-backup.$$"

trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir "$backup_dir"
cp "$generated_file" "$staged_file"
restore_required=true
for migration_file in "$migrations_dir"/*.sql; do
  [ -f "$migration_file" ] || continue
  mv "$migration_file" "$backup_dir/"
done

mv "$staged_file" "$migrations_dir/$expected_name"
staged_file=""
restore_required=false
rm -rf "$backup_dir"
backup_dir=""
rm -f "$generated_file"
rmdir "$generated_dir" 2>/dev/null || true

trap - EXIT HUP INT TERM

echo "Replaced $migration_count migration files with $migrations_dir/$expected_name"
