#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$root"

entry=${CSS_ENTRY:-web/src/css/app.css}
output=${CSS_OUTPUT:-web/dist/css/app.css}
temporary=""
output_tmp=""

cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  [ -n "$output_tmp" ] && rm -f "$output_tmp"
  [ -n "$temporary" ] && rm -rf "$temporary"
  exit "$status"
}

[ -f "$entry" ] || {
  echo "CSS entrypoint not found: $entry" >&2
  exit 1
}

source_dir=$(dirname "$entry")
output_dir=$(dirname "$output")
mkdir -p "$output_dir"

source_dir_abs=$(CDPATH= cd -- "$source_dir" && pwd -P)
output_dir_abs=$(CDPATH= cd -- "$output_dir" && pwd -P)

case "$output_dir_abs/" in
"$source_dir_abs/" | "$source_dir_abs/"*)
  echo "CSS output directory must be outside the source directory: $output_dir" >&2
  exit 1
  ;;
esac

# app.css is an import manifest. Keeping rules in owned partials prevents hidden
# source-order coupling and ensures the simple concatenating build cannot drop them.
if grep -q '[{}]' "$entry"; then
  echo "CSS entrypoint must contain imports/comments only: $entry" >&2
  exit 1
fi

temporary=$(mktemp -d "${TMPDIR:-/tmp}/kumbuka-css.XXXXXX")
imports_file="$temporary/imports"
sources_file="$temporary/sources"

trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

sed -n 's/^@import "\.\/\([^"]*\)";$/\1/p' "$entry" >"$imports_file"
declared=$(grep -c '^@import ' "$entry" || true)
parsed=$(sed '/^$/d' "$imports_file" | wc -l | tr -d ' ')

if [ "$declared" -eq 0 ]; then
  echo "CSS entrypoint has no imports: $entry" >&2
  exit 1
fi
if [ "$declared" -ne "$parsed" ]; then
  echo "CSS entrypoint contains an invalid import: $entry" >&2
  exit 1
fi

duplicates=$(sort "$imports_file" | uniq -d)
if [ -n "$duplicates" ]; then
  echo "CSS entrypoint imports a partial more than once:" >&2
  printf '%s\n' "$duplicates" >&2
  exit 1
fi

while IFS= read -r relative; do
  [ -n "$relative" ] || continue

  case "$relative" in
  /* | ../* | */../* | */..)
    echo "CSS import must stay below $source_dir: $relative" >&2
    exit 1
    ;;
  esac

  source_file="$source_dir/$relative"
  if [ ! -f "$source_file" ]; then
    echo "CSS import not found: $source_file" >&2
    exit 1
  fi
done <"$imports_file"

# Every stylesheet below the source directory belongs to the app bundle. Failing
# on an orphan keeps ownership explicit when files are moved or split.
find "$source_dir" -type f -name '*.css' ! -path "$entry" -print >"$sources_file"
while IFS= read -r source_file; do
  relative=${source_file#"$source_dir"/}
  if ! grep -Fqx "$relative" "$imports_file"; then
    echo "CSS partial is not imported by $entry: $relative" >&2
    exit 1
  fi
done <"$sources_file"

# Build to a sibling temporary file and publish atomically so a failed build does
# not truncate a previously valid bundle.
output_tmp=$(mktemp "$output_dir/.kumbuka-css.XXXXXX")
while IFS= read -r relative; do
  [ -n "$relative" ] || continue
  cat "$source_dir/$relative" >>"$output_tmp"
  printf '\n' >>"$output_tmp"
done <"$imports_file"

mv "$output_tmp" "$output"
output_tmp=""

# The source partials are build inputs only. Keep the embedded distribution to
# the single stylesheet referenced by the templates.
while IFS= read -r relative; do
  [ -n "$relative" ] || continue
  rm -f "$output_dir/$relative"
done <"$imports_file"
find "$output_dir" -depth -type d -empty -delete 2>/dev/null || true
