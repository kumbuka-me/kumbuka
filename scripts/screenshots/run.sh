#!/bin/sh
set -eu

repository=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
compose_file="$repository/scripts/screenshots/compose.yaml"
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/kumbuka-screenshots.XXXXXX")
archive="$work_dir/documentation.zip"
binary="$work_dir/kumbuka"
server_log="$work_dir/kumbuka.log"
server_pid=""

screenshot_port=${SCREENSHOT_PORT:-18080}
database_port=${SCREENSHOT_DB_PORT:-55432}
base_url="http://127.0.0.1:$screenshot_port"

cleanup() {
  if [ -n "$server_pid" ]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  SCREENSHOT_DB_PORT="$database_port" docker compose -f "$compose_file" down --remove-orphans >/dev/null 2>&1 || true
  rm -rf "$work_dir"
}
trap cleanup EXIT INT TERM

cd "$repository"
SCREENSHOT_DB_PORT="$database_port" docker compose -f "$compose_file" down --remove-orphans >/dev/null 2>&1 || true
SCREENSHOT_DB_PORT="$database_port" docker compose -f "$compose_file" up -d --wait

(
  cd docs/content
  find . -type f -name '*.md' -print | LC_ALL=C sort | zip -q "$archive" -@
)

go build -ldflags="-s -w -X main.Version=screenshots -X main.Commit=docs" -o "$binary" ./cmd/kumbuka
"$binary" \
  --auth-mode=none \
  --listen-address="127.0.0.1:$screenshot_port" \
  --public-url="$base_url" \
  --database-url="postgres://kumbuka:kumbuka@127.0.0.1:$database_port/kumbuka?sslmode=disable" \
  --log-format=text >"$server_log" 2>&1 &
server_pid=$!

attempt=0
until curl --fail --silent "$base_url/healthz" >/dev/null; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 60 ]; then
    sed -n '1,160p' "$server_log" >&2
    exit 1
  fi
  sleep 0.25
done

SCREENSHOT_BASE_URL="$base_url" \
  SCREENSHOT_ARCHIVE="$archive" \
  SCREENSHOT_OUTPUT="$repository/docs/assets/screenshots" \
  SCREENSHOT_BROWSER_CHANNEL="${SCREENSHOT_BROWSER_CHANNEL:-chrome}" \
  node "$repository/scripts/screenshots/capture.mjs"

