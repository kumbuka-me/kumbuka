#!/bin/sh
set -eu

entry=${CSS_ENTRY:-web/src/css/app.css}
output=${CSS_OUTPUT:-web/dist/css/app.css}
source_dir=$(dirname "$entry")
output_dir=$(dirname "$output")

if [ ! -f "$entry" ]; then
  echo "CSS entrypoint not found: $entry" >&2
  exit 1
fi

# app.css is an import manifest. Keeping rules in owned partials prevents hidden
# source-order coupling and ensures the simple concatenating build cannot drop them.
if grep -q '[{}]' "$entry"; then
  echo "CSS entrypoint must contain imports/comments only: $entry" >&2
  exit 1
fi

imports=$(sed -n 's/^@import "\.\/\([^"]*\)";$/\1/p' "$entry")
declared=$(grep -c '^@import ' "$entry" || true)
parsed=$(printf '%s\n' "$imports" | sed '/^$/d' | wc -l | tr -d ' ')

if [ "$declared" -eq 0 ]; then
  echo "CSS entrypoint has no imports: $entry" >&2
  exit 1
fi
if [ "$declared" -ne "$parsed" ]; then
  echo "CSS entrypoint contains an invalid import: $entry" >&2
  exit 1
fi

duplicates=$(printf '%s\n' "$imports" | sed '/^$/d' | sort | uniq -d)
if [ -n "$duplicates" ]; then
  echo "CSS entrypoint imports a partial more than once:" >&2
  printf '%s\n' "$duplicates" >&2
  exit 1
fi

# Every stylesheet below the source directory belongs to the app bundle. Failing
# on an orphan keeps ownership explicit when files are moved or split.
find "$source_dir" -type f -name '*.css' ! -path "$entry" | while IFS= read -r source_file; do
  relative=${source_file#"$source_dir"/}
  if ! printf '%s\n' "$imports" | grep -Fqx "$relative"; then
    echo "CSS partial is not imported by $entry: $relative" >&2
    exit 1
  fi
done

mkdir -p "$output_dir"
: >"$output"

printf '%s\n' "$imports" | while IFS= read -r relative; do
  [ -n "$relative" ] || continue
  case "$relative" in
  /* | *../* | ../* | *'/..')
    echo "CSS import must stay below $source_dir: $relative" >&2
    exit 1
    ;;
  esac

  source_file="$source_dir/$relative"
  if [ ! -f "$source_file" ]; then
    echo "CSS import not found: $source_file" >&2
    exit 1
  fi
  cat "$source_file" >>"$output"
done

# The source partials are build inputs only. Keep the embedded distribution to
# the single stylesheet referenced by the templates.
printf '%s\n' "$imports" | while IFS= read -r relative; do
  [ -n "$relative" ] || continue
  rm -f "$output_dir/$relative"
done
find "$output_dir" -depth -type d -empty -delete 2>/dev/null || true

