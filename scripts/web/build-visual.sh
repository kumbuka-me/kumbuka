#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$root"

esbuild=${ESBUILD:-./node_modules/.bin/esbuild}
target_dir=web/dist/js/features/editor
temporary=""

cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  [ -n "$temporary" ] && rm -rf "$temporary"
  exit "$status"
}

if [ ! -x "$esbuild" ] && ! command -v "$esbuild" >/dev/null 2>&1; then
  echo "esbuild not found or not executable: $esbuild" >&2
  exit 1
fi
if [ ! -f web/src/ts/features/editor/visual.ts ]; then
  echo "Visual editor entrypoint not found: web/src/ts/features/editor/visual.ts" >&2
  exit 1
fi
if ! ls web/src/ts/features/editor/visual-deps/*.ts >/dev/null 2>&1; then
  echo "Visual editor dependency entrypoints not found: web/src/ts/features/editor/visual-deps/*.ts" >&2
  exit 1
fi

temporary=$(mktemp -d "${TMPDIR:-/tmp}/kumbuka-visual.XXXXXX")
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

# Explicit dependency entries let esbuild share Tiptap/ProseMirror code without
# duplicating it across the editor, table, Markdown, image and formatting chunks.
# Build into a temporary directory first so a failed esbuild run cannot leave the
# existing distribution with its chunks removed.
"$esbuild" \
  web/src/ts/features/editor/visual.ts \
  web/src/ts/features/editor/visual-deps/*.ts \
  --bundle \
  --splitting \
  --minify \
  --format=esm \
  --platform=browser \
  --target=es2022 \
  --outbase=web/src/ts/features/editor \
  --outdir="$temporary" \
  --chunk-names=chunks/visual-[hash]

mkdir -p "$target_dir"
rm -f "$target_dir/visual.js"
rm -rf "$target_dir/visual-deps" "$target_dir/chunks"
cp -R "$temporary/." "$target_dir/"
