#!/bin/sh

# migration_version prints the decimal migration version encoded in a filename.
migration_version() (
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

  # Strip leading zeroes before shell arithmetic/comparisons so values such as
  # 008 are always treated as decimal rather than as octal-like input.
  version=$(printf '%s\n' "$version" | sed 's/^0*//')
  [ -n "$version" ] || version=0

  printf '%s\n' "$version"
)

# format_migration_version renders migration versions with a minimum width of 3.
# printf grows beyond that width naturally, so 99 -> 099 and 100 -> 100.
format_migration_version() {
  printf '%03d' "$1"
}

# collect_migrations writes migrations sorted by numeric version and rejects duplicates.
collect_migrations() (
  migrations_dir=$1
  destination=$2
  : >"$destination"

  for migration_file in "$migrations_dir"/*.sql; do
    [ -f "$migration_file" ] || continue
    version=$(migration_version "$migration_file") || return 1
    printf '%020d\t%s\n' "$version" "$migration_file" >>"$destination"
  done

  LC_ALL=C sort -n -k1,1 -o "$destination" "$destination"

  if [ ! -s "$destination" ]; then
    echo "No migration SQL files found in $migrations_dir" >&2
    return 1
  fi

  previous_version=""
  while IFS="	" read -r padded_version migration_file; do
    version=$(printf '%s\n' "$padded_version" | sed 's/^0*//')
    [ -n "$version" ] || version=0

    if [ "$version" = "$previous_version" ]; then
      echo "Duplicate migration version $version detected: $(basename "$migration_file")" >&2
      return 1
    fi

    previous_version=$version
  done <"$destination"
)

# highest_migration_version prints the highest numeric version from a collected list.
highest_migration_version() (
  migration_list=$1
  padded_version=$(tail -n 1 "$migration_list" | cut -f1)
  version=$(printf '%s\n' "$padded_version" | sed 's/^0*//')
  [ -n "$version" ] || version=0
  printf '%s\n' "$version"
)

# current_baseline_version prints the sole baseline version, or nothing if none exists.
# Multiple baselines are rejected because only the newest baseline plus later migrations
# should ever exist after consolidation.
current_baseline_version() (
  migration_list=$1
  baseline_version=""
  baseline_count=0

  while IFS="	" read -r _ migration_file; do
    case "$(basename "$migration_file")" in
    *_baseline.sql)
      baseline_count=$((baseline_count + 1))
      baseline_version=$(migration_version "$migration_file") || return 1
      ;;
    esac
  done <"$migration_list"

  if [ "$baseline_count" -gt 1 ]; then
    echo "Multiple baseline migrations detected; expected at most one." >&2
    return 1
  fi

  printf '%s\n' "$baseline_version"
)

# next_baseline_version prints the version for the next consolidated baseline.
next_baseline_version() {
  highest_version=$1
  printf '%s\n' $((highest_version + 1))
}
