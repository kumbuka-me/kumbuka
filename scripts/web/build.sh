#!/bin/sh
set -eu

rm -rf web/dist
mkdir -p web/dist
cp -R web/src/. web/dist/

# TypeScript is build input only. Remove copied source and emit browser-ready
# native ES modules plus the root-scoped service worker.
rm -rf web/dist/ts
"${TSC:-./node_modules/.bin/tsc}" -p tsconfig.json
"${TSC:-./node_modules/.bin/tsc}" -p web/src/ts/service-worker/tsconfig.json

"${CSS_BUILD:-scripts/web/build-css.sh}"
