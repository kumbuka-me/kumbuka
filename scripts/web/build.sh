#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
script_dir="$root/scripts/web"
cd "$root"

tsc=${TSC:-./node_modules/.bin/tsc}
css_build=${CSS_BUILD:-$script_dir/build-css.sh}
visual_build=${VISUAL_BUILD:-$script_dir/build-visual.sh}

if [ ! -x "$tsc" ] && ! command -v "$tsc" >/dev/null 2>&1; then
  echo "TypeScript compiler not found or not executable: $tsc" >&2
  exit 1
fi
[ -x "$css_build" ] || {
  echo "CSS build script not found or not executable: $css_build" >&2
  exit 1
}
[ -x "$visual_build" ] || {
  echo "Visual editor build script not found or not executable: $visual_build" >&2
  exit 1
}
[ -d web/src ] || {
  echo "Web source directory not found: $root/web/src" >&2
  exit 1
}

rm -rf web/dist
mkdir -p web/dist
cp -R web/src/. web/dist/

# TypeScript is build input only. Remove copied source and emit browser-ready
# native ES modules plus the root-scoped service worker.
rm -rf web/dist/ts
"$tsc" -p tsconfig.json

# Build locally hosted, shared chunks for the lazy visual editor.
"$visual_build"

"$tsc" -p web/src/ts/service-worker/tsconfig.json
"$css_build"
