#!/bin/sh
set -eu

# Explicit dependency entries let esbuild share Tiptap/ProseMirror code without
# duplicating it across the editor, table, Markdown, image and formatting chunks.
# Everything remains behind visual-loader's dynamic import.
rm -rf web/dist/js/features/editor/chunks
"${ESBUILD:-./node_modules/.bin/esbuild}" \
  web/src/ts/features/editor/visual.ts \
  web/src/ts/features/editor/visual-deps/*.ts \
  --bundle \
  --splitting \
  --minify \
  --format=esm \
  --platform=browser \
  --target=es2022 \
  --outbase=web/src/ts/features/editor \
  --outdir=web/dist/js/features/editor \
  --chunk-names=chunks/visual-[hash]
